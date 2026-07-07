package geospatial

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"goetl/internal/model"
)

const (
	OperationCropPolygons    = "crop_by_polygons"
	cropModeBBox             = "bbox"
	cropOutputDirectoryName  = "output_directory"
	cropManifestJSONName     = "manifest_json"
	defaultCropMaxFeatures   = 1000
	cropRasterOutputFilename = "crop.tif"
)

var cropEPSGRegex = regexp.MustCompile(`(?i)EPSG[" ]*,?[" ]*([0-9]+)|(?i)ID\["EPSG",\s*([0-9]+)\]`)

type cropPolygonsEnvelope struct {
	APIVersion string             `json:"api_version"`
	Kind       string             `json:"kind"`
	Operation  string             `json:"operation"`
	Inputs     cropPolygonsInputs `json:"inputs"`
	Outputs    cropPolygonsOutput `json:"outputs"`
	Options    cropPolygonsOption `json:"options"`
}

type cropPolygonsInputs struct {
	Rasters  []cropRasterInput `json:"rasters"`
	Polygons cropPolygonInput  `json:"polygons"`
}

type cropRasterInput struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type cropPolygonInput struct {
	Path    string `json:"path"`
	Layer   string `json:"layer"`
	IDField string `json:"id_field"`
}

type cropPolygonsOutput struct {
	OutputDirectory string `json:"output_directory"`
	ManifestJSON    string `json:"manifest_json"`
}

type cropPolygonsOption struct {
	Mode          string `json:"mode"`
	MaskToPolygon bool   `json:"mask_to_polygon"`
	MaxFeatures   *int   `json:"max_features"`
}

type CropByPolygonsRequest struct {
	Rasters         []CropRaster
	Polygons        CropPolygonsSource
	OutputDirectory string
	ManifestJSON    string
	Mode            string
	MaxFeatures     int
}

type CropRaster struct {
	Name string
	Path string
}

type CropPolygonsSource struct {
	Path    string
	Layer   string
	IDField string
}

type CropByPolygonsManifest struct {
	Operation       string                    `json:"operation"`
	Mode            string                    `json:"mode"`
	OutputDirectory string                    `json:"output_directory"`
	Polygons        CropManifestPolygonSource `json:"polygons"`
	Pieces          []CropManifestPiece       `json:"pieces"`
}

type CropManifestPolygonSource struct {
	Path    string `json:"path"`
	Layer   string `json:"layer"`
	IDField string `json:"id_field"`
}

type CropManifestPiece struct {
	SourceRaster string       `json:"source_raster"`
	RasterName   string       `json:"raster_name"`
	FeatureID    string       `json:"feature_id"`
	FeatureFID   string       `json:"feature_fid"`
	CropBounds   RasterBounds `json:"crop_bounds"`
	OutputPath   string       `json:"output_path"`
	PixelWidth   int          `json:"pixel_width"`
	PixelHeight  int          `json:"pixel_height"`
	Mode         string       `json:"mode"`
}

type cropFeature struct {
	FID string
	ID  string
	Box RasterBounds
}

type ogrInfoDocument struct {
	Layers []ogrLayer `json:"layers"`
}

type ogrLayer struct {
	Name           string             `json:"name"`
	FeatureCount   int                `json:"featureCount"`
	GeometryFields []ogrGeometryField `json:"geometryFields"`
	Features       []ogrFeature       `json:"features"`
}

type ogrGeometryField struct {
	CoordinateSystem ogrCoordinateSystem `json:"coordinateSystem"`
}

type ogrCoordinateSystem struct {
	WKT string `json:"wkt"`
}

type ogrFeature struct {
	FID        any            `json:"fid"`
	Properties map[string]any `json:"properties"`
	Geometry   any            `json:"geometry"`
}

func ExecuteCropByPolygons(ctx context.Context, requestData []byte, artifactRoot string) (OperationResult, error) {
	parsed, err := ParseCropByPolygonsRequest(requestData)
	if err != nil {
		return OperationResult{}, err
	}

	outputDirPath, err := cropArtifactPath(artifactRoot, parsed.OutputDirectory)
	if err != nil {
		return OperationResult{}, fmt.Errorf("output_directory path: %w", err)
	}
	manifestPath, err := cropArtifactPath(artifactRoot, parsed.ManifestJSON)
	if err != nil {
		return OperationResult{}, fmt.Errorf("manifest_json path: %w", err)
	}
	if err := os.MkdirAll(outputDirPath, 0o755); err != nil {
		return OperationResult{}, fmt.Errorf("create crop output directory: %w", err)
	}

	vectorEPSG, featureCount, err := inspectCropVectorSummary(ctx, parsed.Polygons)
	if err != nil {
		return OperationResult{}, err
	}
	if featureCount > parsed.MaxFeatures {
		return OperationResult{}, fmt.Errorf("polygon feature count %d exceeds max_features %d", featureCount, parsed.MaxFeatures)
	}

	features, err := readCropVectorFeatures(ctx, parsed.Polygons)
	if err != nil {
		return OperationResult{}, err
	}
	if len(features) > parsed.MaxFeatures {
		return OperationResult{}, fmt.Errorf("polygon feature count %d exceeds max_features %d", len(features), parsed.MaxFeatures)
	}
	sort.Slice(features, func(i, j int) bool {
		if features[i].ID == features[j].ID {
			return features[i].FID < features[j].FID
		}
		return features[i].ID < features[j].ID
	})

	manifest := CropByPolygonsManifest{
		Operation:       OperationCropPolygons,
		Mode:            parsed.Mode,
		OutputDirectory: parsed.OutputDirectory,
		Polygons: CropManifestPolygonSource{
			Path:    parsed.Polygons.Path,
			Layer:   parsed.Polygons.Layer,
			IDField: parsed.Polygons.IDField,
		},
		Pieces: []CropManifestPiece{},
	}

	for _, raster := range parsed.Rasters {
		rasterMetadata, err := collectOneRasterMetadata(raster.Name, InputSpec{Path: raster.Path})
		if err != nil {
			return OperationResult{}, fmt.Errorf("collect metadata for raster %q: %w", raster.Name, err)
		}
		if !rasterMetadata.CRSWKTPresent {
			return OperationResult{}, fmt.Errorf("raster %q CRS is missing; refusing to guess", raster.Name)
		}
		if rasterMetadata.EPSG == 0 {
			return OperationResult{}, fmt.Errorf("raster %q CRS must resolve to an EPSG code", raster.Name)
		}
		if vectorEPSG == 0 {
			return OperationResult{}, fmt.Errorf("polygon layer CRS must resolve to an EPSG code")
		}
		if rasterMetadata.EPSG != vectorEPSG {
			return OperationResult{}, fmt.Errorf("raster %q EPSG:%d does not match polygon layer EPSG:%d", raster.Name, rasterMetadata.EPSG, vectorEPSG)
		}

		rasterSegment := safePathSegment(raster.Name)
		for index, feature := range features {
			featureSegment := featureFilenameSegment(feature.ID, index)
			outputRel := path.Join(parsed.OutputDirectory, rasterSegment, featureSegment, cropRasterOutputFilename)
			outputPath, err := cropArtifactPath(artifactRoot, outputRel)
			if err != nil {
				return OperationResult{}, fmt.Errorf("crop output path: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
				return OperationResult{}, fmt.Errorf("create crop output parent: %w", err)
			}
			if err := runGDALBBoxCrop(ctx, raster.Path, outputPath, feature.Box); err != nil {
				return OperationResult{}, fmt.Errorf("crop raster %q feature %q: %w", raster.Name, feature.ID, err)
			}
			outputMetadata, err := collectOneRasterMetadata("crop", InputSpec{Path: outputPath})
			if err != nil {
				return OperationResult{}, fmt.Errorf("read crop metadata for raster %q feature %q: %w", raster.Name, feature.ID, err)
			}
			manifest.Pieces = append(manifest.Pieces, CropManifestPiece{
				SourceRaster: raster.Path,
				RasterName:   raster.Name,
				FeatureID:    feature.ID,
				FeatureFID:   feature.FID,
				CropBounds:   feature.Box,
				OutputPath:   outputRel,
				PixelWidth:   outputMetadata.Width,
				PixelHeight:  outputMetadata.Height,
				Mode:         parsed.Mode,
			})
		}
	}

	if err := cropWriteJSONFile(manifestPath, manifest); err != nil {
		return OperationResult{}, err
	}

	result := NewValidationResult(OperationCropPolygons)
	result.Artifacts = []ArtifactResult{
		{Name: cropOutputDirectoryName, Path: parsed.OutputDirectory, Kind: "directory", Format: "geotiff"},
		{Name: cropManifestJSONName, Path: parsed.ManifestJSON, Kind: "metadata", Format: "json"},
	}
	result.Summary = map[string]any{
		"output_directory": parsed.OutputDirectory,
		"manifest_json":    parsed.ManifestJSON,
		"mode":             parsed.Mode,
		"rasters":          len(parsed.Rasters),
		"features":         len(features),
		"pieces":           len(manifest.Pieces),
	}
	return result, nil
}

func ParseCropByPolygonsRequest(requestData []byte) (CropByPolygonsRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(requestData))
	decoder.UseNumber()
	var request cropPolygonsEnvelope
	if err := decoder.Decode(&request); err != nil {
		return CropByPolygonsRequest{}, fmt.Errorf("decode request: %w", err)
	}

	if request.APIVersion != APIVersionV1Alpha1 {
		return CropByPolygonsRequest{}, fmt.Errorf("unsupported api_version %q", request.APIVersion)
	}
	if request.Kind != RequestKind {
		return CropByPolygonsRequest{}, fmt.Errorf("unsupported kind %q", request.Kind)
	}
	if request.Operation != OperationCropPolygons {
		return CropByPolygonsRequest{}, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	if len(request.Inputs.Rasters) == 0 {
		return CropByPolygonsRequest{}, fmt.Errorf("%s requires at least one raster input", OperationCropPolygons)
	}

	rasters := make([]CropRaster, 0, len(request.Inputs.Rasters))
	seenRasterNames := map[string]struct{}{}
	for i, raster := range request.Inputs.Rasters {
		name := strings.TrimSpace(raster.Name)
		if name == "" {
			return CropByPolygonsRequest{}, fmt.Errorf("inputs.rasters[%d].name is required", i)
		}
		if _, ok := seenRasterNames[name]; ok {
			return CropByPolygonsRequest{}, fmt.Errorf("duplicate raster name %q", name)
		}
		seenRasterNames[name] = struct{}{}
		rasterPath := strings.TrimSpace(raster.Path)
		if rasterPath == "" {
			return CropByPolygonsRequest{}, fmt.Errorf("inputs.rasters[%d].path is required", i)
		}
		rasters = append(rasters, CropRaster{Name: name, Path: rasterPath})
	}

	polygons := CropPolygonsSource{
		Path:    strings.TrimSpace(request.Inputs.Polygons.Path),
		Layer:   strings.TrimSpace(request.Inputs.Polygons.Layer),
		IDField: strings.TrimSpace(request.Inputs.Polygons.IDField),
	}
	if polygons.Path == "" {
		return CropByPolygonsRequest{}, fmt.Errorf("inputs.polygons.path is required")
	}
	if polygons.Layer == "" {
		return CropByPolygonsRequest{}, fmt.Errorf("inputs.polygons.layer is required")
	}
	if polygons.IDField == "" {
		return CropByPolygonsRequest{}, fmt.Errorf("inputs.polygons.id_field is required")
	}

	outputDirectory, err := normalizeArtifactDirectory(request.Outputs.OutputDirectory)
	if err != nil {
		return CropByPolygonsRequest{}, fmt.Errorf("output %q path: %w", cropOutputDirectoryName, err)
	}
	manifestJSON, okErr := model.ValidateArtifactRelativePath(request.Outputs.ManifestJSON)
	if okErr != nil {
		return CropByPolygonsRequest{}, fmt.Errorf("output %q path: %w", cropManifestJSONName, okErr)
	}
	if !artifactPathIsUnderDirectory(manifestJSON, outputDirectory) {
		return CropByPolygonsRequest{}, fmt.Errorf("output %q must be under output_directory %q", cropManifestJSONName, outputDirectory)
	}

	mode := strings.ToLower(strings.TrimSpace(request.Options.Mode))
	if mode == "" {
		mode = cropModeBBox
	}
	if mode == "cutline" {
		return CropByPolygonsRequest{}, fmt.Errorf("crop_by_polygons mode %q is not supported in this slice; supported mode: %q", mode, cropModeBBox)
	}
	if mode != cropModeBBox {
		return CropByPolygonsRequest{}, fmt.Errorf("unsupported crop_by_polygons mode %q; supported mode: %q", mode, cropModeBBox)
	}
	if request.Options.MaskToPolygon {
		return CropByPolygonsRequest{}, fmt.Errorf("mask_to_polygon=true is not supported in bbox mode")
	}

	maxFeatures := defaultCropMaxFeatures
	if request.Options.MaxFeatures != nil {
		maxFeatures = *request.Options.MaxFeatures
	}
	if maxFeatures <= 0 {
		return CropByPolygonsRequest{}, fmt.Errorf("max_features must be greater than 0")
	}

	sort.Slice(rasters, func(i, j int) bool { return rasters[i].Name < rasters[j].Name })
	return CropByPolygonsRequest{
		Rasters:         rasters,
		Polygons:        polygons,
		OutputDirectory: outputDirectory,
		ManifestJSON:    manifestJSON,
		Mode:            mode,
		MaxFeatures:     maxFeatures,
	}, nil
}

func normalizeArtifactDirectory(value string) (string, error) {
	trimmed := strings.TrimRight(value, "/")
	if trimmed == "" {
		return "", fmt.Errorf("artifact path is required")
	}
	return model.ValidateArtifactRelativePath(trimmed)
}

func artifactPathIsUnderDirectory(relativePath string, directory string) bool {
	return relativePath == directory || strings.HasPrefix(relativePath, directory+"/")
}

func inspectCropVectorSummary(ctx context.Context, source CropPolygonsSource) (int, int, error) {
	doc, err := runOGRInfo(ctx, source, "-so")
	if err != nil {
		return 0, 0, err
	}
	layer, err := singleOGRLayer(doc, source.Layer)
	if err != nil {
		return 0, 0, err
	}
	epsg := 0
	for _, field := range layer.GeometryFields {
		if strings.TrimSpace(field.CoordinateSystem.WKT) == "" {
			continue
		}
		epsg = parseCropEPSGFromWKT(field.CoordinateSystem.WKT)
		if epsg != 0 {
			break
		}
	}
	return epsg, layer.FeatureCount, nil
}

func parseCropEPSGFromWKT(wkt string) int {
	matches := cropEPSGRegex.FindAllStringSubmatch(wkt, -1)
	epsg := 0
	for _, match := range matches {
		for _, candidate := range match[1:] {
			if candidate == "" {
				continue
			}
			value, err := strconv.Atoi(candidate)
			if err != nil {
				continue
			}
			epsg = value
		}
	}
	return epsg
}

func readCropVectorFeatures(ctx context.Context, source CropPolygonsSource) ([]cropFeature, error) {
	doc, err := runOGRInfo(ctx, source, "-features")
	if err != nil {
		return nil, err
	}
	layer, err := singleOGRLayer(doc, source.Layer)
	if err != nil {
		return nil, err
	}

	features := make([]cropFeature, 0, len(layer.Features))
	for i, rawFeature := range layer.Features {
		id, err := cropFeatureID(rawFeature.Properties, source.IDField)
		if err != nil {
			return nil, fmt.Errorf("feature %d: %w", i, err)
		}
		box, err := geometryBounds(rawFeature.Geometry)
		if err != nil {
			return nil, fmt.Errorf("feature %q: %w", id, err)
		}
		features = append(features, cropFeature{
			FID: cropFIDString(rawFeature.FID),
			ID:  id,
			Box: box,
		})
	}
	return features, nil
}

func runOGRInfo(ctx context.Context, source CropPolygonsSource, mode string) (ogrInfoDocument, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{"-json", mode, source.Path, source.Layer}
	cmd := exec.CommandContext(cmdCtx, "ogrinfo", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ogrInfoDocument{}, fmt.Errorf("ogrinfo: %w: %s", err, strings.TrimSpace(string(output)))
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	var doc ogrInfoDocument
	if err := decoder.Decode(&doc); err != nil {
		return ogrInfoDocument{}, fmt.Errorf("parse ogrinfo json: %w", err)
	}
	return doc, nil
}

func singleOGRLayer(doc ogrInfoDocument, name string) (ogrLayer, error) {
	for _, layer := range doc.Layers {
		if layer.Name == name {
			return layer, nil
		}
	}
	return ogrLayer{}, fmt.Errorf("polygon layer %q not found", name)
}

func cropFeatureID(properties map[string]any, idField string) (string, error) {
	if properties == nil {
		return "", fmt.Errorf("properties are missing")
	}
	value, ok := properties[idField]
	if !ok {
		return "", fmt.Errorf("id field %q is missing", idField)
	}
	id := strings.TrimSpace(cropValueString(value))
	if id == "" {
		return "", fmt.Errorf("id field %q is empty", idField)
	}
	return id, nil
}

func cropValueString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		if math.Trunc(typed) == typed {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func cropFIDString(value any) string {
	if value == nil {
		return ""
	}
	return cropValueString(value)
}

func geometryBounds(geometry any) (RasterBounds, error) {
	box := boundsAccumulator{}
	walkGeometryCoordinates(geometry, &box)
	if !box.seen {
		return RasterBounds{}, fmt.Errorf("geometry has no coordinates")
	}
	return RasterBounds{MinX: box.minX, MinY: box.minY, MaxX: box.maxX, MaxY: box.maxY}, nil
}

type boundsAccumulator struct {
	seen bool
	minX float64
	minY float64
	maxX float64
	maxY float64
}

func (box *boundsAccumulator) add(x float64, y float64) {
	if !box.seen {
		box.seen = true
		box.minX, box.maxX = x, x
		box.minY, box.maxY = y, y
		return
	}
	box.minX = math.Min(box.minX, x)
	box.minY = math.Min(box.minY, y)
	box.maxX = math.Max(box.maxX, x)
	box.maxY = math.Max(box.maxY, y)
}

func walkGeometryCoordinates(value any, box *boundsAccumulator) {
	switch typed := value.(type) {
	case map[string]any:
		if coordinates, ok := typed["coordinates"]; ok {
			walkCoordinateArray(coordinates, box)
		}
		if geometries, ok := typed["geometries"]; ok {
			walkGeometryCoordinates(geometries, box)
		}
	case []any:
		for _, item := range typed {
			walkGeometryCoordinates(item, box)
		}
	}
}

func walkCoordinateArray(value any, box *boundsAccumulator) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	if len(items) >= 2 {
		x, xOK := numberAsFloat(items[0])
		y, yOK := numberAsFloat(items[1])
		if xOK && yOK {
			box.add(x, y)
			return
		}
	}
	for _, item := range items {
		walkCoordinateArray(item, box)
	}
}

func numberAsFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		f, err := typed.Float64()
		return f, err == nil
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func runGDALBBoxCrop(ctx context.Context, sourcePath string, outputPath string, box RasterBounds) error {
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{
		"-overwrite",
		"-of", "GTiff",
		"-te",
		cropFormatGDALFloat(box.MinX),
		cropFormatGDALFloat(box.MinY),
		cropFormatGDALFloat(box.MaxX),
		cropFormatGDALFloat(box.MaxY),
		sourcePath,
		outputPath,
	}
	cmd := exec.CommandContext(cmdCtx, "gdalwarp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gdalwarp: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func collectOneRasterMetadata(name string, input InputSpec) (RasterMetadata, error) {
	records, err := CollectRasterMetadata(map[string]InputSpec{name: input})
	if err != nil {
		return RasterMetadata{}, err
	}
	if len(records) != 1 {
		return RasterMetadata{}, fmt.Errorf("expected one raster metadata record, got %d", len(records))
	}
	return records[0], nil
}

func cropArtifactPath(root string, relativePath string) (string, error) {
	clean, err := model.ValidateArtifactRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(rootAbs, filepath.FromSlash(clean))
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("artifact path escapes artifact root")
	}
	return candidate, nil
}

func cropWriteJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metadata output parent: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata json: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write metadata json: %w", err)
	}
	return nil
}

func cropFormatGDALFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	segment := strings.Trim(builder.String(), ".")
	if segment == "" {
		return "id"
	}
	return segment
}

func featureFilenameSegment(featureID string, index int) string {
	sum := sha256.Sum256([]byte(featureID))
	return fmt.Sprintf("%03d-%s-%s", index+1, safePathSegment(featureID), hex.EncodeToString(sum[:4]))
}

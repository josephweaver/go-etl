package geospatial

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"goetl/internal/model"
)

const (
	OperationPolygonizeRaster = "polygonize_raster"

	polygonizeRasterInputName = "raster"
	polygonizeVectorOutName   = "vector"
	polygonizeMetadataOutName = "metadata_json"

	defaultPolygonizeConnectivity = 4
	defaultPolygonizeMaxFeatures  = 10000
	defaultPolygonizeValueField   = "value"
)

type polygonizeRasterEnvelope struct {
	APIVersion string                  `json:"api_version"`
	Kind       string                  `json:"kind"`
	Operation  string                  `json:"operation"`
	Inputs     polygonizeRasterInputs  `json:"inputs"`
	Outputs    polygonizeRasterOutputs `json:"outputs"`
	Options    polygonizeRasterOptions `json:"options"`
}

type polygonizeRasterInputs struct {
	Raster *polygonizeRasterInput `json:"raster"`
}

type polygonizeRasterInput struct {
	Path   string `json:"path"`
	Band   *int   `json:"band"`
	Nodata *int   `json:"nodata"`
}

type polygonizeRasterOutputs struct {
	Vector       string `json:"vector"`
	MetadataJSON string `json:"metadata_json"`
}

type polygonizeRasterOptions struct {
	ValueField   string `json:"value_field"`
	Connectivity *int   `json:"connectivity"`
	MaxFeatures  *int   `json:"max_features"`
}

type PolygonizeRasterRequest struct {
	Raster       polygonizeRasterSource
	VectorPath   string
	MetadataPath string
	ValueField   string
	Connectivity int
	MaxFeatures  int
	LayerName    string
}

type polygonizeRasterSource struct {
	Path   string
	Band   int
	Nodata *int
}

type PolygonizeRasterMetadata struct {
	Operation    string                         `json:"operation"`
	FeatureCount int                            `json:"feature_count"`
	ValueField   string                         `json:"value_field"`
	Connectivity int                            `json:"connectivity"`
	MaxFeatures  int                            `json:"max_features"`
	NodataPolicy PolygonizeNodataPolicy         `json:"nodata_policy"`
	SourceRaster PolygonizeSourceRasterEvidence `json:"source_raster"`
	Vector       PolygonizeVectorEvidence       `json:"vector"`
	GDALVersion  string                         `json:"gdal_version"`
	Warnings     []string                       `json:"warnings"`
}

type PolygonizeNodataPolicy struct {
	Excluded bool   `json:"excluded"`
	Nodata   *int   `json:"nodata,omitempty"`
	Rule     string `json:"rule"`
}

type PolygonizeSourceRasterEvidence struct {
	Path     string         `json:"path"`
	Band     int            `json:"band"`
	Metadata RasterMetadata `json:"metadata"`
}

type PolygonizeVectorEvidence struct {
	Path   string `json:"path"`
	Layer  string `json:"layer"`
	Format string `json:"format"`
}

func ExecutePolygonizeRaster(ctx context.Context, requestData []byte, artifactRoot string) (OperationResult, error) {
	parsed, err := ParsePolygonizeRasterRequest(requestData)
	if err != nil {
		return OperationResult{}, err
	}

	vectorPath, err := artifactPath(artifactRoot, parsed.VectorPath)
	if err != nil {
		return OperationResult{}, fmt.Errorf("vector output path: %w", err)
	}
	metadataPath, err := artifactPath(artifactRoot, parsed.MetadataPath)
	if err != nil {
		return OperationResult{}, fmt.Errorf("metadata_json output path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(vectorPath), 0o755); err != nil {
		return OperationResult{}, fmt.Errorf("create vector output parent: %w", err)
	}

	sourceMetadata, sourceBand, err := polygonizeRasterMetadata(parsed.Raster)
	if err != nil {
		return OperationResult{}, err
	}
	nodata := firstNonNilInt(parsed.Raster.Nodata, sourceBand.Nodata)

	if err := os.Remove(vectorPath); err != nil && !os.IsNotExist(err) {
		return OperationResult{}, fmt.Errorf("remove existing vector output: %w", err)
	}

	sourcePath := parsed.Raster.Path
	sourceBandIndex := parsed.Raster.Band
	cleanup := func() {}
	if parsed.Raster.Nodata != nil {
		preparedPath, preparedCleanup, err := preparePolygonizeRasterSource(ctx, artifactRoot, parsed.Raster)
		if err != nil {
			return OperationResult{}, err
		}
		sourcePath = preparedPath
		sourceBandIndex = 1
		cleanup = preparedCleanup
	}
	defer cleanup()

	if err := runGDALPolygonize(ctx, sourcePath, sourceBandIndex, vectorPath, parsed.LayerName, parsed.ValueField, parsed.Connectivity); err != nil {
		return OperationResult{}, err
	}

	featureCount, err := polygonizedFeatureCount(ctx, vectorPath, parsed.LayerName)
	if err != nil {
		return OperationResult{}, err
	}
	if featureCount > parsed.MaxFeatures {
		_ = os.Remove(vectorPath)
		return OperationResult{}, fmt.Errorf("polygonized feature count %d exceeds max_features %d", featureCount, parsed.MaxFeatures)
	}

	version, err := gdalVersion(ctx)
	if err != nil {
		return OperationResult{}, err
	}

	warnings := []string{
		"polygonize_raster is optional and is not part of the CDL/Yan/Roy raster-pair count workflow",
		"max_features is enforced after GDAL polygonization; large rasters can create large temporary output before rejection",
	}
	metadata := PolygonizeRasterMetadata{
		Operation:    OperationPolygonizeRaster,
		FeatureCount: featureCount,
		ValueField:   parsed.ValueField,
		Connectivity: parsed.Connectivity,
		MaxFeatures:  parsed.MaxFeatures,
		NodataPolicy: PolygonizeNodataPolicy{
			Excluded: nodata != nil,
			Nodata:   nodata,
			Rule:     "declared or raster-band nodata is assigned to the source mask and excluded by GDAL polygonize",
		},
		SourceRaster: PolygonizeSourceRasterEvidence{
			Path:     parsed.Raster.Path,
			Band:     parsed.Raster.Band,
			Metadata: sourceMetadata,
		},
		Vector: PolygonizeVectorEvidence{
			Path:   parsed.VectorPath,
			Layer:  parsed.LayerName,
			Format: "GPKG",
		},
		GDALVersion: version,
		Warnings:    warnings,
	}
	if err := writeJSONFile(metadataPath, metadata); err != nil {
		return OperationResult{}, err
	}

	result := NewValidationResult(OperationPolygonizeRaster)
	result.Artifacts = []ArtifactResult{
		{Name: polygonizeVectorOutName, Path: parsed.VectorPath, Kind: "vector", Format: "gpkg"},
		{Name: polygonizeMetadataOutName, Path: parsed.MetadataPath, Kind: "metadata", Format: "json"},
	}
	result.Summary = map[string]any{
		"vector":        parsed.VectorPath,
		"metadata_json": parsed.MetadataPath,
		"feature_count": featureCount,
		"value_field":   parsed.ValueField,
		"connectivity":  parsed.Connectivity,
		"nodata":        nodata,
	}
	result.Warnings = warnings
	return result, nil
}

func ParsePolygonizeRasterRequest(requestData []byte) (PolygonizeRasterRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(requestData))
	decoder.DisallowUnknownFields()
	var request polygonizeRasterEnvelope
	if err := decoder.Decode(&request); err != nil {
		return PolygonizeRasterRequest{}, fmt.Errorf("decode request: %w", err)
	}

	if request.APIVersion != APIVersionV1Alpha1 {
		return PolygonizeRasterRequest{}, fmt.Errorf("unsupported api_version %q", request.APIVersion)
	}
	if request.Kind != RequestKind {
		return PolygonizeRasterRequest{}, fmt.Errorf("unsupported kind %q", request.Kind)
	}
	if request.Operation != OperationPolygonizeRaster {
		return PolygonizeRasterRequest{}, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	if request.Inputs.Raster == nil {
		return PolygonizeRasterRequest{}, fmt.Errorf("%s requires input %q", OperationPolygonizeRaster, polygonizeRasterInputName)
	}
	rasterPath := strings.TrimSpace(request.Inputs.Raster.Path)
	if rasterPath == "" {
		return PolygonizeRasterRequest{}, fmt.Errorf("input %q path is required", polygonizeRasterInputName)
	}
	if request.Inputs.Raster.Band == nil {
		return PolygonizeRasterRequest{}, fmt.Errorf("input %q requires band", polygonizeRasterInputName)
	}
	if *request.Inputs.Raster.Band <= 0 {
		return PolygonizeRasterRequest{}, fmt.Errorf("input %q band must be greater than 0", polygonizeRasterInputName)
	}
	if err := validateRasterPairNodata("input raster.nodata", request.Inputs.Raster.Nodata); err != nil {
		return PolygonizeRasterRequest{}, err
	}

	vectorPath, err := model.ValidateArtifactRelativePath(request.Outputs.Vector)
	if err != nil {
		return PolygonizeRasterRequest{}, fmt.Errorf("output %q path: %w", polygonizeVectorOutName, err)
	}
	metadataPath, err := model.ValidateArtifactRelativePath(request.Outputs.MetadataJSON)
	if err != nil {
		return PolygonizeRasterRequest{}, fmt.Errorf("output %q path: %w", polygonizeMetadataOutName, err)
	}

	valueField := strings.TrimSpace(request.Options.ValueField)
	if valueField == "" {
		valueField = defaultPolygonizeValueField
	}
	if err := validateOGRFieldName(valueField); err != nil {
		return PolygonizeRasterRequest{}, fmt.Errorf("value_field: %w", err)
	}

	connectivity := defaultPolygonizeConnectivity
	if request.Options.Connectivity != nil {
		connectivity = *request.Options.Connectivity
	}
	if connectivity != 4 && connectivity != 8 {
		return PolygonizeRasterRequest{}, fmt.Errorf("connectivity must be 4 or 8")
	}

	maxFeatures := defaultPolygonizeMaxFeatures
	if request.Options.MaxFeatures != nil {
		maxFeatures = *request.Options.MaxFeatures
	}
	if maxFeatures <= 0 {
		return PolygonizeRasterRequest{}, fmt.Errorf("max_features must be greater than 0")
	}

	return PolygonizeRasterRequest{
		Raster: polygonizeRasterSource{
			Path:   rasterPath,
			Band:   *request.Inputs.Raster.Band,
			Nodata: request.Inputs.Raster.Nodata,
		},
		VectorPath:   vectorPath,
		MetadataPath: metadataPath,
		ValueField:   valueField,
		Connectivity: connectivity,
		MaxFeatures:  maxFeatures,
		LayerName:    polygonizeLayerName(vectorPath),
	}, nil
}

func polygonizeRasterMetadata(source polygonizeRasterSource) (RasterMetadata, RasterBandMetadata, error) {
	metadata, err := collectSingleRasterMetadata("raster", InputSpec{Path: source.Path, Band: &source.Band, Nodata: source.Nodata})
	if err != nil {
		return RasterMetadata{}, RasterBandMetadata{}, fmt.Errorf("collect source raster metadata: %w", err)
	}
	if source.Band > len(metadata.Bands) {
		return RasterMetadata{}, RasterBandMetadata{}, fmt.Errorf("input %q band %d does not exist", polygonizeRasterInputName, source.Band)
	}
	return metadata, metadata.Bands[source.Band-1], nil
}

func preparePolygonizeRasterSource(ctx context.Context, artifactRoot string, source polygonizeRasterSource) (string, func(), error) {
	tempDir, err := os.MkdirTemp(artifactRoot, ".polygonize-*")
	if err != nil {
		return "", nil, fmt.Errorf("create polygonize temp directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	tempPath := filepath.Join(tempDir, "source.tif")

	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{
		"-q",
		"-of", "GTiff",
		"-b", strconv.Itoa(source.Band),
		"-a_nodata", strconv.Itoa(*source.Nodata),
		source.Path,
		tempPath,
	}
	cmd := exec.CommandContext(cmdCtx, "gdal_translate", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("gdal_translate nodata source: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return tempPath, cleanup, nil
}

func runGDALPolygonize(ctx context.Context, rasterPath string, band int, vectorPath string, layerName string, valueField string, connectivity int) error {
	command, err := polygonizeCommand()
	if err != nil {
		return err
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	args := []string{"-q"}
	if connectivity == 8 {
		args = append(args, "-8")
	}
	args = append(args,
		rasterPath,
		"-b", strconv.Itoa(band),
		"-f", "GPKG",
		vectorPath,
		layerName,
		valueField,
	)
	cmd := exec.CommandContext(cmdCtx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", command, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func polygonizeCommand() (string, error) {
	for _, candidate := range []string{"gdal_polygonize.py", "gdal_polygonize"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("gdal_polygonize.py not available in PATH")
}

func polygonizedFeatureCount(ctx context.Context, vectorPath string, layerName string) (int, error) {
	doc, err := runOGRInfo(ctx, CropPolygonsSource{Path: vectorPath, Layer: layerName}, "-so")
	if err != nil {
		return 0, err
	}
	layer, err := singleOGRLayer(doc, layerName)
	if err != nil {
		return 0, err
	}
	return layer.FeatureCount, nil
}

func polygonizeLayerName(vectorPath string) string {
	base := strings.TrimSuffix(path.Base(vectorPath), path.Ext(vectorPath))
	name := safePathSegment(base)
	if name == "" || name == "id" {
		return "polygonized"
	}
	return name
}

func validateOGRFieldName(value string) error {
	if value == "" {
		return fmt.Errorf("is required")
	}
	for i, r := range value {
		switch {
		case r == '_':
		case unicode.IsLetter(r):
		case unicode.IsDigit(r) && i > 0:
		default:
			return fmt.Errorf("must contain only letters, digits after the first character, and underscores")
		}
	}
	return nil
}

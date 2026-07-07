package geospatial

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"goetl/internal/model"
)

const (
	alignmentInputName   = "source_raster"
	alignmentRasterOut   = "raster_tif"
	alignmentMetadataOut = "metadata_json"

	defaultAlignmentResampling = "nearest"
)

type AlignmentRequest struct {
	Operation           string
	Source              InputSpec
	RasterOutputPath    string
	MetadataOutputPath  string
	TargetGrid          *RasterGrid
	LikeRaster          string
	Resampling          string
	AllowUnsafeResample bool
}

type alignmentOptions struct {
	TargetCRS           string    `json:"target_crs"`
	TargetTransform     []float64 `json:"target_transform"`
	TargetWidth         *int      `json:"target_width"`
	TargetHeight        *int      `json:"target_height"`
	LikeRaster          string    `json:"like_raster"`
	Resampling          string    `json:"resampling"`
	AllowUnsafeResample bool      `json:"allow_unsafe_resampling"`
}

type AlignmentMetadata struct {
	Operation    string                  `json:"operation"`
	Source       AlignmentRasterMetadata `json:"source"`
	TargetMode   string                  `json:"target_mode"`
	Target       RasterGrid              `json:"target_grid"`
	Output       AlignmentRasterMetadata `json:"output"`
	Resampling   string                  `json:"resampling"`
	NodataPolicy AlignmentNodataPolicy   `json:"nodata_policy"`
	GDALVersion  string                  `json:"gdal_version"`
}

type AlignmentRasterMetadata struct {
	Path   string     `json:"path"`
	Grid   RasterGrid `json:"grid"`
	DType  string     `json:"dtype"`
	Nodata *int       `json:"nodata,omitempty"`
}

type AlignmentNodataPolicy struct {
	SourceNodata *int   `json:"source_nodata,omitempty"`
	OutputNodata *int   `json:"output_nodata,omitempty"`
	Rule         string `json:"rule"`
}

func ParseAlignmentRequest(request OperationRequest) (AlignmentRequest, error) {
	if request.Operation != OperationAlignToGrid && request.Operation != OperationReprojectCRS {
		return AlignmentRequest{}, fmt.Errorf("unsupported alignment operation %q", request.Operation)
	}

	source, ok := request.Inputs[alignmentInputName]
	if !ok {
		return AlignmentRequest{}, fmt.Errorf("%s operation requires input %q", request.Operation, alignmentInputName)
	}
	if source.Band != nil && *source.Band != 1 {
		return AlignmentRequest{}, fmt.Errorf("input %q band must be 1 for this operation", alignmentInputName)
	}

	rasterOut, ok := request.Outputs[alignmentRasterOut]
	if !ok {
		return AlignmentRequest{}, fmt.Errorf("%s operation requires output %q", request.Operation, alignmentRasterOut)
	}
	metadataOut, ok := request.Outputs[alignmentMetadataOut]
	if !ok {
		return AlignmentRequest{}, fmt.Errorf("%s operation requires output %q", request.Operation, alignmentMetadataOut)
	}

	options, err := decodeAlignmentOptions(request.Options)
	if err != nil {
		return AlignmentRequest{}, err
	}

	parsed := AlignmentRequest{
		Operation:           request.Operation,
		Source:              source,
		RasterOutputPath:    rasterOut,
		MetadataOutputPath:  metadataOut,
		LikeRaster:          strings.TrimSpace(options.LikeRaster),
		Resampling:          normalizeResampling(options.Resampling),
		AllowUnsafeResample: options.AllowUnsafeResample,
	}
	if parsed.Resampling == "" {
		parsed.Resampling = defaultAlignmentResampling
	}
	if err := validateCategoricalResampling(parsed.Resampling, parsed.AllowUnsafeResample); err != nil {
		return AlignmentRequest{}, err
	}

	hasExplicitTarget := strings.TrimSpace(options.TargetCRS) != "" ||
		options.TargetTransform != nil ||
		options.TargetWidth != nil ||
		options.TargetHeight != nil
	if parsed.LikeRaster != "" && hasExplicitTarget {
		return AlignmentRequest{}, fmt.Errorf("target grid must use either like_raster or explicit target_crs/target_transform/target_width/target_height, not both")
	}
	if parsed.LikeRaster == "" && !hasExplicitTarget {
		return AlignmentRequest{}, fmt.Errorf("target grid is required")
	}
	if hasExplicitTarget {
		if strings.TrimSpace(options.TargetCRS) == "" ||
			options.TargetTransform == nil ||
			options.TargetWidth == nil ||
			options.TargetHeight == nil {
			return AlignmentRequest{}, fmt.Errorf("explicit target grid requires target_crs, target_transform, target_width, and target_height")
		}
		target := RasterGrid{
			CRS:           strings.TrimSpace(options.TargetCRS),
			CRSWKTPresent: true,
			GeoTransform:  append([]float64(nil), options.TargetTransform...),
			Width:         *options.TargetWidth,
			Height:        *options.TargetHeight,
		}
		if err := ValidateWarpTargetGrid(target); err != nil {
			return AlignmentRequest{}, err
		}
		parsed.TargetGrid = &target
	}

	return parsed, nil
}

func ExecuteRasterAlignment(ctx context.Context, request OperationRequest, artifactRoot string) (OperationResult, error) {
	parsed, err := ParseAlignmentRequest(request)
	if err != nil {
		return OperationResult{}, err
	}

	sourceMetadata, err := collectSingleRasterMetadata("source", parsed.Source)
	if err != nil {
		return OperationResult{}, err
	}
	if !sourceMetadata.CRSWKTPresent {
		return OperationResult{}, fmt.Errorf("source raster CRS is missing; refusing to guess")
	}

	targetMode := "explicit"
	targetGrid := RasterGrid{}
	if parsed.TargetGrid != nil {
		targetGrid = *parsed.TargetGrid
	} else {
		targetMode = "like_raster"
		likeMetadata, err := collectSingleRasterMetadata("like_raster", InputSpec{Path: parsed.LikeRaster})
		if err != nil {
			return OperationResult{}, err
		}
		if !likeMetadata.CRSWKTPresent {
			return OperationResult{}, fmt.Errorf("like_raster CRS is missing; refusing to guess")
		}
		targetGrid = GridFromMetadata(likeMetadata)
		if targetGrid.CRS == "" {
			return OperationResult{}, fmt.Errorf("like_raster CRS must resolve to an EPSG code or use an explicit target_crs")
		}
		if err := ValidateWarpTargetGrid(targetGrid); err != nil {
			return OperationResult{}, err
		}
	}

	rasterPath, err := artifactPath(artifactRoot, parsed.RasterOutputPath)
	if err != nil {
		return OperationResult{}, fmt.Errorf("raster output path: %w", err)
	}
	metadataPath, err := artifactPath(artifactRoot, parsed.MetadataOutputPath)
	if err != nil {
		return OperationResult{}, fmt.Errorf("metadata output path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(rasterPath), 0o755); err != nil {
		return OperationResult{}, fmt.Errorf("create raster output parent: %w", err)
	}

	sourceBand := firstBandMetadata(sourceMetadata)
	sourceNodata := parsed.Source.Nodata
	if sourceNodata == nil {
		sourceNodata = sourceBand.Nodata
	}
	if err := runGDALWarp(ctx, parsed.Source.Path, rasterPath, targetGrid, parsed.Resampling, sourceNodata); err != nil {
		return OperationResult{}, err
	}

	outputMetadata, err := collectSingleRasterMetadata("output", InputSpec{Path: rasterPath})
	if err != nil {
		return OperationResult{}, fmt.Errorf("read output raster metadata: %w", err)
	}
	outputBand := firstBandMetadata(outputMetadata)

	version, err := gdalVersion(ctx)
	if err != nil {
		return OperationResult{}, err
	}

	metadata := AlignmentMetadata{
		Operation:  parsed.Operation,
		TargetMode: targetMode,
		Source: AlignmentRasterMetadata{
			Path:   parsed.Source.Path,
			Grid:   GridFromMetadata(sourceMetadata),
			DType:  sourceBand.DType,
			Nodata: sourceNodata,
		},
		Target: targetGrid,
		Output: AlignmentRasterMetadata{
			Path:   parsed.RasterOutputPath,
			Grid:   GridFromMetadata(outputMetadata),
			DType:  outputBand.DType,
			Nodata: outputBand.Nodata,
		},
		Resampling: parsed.Resampling,
		NodataPolicy: AlignmentNodataPolicy{
			SourceNodata: sourceNodata,
			OutputNodata: outputBand.Nodata,
			Rule:         "source nodata is used as srcnodata and dstnodata when declared or present on band 1",
		},
		GDALVersion: version,
	}

	if err := writeJSONFile(metadataPath, metadata); err != nil {
		return OperationResult{}, err
	}

	result := NewValidationResult(parsed.Operation)
	result.Artifacts = []ArtifactResult{
		{Name: alignmentRasterOut, Path: parsed.RasterOutputPath, Kind: "raster", Format: "geotiff"},
		{Name: alignmentMetadataOut, Path: parsed.MetadataOutputPath, Kind: "metadata", Format: "json"},
	}
	result.Summary = map[string]any{
		"raster":      parsed.RasterOutputPath,
		"metadata":    parsed.MetadataOutputPath,
		"target_grid": targetGrid,
		"resampling":  parsed.Resampling,
	}
	return result, nil
}

func decodeAlignmentOptions(options map[string]any) (alignmentOptions, error) {
	if options == nil {
		options = map[string]any{}
	}
	data, err := json.Marshal(options)
	if err != nil {
		return alignmentOptions{}, fmt.Errorf("encode alignment options: %w", err)
	}
	var decoded alignmentOptions
	if err := json.Unmarshal(data, &decoded); err != nil {
		return alignmentOptions{}, fmt.Errorf("decode alignment options: %w", err)
	}
	return decoded, nil
}

func validateCategoricalResampling(resampling string, allowUnsafe bool) error {
	if _, ok := gdalResamplingArg(resampling); !ok {
		return fmt.Errorf("unsupported resampling %q", resampling)
	}
	if resampling != defaultAlignmentResampling && !allowUnsafe {
		return fmt.Errorf("resampling %q is unsafe for categorical rasters unless allow_unsafe_resampling is true", resampling)
	}
	return nil
}

func normalizeResampling(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "near":
		return defaultAlignmentResampling
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func gdalResamplingArg(value string) (string, bool) {
	switch value {
	case "nearest":
		return "near", true
	case "bilinear", "cubic", "cubicspline", "lanczos", "average", "mode", "max", "min", "med", "q1", "q3", "sum", "rms":
		return value, true
	default:
		return "", false
	}
}

func collectSingleRasterMetadata(name string, input InputSpec) (RasterMetadata, error) {
	records, err := CollectRasterMetadata(map[string]InputSpec{name: input})
	if err != nil {
		return RasterMetadata{}, err
	}
	if len(records) != 1 {
		return RasterMetadata{}, fmt.Errorf("expected one raster metadata record, got %d", len(records))
	}
	return records[0], nil
}

func firstBandMetadata(metadata RasterMetadata) RasterBandMetadata {
	if len(metadata.Bands) == 0 {
		return RasterBandMetadata{}
	}
	return metadata.Bands[0]
}

func runGDALWarp(ctx context.Context, sourcePath string, outputPath string, targetGrid RasterGrid, resampling string, nodata *int) error {
	bounds, err := BoundsFromGrid(targetGrid)
	if err != nil {
		return err
	}
	resamplingArg, ok := gdalResamplingArg(resampling)
	if !ok {
		return fmt.Errorf("unsupported resampling %q", resampling)
	}

	warpCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	args := []string{
		"-overwrite",
		"-of", "GTiff",
		"-t_srs", targetGrid.CRS,
		"-te", formatGDALFloat(bounds.MinX), formatGDALFloat(bounds.MinY), formatGDALFloat(bounds.MaxX), formatGDALFloat(bounds.MaxY),
		"-ts", strconv.Itoa(targetGrid.Width), strconv.Itoa(targetGrid.Height),
		"-r", resamplingArg,
	}
	if nodata != nil {
		nodataValue := strconv.Itoa(*nodata)
		args = append(args, "-srcnodata", nodataValue, "-dstnodata", nodataValue)
	}
	args = append(args, sourcePath, outputPath)

	cmd := exec.CommandContext(warpCtx, "gdalwarp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gdalwarp: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func gdalVersion(ctx context.Context) (string, error) {
	versionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(versionCtx, "gdalinfo", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gdalinfo --version: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func artifactPath(root string, relativePath string) (string, error) {
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

func writeJSONFile(path string, value any) error {
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

func formatGDALFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

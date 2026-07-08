package geospatial

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	stackAlignedDefaultDType       = "uint16"
	stackAlignedOutputDTypeUInt16  = "UInt16"
	stackAlignedRequireAlignedGrid = true
)

type StackAlignedInput struct {
	Name       string
	Path       string
	Band       int
	OutputBand int
	DType      string
	Nodata     *int
	Grid       RasterGrid
}

type StackAlignedOutputBand struct {
	OutputBand   int    `json:"output_band"`
	SourceName   string `json:"source_name"`
	SourceBand   int    `json:"source_band"`
	SourcePath   string `json:"source_path"`
	SourceDType  string `json:"source_dtype"`
	SourceNodata *int   `json:"source_nodata,omitempty"`
}

type StackAlignedMetadata struct {
	Operation      string                   `json:"operation"`
	RequireAligned bool                     `json:"require_aligned_grid"`
	OutputDType    string                   `json:"dtype"`
	OutputNodata   *int                     `json:"nodata"`
	Grid           RasterGrid               `json:"grid"`
	Sources        []StackAlignedOutputBand `json:"sources"`
	GDALVersion    string                   `json:"gdal_version"`
}

type stackAlignedRequest struct {
	Inputs         []StackAlignedInput
	OutputPath     string
	MetadataPath   string
	OutputDType    string
	OutputGDALType string
	OutputNodata   *int
}

type stackAlignedOptions struct {
	DType              string `json:"dtype"`
	Nodata             *int   `json:"nodata"`
	RequireAlignedGrid *bool  `json:"require_aligned_grid"`
}

func ParseStackAlignedRequest(request OperationRequest) (stackAlignedRequest, error) {
	if request.Operation != OperationStackAligned {
		return stackAlignedRequest{}, fmt.Errorf("unsupported stack operation %q", request.Operation)
	}
	if len(request.Inputs) == 0 {
		return stackAlignedRequest{}, fmt.Errorf("stack_aligned_rasters requires at least one input")
	}

	stackedPath, ok := request.Outputs["stacked_raster"]
	if !ok {
		return stackAlignedRequest{}, fmt.Errorf("stack_aligned_rasters requires output %q", "stacked_raster")
	}
	metadataPath, ok := request.Outputs["metadata_json"]
	if !ok {
		return stackAlignedRequest{}, fmt.Errorf("stack_aligned_rasters requires output %q", "metadata_json")
	}

	options, err := decodeStackAlignedOptions(request.Options)
	if err != nil {
		return stackAlignedRequest{}, err
	}
	if options.RequireAlignedGrid != nil && !*options.RequireAlignedGrid {
		return stackAlignedRequest{}, fmt.Errorf("require_aligned_grid=false is not supported; aligned grids are required for this operation")
	}

	outputDType := strings.ToLower(strings.TrimSpace(options.DType))
	if outputDType == "" {
		outputDType = stackAlignedDefaultDType
	}
	if outputDType != stackAlignedDefaultDType {
		return stackAlignedRequest{}, fmt.Errorf("unsupported dtype %q; supported: %q", outputDType, stackAlignedDefaultDType)
	}

	outputNodata := options.Nodata
	if outputNodata == nil {
		defaultNodata := 0
		outputNodata = &defaultNodata
	}
	if err := validateNodataForUInt16(*outputNodata); err != nil {
		return stackAlignedRequest{}, err
	}

	rawInputs := make([]StackAlignedInput, 0, len(request.Inputs))
	seenOutputBand := map[int]bool{}
	for name, input := range request.Inputs {
		if input.Band == nil {
			return stackAlignedRequest{}, fmt.Errorf("input %q requires band", name)
		}
		if *input.Band <= 0 {
			return stackAlignedRequest{}, fmt.Errorf("input %q band must be greater than 0", name)
		}
		if input.OutputBand == nil {
			return stackAlignedRequest{}, fmt.Errorf("input %q requires output_band", name)
		}
		if *input.OutputBand <= 0 {
			return stackAlignedRequest{}, fmt.Errorf("input %q output_band must be greater than 0", name)
		}
		if seenOutputBand[*input.OutputBand] {
			return stackAlignedRequest{}, fmt.Errorf("duplicate output_band %d", *input.OutputBand)
		}
		seenOutputBand[*input.OutputBand] = true
		rawInputs = append(rawInputs, StackAlignedInput{
			Name:       name,
			Path:       input.Path,
			Band:       *input.Band,
			OutputBand: *input.OutputBand,
			Nodata:     input.Nodata,
		})
	}

	sort.Slice(rawInputs, func(i, j int) bool {
		if rawInputs[i].OutputBand == rawInputs[j].OutputBand {
			return rawInputs[i].Name < rawInputs[j].Name
		}
		return rawInputs[i].OutputBand < rawInputs[j].OutputBand
	})

	for idx, input := range rawInputs {
		if input.OutputBand != idx+1 {
			return stackAlignedRequest{}, fmt.Errorf("output_band values must be contiguous and start at 1")
		}
	}

	return stackAlignedRequest{
		Inputs:         rawInputs,
		OutputPath:     stackedPath,
		MetadataPath:   metadataPath,
		OutputDType:    outputDType,
		OutputGDALType: stackAlignedOutputDTypeUInt16,
		OutputNodata:   outputNodata,
	}, nil
}

func ExecuteStackAlignedRasters(ctx context.Context, request OperationRequest, artifactRoot string) (OperationResult, error) {
	parsed, err := ParseStackAlignedRequest(request)
	if err != nil {
		return OperationResult{}, err
	}

	var referenceGrid RasterGrid
	for i := range parsed.Inputs {
		input := &parsed.Inputs[i]
		metadata, err := collectSingleRasterMetadata(input.Name, InputSpec{Path: input.Path})
		if err != nil {
			return OperationResult{}, fmt.Errorf("collect metadata for input %q: %w", input.Name, err)
		}

		if !metadata.CRSWKTPresent {
			return OperationResult{}, fmt.Errorf("input %q CRS is missing; refusing to guess", input.Name)
		}

		if input.Band > len(metadata.Bands) {
			return OperationResult{}, fmt.Errorf("input %q band %d does not exist", input.Name, input.Band)
		}

		sourceBand := metadata.Bands[input.Band-1]
		if sourceBand.DType != parsed.OutputGDALType {
			return OperationResult{}, fmt.Errorf("input %q dtype %q is not compatible with output dtype %q", input.Name, sourceBand.DType, parsed.OutputGDALType)
		}

		input.DType = sourceBand.DType
		input.Nodata = firstNonNilInt(input.Nodata, sourceBand.Nodata)
		input.Grid = GridFromMetadata(metadata)
		if input.Grid.CRS == "" {
			return OperationResult{}, fmt.Errorf("input %q CRS must resolve to an EPSG code", input.Name)
		}

		if i == 0 {
			referenceGrid = input.Grid
			continue
		}
		if !GridsEqual(referenceGrid, input.Grid) {
			return OperationResult{}, fmt.Errorf("input %q grid is not aligned to the first input grid", input.Name)
		}
	}

	outputPath, err := artifactPath(artifactRoot, parsed.OutputPath)
	if err != nil {
		return OperationResult{}, fmt.Errorf("stacked raster output path: %w", err)
	}
	metadataPath, err := artifactPath(artifactRoot, parsed.MetadataPath)
	if err != nil {
		return OperationResult{}, fmt.Errorf("metadata output path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return OperationResult{}, fmt.Errorf("create raster output parent: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		return OperationResult{}, fmt.Errorf("create metadata output parent: %w", err)
	}

	if err := runGDALStack(ctx, outputPath, parsed); err != nil {
		return OperationResult{}, err
	}

	outputMetadata, err := collectSingleRasterMetadata("output", InputSpec{Path: outputPath})
	if err != nil {
		return OperationResult{}, fmt.Errorf("read output raster metadata: %w", err)
	}
	outputGrid := GridFromMetadata(outputMetadata)
	version, err := gdalVersion(ctx)
	if err != nil {
		return OperationResult{}, err
	}

	sources := make([]StackAlignedOutputBand, 0, len(parsed.Inputs))
	for _, input := range parsed.Inputs {
		sources = append(sources, StackAlignedOutputBand{
			OutputBand:   input.OutputBand,
			SourceName:   input.Name,
			SourceBand:   input.Band,
			SourcePath:   input.Path,
			SourceDType:  input.DType,
			SourceNodata: input.Nodata,
		})
	}

	metadata := StackAlignedMetadata{
		Operation:      OperationStackAligned,
		RequireAligned: stackAlignedRequireAlignedGrid,
		OutputDType:    parsed.OutputGDALType,
		OutputNodata:   parsed.OutputNodata,
		Grid:           outputGrid,
		Sources:        sources,
		GDALVersion:    version,
	}
	if err := writeJSONFile(metadataPath, metadata); err != nil {
		return OperationResult{}, err
	}

	result := NewValidationResult(OperationStackAligned)
	result.Artifacts = []ArtifactResult{
		{Name: "stacked_raster", Path: parsed.OutputPath, Kind: "raster", Format: "geotiff"},
		{Name: "metadata_json", Path: parsed.MetadataPath, Kind: "metadata", Format: "json"},
	}
	result.Summary = map[string]any{
		"stacked_raster": parsed.OutputPath,
		"metadata_json":  parsed.MetadataPath,
		"band_count":     len(parsed.Inputs),
		"dtype":          parsed.OutputGDALType,
		"output_nodata":  parsed.OutputNodata,
	}
	return result, nil
}

func decodeStackAlignedOptions(options map[string]any) (stackAlignedOptions, error) {
	if options == nil {
		options = map[string]any{}
	}
	data, err := json.Marshal(options)
	if err != nil {
		return stackAlignedOptions{}, fmt.Errorf("encode stack options: %w", err)
	}
	var parsed stackAlignedOptions
	if err := json.Unmarshal(data, &parsed); err != nil {
		return stackAlignedOptions{}, fmt.Errorf("decode stack options: %w", err)
	}
	return parsed, nil
}

func runGDALStack(ctx context.Context, outputPath string, parsed stackAlignedRequest) error {
	tempRoot, err := os.MkdirTemp(filepath.Dir(outputPath), "stack-aligned-")
	if err != nil {
		return fmt.Errorf("create stack temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tempRoot)
	}()

	translated := make([]string, 0, len(parsed.Inputs))
	for idx, input := range parsed.Inputs {
		vrtPath := filepath.Join(tempRoot, fmt.Sprintf("input_%03d.vrt", idx+1))
		args := []string{
			"-of", "VRT",
			"-b", strconv.Itoa(input.Band),
			input.Path,
			vrtPath,
		}
		if err := runGDALCommand(ctx, "gdal_translate", args...); err != nil {
			return fmt.Errorf("extract band for %q: %w", input.Name, err)
		}
		translated = append(translated, vrtPath)
	}

	vrtPath := filepath.Join(tempRoot, "stack.vrt")
	buildArgs := append([]string{"-separate", vrtPath}, translated...)
	if err := runGDALCommand(ctx, "gdalbuildvrt", buildArgs...); err != nil {
		return fmt.Errorf("build separated VRT: %w", err)
	}

	translateArgs := []string{
		"-of", "GTiff",
		"-ot", parsed.OutputGDALType,
		"-a_nodata", strconv.Itoa(*parsed.OutputNodata),
		vrtPath,
		outputPath,
	}
	return runGDALCommand(ctx, "gdal_translate", translateArgs...)
}

func runGDALCommand(ctx context.Context, command string, args ...string) error {
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", command, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func validateNodataForUInt16(nodata int) error {
	if nodata < 0 || nodata > 65535 {
		return fmt.Errorf("nodata %d is out of range for dtype uint16", nodata)
	}
	return nil
}

func firstNonNilInt(left *int, right *int) *int {
	if left != nil {
		return left
	}
	return right
}

package geospatial

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"goetl/internal/model"
)

const (
	OperationRasterPairValueCounts = "raster_pair_value_counts"
	defaultChunkRows               = 1024
	uint16MaxValue                 = math.MaxUint16
	uint32MaxValue                 = math.MaxUint32
	countDTypeUint64               = "uint64"
)

type rasterPairValueCountsRequestEnvelope struct {
	APIVersion string                      `json:"api_version"`
	Kind       string                      `json:"kind"`
	Operation  string                      `json:"operation"`
	Inputs     rasterPairValueCountsInputs `json:"inputs"`
	Outputs    map[string]string           `json:"outputs"`
	Options    map[string]any              `json:"options"`
}

type rasterPairValueCountsInputs struct {
	FieldRaster   *rasterPairInputSpec        `json:"field_raster"`
	ValueRaster   *rasterPairInputSpec        `json:"value_raster"`
	StackedRaster *rasterPairStackedInputSpec `json:"stacked_raster"`
}

type rasterPairInputSpec struct {
	Path   string `json:"path"`
	Band   *int   `json:"band"`
	Nodata *int   `json:"nodata"`
}

type rasterPairStackedInputSpec struct {
	Path        string `json:"path"`
	FieldBand   *int   `json:"field_band"`
	ValueBand   *int   `json:"value_band"`
	FieldNodata *int   `json:"field_nodata"`
	ValueNodata *int   `json:"value_nodata"`
}

type rasterPairValueCountsOptions struct {
	RequireAlignedGrid *bool  `json:"require_aligned_grid"`
	ChunkRows          *int   `json:"chunk_rows"`
	FieldDType         string `json:"field_dtype"`
	ValueDType         string `json:"value_dtype"`
	IncludeValueNodata bool   `json:"include_value_nodata"`
}

type RasterPairValueCountsRequest struct {
	Mode               string
	Field              rasterPairBandSource
	Value              rasterPairBandSource
	CountsCSVPath      string
	MetadataJSONPath   string
	ChunkRows          int
	IncludeValueNodata bool
	RequireAlignedGrid bool
	FieldDType         string
	ValueDType         string
}

type rasterPairBandSource struct {
	Path   string
	Band   int
	Nodata *int
	Name   string
}

type PairCountRow struct {
	FieldID uint32
	ValueID uint16
	Count   uint64
}

type PairCountMetadata struct {
	ValidPixels        uint64 `json:"valid_pixels"`
	SkippedFieldNodata uint64 `json:"skipped_field_nodata"`
	SkippedValueNodata uint64 `json:"skipped_value_nodata"`
	DistinctFields     uint64 `json:"distinct_fields"`
	DistinctValues     uint64 `json:"distinct_values"`
	DistinctPairs      uint64 `json:"distinct_pairs"`
	CountDType         string `json:"count_dtype"`
}

type pairCountAccumulator struct {
	counts             map[uint64]uint64
	distinctFields     map[uint32]struct{}
	distinctValues     map[uint16]struct{}
	validPixels        uint64
	skippedFieldNodata uint64
	skippedValueNodata uint64
	includeValueNodata bool
}

type pairCountExecutionResult struct {
	rows     []PairCountRow
	metadata PairCountMetadata
}

var executeRasterPairValueCountsImpl = func(_ context.Context, _ RasterPairValueCountsRequest) (pairCountExecutionResult, error) {
	return pairCountExecutionResult{}, fmt.Errorf("%s requires a GDAL-enabled build (go build -tags gdal)", OperationRasterPairValueCounts)
}

func ExecuteRasterPairValueCounts(ctx context.Context, requestData []byte, artifactRoot string) (OperationResult, error) {
	parsed, err := ParseRasterPairValueCountsRequest(requestData)
	if err != nil {
		return OperationResult{}, err
	}

	executed, err := executeRasterPairValueCountsImpl(ctx, parsed)
	if err != nil {
		return OperationResult{}, err
	}

	countsPath, err := artifactPath(artifactRoot, parsed.CountsCSVPath)
	if err != nil {
		return OperationResult{}, fmt.Errorf("counts_csv output path: %w", err)
	}
	metadataPath, err := artifactPath(artifactRoot, parsed.MetadataJSONPath)
	if err != nil {
		return OperationResult{}, fmt.Errorf("metadata_json output path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(countsPath), 0o755); err != nil {
		return OperationResult{}, fmt.Errorf("create counts output parent: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o755); err != nil {
		return OperationResult{}, fmt.Errorf("create metadata output parent: %w", err)
	}
	if err := writePairCountsCSV(countsPath, executed.rows); err != nil {
		return OperationResult{}, err
	}
	if err := writeJSONFile(metadataPath, executed.metadata); err != nil {
		return OperationResult{}, err
	}

	result := NewValidationResult(OperationRasterPairValueCounts)
	result.Artifacts = []ArtifactResult{
		{Name: "counts_csv", Path: parsed.CountsCSVPath, Kind: "table", Format: "csv"},
		{Name: "metadata_json", Path: parsed.MetadataJSONPath, Kind: "metadata", Format: "json"},
	}
	result.Summary = pairCountSummaryMap(executed.metadata)
	return result, nil
}

func ParseRasterPairValueCountsRequest(requestData []byte) (RasterPairValueCountsRequest, error) {
	var request rasterPairValueCountsRequestEnvelope
	if err := json.Unmarshal(requestData, &request); err != nil {
		return RasterPairValueCountsRequest{}, fmt.Errorf("decode request: %w", err)
	}

	if request.APIVersion != APIVersionV1Alpha1 {
		return RasterPairValueCountsRequest{}, fmt.Errorf("unsupported api_version %q", request.APIVersion)
	}
	if request.Kind != RequestKind {
		return RasterPairValueCountsRequest{}, fmt.Errorf("unsupported kind %q", request.Kind)
	}
	if request.Operation != OperationRasterPairValueCounts {
		return RasterPairValueCountsRequest{}, fmt.Errorf("unsupported operation %q", request.Operation)
	}

	countsCSVPath, ok := request.Outputs["counts_csv"]
	if !ok {
		return RasterPairValueCountsRequest{}, fmt.Errorf("%s requires output %q", OperationRasterPairValueCounts, "counts_csv")
	}
	if _, err := model.ValidateArtifactRelativePath(countsCSVPath); err != nil {
		return RasterPairValueCountsRequest{}, fmt.Errorf("output %q path: %w", "counts_csv", err)
	}
	metadataJSONPath, ok := request.Outputs["metadata_json"]
	if !ok {
		return RasterPairValueCountsRequest{}, fmt.Errorf("%s requires output %q", OperationRasterPairValueCounts, "metadata_json")
	}
	if _, err := model.ValidateArtifactRelativePath(metadataJSONPath); err != nil {
		return RasterPairValueCountsRequest{}, fmt.Errorf("output %q path: %w", "metadata_json", err)
	}

	options, err := decodeRasterPairValueCountsOptions(request.Options)
	if err != nil {
		return RasterPairValueCountsRequest{}, err
	}
	requireAlignedGrid := true
	if options.RequireAlignedGrid != nil {
		requireAlignedGrid = *options.RequireAlignedGrid
	}
	if !requireAlignedGrid {
		return RasterPairValueCountsRequest{}, fmt.Errorf("require_aligned_grid=false is not supported; aligned grids are required for this operation")
	}

	chunkRows := defaultChunkRows
	if options.ChunkRows != nil {
		chunkRows = *options.ChunkRows
	}
	if chunkRows <= 0 {
		return RasterPairValueCountsRequest{}, fmt.Errorf("chunk_rows must be greater than 0")
	}

	fieldDType, err := normalizeRasterPairFieldDType(options.FieldDType)
	if err != nil {
		return RasterPairValueCountsRequest{}, err
	}
	valueDType, err := normalizeRasterPairValueDType(options.ValueDType)
	if err != nil {
		return RasterPairValueCountsRequest{}, err
	}

	parsed := RasterPairValueCountsRequest{
		CountsCSVPath:      countsCSVPath,
		MetadataJSONPath:   metadataJSONPath,
		ChunkRows:          chunkRows,
		IncludeValueNodata: options.IncludeValueNodata,
		RequireAlignedGrid: requireAlignedGrid,
		FieldDType:         fieldDType,
		ValueDType:         valueDType,
	}

	hasSeparate := request.Inputs.FieldRaster != nil || request.Inputs.ValueRaster != nil
	hasStacked := request.Inputs.StackedRaster != nil
	switch {
	case hasSeparate && hasStacked:
		return RasterPairValueCountsRequest{}, fmt.Errorf("inputs must use either field_raster/value_raster or stacked_raster, not both")
	case hasSeparate:
		if request.Inputs.FieldRaster == nil || request.Inputs.ValueRaster == nil {
			return RasterPairValueCountsRequest{}, fmt.Errorf("separate raster mode requires both %q and %q", "field_raster", "value_raster")
		}
		field, err := parseRasterPairBandSource("field_raster", request.Inputs.FieldRaster)
		if err != nil {
			return RasterPairValueCountsRequest{}, err
		}
		value, err := parseRasterPairBandSource("value_raster", request.Inputs.ValueRaster)
		if err != nil {
			return RasterPairValueCountsRequest{}, err
		}
		parsed.Mode = "separate"
		parsed.Field = field
		parsed.Value = value
	case hasStacked:
		stacked := request.Inputs.StackedRaster
		if strings.TrimSpace(stacked.Path) == "" {
			return RasterPairValueCountsRequest{}, fmt.Errorf("input %q path is required", "stacked_raster")
		}
		if stacked.FieldBand == nil {
			return RasterPairValueCountsRequest{}, fmt.Errorf("input %q requires field_band", "stacked_raster")
		}
		if stacked.ValueBand == nil {
			return RasterPairValueCountsRequest{}, fmt.Errorf("input %q requires value_band", "stacked_raster")
		}
		if *stacked.FieldBand <= 0 {
			return RasterPairValueCountsRequest{}, fmt.Errorf("input %q field_band must be greater than 0", "stacked_raster")
		}
		if *stacked.ValueBand <= 0 {
			return RasterPairValueCountsRequest{}, fmt.Errorf("input %q value_band must be greater than 0", "stacked_raster")
		}
		if *stacked.FieldBand == *stacked.ValueBand {
			return RasterPairValueCountsRequest{}, fmt.Errorf("input %q field_band and value_band must be different", "stacked_raster")
		}
		if err := validateRasterPairNodata("stacked_raster.field_nodata", stacked.FieldNodata); err != nil {
			return RasterPairValueCountsRequest{}, err
		}
		if err := validateRasterPairNodata("stacked_raster.value_nodata", stacked.ValueNodata); err != nil {
			return RasterPairValueCountsRequest{}, err
		}
		parsed.Mode = "stacked"
		parsed.Field = rasterPairBandSource{
			Path:   stacked.Path,
			Band:   *stacked.FieldBand,
			Nodata: stacked.FieldNodata,
			Name:   "stacked_raster.field_band",
		}
		parsed.Value = rasterPairBandSource{
			Path:   stacked.Path,
			Band:   *stacked.ValueBand,
			Nodata: stacked.ValueNodata,
			Name:   "stacked_raster.value_band",
		}
	default:
		return RasterPairValueCountsRequest{}, fmt.Errorf("%s requires either field_raster/value_raster or stacked_raster inputs", OperationRasterPairValueCounts)
	}

	return parsed, nil
}

func decodeRasterPairValueCountsOptions(options map[string]any) (rasterPairValueCountsOptions, error) {
	if options == nil {
		options = map[string]any{}
	}
	data, err := json.Marshal(options)
	if err != nil {
		return rasterPairValueCountsOptions{}, fmt.Errorf("encode pair count options: %w", err)
	}
	var parsed rasterPairValueCountsOptions
	if err := json.Unmarshal(data, &parsed); err != nil {
		return rasterPairValueCountsOptions{}, fmt.Errorf("decode pair count options: %w", err)
	}
	return parsed, nil
}

func parseRasterPairBandSource(name string, spec *rasterPairInputSpec) (rasterPairBandSource, error) {
	if strings.TrimSpace(spec.Path) == "" {
		return rasterPairBandSource{}, fmt.Errorf("input %q path is required", name)
	}
	if spec.Band == nil {
		return rasterPairBandSource{}, fmt.Errorf("input %q requires band", name)
	}
	if *spec.Band <= 0 {
		return rasterPairBandSource{}, fmt.Errorf("input %q band must be greater than 0", name)
	}
	if err := validateRasterPairNodata(name+".nodata", spec.Nodata); err != nil {
		return rasterPairBandSource{}, err
	}
	return rasterPairBandSource{
		Path:   spec.Path,
		Band:   *spec.Band,
		Nodata: spec.Nodata,
		Name:   name,
	}, nil
}

func validateRasterPairNodata(name string, value *int) error {
	if value == nil {
		return nil
	}
	if *value < 0 || *value > uint16MaxValue {
		return fmt.Errorf("%s %d is out of range for dtype uint16", name, *value)
	}
	return nil
}

func normalizeRasterPairFieldDType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "uint16", nil
	}
	switch normalized {
	case "uint16", "uint32", "int32":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported field_dtype %q; supported: %q, %q, %q", normalized, "uint16", "uint32", "int32")
	}
}

func normalizeRasterPairValueDType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "uint16", nil
	}
	switch normalized {
	case "byte", "uint16":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported value_dtype %q; supported: %q, %q", normalized, "byte", "uint16")
	}
}

func newPairCountAccumulator(includeValueNodata bool) *pairCountAccumulator {
	return &pairCountAccumulator{
		counts:             map[uint64]uint64{},
		distinctFields:     map[uint32]struct{}{},
		distinctValues:     map[uint16]struct{}{},
		includeValueNodata: includeValueNodata,
	}
}

func (acc *pairCountAccumulator) AddChunk(fieldValues []uint32, valueValues []uint16, fieldNodata uint32, valueNodata uint16) error {
	if len(fieldValues) != len(valueValues) {
		return fmt.Errorf("field/value chunk lengths differ: %d != %d", len(fieldValues), len(valueValues))
	}

	for i := range fieldValues {
		fieldID := fieldValues[i]
		if fieldID == fieldNodata {
			acc.skippedFieldNodata++
			continue
		}

		valueID := valueValues[i]
		if !acc.includeValueNodata && valueID == valueNodata {
			acc.skippedValueNodata++
			continue
		}

		key := uint64(fieldID)<<16 | uint64(valueID)
		if acc.counts[key] == math.MaxUint64 {
			return fmt.Errorf("count overflow for field_id=%d crop_id=%d", fieldID, valueID)
		}
		acc.counts[key]++
		acc.validPixels++
		acc.distinctFields[fieldID] = struct{}{}
		acc.distinctValues[valueID] = struct{}{}
	}
	return nil
}

func (acc *pairCountAccumulator) Rows() []PairCountRow {
	rows := make([]PairCountRow, 0, len(acc.counts))
	for key, count := range acc.counts {
		rows = append(rows, PairCountRow{
			FieldID: uint32(key >> 16),
			ValueID: uint16(key & uint16MaxValue),
			Count:   count,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].FieldID == rows[j].FieldID {
			return rows[i].ValueID < rows[j].ValueID
		}
		return rows[i].FieldID < rows[j].FieldID
	})
	return rows
}

func (acc *pairCountAccumulator) Metadata() PairCountMetadata {
	return PairCountMetadata{
		ValidPixels:        acc.validPixels,
		SkippedFieldNodata: acc.skippedFieldNodata,
		SkippedValueNodata: acc.skippedValueNodata,
		DistinctFields:     uint64(len(acc.distinctFields)),
		DistinctValues:     uint64(len(acc.distinctValues)),
		DistinctPairs:      uint64(len(acc.counts)),
		CountDType:         countDTypeUint64,
	}
}

func writePairCountsCSV(path string, rows []PairCountRow) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create counts csv: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"field_id", "crop_id", "count"}); err != nil {
		return fmt.Errorf("write counts csv header: %w", err)
	}
	for _, row := range rows {
		record := []string{
			strconv.FormatUint(uint64(row.FieldID), 10),
			strconv.FormatUint(uint64(row.ValueID), 10),
			strconv.FormatUint(row.Count, 10),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write counts csv row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush counts csv: %w", err)
	}
	return nil
}

func pairCountSummaryMap(metadata PairCountMetadata) map[string]any {
	return map[string]any{
		"valid_pixels":         metadata.ValidPixels,
		"skipped_field_nodata": metadata.SkippedFieldNodata,
		"skipped_value_nodata": metadata.SkippedValueNodata,
		"distinct_fields":      metadata.DistinctFields,
		"distinct_values":      metadata.DistinctValues,
		"distinct_pairs":       metadata.DistinctPairs,
		"count_dtype":          metadata.CountDType,
	}
}

func renderPairCountsCSV(rows []PairCountRow) string {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"field_id", "crop_id", "count"})
	for _, row := range rows {
		_ = writer.Write([]string{
			strconv.FormatUint(uint64(row.FieldID), 10),
			strconv.FormatUint(uint64(row.ValueID), 10),
			strconv.FormatUint(row.Count, 10),
		})
	}
	writer.Flush()
	return buf.String()
}

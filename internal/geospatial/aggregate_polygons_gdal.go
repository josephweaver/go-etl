package geospatial

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"goetl/internal/model"
)

const (
	OperationAggregateByPolygons = "aggregate_by_polygons"

	aggregateValueRasterInputName = "value_raster"
	aggregatePolygonsInputName    = "polygons"
	aggregateCountsCSVOutName     = "counts_csv"
	aggregateMetadataJSONOutName  = "metadata_json"

	aggregateZoneNodata       = 0
	aggregateProportionDigits = 6
)

type aggregateByPolygonsEnvelope struct {
	APIVersion string                       `json:"api_version"`
	Kind       string                       `json:"kind"`
	Operation  string                       `json:"operation"`
	Inputs     aggregateByPolygonsInputs    `json:"inputs"`
	Outputs    aggregateByPolygonsOutputs   `json:"outputs"`
	Options    aggregateByPolygonsOptionsIn `json:"options"`
}

type aggregateByPolygonsInputs struct {
	ValueRaster *aggregateRasterInput  `json:"value_raster"`
	Polygons    *aggregatePolygonInput `json:"polygons"`
}

type aggregateRasterInput struct {
	Path   string `json:"path"`
	Band   *int   `json:"band"`
	Nodata *int   `json:"nodata"`
}

type aggregatePolygonInput struct {
	Path    string `json:"path"`
	Layer   string `json:"layer"`
	IDField string `json:"id_field"`
}

type aggregateByPolygonsOutputs struct {
	CountsCSV    string `json:"counts_csv"`
	MetadataJSON string `json:"metadata_json"`
}

type aggregateByPolygonsOptionsIn struct {
	Categorical        *bool `json:"categorical"`
	AllTouched         *bool `json:"all_touched"`
	IncludeValueNodata *bool `json:"include_value_nodata"`
	ChunkRows          *int  `json:"chunk_rows"`
}

type AggregateByPolygonsRequest struct {
	ValueRaster        aggregateRasterSource
	Polygons           aggregatePolygonSource
	CountsCSVPath      string
	MetadataJSONPath   string
	Categorical        bool
	AllTouched         bool
	IncludeValueNodata bool
	ChunkRows          int
}

type aggregateRasterSource struct {
	Path   string
	Band   int
	Nodata *int
}

type aggregatePolygonSource struct {
	Path    string
	Layer   string
	IDField string
}

type AggregatePolygonCountRow struct {
	PolygonID   string
	RasterValue uint16
	Count       uint64
	Proportion  float64
}

type aggregateByPolygonsExecutionResult struct {
	rows     []AggregatePolygonCountRow
	metadata AggregateByPolygonsMetadata
}

type AggregateByPolygonsMetadata struct {
	Operation           string                         `json:"operation"`
	Categorical         bool                           `json:"categorical"`
	AllTouched          bool                           `json:"all_touched"`
	InclusionPolicy     string                         `json:"inclusion_policy"`
	ProportionPrecision AggregateProportionPrecision   `json:"proportion_precision"`
	NodataPolicy        AggregateNodataPolicy          `json:"nodata_policy"`
	ValueRaster         AggregateValueRasterEvidence   `json:"value_raster"`
	Polygons            AggregatePolygonSourceEvidence `json:"polygons"`
	TemporaryZoneRaster AggregateTemporaryArtifact     `json:"temporary_zone_raster"`
	PairCounts          PairCountMetadata              `json:"pair_counts"`
	Rows                int                            `json:"rows"`
	GDALVersion         string                         `json:"gdal_version"`
}

type AggregateProportionPrecision struct {
	Digits int    `json:"digits"`
	Rule   string `json:"rule"`
}

type AggregateNodataPolicy struct {
	ZoneNodata         int    `json:"zone_nodata"`
	ValueNodata        *int   `json:"value_nodata,omitempty"`
	IncludeValueNodata bool   `json:"include_value_nodata"`
	SkippedZoneNodata  uint64 `json:"skipped_zone_nodata"`
	SkippedValueNodata uint64 `json:"skipped_value_nodata"`
	Rule               string `json:"rule"`
}

type AggregateValueRasterEvidence struct {
	Path     string         `json:"path"`
	Band     int            `json:"band"`
	Metadata RasterMetadata `json:"metadata"`
}

type AggregatePolygonSourceEvidence struct {
	Path         string `json:"path"`
	Layer        string `json:"layer"`
	IDField      string `json:"id_field"`
	FeatureCount int    `json:"feature_count"`
	EPSG         int    `json:"epsg"`
}

type AggregateTemporaryArtifact struct {
	Policy    string `json:"policy"`
	CleanedUp bool   `json:"cleaned_up"`
}

var executeAggregateByPolygonsImpl = func(_ context.Context, _ AggregateByPolygonsRequest, _ string) (aggregateByPolygonsExecutionResult, error) {
	return aggregateByPolygonsExecutionResult{}, fmt.Errorf("%s requires a GDAL-enabled build (go build -tags gdal)", OperationAggregateByPolygons)
}

func ExecuteAggregateByPolygons(ctx context.Context, requestData []byte, artifactRoot string) (OperationResult, error) {
	parsed, err := ParseAggregateByPolygonsRequest(requestData)
	if err != nil {
		return OperationResult{}, err
	}

	executed, err := executeAggregateByPolygonsImpl(ctx, parsed, artifactRoot)
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
	if err := writeAggregatePolygonCountsCSV(countsPath, executed.rows); err != nil {
		return OperationResult{}, err
	}
	if err := writeJSONFile(metadataPath, executed.metadata); err != nil {
		return OperationResult{}, err
	}

	result := NewValidationResult(OperationAggregateByPolygons)
	result.Artifacts = []ArtifactResult{
		{Name: aggregateCountsCSVOutName, Path: parsed.CountsCSVPath, Kind: "table", Format: "csv"},
		{Name: aggregateMetadataJSONOutName, Path: parsed.MetadataJSONPath, Kind: "metadata", Format: "json"},
	}
	result.Summary = map[string]any{
		"counts_csv":           parsed.CountsCSVPath,
		"metadata_json":        parsed.MetadataJSONPath,
		"rows":                 len(executed.rows),
		"valid_pixels":         executed.metadata.PairCounts.ValidPixels,
		"skipped_zone_nodata":  executed.metadata.NodataPolicy.SkippedZoneNodata,
		"skipped_value_nodata": executed.metadata.NodataPolicy.SkippedValueNodata,
		"all_touched":          parsed.AllTouched,
	}
	return result, nil
}

func ParseAggregateByPolygonsRequest(requestData []byte) (AggregateByPolygonsRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(requestData))
	decoder.DisallowUnknownFields()
	var request aggregateByPolygonsEnvelope
	if err := decoder.Decode(&request); err != nil {
		return AggregateByPolygonsRequest{}, fmt.Errorf("decode request: %w", err)
	}

	if request.APIVersion != APIVersionV1Alpha1 {
		return AggregateByPolygonsRequest{}, fmt.Errorf("unsupported api_version %q", request.APIVersion)
	}
	if request.Kind != RequestKind {
		return AggregateByPolygonsRequest{}, fmt.Errorf("unsupported kind %q", request.Kind)
	}
	if request.Operation != OperationAggregateByPolygons {
		return AggregateByPolygonsRequest{}, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	if request.Inputs.ValueRaster == nil {
		return AggregateByPolygonsRequest{}, fmt.Errorf("%s requires input %q", OperationAggregateByPolygons, aggregateValueRasterInputName)
	}
	if request.Inputs.Polygons == nil {
		return AggregateByPolygonsRequest{}, fmt.Errorf("%s requires input %q", OperationAggregateByPolygons, aggregatePolygonsInputName)
	}

	valueRaster, err := parseAggregateRasterInput(request.Inputs.ValueRaster)
	if err != nil {
		return AggregateByPolygonsRequest{}, err
	}
	polygons, err := parseAggregatePolygonInput(request.Inputs.Polygons)
	if err != nil {
		return AggregateByPolygonsRequest{}, err
	}

	countsCSV, err := model.ValidateArtifactRelativePath(request.Outputs.CountsCSV)
	if err != nil {
		return AggregateByPolygonsRequest{}, fmt.Errorf("output %q path: %w", aggregateCountsCSVOutName, err)
	}
	metadataJSON, err := model.ValidateArtifactRelativePath(request.Outputs.MetadataJSON)
	if err != nil {
		return AggregateByPolygonsRequest{}, fmt.Errorf("output %q path: %w", aggregateMetadataJSONOutName, err)
	}

	categorical := true
	if request.Options.Categorical != nil {
		categorical = *request.Options.Categorical
	}
	if !categorical {
		return AggregateByPolygonsRequest{}, fmt.Errorf("categorical=false is not supported by %s", OperationAggregateByPolygons)
	}
	if request.Options.AllTouched == nil {
		return AggregateByPolygonsRequest{}, fmt.Errorf("options.all_touched is required so polygon inclusion policy is explicit")
	}
	if request.Options.IncludeValueNodata == nil {
		return AggregateByPolygonsRequest{}, fmt.Errorf("options.include_value_nodata is required so nodata behavior is explicit")
	}

	chunkRows := defaultChunkRows
	if request.Options.ChunkRows != nil {
		chunkRows = *request.Options.ChunkRows
	}
	if chunkRows <= 0 {
		return AggregateByPolygonsRequest{}, fmt.Errorf("chunk_rows must be greater than 0")
	}

	return AggregateByPolygonsRequest{
		ValueRaster:        valueRaster,
		Polygons:           polygons,
		CountsCSVPath:      countsCSV,
		MetadataJSONPath:   metadataJSON,
		Categorical:        categorical,
		AllTouched:         *request.Options.AllTouched,
		IncludeValueNodata: *request.Options.IncludeValueNodata,
		ChunkRows:          chunkRows,
	}, nil
}

func parseAggregateRasterInput(input *aggregateRasterInput) (aggregateRasterSource, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return aggregateRasterSource{}, fmt.Errorf("input %q path is required", aggregateValueRasterInputName)
	}
	if input.Band == nil {
		return aggregateRasterSource{}, fmt.Errorf("input %q requires band", aggregateValueRasterInputName)
	}
	if *input.Band <= 0 {
		return aggregateRasterSource{}, fmt.Errorf("input %q band must be greater than 0", aggregateValueRasterInputName)
	}
	if err := validateRasterPairNodata("input value_raster.nodata", input.Nodata); err != nil {
		return aggregateRasterSource{}, err
	}
	return aggregateRasterSource{Path: path, Band: *input.Band, Nodata: input.Nodata}, nil
}

func parseAggregatePolygonInput(input *aggregatePolygonInput) (aggregatePolygonSource, error) {
	polygons := aggregatePolygonSource{
		Path:    strings.TrimSpace(input.Path),
		Layer:   strings.TrimSpace(input.Layer),
		IDField: strings.TrimSpace(input.IDField),
	}
	if polygons.Path == "" {
		return aggregatePolygonSource{}, fmt.Errorf("inputs.polygons.path is required")
	}
	if polygons.Layer == "" {
		return aggregatePolygonSource{}, fmt.Errorf("inputs.polygons.layer is required")
	}
	if polygons.IDField == "" {
		return aggregatePolygonSource{}, fmt.Errorf("inputs.polygons.id_field is required")
	}
	return polygons, nil
}

func aggregateRowsFromPairCounts(pairRows []PairCountRow, zonePolygonIDs map[uint32]string) ([]AggregatePolygonCountRow, error) {
	totals := map[uint32]uint64{}
	for _, row := range pairRows {
		if _, ok := zonePolygonIDs[row.FieldID]; !ok {
			return nil, fmt.Errorf("zone raster produced unknown polygon zone id %d", row.FieldID)
		}
		totals[row.FieldID] += row.Count
	}

	rows := make([]AggregatePolygonCountRow, 0, len(pairRows))
	for _, row := range pairRows {
		total := totals[row.FieldID]
		if total == 0 {
			return nil, fmt.Errorf("polygon zone id %d has zero total after counting", row.FieldID)
		}
		rows = append(rows, AggregatePolygonCountRow{
			PolygonID:   zonePolygonIDs[row.FieldID],
			RasterValue: row.ValueID,
			Count:       row.Count,
			Proportion:  float64(row.Count) / float64(total),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].PolygonID == rows[j].PolygonID {
			return rows[i].RasterValue < rows[j].RasterValue
		}
		return rows[i].PolygonID < rows[j].PolygonID
	})
	return rows, nil
}

func writeAggregatePolygonCountsCSV(path string, rows []AggregatePolygonCountRow) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create aggregate counts csv: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"polygon_id", "raster_value", "count", "proportion"}); err != nil {
		return fmt.Errorf("write aggregate counts csv header: %w", err)
	}
	for _, row := range rows {
		record := []string{
			row.PolygonID,
			strconv.FormatUint(uint64(row.RasterValue), 10),
			strconv.FormatUint(row.Count, 10),
			strconv.FormatFloat(row.Proportion, 'f', aggregateProportionDigits, 64),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write aggregate counts csv row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush aggregate counts csv: %w", err)
	}
	return nil
}

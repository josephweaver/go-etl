//go:build gdal
// +build gdal

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
)

const (
	aggregateZoneIDField    = "goet_zone_id"
	aggregatePolygonIDField = "goet_polygon_id"
	aggregateZoneLayerName  = "zones"
)

type aggregatePolygonFeature struct {
	ZoneID    uint16
	PolygonID string
	FID       string
	Geometry  any
}

type aggregateZoneFeatureCollection struct {
	Type     string                     `json:"type"`
	Name     string                     `json:"name"`
	CRS      aggregateGeoJSONCRS        `json:"crs"`
	Features []aggregateZoneJSONFeature `json:"features"`
}

type aggregateGeoJSONCRS struct {
	Type       string                      `json:"type"`
	Properties aggregateGeoJSONCRSProperty `json:"properties"`
}

type aggregateGeoJSONCRSProperty struct {
	Name string `json:"name"`
}

type aggregateZoneJSONFeature struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Geometry   any            `json:"geometry"`
}

func init() {
	executeAggregateByPolygonsImpl = executeAggregateByPolygonsGDAL
}

func executeAggregateByPolygonsGDAL(ctx context.Context, request AggregateByPolygonsRequest, artifactRoot string) (aggregateByPolygonsExecutionResult, error) {
	valueMetadata, valueBand, err := aggregateValueRasterMetadata(request.ValueRaster)
	if err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}
	if !valueMetadata.CRSWKTPresent {
		return aggregateByPolygonsExecutionResult{}, fmt.Errorf("value_raster CRS is missing; refusing to guess")
	}
	if valueMetadata.EPSG == 0 {
		return aggregateByPolygonsExecutionResult{}, fmt.Errorf("value_raster CRS must resolve to an EPSG code")
	}

	polygonSource := CropPolygonsSource{
		Path:    request.Polygons.Path,
		Layer:   request.Polygons.Layer,
		IDField: request.Polygons.IDField,
	}
	polygonEPSG, featureCount, err := inspectCropVectorSummary(ctx, polygonSource)
	if err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}
	if polygonEPSG == 0 {
		return aggregateByPolygonsExecutionResult{}, fmt.Errorf("polygon layer CRS must resolve to an EPSG code")
	}
	if polygonEPSG != valueMetadata.EPSG {
		return aggregateByPolygonsExecutionResult{}, fmt.Errorf("value_raster EPSG:%d does not match polygon layer EPSG:%d", valueMetadata.EPSG, polygonEPSG)
	}
	if featureCount > uint16MaxValue {
		return aggregateByPolygonsExecutionResult{}, fmt.Errorf("polygon feature count %d exceeds supported zone id limit %d", featureCount, uint16MaxValue)
	}

	features, err := readAggregatePolygonFeatures(ctx, polygonSource)
	if err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}
	if len(features) > int(uint16MaxValue) {
		return aggregateByPolygonsExecutionResult{}, fmt.Errorf("polygon feature count %d exceeds supported zone id limit %d", len(features), uint16MaxValue)
	}
	if len(features) == 0 {
		return aggregateByPolygonsExecutionResult{}, fmt.Errorf("polygon layer %q has no features", request.Polygons.Layer)
	}

	tempDir, cleanup, err := createAggregateTempDir(artifactRoot)
	if err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}
	defer cleanup()

	zoneVectorPath := filepath.Join(tempDir, aggregateZoneLayerName+".geojson")
	if err := writeAggregateZoneVector(zoneVectorPath, valueMetadata.EPSG, features); err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}
	zoneRasterPath := filepath.Join(tempDir, "zones.tif")
	if err := rasterizeAggregateZones(ctx, zoneVectorPath, zoneRasterPath, GridFromMetadata(valueMetadata), request.AllTouched); err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}

	valueNodata, err := resolveRasterPairNodata(rasterPairBandSource{
		Path:   request.ValueRaster.Path,
		Band:   request.ValueRaster.Band,
		Nodata: request.ValueRaster.Nodata,
		Name:   aggregateValueRasterInputName,
	}, valueBand)
	if err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}

	pairResult, err := executeRasterPairValueCountsImpl(ctx, RasterPairValueCountsRequest{
		Mode: "separate",
		Field: rasterPairBandSource{
			Path:   zoneRasterPath,
			Band:   1,
			Nodata: intPtrValue(aggregateZoneNodata),
			Name:   "temporary_zone_raster",
		},
		Value: rasterPairBandSource{
			Path:   request.ValueRaster.Path,
			Band:   request.ValueRaster.Band,
			Nodata: &valueNodata,
			Name:   aggregateValueRasterInputName,
		},
		ChunkRows:          request.ChunkRows,
		IncludeValueNodata: request.IncludeValueNodata,
		RequireAlignedGrid: true,
		FieldDType:         "uint16",
		ValueDType:         "uint16",
	})
	if err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}

	zonePolygonIDs := map[uint16]string{}
	for _, feature := range features {
		zonePolygonIDs[feature.ZoneID] = feature.PolygonID
	}
	rows, err := aggregateRowsFromPairCounts(pairResult.rows, zonePolygonIDs)
	if err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}

	version, err := gdalVersion(ctx)
	if err != nil {
		return aggregateByPolygonsExecutionResult{}, err
	}

	metadata := AggregateByPolygonsMetadata{
		Operation:       OperationAggregateByPolygons,
		Categorical:     request.Categorical,
		AllTouched:      request.AllTouched,
		InclusionPolicy: aggregateInclusionPolicy(request.AllTouched),
		ProportionPrecision: AggregateProportionPrecision{
			Digits: aggregateProportionDigits,
			Rule:   "CSV proportions are count divided by the per-polygon included-value total and rounded to six decimal places",
		},
		NodataPolicy: AggregateNodataPolicy{
			ZoneNodata:         aggregateZoneNodata,
			ValueNodata:        &valueNodata,
			IncludeValueNodata: request.IncludeValueNodata,
			SkippedZoneNodata:  pairResult.metadata.SkippedFieldNodata,
			SkippedValueNodata: pairResult.metadata.SkippedValueNodata,
			Rule:               "pixels outside rasterized polygons use zone nodata 0; value nodata is included only when include_value_nodata is true",
		},
		ValueRaster: AggregateValueRasterEvidence{
			Path:     request.ValueRaster.Path,
			Band:     request.ValueRaster.Band,
			Metadata: valueMetadata,
		},
		Polygons: AggregatePolygonSourceEvidence{
			Path:         request.Polygons.Path,
			Layer:        request.Polygons.Layer,
			IDField:      request.Polygons.IDField,
			FeatureCount: len(features),
			EPSG:         polygonEPSG,
		},
		TemporaryZoneRaster: AggregateTemporaryArtifact{
			Policy:    "created under the artifact root in a hidden temporary directory and removed before the operation returns",
			CleanedUp: true,
		},
		PairCounts:  pairResult.metadata,
		Rows:        len(rows),
		GDALVersion: version,
	}

	return aggregateByPolygonsExecutionResult{rows: rows, metadata: metadata}, nil
}

func aggregateValueRasterMetadata(source aggregateRasterSource) (RasterMetadata, RasterBandMetadata, error) {
	metadata, err := collectSingleRasterMetadata(aggregateValueRasterInputName, InputSpec{Path: source.Path, Band: &source.Band, Nodata: source.Nodata})
	if err != nil {
		return RasterMetadata{}, RasterBandMetadata{}, fmt.Errorf("collect value_raster metadata: %w", err)
	}
	if source.Band > len(metadata.Bands) {
		return RasterMetadata{}, RasterBandMetadata{}, fmt.Errorf("input %q band %d does not exist", aggregateValueRasterInputName, source.Band)
	}
	band := metadata.Bands[source.Band-1]
	if band.DType != gdalUInt16DType {
		return RasterMetadata{}, RasterBandMetadata{}, fmt.Errorf("input %q dtype %q is not compatible with requested dtype %q", aggregateValueRasterInputName, band.DType, "uint16")
	}
	return metadata, band, nil
}

func readAggregatePolygonFeatures(ctx context.Context, source CropPolygonsSource) ([]aggregatePolygonFeature, error) {
	doc, err := runOGRInfo(ctx, source, "-features")
	if err != nil {
		return nil, err
	}
	layer, err := singleOGRLayer(doc, source.Layer)
	if err != nil {
		return nil, err
	}

	features := make([]aggregatePolygonFeature, 0, len(layer.Features))
	for i, rawFeature := range layer.Features {
		id, err := cropFeatureID(rawFeature.Properties, source.IDField)
		if err != nil {
			return nil, fmt.Errorf("feature %d: %w", i, err)
		}
		if rawFeature.Geometry == nil {
			return nil, fmt.Errorf("feature %q geometry is missing", id)
		}
		features = append(features, aggregatePolygonFeature{
			ZoneID:    uint16(i + 1),
			PolygonID: id,
			FID:       cropFIDString(rawFeature.FID),
			Geometry:  rawFeature.Geometry,
		})
	}
	return features, nil
}

func createAggregateTempDir(root string) (string, func(), error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(rootAbs, 0o755); err != nil {
		return "", nil, fmt.Errorf("create artifact root: %w", err)
	}
	tempDir, err := os.MkdirTemp(rootAbs, ".aggregate-by-polygons-*")
	if err != nil {
		return "", nil, fmt.Errorf("create aggregate temp directory: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, tempDir)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		_ = os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("temporary zone raster path escapes artifact root")
	}
	return tempDir, func() { _ = os.RemoveAll(tempDir) }, nil
}

func writeAggregateZoneVector(path string, epsg int, features []aggregatePolygonFeature) error {
	collection := aggregateZoneFeatureCollection{
		Type: "FeatureCollection",
		Name: aggregateZoneLayerName,
		CRS: aggregateGeoJSONCRS{
			Type: "name",
			Properties: aggregateGeoJSONCRSProperty{
				Name: fmt.Sprintf("EPSG:%d", epsg),
			},
		},
		Features: make([]aggregateZoneJSONFeature, 0, len(features)),
	}
	for _, feature := range features {
		collection.Features = append(collection.Features, aggregateZoneJSONFeature{
			Type: "Feature",
			Properties: map[string]any{
				aggregateZoneIDField:    feature.ZoneID,
				aggregatePolygonIDField: feature.PolygonID,
			},
			Geometry: feature.Geometry,
		})
	}

	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode temporary zone vector: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write temporary zone vector: %w", err)
	}
	return nil
}

func rasterizeAggregateZones(ctx context.Context, vectorPath string, outputPath string, grid RasterGrid, allTouched bool) error {
	bounds, err := BoundsFromGrid(grid)
	if err != nil {
		return err
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	args := []string{
		"-q",
		"-of", "GTiff",
		"-ot", "UInt16",
		"-init", strconv.Itoa(aggregateZoneNodata),
		"-a_nodata", strconv.Itoa(aggregateZoneNodata),
		"-a", aggregateZoneIDField,
		"-l", aggregateZoneLayerName,
		"-te",
		formatGDALFloat(bounds.MinX),
		formatGDALFloat(bounds.MinY),
		formatGDALFloat(bounds.MaxX),
		formatGDALFloat(bounds.MaxY),
		"-ts", strconv.Itoa(grid.Width), strconv.Itoa(grid.Height),
	}
	if grid.CRS != "" {
		args = append(args, "-a_srs", grid.CRS)
	}
	if allTouched {
		args = append(args, "-at")
	}
	args = append(args, vectorPath, outputPath)

	cmd := exec.CommandContext(cmdCtx, "gdal_rasterize", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gdal_rasterize: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func aggregateInclusionPolicy(allTouched bool) string {
	if allTouched {
		return "all_touched=true: rasterize every pixel touched by a polygon"
	}
	return "all_touched=false: rasterize pixels whose centers fall inside a polygon"
}

func intPtrValue(value int) *int {
	return &value
}

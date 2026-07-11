package geospatial

import (
	"math"
	"strings"
	"testing"
)

func TestParseRasterPairValueCountsRequestSeparateMode(t *testing.T) {
	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "field_raster": {"path": "/worker/data/fields.tif", "band": 1, "nodata": 0},
    "value_raster": {"path": "/worker/data/crops.tif", "band": 1}
  },
  "outputs": {
    "counts_csv": "counts/field_crop.csv",
    "metadata_json": "counts/field_crop.metadata.json"
  },
  "options": {
    "chunk_rows": 64,
    "field_dtype": "uint16",
    "value_dtype": "uint16"
  }
}`

	parsed, err := ParseRasterPairValueCountsRequest([]byte(requestJSON))
	if err != nil {
		t.Fatalf("ParseRasterPairValueCountsRequest() error = %v", err)
	}
	if parsed.Mode != "separate" {
		t.Fatalf("mode = %q, want separate", parsed.Mode)
	}
	if parsed.Field.Path != "/worker/data/fields.tif" || parsed.Value.Path != "/worker/data/crops.tif" {
		t.Fatalf("unexpected parsed paths: %#v", parsed)
	}
	if parsed.ChunkRows != 64 {
		t.Fatalf("chunk_rows = %d, want 64", parsed.ChunkRows)
	}
	if parsed.Value.Nodata != nil {
		t.Fatalf("value nodata = %v, want nil so GDAL metadata fallback can apply", parsed.Value.Nodata)
	}
}

func TestParseRasterPairValueCountsRequestStackedMode(t *testing.T) {
	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "stacked_raster": {
      "path": "/worker/data/stacked.tif",
      "field_band": 1,
      "value_band": 2,
      "field_nodata": 0,
      "value_nodata": 0
    }
  },
  "outputs": {
    "counts_csv": "counts.csv",
    "metadata_json": "counts.metadata.json"
  },
  "options": {}
}`

	parsed, err := ParseRasterPairValueCountsRequest([]byte(requestJSON))
	if err != nil {
		t.Fatalf("ParseRasterPairValueCountsRequest() error = %v", err)
	}
	if parsed.Mode != "stacked" {
		t.Fatalf("mode = %q, want stacked", parsed.Mode)
	}
	if parsed.Field.Path != parsed.Value.Path {
		t.Fatalf("stacked mode paths differ: field=%q value=%q", parsed.Field.Path, parsed.Value.Path)
	}
	if parsed.Field.Band != 1 || parsed.Value.Band != 2 {
		t.Fatalf("bands = %d,%d, want 1,2", parsed.Field.Band, parsed.Value.Band)
	}
}

func TestParseRasterPairValueCountsRequestRejectsMixedInputModes(t *testing.T) {
	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "field_raster": {"path": "/worker/data/fields.tif", "band": 1, "nodata": 0},
    "value_raster": {"path": "/worker/data/crops.tif", "band": 1, "nodata": 0},
    "stacked_raster": {"path": "/worker/data/stacked.tif", "field_band": 1, "value_band": 2}
  },
  "outputs": {
    "counts_csv": "counts.csv",
    "metadata_json": "counts.metadata.json"
  },
  "options": {}
}`

	_, err := ParseRasterPairValueCountsRequest([]byte(requestJSON))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "either field_raster/value_raster or stacked_raster") {
		t.Fatalf("error = %v, want mixed mode context", err)
	}
}

func TestParseRasterPairValueCountsRequestRejectsUnsupportedDType(t *testing.T) {
	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "field_raster": {"path": "/worker/data/fields.tif", "band": 1, "nodata": 0},
    "value_raster": {"path": "/worker/data/crops.tif", "band": 1, "nodata": 0}
  },
  "outputs": {
    "counts_csv": "counts.csv",
    "metadata_json": "counts.metadata.json"
  },
  "options": {
    "field_dtype": "float32"
  }
}`

	_, err := ParseRasterPairValueCountsRequest([]byte(requestJSON))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "unsupported field_dtype") {
		t.Fatalf("error = %v, want dtype context", err)
	}
}

func TestPairCountAccumulatorProducesSortedRowsAndMetadata(t *testing.T) {
	fieldValues := []uint32{
		2, 2, 0,
		1, 1, 1,
	}
	valueValues := []uint16{
		10, 10, 12,
		11, 0, 11,
	}

	accumulator := newPairCountAccumulator(false)
	if err := accumulator.AddChunk(fieldValues, valueValues, 0, 0); err != nil {
		t.Fatalf("AddChunk() error = %v", err)
	}

	rows := accumulator.Rows()
	gotCSV := renderPairCountsCSV(rows)
	wantCSV := "field_id,crop_id,count\n1,11,2\n2,10,2\n"
	if gotCSV != wantCSV {
		t.Fatalf("CSV = %q, want %q", gotCSV, wantCSV)
	}

	metadata := accumulator.Metadata()
	if metadata.ValidPixels != 4 {
		t.Fatalf("valid_pixels = %d, want 4", metadata.ValidPixels)
	}
	if metadata.SkippedFieldNodata != 1 {
		t.Fatalf("skipped_field_nodata = %d, want 1", metadata.SkippedFieldNodata)
	}
	if metadata.SkippedValueNodata != 1 {
		t.Fatalf("skipped_value_nodata = %d, want 1", metadata.SkippedValueNodata)
	}
	if metadata.DistinctFields != 2 || metadata.DistinctValues != 2 || metadata.DistinctPairs != 2 {
		t.Fatalf("metadata distinct counts = %#v", metadata)
	}
	if metadata.CountDType != countDTypeUint64 {
		t.Fatalf("count_dtype = %q, want %q", metadata.CountDType, countDTypeUint64)
	}
}

func TestPairCountAccumulatorIncludesValueNodataWhenRequested(t *testing.T) {
	accumulator := newPairCountAccumulator(true)
	if err := accumulator.AddChunk([]uint32{7, 7}, []uint16{0, 9}, 0, 0); err != nil {
		t.Fatalf("AddChunk() error = %v", err)
	}

	rows := accumulator.Rows()
	if len(rows) != 2 {
		t.Fatalf("row count = %d, want 2", len(rows))
	}
	if rows[0].FieldID != 7 || rows[0].ValueID != 0 || rows[0].Count != 1 {
		t.Fatalf("first row = %#v, want field 7 value 0 count 1", rows[0])
	}
	if accumulator.Metadata().SkippedValueNodata != 0 {
		t.Fatalf("skipped_value_nodata = %d, want 0", accumulator.Metadata().SkippedValueNodata)
	}
}

func TestPairCountAccumulatorPreservesUInt32FieldIDs(t *testing.T) {
	accumulator := newPairCountAccumulator(false)
	if err := accumulator.AddChunk([]uint32{70000}, []uint16{9}, 0, 0); err != nil {
		t.Fatalf("AddChunk() error = %v", err)
	}

	gotCSV := renderPairCountsCSV(accumulator.Rows())
	wantCSV := "field_id,crop_id,count\n70000,9,1\n"
	if gotCSV != wantCSV {
		t.Fatalf("CSV = %q, want %q", gotCSV, wantCSV)
	}
}

func TestPairCountAccumulatorRejectsOverflow(t *testing.T) {
	accumulator := newPairCountAccumulator(false)
	key := uint64(4)<<16 | uint64(9)
	accumulator.counts[key] = math.MaxUint64

	err := accumulator.AddChunk([]uint32{4}, []uint16{9}, 0, 0)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "count overflow") {
		t.Fatalf("error = %v, want overflow context", err)
	}
}

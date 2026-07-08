//go:build gdal
// +build gdal

package geospatial

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteRasterPairValueCountsSeparateModeWritesExpectedCSVAndMetadata(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	fieldPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "field_id",
		width:  3,
		height: 2,
		ullr:   [4]float64{0, 60, 90, 0},
		crs:    "EPSG:5070",
		rows:   "1 1 0\n2 2 2",
	})
	cropPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "crop_id",
		width:  3,
		height: 2,
		ullr:   [4]float64{0, 60, 90, 0},
		crs:    "EPSG:5070",
		rows:   "5 0 7\n5 5 9",
	})

	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "field_raster": {"path": "` + filepath.ToSlash(fieldPath) + `", "band": 1, "nodata": 0},
    "value_raster": {"path": "` + filepath.ToSlash(cropPath) + `", "band": 1, "nodata": 0}
  },
  "outputs": {
    "counts_csv": "counts/field_crop.csv",
    "metadata_json": "counts/field_crop.metadata.json"
  },
  "options": {
    "chunk_rows": 1
  }
}`

	result, err := ExecuteRasterPairValueCounts(context.Background(), []byte(requestJSON), dir)
	if err != nil {
		t.Fatalf("ExecuteRasterPairValueCounts() error = %v", err)
	}
	if got, want := result.Operation, OperationRasterPairValueCounts; got != want {
		t.Fatalf("operation = %q, want %q", got, want)
	}

	countsData, err := os.ReadFile(filepath.Join(dir, "counts", "field_crop.csv"))
	if err != nil {
		t.Fatalf("read counts csv: %v", err)
	}
	wantCSV := "field_id,crop_id,count\n1,5,1\n2,5,2\n2,9,1\n"
	if string(countsData) != wantCSV {
		t.Fatalf("counts csv = %q, want %q", string(countsData), wantCSV)
	}

	metadataData, err := os.ReadFile(filepath.Join(dir, "counts", "field_crop.metadata.json"))
	if err != nil {
		t.Fatalf("read metadata json: %v", err)
	}
	metadataText := string(metadataData)
	for _, want := range []string{
		`"valid_pixels": 4`,
		`"skipped_field_nodata": 1`,
		`"skipped_value_nodata": 1`,
		`"distinct_fields": 2`,
		`"distinct_values": 2`,
		`"distinct_pairs": 3`,
		`"count_dtype": "uint64"`,
	} {
		if !strings.Contains(metadataText, want) {
			t.Fatalf("metadata json missing %s: %s", want, metadataText)
		}
	}
}

func TestExecuteRasterPairValueCountsStackedModeSupportsBandSpecificNodata(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	fieldPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "field_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "3 3\n4 4",
	})
	cropPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "crop_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "10 99\n10 11",
	})

	stackRequest := stackRequest(fieldPath, cropPath, map[string]any{"dtype": "uint16", "nodata": 99})
	stackedDir := filepath.Join(dir, "stack")
	if _, err := ExecuteStackAlignedRasters(context.Background(), stackRequest, stackedDir); err != nil {
		t.Fatalf("ExecuteStackAlignedRasters() error = %v", err)
	}
	stackedPath := filepath.Join(stackedDir, "stack", "stacked.tif")

	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "stacked_raster": {
      "path": "` + filepath.ToSlash(stackedPath) + `",
      "field_band": 1,
      "value_band": 2,
      "field_nodata": 0,
      "value_nodata": 99
    }
  },
  "outputs": {
    "counts_csv": "counts.csv",
    "metadata_json": "counts.metadata.json"
  },
  "options": {}
}`

	if _, err := ExecuteRasterPairValueCounts(context.Background(), []byte(requestJSON), dir); err != nil {
		t.Fatalf("ExecuteRasterPairValueCounts() error = %v", err)
	}

	countsData, err := os.ReadFile(filepath.Join(dir, "counts.csv"))
	if err != nil {
		t.Fatalf("read counts csv: %v", err)
	}
	wantCSV := "field_id,crop_id,count\n3,10,1\n4,10,1\n4,11,1\n"
	if string(countsData) != wantCSV {
		t.Fatalf("counts csv = %q, want %q", string(countsData), wantCSV)
	}
}

func TestExecuteRasterPairValueCountsRejectsMisalignedSeparateInputsBeforeWrite(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	fieldPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "field_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "1 1\n2 2",
	})
	cropPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "crop_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{30, 60, 90, 0},
		crs:    "EPSG:5070",
		rows:   "5 5\n6 6",
	})

	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "field_raster": {"path": "` + filepath.ToSlash(fieldPath) + `", "band": 1, "nodata": 0},
    "value_raster": {"path": "` + filepath.ToSlash(cropPath) + `", "band": 1, "nodata": 0}
  },
  "outputs": {
    "counts_csv": "counts.csv",
    "metadata_json": "counts.metadata.json"
  },
  "options": {}
}`

	_, err := ExecuteRasterPairValueCounts(context.Background(), []byte(requestJSON), dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not aligned") {
		t.Fatalf("error = %v, want alignment context", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "counts.csv")); !os.IsNotExist(statErr) {
		t.Fatalf("counts csv exists after failure, stat error = %v", statErr)
	}
}

func TestExecuteRasterPairValueCountsUsesBandNodataWhenRequestOmitsIt(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	fieldPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "field_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "1 0\n2 2",
	})
	cropPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "crop_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "8 8\n0 9",
	})

	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "field_raster": {"path": "` + filepath.ToSlash(fieldPath) + `", "band": 1},
    "value_raster": {"path": "` + filepath.ToSlash(cropPath) + `", "band": 1}
  },
  "outputs": {
    "counts_csv": "counts.csv",
    "metadata_json": "counts.metadata.json"
  },
  "options": {}
}`

	if _, err := ExecuteRasterPairValueCounts(context.Background(), []byte(requestJSON), dir); err != nil {
		t.Fatalf("ExecuteRasterPairValueCounts() error = %v", err)
	}

	countsData, err := os.ReadFile(filepath.Join(dir, "counts.csv"))
	if err != nil {
		t.Fatalf("read counts csv: %v", err)
	}
	wantCSV := "field_id,crop_id,count\n1,8,1\n2,9,1\n"
	if string(countsData) != wantCSV {
		t.Fatalf("counts csv = %q, want %q", string(countsData), wantCSV)
	}
}

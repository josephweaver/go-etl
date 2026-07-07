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
	"strings"
	"testing"
	"time"
)

func TestExecuteStackAlignedRastersWritesTwoBandRasterAndMetadata(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	fieldPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "field_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "1 2\n3 4",
	})
	cropPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "crop_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "11 12\n13 14",
	})

	result, err := ExecuteStackAlignedRasters(context.Background(), stackRequest(fieldPath, cropPath, nil), dir)
	if err != nil {
		t.Fatalf("ExecuteStackAlignedRasters() error = %v", err)
	}
	if result.Operation != OperationStackAligned {
		t.Fatalf("operation = %q, want %q", result.Operation, OperationStackAligned)
	}
	if got, want := result.Summary["band_count"], 2; got != want {
		t.Fatalf("summary band_count = %v, want %d", got, want)
	}

	outputPath := filepath.Join(dir, "stack", "stacked.tif")
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("missing stacked raster: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stack", "stacked.metadata.json")); err != nil {
		t.Fatalf("missing metadata artifact: %v", err)
	}

	outputMetadata, err := collectSingleRasterMetadata("output", InputSpec{Path: outputPath})
	if err != nil {
		t.Fatalf("collect output metadata: %v", err)
	}
	if outputMetadata.BandCount != 2 {
		t.Fatalf("output band_count = %d, want 2", outputMetadata.BandCount)
	}
	if outputMetadata.Width != 2 || outputMetadata.Height != 2 {
		t.Fatalf("output size = [%d,%d], want [2,2]", outputMetadata.Width, outputMetadata.Height)
	}

	var metadata StackAlignedMetadata
	metadataData, err := os.ReadFile(filepath.Join(dir, "stack", "stacked.metadata.json"))
	if err != nil {
		t.Fatalf("read stack metadata: %v", err)
	}
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatalf("decode stack metadata: %v", err)
	}
	if len(metadata.Sources) != 2 {
		t.Fatalf("metadata sources = %d, want 2", len(metadata.Sources))
	}
	if metadata.OutputNodata == nil || *metadata.OutputNodata != 0 {
		t.Fatalf("metadata output_nodata = %v, want 0", metadata.OutputNodata)
	}
	if metadata.Sources[0].OutputBand != 1 || metadata.Sources[1].OutputBand != 2 {
		t.Fatalf("metadata output bands = %v,%v, want 1,2", metadata.Sources[0].OutputBand, metadata.Sources[1].OutputBand)
	}
	if metadata.Sources[0].SourceName == metadata.Sources[1].SourceName {
		t.Fatalf("metadata source names are not unique")
	}
}

func TestExecuteStackAlignedRastersRejectsMisalignedInputsBeforeWrite(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	fieldPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "field_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "1 2\n3 4",
	})
	cropPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "crop_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{10, 60, 70, 10},
		crs:    "EPSG:5070",
		rows:   "11 12\n13 14",
	})

	_, err := ExecuteStackAlignedRasters(context.Background(), stackRequest(fieldPath, cropPath, nil), dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "grid is not aligned") {
		t.Fatalf("error = %v, want misalignment context", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "stack", "stacked.tif")); !os.IsNotExist(statErr) {
		t.Fatalf("stack output exists after failure, stat error = %v", statErr)
	}
}

func TestExecuteStackAlignedRastersRejectsUnsupportedInputDType(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	fieldPath := createAlignmentRasterWithType(t, dir, alignmentRasterSpec{
		name:   "field_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "1 2\n3 4",
	}, "UInt16")
	cropPath := createAlignmentRasterWithType(t, dir, alignmentRasterSpec{
		name:   "crop_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "11 12\n13 14",
	}, "UInt32")

	_, err := ExecuteStackAlignedRasters(context.Background(), stackRequest(fieldPath, cropPath, map[string]any{"dtype": "uint16"}), dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not compatible with output dtype") {
		t.Fatalf("error = %v, want dtype compatibility context", err)
	}
}

func TestExecuteStackAlignedRastersDefaultsOutputNodata(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	fieldPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "field_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "1 2\n3 4",
	})
	cropPath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "crop_id",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 60, 60, 0},
		crs:    "EPSG:5070",
		rows:   "11 12\n13 14",
	})

	request := stackRequest(fieldPath, cropPath, map[string]any{"dtype": "uint16"})
	delete(request.Options, "nodata")
	if _, err := ExecuteStackAlignedRasters(context.Background(), request, dir); err != nil {
		t.Fatalf("ExecuteStackAlignedRasters() error = %v", err)
	}

	var metadata StackAlignedMetadata
	metadataData, err := os.ReadFile(filepath.Join(dir, "stack", "stacked.metadata.json"))
	if err != nil {
		t.Fatalf("read stack metadata: %v", err)
	}
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatalf("decode stack metadata: %v", err)
	}
	if metadata.OutputNodata == nil || *metadata.OutputNodata != 0 {
		t.Fatalf("metadata output_nodata = %v, want 0", metadata.OutputNodata)
	}
}

func stackRequest(fieldPath string, cropPath string, extraOptions map[string]any) OperationRequest {
	options := map[string]any{
		"dtype":                "uint16",
		"nodata":               0,
		"require_aligned_grid": true,
	}
	for k, v := range extraOptions {
		options[k] = v
	}
	return OperationRequest{
		APIVersion: APIVersionV1Alpha1,
		Kind:       RequestKind,
		Operation:  OperationStackAligned,
		Inputs: map[string]InputSpec{
			"field_id": {Path: fieldPath, Band: intPtr(1), OutputBand: intPtr(1)},
			"crop_id":  {Path: cropPath, Band: intPtr(1), OutputBand: intPtr(2)},
		},
		Outputs: map[string]string{
			"stacked_raster": "stack/stacked.tif",
			"metadata_json":  "stack/stacked.metadata.json",
		},
		Options: options,
	}
}

func createAlignmentRasterWithType(t *testing.T, dir string, spec alignmentRasterSpec, dtype string) string {
	t.Helper()
	ascPath := filepath.Join(dir, fmt.Sprintf("%s.asc", spec.name))
	tifPath := filepath.Join(dir, fmt.Sprintf("%s.tif", spec.name))

	content := fmt.Sprintf("ncols %d\nnrows %d\nxllcorner 0 0\ncellsize 30\nNODATA_value 0\n%s\n", spec.width, spec.height, spec.rows)
	if err := os.WriteFile(ascPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write ascii raster: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{
		"-of", "GTiff",
		"-ot", dtype,
		"-a_nodata", "0",
		"-a_srs", spec.crs,
		"-a_ullr",
		formatGDALFloat(spec.ullr[0]),
		formatGDALFloat(spec.ullr[1]),
		formatGDALFloat(spec.ullr[2]),
		formatGDALFloat(spec.ullr[3]),
		ascPath,
		tifPath,
	}

	cmd := exec.CommandContext(ctx, "gdal_translate", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gdal_translate: %v: %s", err, output)
	}

	return tifPath
}

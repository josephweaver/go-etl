//go:build gdal
// +build gdal

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWritesRasterInfoMetadataArtifact(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	responsePath := filepath.Join(dir, "result.json")
	metadataPath := filepath.Join(dir, "metadata", "raster_info.json")
	inputPath := createTestRasterForCli(t, dir)

	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_info",
  "inputs": {
    "field": {"path": "` + inputPath + `"},
    "cdl": {"path": "` + inputPath + `"}
  },
  "outputs": {
    "metadata_json": "metadata/raster_info.json"
  },
  "options": {}
}
`
	requestPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(requestPath, []byte(requestJSON), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	if err := run([]string{"--request", requestPath, "--response", responsePath}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	resultData, err := os.ReadFile(responsePath)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var result struct {
		Operation string         `json:"operation"`
		Summary   map[string]any `json:"summary"`
		Artifacts []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Operation != "raster_info" {
		t.Fatalf("operation = %q, want %q", result.Operation, "raster_info")
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Name != "metadata_json" || result.Artifacts[0].Path != "metadata/raster_info.json" {
		t.Fatalf("artifacts = %#v", result.Artifacts)
	}
	if _, ok := result.Summary["rasters"]; !ok {
		t.Fatalf("missing summary rasters key")
	}

	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read metadata artifact: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	rastersList, ok := metadata["rasters"]
	if !ok {
		t.Fatalf("missing metadata.rasters")
	}
	rasters, ok := rastersList.([]any)
	if !ok {
		t.Fatalf("metadata.rasters type %T, want []any", rastersList)
	}
	if len(rasters) != 2 {
		t.Fatalf("metadata raster count = %d, want 2", len(rasters))
	}
	if !strings.Contains(string(metadataData), `"name": "cdl"`) || !strings.Contains(string(metadataData), `"name": "field"`) {
		t.Fatalf("metadata record names missing")
	}
}

func TestRunAlignToGridWritesRasterAndMetadataArtifacts(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	responsePath := filepath.Join(dir, "result.json")
	inputPath := createTestRasterForCli(t, dir)

	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "align_to_grid",
  "inputs": {
    "source_raster": {"path": "` + inputPath + `", "band": 1, "nodata": 0}
  },
  "outputs": {
    "raster_tif": "aligned/cdl.tif",
    "metadata_json": "aligned/cdl.metadata.json"
  },
  "options": {
    "target_crs": "EPSG:5070",
    "target_transform": [0, 30, 0, 60, 0, -30],
    "target_width": 2,
    "target_height": 2
  }
}
`
	requestPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(requestPath, []byte(requestJSON), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	if err := run([]string{"--request", requestPath, "--response", responsePath}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	resultData, err := os.ReadFile(responsePath)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var result struct {
		Operation string `json:"operation"`
		Artifacts []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Operation != "align_to_grid" {
		t.Fatalf("operation = %q, want align_to_grid", result.Operation)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifacts = %#v, want 2 artifacts", result.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(dir, "aligned", "cdl.tif")); err != nil {
		t.Fatalf("stat raster artifact: %v", err)
	}
	metadataData, err := os.ReadFile(filepath.Join(dir, "aligned", "cdl.metadata.json"))
	if err != nil {
		t.Fatalf("read metadata artifact: %v", err)
	}
	if !strings.Contains(string(metadataData), `"resampling": "nearest"`) || !strings.Contains(string(metadataData), `"gdal_version":`) {
		t.Fatalf("metadata artifact missing resampling or GDAL version: %s", metadataData)
	}
}

func TestRunStackAlignedRastersWritesStackAndMetadataArtifacts(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	responsePath := filepath.Join(dir, "result.json")
	fieldPath := createTestRasterForCli(t, dir)
	cropPath := createTestRasterForCli(t, dir)

	requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "stack_aligned_rasters",
  "inputs": {
    "field_id": {"path": "` + fieldPath + `", "band": 1, "output_band": 1},
    "crop_id": {"path": "` + cropPath + `", "band": 1, "output_band": 2}
  },
  "outputs": {
    "stacked_raster": "stack/stacked.tif",
    "metadata_json": "stack/stacked.metadata.json"
  },
  "options": {
    "dtype": "uint16",
    "nodata": 0,
    "require_aligned_grid": true
  }
}
`
	requestPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(requestPath, []byte(requestJSON), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	if err := run([]string{"--request", requestPath, "--response", responsePath}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	resultData, err := os.ReadFile(responsePath)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var result struct {
		Operation string `json:"operation"`
		Artifacts []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Operation != "stack_aligned_rasters" {
		t.Fatalf("operation = %q, want %q", result.Operation, "stack_aligned_rasters")
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifacts = %#v, want 2 artifacts", result.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(dir, "stack", "stacked.tif")); err != nil {
		t.Fatalf("stat raster artifact: %v", err)
	}
	metadataData, err := os.ReadFile(filepath.Join(dir, "stack", "stacked.metadata.json"))
	if err != nil {
		t.Fatalf("read metadata artifact: %v", err)
	}
	if !strings.Contains(string(metadataData), `"dtype": "UInt16"`) {
		t.Fatalf("metadata artifact missing dtype: %s", metadataData)
	}
}

func createTestRasterForCli(t *testing.T, dir string) string {
	return createTestRaster(t, dir, "cli", "1 2\n3 4")
}

func createTestRaster(t *testing.T, dir, name, rows string) string {
	ascPath := filepath.Join(dir, name+".asc")
	tifPath := filepath.Join(dir, name+".tif")

	content := "ncols 2\nnrows 2\nxllcorner 0 0\ncellsize 30\nNODATA_value 0\n" + rows + "\n"
	if err := os.WriteFile(ascPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write ascii raster: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gdal_translate",
		"-of", "GTiff",
		"-ot", "UInt16",
		"-a_nodata", "0",
		"-a_srs", "EPSG:5070",
		"-a_ullr", "0", "60", "60", "0",
		ascPath, tifPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gdal_translate: %v: %s", err, output)
	}

	return tifPath
}

func skipIfGDALMissing(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gdal_translate"); err != nil {
		t.Skip("gdal_translate not available in PATH")
	}
}

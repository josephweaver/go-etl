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

func TestExecuteRasterAlignmentExplicitGridWritesRasterAndMetadata(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	sourcePath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "source",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 120, 120, 0},
		crs:    "EPSG:5070",
		rows:   "0 1\n2 3",
	})
	request := alignmentRequest(sourcePath, map[string]any{
		"target_crs":       "EPSG:5070",
		"target_transform": []float64{0, 30, 0, 120, 0, -30},
		"target_width":     4,
		"target_height":    4,
	})

	result, err := ExecuteRasterAlignment(context.Background(), request, dir)
	if err != nil {
		t.Fatalf("ExecuteRasterAlignment() error = %v", err)
	}
	if result.Operation != OperationAlignToGrid {
		t.Fatalf("operation = %q, want %q", result.Operation, OperationAlignToGrid)
	}
	if len(result.Artifacts) != 2 {
		t.Fatalf("artifact count = %d, want 2", len(result.Artifacts))
	}

	outputPath := filepath.Join(dir, "aligned", "cdl.tif")
	output, err := collectSingleRasterMetadata("output", InputSpec{Path: outputPath})
	if err != nil {
		t.Fatalf("collect output metadata: %v", err)
	}
	if output.Width != 4 || output.Height != 4 {
		t.Fatalf("output size = [%d,%d], want [4,4]", output.Width, output.Height)
	}
	if got, want := output.EPSG, 5070; got != want {
		t.Fatalf("output EPSG = %d, want %d", got, want)
	}
	assertTransform(t, output.GeoTransform, []float64{0, 30, 0, 120, 0, -30})
	if got, want := output.Bands[0].DType, "UInt16"; got != want {
		t.Fatalf("output dtype = %q, want %q", got, want)
	}
	if output.Bands[0].Nodata == nil || *output.Bands[0].Nodata != 0 {
		t.Fatalf("output nodata = %v, want 0", output.Bands[0].Nodata)
	}

	metadata := readAlignmentMetadata(t, filepath.Join(dir, "aligned", "cdl.metadata.json"))
	if metadata.Resampling != "nearest" {
		t.Fatalf("metadata resampling = %q, want nearest", metadata.Resampling)
	}
	if metadata.GDALVersion == "" {
		t.Fatal("metadata GDALVersion is empty")
	}
	if metadata.NodataPolicy.SourceNodata == nil || *metadata.NodataPolicy.SourceNodata != 0 {
		t.Fatalf("source nodata = %v, want 0", metadata.NodataPolicy.SourceNodata)
	}
	if metadata.Source.Grid.Width != 2 || metadata.Target.Width != 4 || metadata.Output.Grid.Width != 4 {
		t.Fatalf("metadata grids = source %d target %d output %d", metadata.Source.Grid.Width, metadata.Target.Width, metadata.Output.Grid.Width)
	}
}

func TestExecuteRasterAlignmentLikeRasterMatchesTargetGrid(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	sourcePath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "source",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 120, 120, 0},
		crs:    "EPSG:5070",
		rows:   "1 2\n3 4",
	})
	likePath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "like",
		width:  3,
		height: 2,
		ullr:   [4]float64{30, 90, 120, 30},
		crs:    "EPSG:5070",
		rows:   "1 1 1\n1 1 1",
	})
	likeMetadata, err := collectSingleRasterMetadata("like", InputSpec{Path: likePath})
	if err != nil {
		t.Fatalf("collect like metadata: %v", err)
	}

	request := alignmentRequest(sourcePath, map[string]any{"like_raster": likePath})
	if _, err := ExecuteRasterAlignment(context.Background(), request, dir); err != nil {
		t.Fatalf("ExecuteRasterAlignment() error = %v", err)
	}

	output, err := collectSingleRasterMetadata("output", InputSpec{Path: filepath.Join(dir, "aligned", "cdl.tif")})
	if err != nil {
		t.Fatalf("collect output metadata: %v", err)
	}
	if !GridsEqual(GridFromMetadata(output), GridFromMetadata(likeMetadata)) {
		t.Fatalf("output grid = %#v, want like grid %#v", GridFromMetadata(output), GridFromMetadata(likeMetadata))
	}
}

func TestExecuteRasterAlignmentFailsWhenSourceCRSMissing(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	sourcePath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "source-no-crs",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 120, 120, 0},
		rows:   "1 2\n3 4",
	})
	request := alignmentRequest(sourcePath, map[string]any{
		"target_crs":       "EPSG:5070",
		"target_transform": []float64{0, 30, 0, 120, 0, -30},
		"target_width":     4,
		"target_height":    4,
	})

	err := request.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	_, err = ExecuteRasterAlignment(context.Background(), request, dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "source raster CRS is missing") {
		t.Fatalf("ExecuteRasterAlignment() error = %v, want source CRS context", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "aligned", "cdl.tif")); !os.IsNotExist(statErr) {
		t.Fatalf("output raster exists after missing CRS failure, stat error = %v", statErr)
	}
}

func TestExecuteRasterAlignmentRejectsIncompleteTargetBeforeWriting(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	sourcePath := createAlignmentRaster(t, dir, alignmentRasterSpec{
		name:   "source",
		width:  2,
		height: 2,
		ullr:   [4]float64{0, 120, 120, 0},
		crs:    "EPSG:5070",
		rows:   "1 2\n3 4",
	})
	request := alignmentRequest(sourcePath, map[string]any{
		"target_crs":   "EPSG:5070",
		"target_width": 4,
	})

	_, err := ExecuteRasterAlignment(context.Background(), request, dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "explicit target grid requires") {
		t.Fatalf("ExecuteRasterAlignment() error = %v, want incomplete grid context", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "aligned", "cdl.tif")); !os.IsNotExist(statErr) {
		t.Fatalf("output raster exists after target validation failure, stat error = %v", statErr)
	}
}

type alignmentRasterSpec struct {
	name   string
	width  int
	height int
	ullr   [4]float64
	crs    string
	rows   string
}

func createAlignmentRaster(t *testing.T, dir string, spec alignmentRasterSpec) string {
	t.Helper()
	ascPath := filepath.Join(dir, spec.name+".asc")
	tifPath := filepath.Join(dir, spec.name+".tif")

	content := fmt.Sprintf("ncols %d\nnrows %d\nxllcorner 0 0\ncellsize 30\nNODATA_value 0\n%s\n", spec.width, spec.height, spec.rows)
	if err := os.WriteFile(ascPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write ascii raster: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{
		"-of", "GTiff",
		"-ot", "UInt16",
		"-a_nodata", "0",
		"-a_ullr",
		formatGDALFloat(spec.ullr[0]),
		formatGDALFloat(spec.ullr[1]),
		formatGDALFloat(spec.ullr[2]),
		formatGDALFloat(spec.ullr[3]),
	}
	if spec.crs != "" {
		args = append(args, "-a_srs", spec.crs)
	}
	args = append(args, ascPath, tifPath)

	cmd := exec.CommandContext(ctx, "gdal_translate", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gdal_translate: %v: %s", err, output)
	}

	return tifPath
}

func alignmentRequest(sourcePath string, options map[string]any) OperationRequest {
	return OperationRequest{
		APIVersion: APIVersionV1Alpha1,
		Kind:       RequestKind,
		Operation:  OperationAlignToGrid,
		Inputs: map[string]InputSpec{
			"source_raster": {Path: sourcePath, Band: intPtr(1), Nodata: intPtr(0)},
		},
		Outputs: map[string]string{
			"raster_tif":    "aligned/cdl.tif",
			"metadata_json": "aligned/cdl.metadata.json",
		},
		Options: options,
	}
}

func readAlignmentMetadata(t *testing.T, path string) AlignmentMetadata {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var metadata AlignmentMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	return metadata
}

func assertTransform(t *testing.T, got []float64, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("transform length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("transform[%d] = %v, want %v; full transform %v", i, got[i], want[i], got)
		}
	}
}

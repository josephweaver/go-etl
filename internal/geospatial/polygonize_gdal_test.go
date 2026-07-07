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
	"testing"
	"time"
)

func TestParsePolygonizeRasterRequestAppliesDefaultsAndValidatesOptions(t *testing.T) {
	parsed, err := ParsePolygonizeRasterRequest([]byte(validPolygonizeRasterRequestJSON(
		"/worker/data/classes.tif",
		`"value_field": "class_id", "connectivity": 8, "max_features": 4`,
	)))
	if err != nil {
		t.Fatalf("ParsePolygonizeRasterRequest() error = %v", err)
	}
	if parsed.Raster.Path != "/worker/data/classes.tif" {
		t.Fatalf("raster path = %q", parsed.Raster.Path)
	}
	if parsed.Raster.Band != 1 {
		t.Fatalf("band = %d, want 1", parsed.Raster.Band)
	}
	if parsed.ValueField != "class_id" {
		t.Fatalf("value field = %q, want class_id", parsed.ValueField)
	}
	if parsed.Connectivity != 8 {
		t.Fatalf("connectivity = %d, want 8", parsed.Connectivity)
	}
	if parsed.MaxFeatures != 4 {
		t.Fatalf("max features = %d, want 4", parsed.MaxFeatures)
	}
	if parsed.LayerName != "polygonized_classes" {
		t.Fatalf("layer name = %q, want polygonized_classes", parsed.LayerName)
	}
}

func TestParsePolygonizeRasterRequestRejectsInvalidGuards(t *testing.T) {
	cases := []struct {
		name    string
		request string
		want    string
	}{
		{
			name: "missing raster",
			request: `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "polygonize_raster",
  "inputs": {},
  "outputs": {"vector": "out.gpkg", "metadata_json": "out.metadata.json"},
  "options": {}
}`,
			want: `requires input "raster"`,
		},
		{
			name:    "bad connectivity",
			request: validPolygonizeRasterRequestJSON("/worker/data/classes.tif", `"connectivity": 6`),
			want:    "connectivity must be 4 or 8",
		},
		{
			name:    "zero max features",
			request: validPolygonizeRasterRequestJSON("/worker/data/classes.tif", `"max_features": 0`),
			want:    "max_features must be greater than 0",
		},
		{
			name:    "unsafe vector path",
			request: strings.Replace(validPolygonizeRasterRequestJSON("/worker/data/classes.tif", `{}`), `"polygonized_classes.gpkg"`, `"../escape.gpkg"`, 1),
			want:    `output "vector" path`,
		},
		{
			name:    "invalid value field",
			request: validPolygonizeRasterRequestJSON("/worker/data/classes.tif", `"value_field": "bad-field"`),
			want:    "value_field",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePolygonizeRasterRequest([]byte(tc.request))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestExecutePolygonizeRasterWritesVectorAndMetadata(t *testing.T) {
	skipIfPolygonizeGDALMissing(t)
	dir := t.TempDir()
	rasterPath := createPolygonizeTestRaster(t, dir, "classes", 3, 3, "1 1 0\n1 2 2\n3 2 2")

	result, err := ExecutePolygonizeRaster(context.Background(), []byte(validPolygonizeRasterRequestJSON(
		rasterPath,
		`"value_field": "value", "connectivity": 4, "max_features": 10`,
	)), dir)
	if err != nil {
		t.Fatalf("ExecutePolygonizeRaster() error = %v", err)
	}
	if result.Operation != OperationPolygonizeRaster {
		t.Fatalf("operation = %q, want %q", result.Operation, OperationPolygonizeRaster)
	}

	vectorPath := filepath.Join(dir, "polygonized_classes.gpkg")
	if _, err := os.Stat(vectorPath); err != nil {
		t.Fatalf("stat vector output: %v", err)
	}
	values := polygonizedValues(t, vectorPath, "polygonized_classes", "value")
	wantValues := []int{1, 2, 3}
	if fmt.Sprint(values) != fmt.Sprint(wantValues) {
		t.Fatalf("polygonized values = %v, want %v", values, wantValues)
	}

	metadataData, err := os.ReadFile(filepath.Join(dir, "polygonized_classes.metadata.json"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var metadata PolygonizeRasterMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.FeatureCount != 3 {
		t.Fatalf("feature count = %d, want 3; metadata %s", metadata.FeatureCount, metadataData)
	}
	if metadata.ValueField != "value" || metadata.Connectivity != 4 {
		t.Fatalf("metadata value_field/connectivity = %q/%d", metadata.ValueField, metadata.Connectivity)
	}
	if !metadata.NodataPolicy.Excluded || metadata.NodataPolicy.Nodata == nil || *metadata.NodataPolicy.Nodata != 0 {
		t.Fatalf("nodata policy = %#v, want excluded nodata 0", metadata.NodataPolicy)
	}
	if metadata.SourceRaster.Path != rasterPath || metadata.SourceRaster.Band != 1 {
		t.Fatalf("source raster evidence = %#v", metadata.SourceRaster)
	}
	if len(metadata.Warnings) == 0 {
		t.Fatalf("metadata warnings missing")
	}
}

func TestExecutePolygonizeRasterConnectivityChangesDiagonalRegions(t *testing.T) {
	skipIfPolygonizeGDALMissing(t)
	dir := t.TempDir()
	rasterPath := createPolygonizeTestRaster(t, dir, "diagonal", 2, 2, "1 0\n0 1")

	result, err := ExecutePolygonizeRaster(context.Background(), []byte(validPolygonizeRasterRequestJSON(
		rasterPath,
		`"value_field": "value", "connectivity": 8, "max_features": 10`,
	)), dir)
	if err != nil {
		t.Fatalf("ExecutePolygonizeRaster() error = %v", err)
	}
	if got := int(result.Summary["feature_count"].(int)); got != 1 {
		t.Fatalf("feature_count = %d, want 1 for 8-connected diagonal cells", got)
	}
}

func TestExecutePolygonizeRasterEnforcesMaxFeatures(t *testing.T) {
	skipIfPolygonizeGDALMissing(t)
	dir := t.TempDir()
	rasterPath := createPolygonizeTestRaster(t, dir, "classes", 3, 3, "1 1 0\n1 2 2\n3 2 2")

	_, err := ExecutePolygonizeRaster(context.Background(), []byte(validPolygonizeRasterRequestJSON(
		rasterPath,
		`"value_field": "value", "connectivity": 4, "max_features": 2`,
	)), dir)
	if err == nil {
		t.Fatal("expected max_features error")
	}
	if !strings.Contains(err.Error(), "exceeds max_features") {
		t.Fatalf("error = %v, want max_features context", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "polygonized_classes.gpkg")); !os.IsNotExist(statErr) {
		t.Fatalf("vector exists after max_features failure, stat error = %v", statErr)
	}
}

func validPolygonizeRasterRequestJSON(rasterPath string, options string) string {
	if strings.TrimSpace(options) == "{}" {
		options = ""
	}
	return `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "polygonize_raster",
  "inputs": {
    "raster": {"path": "` + filepath.ToSlash(rasterPath) + `", "band": 1, "nodata": 0}
  },
  "outputs": {
    "vector": "polygonized_classes.gpkg",
    "metadata_json": "polygonized_classes.metadata.json"
  },
  "options": {` + options + `}
}`
}

func createPolygonizeTestRaster(t *testing.T, dir string, name string, width int, height int, rows string) string {
	t.Helper()
	ascPath := filepath.Join(dir, name+".asc")
	tifPath := filepath.Join(dir, name+".tif")

	content := fmt.Sprintf("ncols %d\nnrows %d\nxllcorner 0\nyllcorner 0\ncellsize 30\nNODATA_value 0\n%s\n", width, height, rows)
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
		"-a_ullr", "0", strconv.Itoa(height*30), strconv.Itoa(width*30), "0",
		ascPath,
		tifPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gdal_translate: %v: %s", err, output)
	}
	return tifPath
}

func polygonizedValues(t *testing.T, vectorPath string, layerName string, fieldName string) []int {
	t.Helper()
	doc, err := runOGRInfo(context.Background(), CropPolygonsSource{Path: vectorPath, Layer: layerName}, "-features")
	if err != nil {
		t.Fatalf("ogrinfo features: %v", err)
	}
	layer, err := singleOGRLayer(doc, layerName)
	if err != nil {
		t.Fatalf("single layer: %v", err)
	}
	values := make([]int, 0, len(layer.Features))
	for _, feature := range layer.Features {
		raw, ok := feature.Properties[fieldName]
		if !ok {
			t.Fatalf("feature properties missing %q: %#v", fieldName, feature.Properties)
		}
		switch typed := raw.(type) {
		case float64:
			values = append(values, int(typed))
		case json.Number:
			value, err := typed.Int64()
			if err != nil {
				t.Fatalf("value %q is not int: %v", typed, err)
			}
			values = append(values, int(value))
		default:
			t.Fatalf("value type = %T, want numeric", raw)
		}
	}
	sort.Ints(values)
	return values
}

func skipIfPolygonizeGDALMissing(t *testing.T) {
	t.Helper()
	if _, err := CollectRasterMetadata(map[string]InputSpec{}); err != nil {
		t.Skipf("GDAL-enabled Go build not active: %v", err)
	}
	for _, command := range []string{"gdal_translate", "ogrinfo"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s not available in PATH", command)
		}
	}
	if _, err := polygonizeCommand(); err != nil {
		t.Skip(err.Error())
	}
}

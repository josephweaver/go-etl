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
	"testing"
	"time"
)

func TestParseAggregateByPolygonsRequestRequiresExplicitPolicies(t *testing.T) {
	parsed, err := ParseAggregateByPolygonsRequest([]byte(validAggregateByPolygonsRequestJSON(
		"/worker/data/cdl.tif",
		"/worker/data/fields.geojson",
		`"categorical": true, "all_touched": false, "include_value_nodata": false, "chunk_rows": 8`,
	)))
	if err != nil {
		t.Fatalf("ParseAggregateByPolygonsRequest() error = %v", err)
	}
	if parsed.ValueRaster.Path != "/worker/data/cdl.tif" || parsed.ValueRaster.Band != 1 {
		t.Fatalf("value raster = %#v", parsed.ValueRaster)
	}
	if parsed.Polygons.Path != "/worker/data/fields.geojson" || parsed.Polygons.Layer != "fields" || parsed.Polygons.IDField != "field_id" {
		t.Fatalf("polygons = %#v", parsed.Polygons)
	}
	if parsed.AllTouched {
		t.Fatal("all_touched = true, want false")
	}
	if parsed.IncludeValueNodata {
		t.Fatal("include_value_nodata = true, want false")
	}
	if parsed.ChunkRows != 8 {
		t.Fatalf("chunk_rows = %d, want 8", parsed.ChunkRows)
	}
}

func TestParseAggregateByPolygonsRequestRejectsInvalidGuards(t *testing.T) {
	cases := []struct {
		name    string
		request string
		want    string
	}{
		{
			name:    "missing all_touched",
			request: validAggregateByPolygonsRequestJSON("/worker/data/cdl.tif", "/worker/data/fields.geojson", `"include_value_nodata": false`),
			want:    "options.all_touched is required",
		},
		{
			name:    "missing include value nodata",
			request: validAggregateByPolygonsRequestJSON("/worker/data/cdl.tif", "/worker/data/fields.geojson", `"all_touched": false`),
			want:    "options.include_value_nodata is required",
		},
		{
			name:    "continuous mode",
			request: validAggregateByPolygonsRequestJSON("/worker/data/cdl.tif", "/worker/data/fields.geojson", `"categorical": false, "all_touched": false, "include_value_nodata": false`),
			want:    "categorical=false is not supported",
		},
		{
			name:    "unsafe output",
			request: strings.Replace(validAggregateByPolygonsRequestJSON("/worker/data/cdl.tif", "/worker/data/fields.geojson", `"all_touched": false, "include_value_nodata": false`), `"polygon_crop_counts.csv"`, `"../escape.csv"`, 1),
			want:    `output "counts_csv" path`,
		},
		{
			name:    "bad chunk rows",
			request: validAggregateByPolygonsRequestJSON("/worker/data/cdl.tif", "/worker/data/fields.geojson", `"all_touched": false, "include_value_nodata": false, "chunk_rows": 0`),
			want:    "chunk_rows must be greater than 0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAggregateByPolygonsRequest([]byte(tc.request))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestAggregateRowsFromPairCountsSortsByPolygonIDAndValue(t *testing.T) {
	rows, err := aggregateRowsFromPairCounts([]PairCountRow{
		{FieldID: 2, ValueID: 7, Count: 1},
		{FieldID: 1, ValueID: 5, Count: 3},
		{FieldID: 1, ValueID: 2, Count: 1},
	}, map[uint32]string{
		1: "B002",
		2: "A001",
	})
	if err != nil {
		t.Fatalf("aggregateRowsFromPairCounts() error = %v", err)
	}
	got := fmt.Sprintf("%s:%d:%d:%.6f|%s:%d:%d:%.6f|%s:%d:%d:%.6f",
		rows[0].PolygonID, rows[0].RasterValue, rows[0].Count, rows[0].Proportion,
		rows[1].PolygonID, rows[1].RasterValue, rows[1].Count, rows[1].Proportion,
		rows[2].PolygonID, rows[2].RasterValue, rows[2].Count, rows[2].Proportion,
	)
	want := "A001:7:1:1.000000|B002:2:1:0.250000|B002:5:3:0.750000"
	if got != want {
		t.Fatalf("rows = %s, want %s", got, want)
	}
}

func TestExecuteAggregateByPolygonsWritesExpectedCountsAndMetadata(t *testing.T) {
	skipIfAggregateGDALMissing(t)

	cases := []struct {
		name                string
		allTouched          bool
		includeValueNodata  bool
		wantCSV             string
		wantSkippedZone     uint64
		wantSkippedValue    uint64
		wantValidPixels     uint64
		wantInclusionPolicy string
	}{
		{
			name:                "center only excludes value nodata",
			allTouched:          false,
			includeValueNodata:  false,
			wantCSV:             "polygon_id,raster_value,count,proportion\nA001,1,2,1.000000\n",
			wantSkippedZone:     6,
			wantSkippedValue:    1,
			wantValidPixels:     2,
			wantInclusionPolicy: "all_touched=false",
		},
		{
			name:                "all touched excludes value nodata",
			allTouched:          true,
			includeValueNodata:  false,
			wantCSV:             "polygon_id,raster_value,count,proportion\nA001,1,2,0.400000\nA001,2,3,0.600000\n",
			wantSkippedZone:     3,
			wantSkippedValue:    1,
			wantValidPixels:     5,
			wantInclusionPolicy: "all_touched=true",
		},
		{
			name:                "center only includes value nodata",
			allTouched:          false,
			includeValueNodata:  true,
			wantCSV:             "polygon_id,raster_value,count,proportion\nA001,0,1,0.333333\nA001,1,2,0.666667\n",
			wantSkippedZone:     6,
			wantSkippedValue:    0,
			wantValidPixels:     3,
			wantInclusionPolicy: "all_touched=false",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			rasterPath := createAggregateValueRaster(t, dir, "cdl", "1 2 9\n1 2 9\n0 2 9", "EPSG:5070")
			polygonPath := createAggregatePolygonVector(t, dir, "fields.geojson", "EPSG:5070")

			options := fmt.Sprintf(`"categorical": true, "all_touched": %t, "include_value_nodata": %t, "chunk_rows": 1`, tc.allTouched, tc.includeValueNodata)
			result, err := ExecuteAggregateByPolygons(context.Background(), []byte(validAggregateByPolygonsRequestJSON(rasterPath, polygonPath, options)), dir)
			if err != nil {
				t.Fatalf("ExecuteAggregateByPolygons() error = %v", err)
			}
			if result.Operation != OperationAggregateByPolygons {
				t.Fatalf("operation = %q, want %q", result.Operation, OperationAggregateByPolygons)
			}

			countsData, err := os.ReadFile(filepath.Join(dir, "polygon_crop_counts.csv"))
			if err != nil {
				t.Fatalf("read counts csv: %v", err)
			}
			if string(countsData) != tc.wantCSV {
				t.Fatalf("counts csv = %q, want %q", string(countsData), tc.wantCSV)
			}

			metadata := readAggregateMetadata(t, filepath.Join(dir, "polygon_crop_counts.metadata.json"))
			if metadata.AllTouched != tc.allTouched {
				t.Fatalf("metadata all_touched = %t, want %t", metadata.AllTouched, tc.allTouched)
			}
			if !strings.Contains(metadata.InclusionPolicy, tc.wantInclusionPolicy) {
				t.Fatalf("inclusion policy = %q, want %q context", metadata.InclusionPolicy, tc.wantInclusionPolicy)
			}
			if metadata.NodataPolicy.IncludeValueNodata != tc.includeValueNodata {
				t.Fatalf("metadata include_value_nodata = %t, want %t", metadata.NodataPolicy.IncludeValueNodata, tc.includeValueNodata)
			}
			if metadata.NodataPolicy.SkippedZoneNodata != tc.wantSkippedZone || metadata.NodataPolicy.SkippedValueNodata != tc.wantSkippedValue {
				t.Fatalf("metadata skipped nodata = zone %d value %d, want zone %d value %d", metadata.NodataPolicy.SkippedZoneNodata, metadata.NodataPolicy.SkippedValueNodata, tc.wantSkippedZone, tc.wantSkippedValue)
			}
			if metadata.PairCounts.ValidPixels != tc.wantValidPixels {
				t.Fatalf("valid pixels = %d, want %d", metadata.PairCounts.ValidPixels, tc.wantValidPixels)
			}
			if metadata.TemporaryZoneRaster.Policy == "" || !metadata.TemporaryZoneRaster.CleanedUp {
				t.Fatalf("temporary zone raster policy = %#v", metadata.TemporaryZoneRaster)
			}
			assertNoAggregateTempDirs(t, dir)
		})
	}
}

func TestExecuteAggregateByPolygonsRejectsCRSMismatch(t *testing.T) {
	skipIfAggregateGDALMissing(t)
	dir := t.TempDir()
	rasterPath := createAggregateValueRaster(t, dir, "cdl", "1 2 9\n1 2 9\n0 2 9", "EPSG:5070")
	polygonPath := createAggregatePolygonVector(t, dir, "fields.geojson", "EPSG:4326")

	_, err := ExecuteAggregateByPolygons(context.Background(), []byte(validAggregateByPolygonsRequestJSON(
		rasterPath,
		polygonPath,
		`"categorical": true, "all_touched": false, "include_value_nodata": false`,
	)), dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "does not match polygon layer EPSG") {
		t.Fatalf("error = %v, want CRS mismatch context", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "polygon_crop_counts.csv")); !os.IsNotExist(statErr) {
		t.Fatalf("counts csv exists after CRS failure, stat error = %v", statErr)
	}
}

func validAggregateByPolygonsRequestJSON(rasterPath string, polygonPath string, options string) string {
	return `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "aggregate_by_polygons",
  "inputs": {
    "value_raster": {"path": "` + filepath.ToSlash(rasterPath) + `", "band": 1, "nodata": 0},
    "polygons": {"path": "` + filepath.ToSlash(polygonPath) + `", "layer": "fields", "id_field": "field_id"}
  },
  "outputs": {
    "counts_csv": "polygon_crop_counts.csv",
    "metadata_json": "polygon_crop_counts.metadata.json"
  },
  "options": {` + options + `}
}`
}

func createAggregateValueRaster(t *testing.T, dir string, name string, rows string, crs string) string {
	t.Helper()
	ascPath := filepath.Join(dir, name+".asc")
	tifPath := filepath.Join(dir, name+".tif")
	content := "ncols 3\nnrows 3\nxllcorner 0\nyllcorner 0\ncellsize 1\nNODATA_value 0\n" + rows + "\n"
	if err := os.WriteFile(ascPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write ascii raster: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{
		"-of", "GTiff",
		"-ot", "UInt16",
		"-a_nodata", "0",
		"-a_ullr", "0", "3", "3", "0",
	}
	if crs != "" {
		args = append(args, "-a_srs", crs)
	}
	args = append(args, ascPath, tifPath)
	cmd := exec.CommandContext(ctx, "gdal_translate", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gdal_translate: %v: %s", err, output)
	}
	return tifPath
}

func createAggregatePolygonVector(t *testing.T, dir string, name string, crs string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	epsg := strings.TrimPrefix(crs, "EPSG:")
	if _, err := strconv.Atoi(epsg); err != nil {
		t.Fatalf("test CRS %q must be EPSG:<code>", crs)
	}
	content := `{
  "type": "FeatureCollection",
  "name": "fields",
  "crs": {"type": "name", "properties": {"name": "` + crs + `"}},
  "features": [
    {
      "type": "Feature",
      "properties": {"field_id": "A001"},
      "geometry": {
        "type": "Polygon",
        "coordinates": [[[0.1, 0.1], [1.1, 0.1], [1.1, 2.9], [0.1, 2.9], [0.1, 0.1]]]
      }
    }
  ]
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write polygon vector: %v", err)
	}
	return path
}

func readAggregateMetadata(t *testing.T, path string) AggregateByPolygonsMetadata {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var metadata AggregateByPolygonsMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	return metadata
}

func assertNoAggregateTempDirs(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read artifact root: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".aggregate-by-polygons-") {
			t.Fatalf("temporary aggregate directory was not cleaned up: %s", entry.Name())
		}
	}
}

func skipIfAggregateGDALMissing(t *testing.T) {
	t.Helper()
	if _, err := CollectRasterMetadata(map[string]InputSpec{}); err != nil {
		t.Skipf("GDAL-enabled Go build not active: %v", err)
	}
	for _, command := range []string{"gdal_translate", "gdal_rasterize", "ogrinfo"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s not available in PATH", command)
		}
	}
}

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

func TestParseCropByPolygonsRequestAcceptsBBoxAndNormalizesDirectory(t *testing.T) {
	parsed, err := ParseCropByPolygonsRequest([]byte(validCropByPolygonsRequestJSON(
		"/worker/data/cdl.tif",
		"/worker/data/regions.gpkg",
		`"mode": "bbox", "mask_to_polygon": false, "max_features": 2`,
	)))
	if err != nil {
		t.Fatalf("ParseCropByPolygonsRequest() error = %v", err)
	}
	if parsed.Mode != cropModeBBox {
		t.Fatalf("Mode = %q, want %q", parsed.Mode, cropModeBBox)
	}
	if parsed.OutputDirectory != "cropped_rasters" {
		t.Fatalf("OutputDirectory = %q, want cropped_rasters", parsed.OutputDirectory)
	}
	if parsed.ManifestJSON != "cropped_rasters/manifest.json" {
		t.Fatalf("ManifestJSON = %q, want cropped_rasters/manifest.json", parsed.ManifestJSON)
	}
	if parsed.MaxFeatures != 2 {
		t.Fatalf("MaxFeatures = %d, want 2", parsed.MaxFeatures)
	}
}

func TestParseCropByPolygonsRequestRejectsUnsupportedCutlineMode(t *testing.T) {
	_, err := ParseCropByPolygonsRequest([]byte(validCropByPolygonsRequestJSON(
		"/worker/data/cdl.tif",
		"/worker/data/regions.gpkg",
		`"mode": "cutline", "max_features": 2`,
	)))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), `mode "cutline" is not supported`) {
		t.Fatalf("error = %v, want cutline context", err)
	}
}

func TestParseCropByPolygonsRequestRejectsInvalidGuards(t *testing.T) {
	cases := []struct {
		name    string
		outputs string
		options string
		want    string
	}{
		{
			name:    "manifest outside output directory",
			outputs: `"output_directory": "cropped_rasters/", "manifest_json": "manifest.json"`,
			options: `"mode": "bbox", "max_features": 2`,
			want:    "must be under output_directory",
		},
		{
			name:    "zero max features",
			outputs: `"output_directory": "cropped_rasters/", "manifest_json": "cropped_rasters/manifest.json"`,
			options: `"mode": "bbox", "max_features": 0`,
			want:    "max_features must be greater than 0",
		},
		{
			name:    "mask to polygon",
			outputs: `"output_directory": "cropped_rasters/", "manifest_json": "cropped_rasters/manifest.json"`,
			options: `"mode": "bbox", "mask_to_polygon": true, "max_features": 2`,
			want:    "mask_to_polygon=true is not supported",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requestJSON := `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "crop_by_polygons",
  "inputs": {
    "rasters": [{"name": "cdl", "path": "/worker/data/cdl.tif"}],
    "polygons": {"path": "/worker/data/regions.gpkg", "layer": "regions", "id_field": "region_id"}
  },
  "outputs": {` + tc.outputs + `},
  "options": {` + tc.options + `}
}`
			_, err := ParseCropByPolygonsRequest([]byte(requestJSON))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func validCropByPolygonsRequestJSON(rasterPath string, vectorPath string, options string) string {
	return `{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "crop_by_polygons",
  "inputs": {
    "rasters": [{"name": "cdl", "path": "` + filepath.ToSlash(rasterPath) + `"}],
    "polygons": {"path": "` + filepath.ToSlash(vectorPath) + `", "layer": "regions", "id_field": "region_id"}
  },
  "outputs": {
    "output_directory": "cropped_rasters/",
    "manifest_json": "cropped_rasters/manifest.json"
  },
  "options": {` + options + `}
}`
}

func TestSafePathSegmentForCropFeatureFilenames(t *testing.T) {
	segment := featureFilenameSegment(`../unsafe/id`, 0)
	if strings.Contains(segment, "/") || strings.Contains(segment, `\`) || strings.Contains(segment, "..") {
		t.Fatalf("feature filename segment = %q, want safe single segment", segment)
	}
	if segment == "../unsafe/id" {
		t.Fatalf("feature filename segment exposed raw unsafe id")
	}
}

func TestGeometryBoundsFromOGRGeometry(t *testing.T) {
	var geometry any
	data := []byte(`{"type":"Polygon","coordinates":[[[30,60],[90,60],[90,120],[30,120],[30,60]]]}`)
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&geometry); err != nil {
		t.Fatalf("decode geometry: %v", err)
	}
	box, err := geometryBounds(geometry)
	if err != nil {
		t.Fatalf("geometryBounds() error = %v", err)
	}
	want := RasterBounds{MinX: 30, MinY: 60, MaxX: 90, MaxY: 120}
	if box != want {
		t.Fatalf("bounds = %#v, want %#v", box, want)
	}
}

func TestExecuteCropByPolygonsBBoxWritesManifestAndCrops(t *testing.T) {
	skipIfCropGDALMissing(t)
	dir := t.TempDir()
	rasterPath := createCropTestRaster(t, dir, cropTestRasterSpec{
		name:   "cdl",
		width:  4,
		height: 4,
		ullr:   [4]float64{0, 120, 120, 0},
		crs:    "EPSG:5070",
		rows:   "1 1 2 2\n1 1 2 2\n3 3 4 4\n3 3 4 4",
	})
	vectorPath := createCropVector(t, dir, "EPSG:5070")

	result, err := ExecuteCropByPolygons(context.Background(), []byte(validCropByPolygonsRequestJSON(
		rasterPath,
		vectorPath,
		`"mode": "bbox", "max_features": 2`,
	)), dir)
	if err != nil {
		t.Fatalf("ExecuteCropByPolygons() error = %v", err)
	}
	if result.Operation != OperationCropPolygons {
		t.Fatalf("operation = %q, want %q", result.Operation, OperationCropPolygons)
	}

	manifestPath := filepath.Join(dir, "cropped_rasters", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest CropByPolygonsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Pieces) != 2 {
		t.Fatalf("piece count = %d, want 2; manifest %s", len(manifest.Pieces), data)
	}

	wantDimensions := map[string][2]int{
		"west/unsafe": {2, 4},
		"northeast":   {2, 2},
	}
	for _, piece := range manifest.Pieces {
		if strings.Contains(piece.OutputPath, "west/unsafe") {
			t.Fatalf("output path exposes unsafe raw feature id: %s", piece.OutputPath)
		}
		want, ok := wantDimensions[piece.FeatureID]
		if !ok {
			t.Fatalf("unexpected feature id %q", piece.FeatureID)
		}
		if piece.PixelWidth != want[0] || piece.PixelHeight != want[1] {
			t.Fatalf("piece %q dimensions = [%d,%d], want %v", piece.FeatureID, piece.PixelWidth, piece.PixelHeight, want)
		}
		if piece.Mode != "bbox" {
			t.Fatalf("piece mode = %q, want bbox", piece.Mode)
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(piece.OutputPath))); err != nil {
			t.Fatalf("stat crop output %s: %v", piece.OutputPath, err)
		}
	}
}

func TestExecuteCropByPolygonsRejectsFeatureCountAboveMaxBeforeWriting(t *testing.T) {
	skipIfCropGDALMissing(t)
	dir := t.TempDir()
	rasterPath := createCropTestRaster(t, dir, cropTestRasterSpec{
		name:   "cdl",
		width:  4,
		height: 4,
		ullr:   [4]float64{0, 120, 120, 0},
		crs:    "EPSG:5070",
		rows:   "1 1 2 2\n1 1 2 2\n3 3 4 4\n3 3 4 4",
	})
	vectorPath := createCropVector(t, dir, "EPSG:5070")

	_, err := ExecuteCropByPolygons(context.Background(), []byte(validCropByPolygonsRequestJSON(
		rasterPath,
		vectorPath,
		`"mode": "bbox", "max_features": 1`,
	)), dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "exceeds max_features") {
		t.Fatalf("error = %v, want max_features context", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "cropped_rasters", "manifest.json")); !os.IsNotExist(statErr) {
		t.Fatalf("manifest exists after feature-count failure, stat error = %v", statErr)
	}
}

func TestExecuteCropByPolygonsRejectsCRSMismatch(t *testing.T) {
	skipIfCropGDALMissing(t)
	dir := t.TempDir()
	rasterPath := createCropTestRaster(t, dir, cropTestRasterSpec{
		name:   "cdl",
		width:  4,
		height: 4,
		ullr:   [4]float64{0, 120, 120, 0},
		crs:    "EPSG:5070",
		rows:   "1 1 2 2\n1 1 2 2\n3 3 4 4\n3 3 4 4",
	})
	vectorPath := createCropVector(t, dir, "EPSG:4326")

	_, err := ExecuteCropByPolygons(context.Background(), []byte(validCropByPolygonsRequestJSON(
		rasterPath,
		vectorPath,
		`"mode": "bbox", "max_features": 2`,
	)), dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "does not match polygon layer") {
		t.Fatalf("error = %v, want CRS mismatch context", err)
	}
}

type cropTestRasterSpec struct {
	name   string
	width  int
	height int
	ullr   [4]float64
	crs    string
	rows   string
}

func createCropTestRaster(t *testing.T, dir string, spec cropTestRasterSpec) string {
	t.Helper()
	ascPath := filepath.Join(dir, spec.name+".asc")
	tifPath := filepath.Join(dir, spec.name+".tif")

	content := fmt.Sprintf("ncols %d\nnrows %d\nxllcorner 0 0\ncellsize 30\nNODATA_value 0\n%s\n", spec.width, spec.height, spec.rows)
	if err := os.WriteFile(ascPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write ascii raster: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gdal_translate",
		"-of", "GTiff",
		"-ot", "UInt16",
		"-a_nodata", "0",
		"-a_srs", spec.crs,
		"-a_ullr",
		cropFormatGDALFloat(spec.ullr[0]),
		cropFormatGDALFloat(spec.ullr[1]),
		cropFormatGDALFloat(spec.ullr[2]),
		cropFormatGDALFloat(spec.ullr[3]),
		ascPath,
		tifPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gdal_translate: %v: %s", err, output)
	}
	return tifPath
}

func skipIfCropGDALMissing(t *testing.T) {
	t.Helper()
	for _, command := range []string{"gdal_translate", "gdalwarp", "ogrinfo", "ogr2ogr"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s not available in PATH", command)
		}
	}
}

func createCropVector(t *testing.T, dir string, crs string) string {
	t.Helper()
	geoJSONPath := filepath.Join(dir, "regions.geojson")
	gpkgPath := filepath.Join(dir, "regions.gpkg")
	geoJSON := `{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "properties": {"region_id": "west/unsafe"},
      "geometry": {"type": "Polygon", "coordinates": [[[0,0],[60,0],[60,120],[0,120],[0,0]]]}
    },
    {
      "type": "Feature",
      "properties": {"region_id": "northeast"},
      "geometry": {"type": "Polygon", "coordinates": [[[60,60],[120,60],[120,120],[60,120],[60,60]]]}
    }
  ]
}`
	if err := os.WriteFile(geoJSONPath, []byte(geoJSON), 0o644); err != nil {
		t.Fatalf("write crop vector geojson: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ogr2ogr",
		"-overwrite",
		"-f", "GPKG",
		"-a_srs", crs,
		"-nln", "regions",
		gpkgPath,
		geoJSONPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ogr2ogr: %v: %s", err, output)
	}
	return gpkgPath
}

func ExampleCropByPolygonsRequest() {
	fmt.Println(OperationCropPolygons)
	// Output: crop_by_polygons
}

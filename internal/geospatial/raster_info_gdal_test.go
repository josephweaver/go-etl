//go:build gdal
// +build gdal

package geospatial

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectRasterMetadataSortsInputsAndCollectsRecords(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	rasterPath := createTestRaster(t, dir, "field", "0 1\n2 3")

	metadata, err := CollectRasterMetadata(map[string]InputSpec{
		"z_field": {Path: rasterPath},
		"a_field": {Path: rasterPath},
	})
	if err != nil {
		t.Fatalf("CollectRasterMetadata() error = %v", err)
	}
	if got, want := len(metadata), 2; got != want {
		t.Fatalf("records = %d, want %d", got, want)
	}
	if got, want := metadata[0].Name, "a_field"; got != want {
		t.Fatalf("first name = %q, want %q", got, want)
	}
	if got, want := metadata[1].Name, "z_field"; got != want {
		t.Fatalf("second name = %q, want %q", got, want)
	}

	record := metadata[0]
	if record.PathRole != "input" {
		t.Fatalf("path_role = %q, want %q", record.PathRole, "input")
	}
	if record.Driver != "GTiff" {
		t.Fatalf("driver = %q, want %q", record.Driver, "GTiff")
	}
	if record.Width != 2 || record.Height != 2 {
		t.Fatalf("size = [%d, %d], want [2, 2]", record.Width, record.Height)
	}
	if record.BandCount != 1 {
		t.Fatalf("band_count = %d, want 1", record.BandCount)
	}
	if got, want := record.EPSG, 5070; got != want {
		t.Fatalf("epsg = %d, want %d", got, want)
	}
	if !record.CRSWKTPresent {
		t.Fatal("crs_wkt_present = false, want true")
	}
	if len(record.GeoTransform) != 6 {
		t.Fatalf("geotransform length = %d, want 6", len(record.GeoTransform))
	}
	if got, want := record.Bands[0].Index, 1; got != want {
		t.Fatalf("band index = %d, want %d", got, want)
	}
	if got, want := record.Bands[0].DType, "UInt16"; got != want {
		t.Fatalf("band dtype = %q, want %q", got, want)
	}
}

func TestCollectRasterMetadataErrorsWhenInputMissing(t *testing.T) {
	skipIfGDALMissing(t)
	_, err := CollectRasterMetadata(map[string]InputSpec{"missing": {Path: "does-not-exist.tif"}})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCollectRasterMetadataErrorsForNonRaster(t *testing.T) {
	skipIfGDALMissing(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "not_raster.txt")
	if err := os.WriteFile(path, []byte("not raster data"), 0o644); err != nil {
		t.Fatalf("write non-raster: %v", err)
	}

	_, err := CollectRasterMetadata(map[string]InputSpec{"bad": {Path: path}})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseEPSGFromWKTRequiresCRSAuthority(t *testing.T) {
	yanRoyWKT := `PROJCRS["Albers",BASEGEOGCRS["WGS 84",DATUM["World Geodetic System 1984",ELLIPSOID["WGS 84",6378137,298.257223563,LENGTHUNIT["metre",1]],ID["EPSG",6326]],PRIMEM["Greenwich",0,ANGLEUNIT["Degree",0.0174532925199433]]],CONVERSION["unnamed",METHOD["Albers Equal Area",ID["EPSG",9822]],PARAMETER["Northing at false origin",0,LENGTHUNIT["metre",1],ID["EPSG",8827]]],CS[Cartesian,2],AXIS["(E)",east,ORDER[1],LENGTHUNIT["Meter",1]],AXIS["(N)",north,ORDER[2],LENGTHUNIT["Meter",1]]]`
	if got := parseEPSGFromWKT(yanRoyWKT); got != 0 {
		t.Fatalf("yan/roy epsg = %d, want 0 because only parameter IDs are present", got)
	}

	crsAuthorityWKT := `PROJCRS["NAD83 / Conus Albers",BASEGEOGCRS["NAD83",ID["EPSG",4269]],CONVERSION["Conus Albers",METHOD["Albers Equal Area",ID["EPSG",9822]]],CS[Cartesian,2],AXIS["easting",east],AXIS["northing",north],ID["EPSG",5070]]`
	if got := parseEPSGFromWKT(crsAuthorityWKT); got != 5070 {
		t.Fatalf("crs authority epsg = %d, want 5070", got)
	}

	unitAuthorityWKT := `PROJCRS["Albers",BASEGEOGCRS["WGS 84"],CONVERSION["unnamed"],CS[Cartesian,2],AXIS["(E)",east,LENGTHUNIT["Meter",1,ID["EPSG",9001]]],AXIS["(N)",north,LENGTHUNIT["Meter",1,ID["EPSG",9001]]]]`
	if got := parseEPSGFromWKT(unitAuthorityWKT); got != 0 {
		t.Fatalf("unit authority epsg = %d, want 0", got)
	}
}

func TestResolveGDALDriverNameFromJSONSupportsDriverShortName(t *testing.T) {
	raw := map[string]any{
		"description":     "/tmp/in.tif",
		"driverShortName": "GTiff",
		"driverLongName":  "GeoTIFF",
	}

	got := resolveGDALDriverNameFromJSON(raw, nil)
	if got != "GTiff" {
		t.Fatalf("driver = %q, want %q", got, "GTiff")
	}
}

func createTestRaster(t *testing.T, dir, name string, rows string) string {
	ascPath := filepath.Join(dir, fmt.Sprintf("%s.asc", name))
	tifPath := filepath.Join(dir, fmt.Sprintf("%s.tif", name))

	ascData := fmt.Sprintf("ncols 2\nnrows 2\nxllcorner 0 0\ncellsize 30\nNODATA_value 0\n%s\n", rows)
	if err := os.WriteFile(ascPath, []byte(ascData), 0o644); err != nil {
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

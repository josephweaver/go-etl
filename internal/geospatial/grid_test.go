package geospatial

import "testing"

func TestGridsEqualAcceptsEquivalentAlbersWKTRepresentations(t *testing.T) {
	wkt1 := `PROJCS["unnamed",GEOGCS["WGS 84",DATUM["WGS_1984",SPHEROID["WGS 84",6378137,298.257223563]],PRIMEM["Greenwich",0],UNIT["degree",0.0174532925199433]],PROJECTION["Albers_Conic_Equal_Area"],PARAMETER["latitude_of_center",40],PARAMETER["longitude_of_center",-96],PARAMETER["standard_parallel_1",20],PARAMETER["standard_parallel_2",60],PARAMETER["false_easting",0],PARAMETER["false_northing",0],UNIT["Meter",1]]`
	wkt2 := `PROJCRS["unnamed",BASEGEOGCRS["WGS 84",DATUM["World Geodetic System 1984",ELLIPSOID["WGS 84",6378137,298.257223563,LENGTHUNIT["metre",1]]]],CONVERSION["unnamed",METHOD["Albers Equal Area"],PARAMETER["Latitude of false origin",40,ANGLEUNIT["degree",0.0174532925199433]],PARAMETER["Longitude of false origin",-96,ANGLEUNIT["degree",0.0174532925199433]],PARAMETER["Latitude of 1st standard parallel",20,ANGLEUNIT["degree",0.0174532925199433]],PARAMETER["Latitude of 2nd standard parallel",60,ANGLEUNIT["degree",0.0174532925199433]],PARAMETER["Easting at false origin",0,LENGTHUNIT["metre",1]],PARAMETER["Northing at false origin",0,LENGTHUNIT["metre",1]]],CS[Cartesian,2],AXIS["easting",east],AXIS["northing",north],LENGTHUNIT["metre",1]]`

	a := RasterGrid{
		CRS:          wkt1,
		GeoTransform: []float64{0, 30, 0, 120, 0, -30},
		Width:        4,
		Height:       4,
	}
	b := RasterGrid{
		CRS:          wkt2,
		GeoTransform: []float64{0, 30, 0, 120, 0, -30},
		Width:        4,
		Height:       4,
	}

	if !GridsEqual(a, b) {
		t.Fatalf("GridsEqual() = false, want true for equivalent Albers WKT")
	}
}

func TestGridsEqualRejectsDifferentAlbersParameters(t *testing.T) {
	yanRoyWKT := `PROJCS["unnamed",GEOGCS["WGS 84",DATUM["WGS_1984",SPHEROID["WGS 84",6378137,298.257223563]],PRIMEM["Greenwich",0],UNIT["degree",0.0174532925199433]],PROJECTION["Albers_Conic_Equal_Area"],PARAMETER["latitude_of_center",40],PARAMETER["longitude_of_center",-96],PARAMETER["standard_parallel_1",20],PARAMETER["standard_parallel_2",60],PARAMETER["false_easting",0],PARAMETER["false_northing",0],UNIT["Meter",1]]`
	epsg5070LikeWKT := `PROJCRS["NAD83 / Conus Albers",BASEGEOGCRS["NAD83",DATUM["North American Datum 1983"]],CONVERSION["Conus Albers",METHOD["Albers Equal Area"],PARAMETER["Latitude of false origin",23],PARAMETER["Longitude of false origin",-96],PARAMETER["Latitude of 1st standard parallel",29.5],PARAMETER["Latitude of 2nd standard parallel",45.5],PARAMETER["Easting at false origin",0],PARAMETER["Northing at false origin",0]],CS[Cartesian,2],AXIS["easting",east],AXIS["northing",north],LENGTHUNIT["metre",1]]`

	a := RasterGrid{
		CRS:          yanRoyWKT,
		GeoTransform: []float64{0, 30, 0, 120, 0, -30},
		Width:        4,
		Height:       4,
	}
	b := RasterGrid{
		CRS:          epsg5070LikeWKT,
		GeoTransform: []float64{0, 30, 0, 120, 0, -30},
		Width:        4,
		Height:       4,
	}

	if GridsEqual(a, b) {
		t.Fatalf("GridsEqual() = true, want false for different Albers parameters")
	}
}

func TestGridsEqualRejectsDifferentEPSG(t *testing.T) {
	a := RasterGrid{
		CRS:          "EPSG:5070",
		EPSG:         5070,
		GeoTransform: []float64{0, 30, 0, 120, 0, -30},
		Width:        4,
		Height:       4,
	}
	b := RasterGrid{
		CRS:          "EPSG:4326",
		EPSG:         4326,
		GeoTransform: []float64{0, 30, 0, 120, 0, -30},
		Width:        4,
		Height:       4,
	}

	if GridsEqual(a, b) {
		t.Fatalf("GridsEqual() = true, want false for different EPSG codes")
	}
}

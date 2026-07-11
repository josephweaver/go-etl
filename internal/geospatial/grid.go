package geospatial

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const gridFloatTolerance = 1e-9

var (
	wktMethodRegex    = regexp.MustCompile(`(?i)(?:PROJECTION|METHOD)\s*\[\s*"([^"]+)"`)
	wktParameterRegex = regexp.MustCompile(`(?i)PARAMETER\s*\[\s*"([^"]+)"\s*,\s*([-+0-9.eE]+)`)
)

type RasterGrid struct {
	CRS           string    `json:"crs"`
	EPSG          int       `json:"epsg,omitempty"`
	CRSWKTPresent bool      `json:"crs_wkt_present"`
	GeoTransform  []float64 `json:"geo_transform"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
}

type GridBounds struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

func GridFromMetadata(metadata RasterMetadata) RasterGrid {
	crs := ""
	if metadata.EPSG > 0 {
		crs = fmt.Sprintf("EPSG:%d", metadata.EPSG)
	} else if metadata.CRSWKTPresent {
		crs = metadata.CRSWKT
	}
	transform := append([]float64(nil), metadata.GeoTransform...)
	return RasterGrid{
		CRS:           crs,
		EPSG:          metadata.EPSG,
		CRSWKTPresent: metadata.CRSWKTPresent,
		GeoTransform:  transform,
		Width:         metadata.Width,
		Height:        metadata.Height,
	}
}

func ValidateWarpTargetGrid(grid RasterGrid) error {
	if grid.CRS == "" {
		return fmt.Errorf("target_crs is required")
	}
	if grid.Width <= 0 {
		return fmt.Errorf("target_width must be greater than 0")
	}
	if grid.Height <= 0 {
		return fmt.Errorf("target_height must be greater than 0")
	}
	if len(grid.GeoTransform) != 6 {
		return fmt.Errorf("target_transform must include 6 values")
	}
	if !almostZero(grid.GeoTransform[2]) || !almostZero(grid.GeoTransform[4]) {
		return fmt.Errorf("target_transform with rotation or shear is not supported")
	}
	if grid.GeoTransform[1] <= 0 {
		return fmt.Errorf("target_transform pixel width must be greater than 0")
	}
	if grid.GeoTransform[5] >= 0 {
		return fmt.Errorf("target_transform pixel height must be negative for north-up rasters")
	}
	return nil
}

func BoundsFromGrid(grid RasterGrid) (GridBounds, error) {
	if err := ValidateWarpTargetGrid(grid); err != nil {
		return GridBounds{}, err
	}
	minX := grid.GeoTransform[0]
	maxY := grid.GeoTransform[3]
	maxX := minX + float64(grid.Width)*grid.GeoTransform[1]
	minY := maxY + float64(grid.Height)*grid.GeoTransform[5]
	return GridBounds{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY}, nil
}

func GridsEqual(a RasterGrid, b RasterGrid) bool {
	if !sameGridCRS(a, b) || a.Width != b.Width || a.Height != b.Height {
		return false
	}
	if len(a.GeoTransform) != len(b.GeoTransform) {
		return false
	}
	for i := range a.GeoTransform {
		if math.Abs(a.GeoTransform[i]-b.GeoTransform[i]) > gridFloatTolerance {
			return false
		}
	}
	return true
}

func sameGridCRS(a RasterGrid, b RasterGrid) bool {
	if a.EPSG > 0 || b.EPSG > 0 {
		return a.EPSG == b.EPSG
	}
	if strings.TrimSpace(a.CRS) == strings.TrimSpace(b.CRS) {
		return true
	}

	aAlbers, aOK := parseAlbersCRS(a.CRS)
	bAlbers, bOK := parseAlbersCRS(b.CRS)
	if !aOK || !bOK {
		return false
	}
	return aAlbers.equal(bAlbers)
}

type albersCRS struct {
	Geographic string
	Parameters map[string]float64
}

func (a albersCRS) equal(b albersCRS) bool {
	if a.Geographic != "" && b.Geographic != "" && a.Geographic != b.Geographic {
		return false
	}

	for _, key := range []string{"lat_0", "lon_0", "sp_1", "sp_2", "false_easting", "false_northing"} {
		aValue, aOK := a.Parameters[key]
		bValue, bOK := b.Parameters[key]
		if !aOK || !bOK {
			return false
		}
		if math.Abs(aValue-bValue) > gridFloatTolerance {
			return false
		}
	}
	return true
}

func parseAlbersCRS(crs string) (albersCRS, bool) {
	crs = strings.TrimSpace(crs)
	if crs == "" {
		return albersCRS{}, false
	}

	methodMatch := wktMethodRegex.FindStringSubmatch(crs)
	if len(methodMatch) != 2 || normalizeWKTName(methodMatch[1]) != "albers_equal_area" {
		return albersCRS{}, false
	}

	parameters := map[string]float64{}
	for _, match := range wktParameterRegex.FindAllStringSubmatch(crs, -1) {
		if len(match) != 3 {
			continue
		}
		key := normalizeAlbersParameterName(match[1])
		if key == "" {
			continue
		}
		value, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		parameters[key] = value
	}

	return albersCRS{
		Geographic: inferGeographicCRSName(crs),
		Parameters: parameters,
	}, true
}

func normalizeWKTName(name string) string {
	compact := strings.Builder{}
	for _, char := range strings.ToLower(name) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			compact.WriteRune(char)
		}
	}

	switch compact.String() {
	case "albersconicequalarea", "albersequalarea":
		return "albers_equal_area"
	default:
		return compact.String()
	}
}

func normalizeAlbersParameterName(name string) string {
	switch normalizeWKTName(name) {
	case "latitudeofcenter", "latitudeoffalseorigin", "latitudeoforigin":
		return "lat_0"
	case "longitudeofcenter", "longitudeoffalseorigin", "longitudeoforigin", "centralmeridian":
		return "lon_0"
	case "standardparallel1", "latitudeof1ststandardparallel", "firststandardparallel":
		return "sp_1"
	case "standardparallel2", "latitudeof2ndstandardparallel", "secondstandardparallel":
		return "sp_2"
	case "falseeasting", "eastingatfalseorigin":
		return "false_easting"
	case "falsenorthing", "northingatfalseorigin":
		return "false_northing"
	default:
		return ""
	}
}

func inferGeographicCRSName(crs string) string {
	normalized := normalizeWKTName(crs)
	switch {
	case strings.Contains(normalized, "wgs84") || strings.Contains(normalized, "worldgeodeticsystem1984"):
		return "wgs84"
	case strings.Contains(normalized, "nad83") || strings.Contains(normalized, "northamericandatum1983"):
		return "nad83"
	default:
		return ""
	}
}

func almostZero(value float64) bool {
	return math.Abs(value) <= gridFloatTolerance
}

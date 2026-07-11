package geospatial

import (
	"fmt"
	"math"
)

const gridFloatTolerance = 1e-9

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
	if a.CRS != b.CRS || a.EPSG != b.EPSG || a.Width != b.Width || a.Height != b.Height {
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

func almostZero(value float64) bool {
	return math.Abs(value) <= gridFloatTolerance
}

//go:build !gdal
// +build !gdal

package geospatial

import "fmt"

func CollectRasterMetadata(_ map[string]InputSpec) ([]RasterMetadata, error) {
	return nil, fmt.Errorf("raster_info requires a GDAL-enabled build (go build -tags gdal)")
}

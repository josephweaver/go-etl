# Revision Notes

## 2026-07-07 Initial CDL/Yan/Roy geospatial worker plugin design

- Assumes Data Assets and Materialized Outputs is complete.
- Moves GDAL into a dedicated worker image/plugin boundary, not controller core.
- Uses `github.com/airbusgeo/godal` where appropriate but allows strict GDAL CLI wrappers for operations such as warp, rasterize, and polygonize.
- Preserves default `go test ./...` without native GDAL by isolating GDAL code behind container/build-tag paths.
- Makes `raster_pair_value_counts` the core CDL/Yan/Roy path because Yan/Roy already supplies `field_id` per pixel and CDL supplies `crop_id` per pixel.
- Treats stacked rasters as an optimization, not the canonical logical model.
- Marks model recommendations per slice so low-risk work can use GPT-5.3-Codex-Spark and correctness-sensitive geospatial algorithms can use GPT-5.5 high reasoning.

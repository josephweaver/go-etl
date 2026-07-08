# 004 Reproject And Align Raster

Status: implemented
Recommended model: GPT-5.5 with high reasoning  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Implement GDAL-backed raster reprojection and grid alignment operations for categorical rasters, with explicit CRS, transform, resolution, dimensions, nodata, and resampling semantics.

This slice prepares CDL and Yan/Roy rasters for safe pixelwise pair counting.

## Current State

Raster metadata can be inspected, but rasters cannot yet be aligned or reprojected by a GOET geospatial worker operation.

The pair-count operation requires same CRS, transform, width, height, resolution, and pixel alignment.

## Target State

The `goet-geospatial` CLI supports:

```text
reproject_crs
align_to_grid
```

The operation should support two target modes:

1. explicit target grid:

```json
{
  "target_crs": "EPSG:5070",
  "target_transform": [0, 30, 0, 0, 0, -30],
  "target_width": 100,
  "target_height": 100
}
```

2. align like another raster:

```json
{
  "like_raster": "/worker/data/yanroy_field_grid.tif"
}
```

For CDL/Yan/Roy categorical rasters, the default resampling must be:

```text
nearest
```

The output is a GeoTIFF plus metadata JSON.

## Concept Decision

Categorical rasters must not use bilinear, cubic, average, or other continuous resampling unless the request explicitly opts into it and tests cover it. The default and recommended CDL/Yan/Roy mode is nearest-neighbor.

If `godal` warp APIs are not sufficient or too unstable for the needed behavior, this operation may call `gdalwarp` through `exec.CommandContext` with strict argv construction. Do not shell-concatenate arguments.

## Required Context

Read these files first:

- `internal/geospatial/raster_info*`
- `cmd/goet-geospatial/main.go`
- `containers/goetl-worker-gdal/README.md`
- `docs/concepts/geospatial-worker-plugins/README.md`
- Data Assets implementation for artifact output path expectations

## Allowed Production Files

- `internal/geospatial/align_gdal.go`
- `internal/geospatial/grid.go`
- `internal/geospatial/raster_info_gdal.go` only for shared metadata helpers
- `cmd/goet-geospatial/main.go`
- `docs/concepts/geospatial-worker-plugins/README.md` only for tracker/status updates

## Allowed Test Files

- `internal/geospatial/align_gdal_test.go`
- synthetic tiny raster fixtures
- container smoke request/expected result files

## Out Of Scope

- Pair counting.
- Stacking rasters.
- Polygon cropping.
- Continuous raster resampling policy beyond explicitly passed-through GDAL options.
- Real CDL/Yan/Roy files.
- Automatic CRS guessing when metadata is missing.
- Silent alignment by filename convention.

## Acceptance Criteria

- Aligning a tiny categorical raster to a fixture target grid produces the expected width, height, transform, CRS, dtype, and nodata.
- The default resampling is nearest-neighbor.
- A request for unsupported or unsafe resampling on a categorical operation is rejected unless explicitly allowed by the request schema.
- `align_to_grid` can use a `like_raster` and the output grid exactly matches that raster's grid.
- Output metadata includes source grid, target grid, resampling method, nodata policy, and GDAL version.
- If source CRS is missing, fail clearly instead of guessing.
- If target grid is incomplete or inconsistent, fail before writing output.
- The operation writes only under the declared artifact output path.
- Default tests do not require GDAL; GDAL tests run in the GDAL container path.

## Notes

This is a scientific-correctness slice. If there is uncertainty about pixel alignment, axis order, nodata propagation, or resampling, stop and append the issue to `docs/concepts/geospatial-worker-plugins/issues.md`.

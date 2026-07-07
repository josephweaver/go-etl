# 005 Stack Aligned Rasters

Status: Proposed  
Recommended model: GPT-5.3-Codex-Spark  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Implement an optional optimization operation that combines aligned categorical rasters into a single multi-band GeoTIFF.

For the CDL/Yan/Roy product, the intended stack is:

```text
Band 1: field_id UInt16
Band 2: crop_id  UInt16
```

## Current State

Raster metadata and alignment operations exist. The pair-count operation can later read separate aligned rasters, but a stacked raster may reduce file-management complexity and improve windowed read locality.

## Target State

The `goet-geospatial` CLI supports:

```text
stack_aligned_rasters
```

Example request:

```json
{
  "operation": "stack_aligned_rasters",
  "inputs": {
    "rasters": [
      {"name": "field_id", "path": "/worker/data/yanroy_aligned.tif", "band": 1, "output_band": 1},
      {"name": "crop_id", "path": "/worker/data/cdl_2023_aligned.tif", "band": 1, "output_band": 2}
    ]
  },
  "outputs": {
    "stacked_raster": "field_cdl_stack.tif",
    "metadata_json": "field_cdl_stack.metadata.json"
  },
  "options": {
    "dtype": "uint16",
    "nodata": 0,
    "require_aligned_grid": true
  }
}
```

## Concept Decision

Stacked rasters are an optimization, not the canonical logical model. The canonical operation remains aligned raster pair counting and should support separate rasters.

The initial stacked-raster implementation should require all input bands to share:

- CRS;
- transform;
- width;
- height;
- pixel alignment;
- compatible categorical dtype;
- nodata policy.

Do not attempt arbitrary mixed band types in the first implementation.

## Required Context

Read these files first:

- `internal/geospatial/raster_info*`
- `internal/geospatial/grid.go`
- `internal/geospatial/align_gdal.go`
- `cmd/goet-geospatial/main.go`
- `docs/concepts/geospatial-worker-plugins/README.md`

## Allowed Production Files

- `internal/geospatial/stack_gdal.go`
- `internal/geospatial/grid.go` only for shared alignment helpers
- `cmd/goet-geospatial/main.go`
- `docs/concepts/geospatial-worker-plugins/README.md` only for tracker/status updates

## Allowed Test Files

- `internal/geospatial/stack_gdal_test.go`
- tiny aligned raster fixtures

## Out Of Scope

- Reprojection or alignment inside this operation.
- Pair counting.
- Cloud Optimized GeoTIFF tuning beyond simple safe creation options.
- Mixed dtype stack support.
- Real CDL/Yan/Roy data.

## Acceptance Criteria

- Two tiny aligned UInt16 rasters stack into a two-band GeoTIFF.
- Band order is exactly the declared request order or explicit `output_band` order.
- Output band descriptions or metadata identify `field_id` and `crop_id` roles when provided.
- Output dtype is UInt16 for the initial CDL/Yan/Roy fixture path.
- Output nodata is set to `0` unless the request declares another safe value.
- Misaligned rasters are rejected before output is written.
- Input values greater than UInt16 range are rejected if output dtype is UInt16.
- Metadata JSON records source rasters, source bands, output bands, grid, dtype, and nodata.
- Default tests do not require GDAL; GDAL tests run in the GDAL container path.

## Notes

Because `field_id` is expected to be less than 64k and CDL crop codes fit in UInt16, UInt16 is the right first stack dtype. Do not generalize before the core path is proven.

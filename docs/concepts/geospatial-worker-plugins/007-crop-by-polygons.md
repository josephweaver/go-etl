# 007 Crop By Polygons

Status: implemented  
Recommended model: GPT-5.5 with high reasoning  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Implement a GDAL-backed operation that chops one or more large rasters into smaller raster pieces using vector polygon features or their bounding boxes.

This operation is useful for tiling, regional partitioning, and fixture workflows. It is not required for the core Yan/Roy raster-pair count path if the field-id raster is already tiled.

## Current State

Raster pair counting can operate on already aligned raster inputs, but there is no operation to derive smaller raster pieces from vector polygons.

## Target State

The `goet-geospatial` CLI supports:

```text
crop_by_polygons
```

Example request:

```json
{
  "operation": "crop_by_polygons",
  "inputs": {
    "rasters": [
      {"name": "cdl", "path": "/worker/data/cdl_2023_aligned.tif"}
    ],
    "polygons": {
      "path": "/worker/data/regions.gpkg",
      "layer": "regions",
      "id_field": "region_id"
    }
  },
  "outputs": {
    "output_directory": "cropped_rasters/",
    "manifest_json": "cropped_rasters/manifest.json"
  },
  "options": {
    "mode": "bbox",
    "mask_to_polygon": false,
    "max_features": 1000
  }
}
```

The operation writes an artifact directory plus a manifest describing each cropped piece.

## Concept Decision

Support two explicit modes:

| Mode | Meaning |
|---|---|
| `bbox` | Crop to the polygon feature bounding box only. Fast and simple. |
| `cutline` | Crop and mask to the polygon geometry. Slower and more semantically complex. |

The first implementation may support only `bbox` if `cutline` proves too large, but it must reject unsupported modes clearly.

Feature IDs must be sanitized into safe relative output filenames. Never use raw feature IDs directly as paths.

## Required Context

Read these files first:

- `internal/geospatial/raster_info*`
- `internal/geospatial/grid.go`
- `internal/geospatial/align_gdal.go`
- `cmd/goet-geospatial/main.go`
- Data Assets artifact directory/promotion implementation
- `docs/concepts/geospatial-worker-plugins/README.md`

## Allowed Production Files

- `internal/geospatial/crop_polygons_gdal.go`
- `internal/geospatial/vector_info_gdal.go` if needed
- `internal/geospatial/pathnames.go` if safe output naming needs helpers
- `cmd/goet-geospatial/main.go`
- `docs/concepts/geospatial-worker-plugins/README.md` only for tracker/status updates

## Allowed Test Files

- `internal/geospatial/crop_polygons_gdal_test.go`
- tiny vector and raster fixtures
- container smoke request/expected manifest files

## Out Of Scope

- Pair counting.
- Polygon aggregation.
- Polygonization.
- Reprojecting vectors or rasters inside this operation unless explicitly implemented with tests.
- Unbounded one-file-per-feature output on large vector layers.
- Real CDL/Yan/Roy data.

## Acceptance Criteria

- A tiny raster and two polygon features produce two cropped raster outputs and a deterministic manifest.
- `bbox` mode crops to expected raster windows.
- Unsupported `cutline` mode is either implemented with tests or rejected clearly.
- Feature count is bounded by `max_features` or another explicit guardrail.
- Output filenames are safe relative paths and do not expose raw unsafe feature IDs.
- CRS mismatch between raster and vector fails clearly unless explicit reprojection support is implemented in this slice.
- The operation writes only under the declared artifact output directory.
- The manifest records source raster, feature ID, crop bounds, output path, pixel dimensions, and mode.
- Default tests do not require GDAL; GDAL tests run in the GDAL container path.

## Notes

This operation can create many files. Keep the first implementation fixture-sized and guard against accidental output explosion.

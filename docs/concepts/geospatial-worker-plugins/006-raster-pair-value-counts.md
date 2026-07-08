# 006 Raster Pair Value Counts

Status: Implemented  
Recommended model: GPT-5.5 with high reasoning  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Implement the core CDL/Yan/Roy raster-to-raster categorical count operation:

```text
field_id raster + crop_id raster -> field_id,crop_id,count
```

This slice produces the compact table that later Python/R analytics can summarize into field-year crop composition, dominant crop, acres, proportions, entropy, and RCI inputs.

## Current State

Rasters can be inspected, aligned, and optionally stacked. The core pixel-pair count operation does not yet exist.

## Target State

The `goet-geospatial` CLI supports:

```text
raster_pair_value_counts
```

The operation supports both input modes.

Separate aligned rasters:

```json
{
  "inputs": {
    "field_raster": {"path": "/worker/data/yanroy.tif", "band": 1, "nodata": 0},
    "value_raster": {"path": "/worker/data/cdl_2023.tif", "band": 1, "nodata": 0}
  }
}
```

Stacked raster:

```json
{
  "inputs": {
    "stacked_raster": {
      "path": "/worker/data/field_cdl_stack.tif",
      "field_band": 1,
      "value_band": 2,
      "field_nodata": 0,
      "value_nodata": 0
    }
  }
}
```

Output CSV:

```csv
field_id,crop_id,count
101,1,817
101,5,29
102,24,1402
```

Output metadata JSON:

```json
{
  "valid_pixels": 224191,
  "skipped_field_nodata": 34,
  "skipped_value_nodata": 12,
  "distinct_fields": 718,
  "distinct_values": 42,
  "distinct_pairs": 1482,
  "count_dtype": "uint64"
}
```

## Concept Decision

This is the core CDL/Yan/Roy data product operation. It must be chunked/windowed and must not load full national rasters into memory.

Use deterministic row ordering:

```text
ORDER BY field_id ASC, crop_id ASC
```

Use `uint64` counts. Use `uint16` categorical values for the initial `field_id` and `crop_id` path. Reject values outside the declared dtype range instead of wrapping.

The fast in-memory key for the initial path may be:

```go
key := uint32(fieldID)<<16 | uint32(cropID)
```

but output must remain explicit columns, not packed keys.

## Required Context

Read these files first:

- `internal/geospatial/raster_info*`
- `internal/geospatial/grid.go`
- `internal/geospatial/stack_gdal.go` if present
- `cmd/goet-geospatial/main.go`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- implemented artifact-output contract files
- `docs/concepts/geospatial-worker-plugins/README.md`

## Allowed Production Files

- `internal/geospatial/pair_counts_gdal.go`
- `internal/geospatial/pair_counts.go` if a non-GDAL pure counting core helps tests
- `internal/geospatial/grid.go` only for shared alignment checks
- `cmd/goet-geospatial/main.go`
- `docs/concepts/geospatial-worker-plugins/README.md` only for tracker/status updates

## Allowed Test Files

- `internal/geospatial/pair_counts_test.go`
- `internal/geospatial/pair_counts_gdal_test.go`
- tiny raster fixtures with known counts
- container smoke request/expected output files

## Out Of Scope

- Proportions or dominant-crop policy.
- Acres calculation unless pixel area is already trivial and emitted only as metadata.
- Parquet output.
- Polygonization.
- Vector polygon aggregation.
- Reprojecting inputs inside this operation.
- Real CDL/Yan/Roy data.

## Acceptance Criteria

- A tiny fixture with known `field_id` and `crop_id` arrays produces the exact expected `field_id,crop_id,count` CSV.
- Both separate-raster mode and stacked-raster mode are supported or the unsupported mode is explicitly deferred in the README tracker.
- Misaligned separate rasters fail before counting.
- Missing nodata declarations use band nodata when available; otherwise the request must be explicit for CDL/Yan/Roy fixtures.
- Pixels with field nodata are skipped.
- Pixels with value nodata are skipped unless `include_value_nodata` is explicitly true.
- Counts use `uint64` and cannot overflow silently.
- Rows are sorted by `field_id,crop_id`.
- Metadata reports valid and skipped pixel counts.
- The implementation reads rasters in chunks/windows and has a configurable chunk size or memory guard.
- Default tests do not require GDAL; GDAL tests run in the GDAL container path.

## Notes

Do not calculate final crop proportions in this plugin. The plugin should produce the irreducible count table. A downstream Python/R step can compute:

```text
total_pixels_by_field
crop_proportion
dominant_crop
acres
entropy/mixedness
RCI inputs
```

If there is any doubt about alignment or nodata semantics, fail closed and append the ambiguity to `docs/concepts/geospatial-worker-plugins/issues.md`.

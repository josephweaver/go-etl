# 009 Aggregate By Polygons

Status: Proposed  
Recommended model: GPT-5.5 with high reasoning  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Implement a categorical zonal-count operation that aggregates raster values by vector polygon features.

This operation supports workflows where zones are vector polygons rather than a Yan/Roy field-id raster.

## Current State

The core CDL/Yan/Roy product can count raster pairs directly. There is no vector-polygon zonal count operation.

## Target State

The `goet-geospatial` CLI supports:

```text
aggregate_by_polygons
```

Example request:

```json
{
  "operation": "aggregate_by_polygons",
  "inputs": {
    "value_raster": {"path": "/worker/data/cdl_2023.tif", "band": 1, "nodata": 0},
    "polygons": {
      "path": "/worker/data/fields.gpkg",
      "layer": "fields",
      "id_field": "field_id"
    }
  },
  "outputs": {
    "counts_csv": "polygon_crop_counts.csv",
    "metadata_json": "polygon_crop_counts.metadata.json"
  },
  "options": {
    "categorical": true,
    "all_touched": false,
    "include_value_nodata": false
  }
}
```

Output CSV:

```csv
polygon_id,raster_value,count,proportion
A001,1,817,0.966
A001,5,29,0.034
A002,24,1402,1.000
```

## Concept Decision

The recommended implementation strategy is:

```text
vector polygons -> temporary aligned zone-id raster -> pair count(zone_id, raster_value)
```

This reuses the raster-pair counting machinery while making the pixel inclusion policy explicit through the rasterization step.

The request must explicitly define the polygon inclusion policy:

| Option | Meaning |
|---|---|
| `all_touched=false` | Default center-of-pixel style rasterization. |
| `all_touched=true` | Count any pixel touched by a polygon. |

Do not silently choose an inclusion policy without recording it in metadata.

## Required Context

Read these files first:

- `internal/geospatial/pair_counts*`
- `internal/geospatial/grid.go`
- `internal/geospatial/raster_info*`
- `cmd/goet-geospatial/main.go`
- `docs/concepts/geospatial-worker-plugins/README.md`

## Allowed Production Files

- `internal/geospatial/aggregate_polygons_gdal.go`
- `internal/geospatial/rasterize_zones_gdal.go`
- `internal/geospatial/pair_counts.go` only for reusable generalized counting helpers
- `cmd/goet-geospatial/main.go`
- `docs/concepts/geospatial-worker-plugins/README.md` only for tracker/status updates

## Allowed Test Files

- `internal/geospatial/aggregate_polygons_gdal_test.go`
- tiny raster/vector fixtures with known counts
- tests for both `all_touched=false` and `all_touched=true` if both are implemented

## Out Of Scope

- Core CDL/Yan/Roy raster-id count path.
- Continuous zonal statistics such as mean, min, max, variance.
- Weighted area overlap fractions.
- Polygon topology repair.
- Real CDL/Yan/Roy data.
- Parquet output.

## Acceptance Criteria

- A tiny polygon/raster fixture produces expected categorical counts.
- Output rows are sorted by `polygon_id,raster_value`.
- Per-polygon proportions sum to 1.0 over included values, within documented rounding rules.
- `all_touched` policy is explicit in request and metadata.
- Nodata behavior is explicit and tested.
- CRS mismatch fails clearly unless explicit reprojection is implemented and tested.
- Temporary zone rasters are written only under safe temp/artifact paths and are cleaned up or declared according to the artifact policy.
- The operation reuses or parallels the chunked counting logic rather than loading large rasters into memory.
- Default tests do not require GDAL; GDAL tests run in the GDAL container path.

## Notes

This operation is valuable, but it is not the fastest path for Yan/Roy because Yan/Roy already gives a field-id raster. Prefer `raster_pair_value_counts` for the CDL/Yan/Roy field composition product.

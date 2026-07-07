# 003 Raster Info And Bounding Boxes

Status: Implemented  
Recommended model: GPT-5.3-Codex-Spark  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Implement a read-only geospatial operation that reports raster metadata and bounding boxes for one or more raster files.

This is the first real GDAL operation and should remain narrow.

## Current State

The geospatial CLI contract exists, but no operation reads raster files.

The CDL/Yan/Roy workflow needs a way to inspect each input raster before alignment and counting.

## Target State

The `goet-geospatial` CLI supports this operation:

```text
raster_info
```

The request preserves the OS-002 geospatial request contract. `inputs` remains
`map[string]InputSpec`, and `raster_info` consumes every named entry in
`request.Inputs`:

```json
{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_info",
  "inputs": {
    "field": {"path": "/worker/data/yanroy.tif"},
    "cdl": {"path": "/worker/data/cdl_2023.tif"}
  },
  "outputs": {
    "metadata_json": "raster_info.json"
  }
}
```

Multiple raster metadata records are emitted in deterministic lexicographic
input-name order, not JSON request order.

For each raster, the operation reports compact metadata:

```json
{
  "name": "field",
  "path_role": "input",
  "driver": "GTiff",
  "width": 100,
  "height": 100,
  "band_count": 1,
  "crs_wkt_present": true,
  "epsg": 5070,
  "geo_transform": [0, 30, 0, 0, 0, -30],
  "bounds": {
    "min_x": 0,
    "min_y": -3000,
    "max_x": 3000,
    "max_y": 0
  },
  "bands": [
    {"index": 1, "dtype": "UInt16", "nodata": 0}
  ]
}
```

## Concept Decision

This operation is read-only. It should not normalize, reproject, crop, or rewrite rasters.

Bounding boxes should initially be reported in the raster's native CRS. Optional WGS84 bounds can be added later, but this slice should not require coordinate transformation unless it is trivial and well-tested.

## Required Context

Read these files first:

- `cmd/goet-geospatial/main.go`
- `internal/geospatial/contract.go`
- `internal/geospatial/validation.go`
- `containers/goetl-worker-gdal/README.md`
- `docs/concepts/geospatial-worker-plugins/README.md`

## Allowed Production Files

- `internal/geospatial/raster_info_gdal.go`
- `internal/geospatial/raster_info.go` if a non-GDAL interface wrapper is helpful
- `internal/geospatial/contract.go`
- `internal/geospatial/validation.go`
- `cmd/goet-geospatial/main.go`
- `containers/goetl-worker-gdal/Dockerfile` if needed to build or expose `goet-geospatial` in the GDAL worker container
- `containers/goetl-worker-gdal/test` if needed to run the operation smoke
- `docs/concepts/geospatial-worker-plugins/README.md` only for tracker/status updates

GDAL-dependent files must use the `gdal` build tag if native GDAL is not required for default tests.

## Allowed Test Files

- `internal/geospatial/raster_info_gdal_test.go`
- tiny raster fixtures under `internal/geospatial/testdata/` or generated inside tests
- container smoke fixture files under `containers/goetl-worker-gdal/fixtures/`

## Out Of Scope

- Reprojection.
- Grid alignment.
- Pair counting.
- Polygon reads.
- Vector/raster overlay.
- Real CDL or Yan/Roy files.
- Writing GeoTIFF outputs.

## Acceptance Criteria

- A fixture raster produces deterministic metadata JSON.
- Width, height, band count, data type, nodata, geotransform, and native bounds are reported correctly.
- Multiple input rasters are reported in deterministic lexicographic input-name order.
- The existing map-based `inputs` contract is preserved; this slice must not replace it with an ordered raster array.
- Missing input files fail with a clear error.
- Non-raster input files fail with a clear error.
- The operation writes its metadata artifact under the declared relative output path.
- GDAL-specific tests run in the GDAL container path.
- Default `go test ./...` remains usable without native GDAL.

## Notes

This operation is also the foundation for future alignment checks. Keep returned metadata precise and machine-readable; avoid prose-only summaries.

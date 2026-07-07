# 008 Polygonize Raster

Status: implemented  
Recommended model: GPT-5.5 with high reasoning  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Implement an optional GDAL-backed operation that converts contiguous raster regions with the same categorical value into vector polygons.

This operation is useful for export, debugging, or alternate workflows. It is **not** part of the core CDL/Yan/Roy count path.

## Current State

The core product can use raster-pair counts directly. There is no operation to polygonize raster values.

## Target State

The `goet-geospatial` CLI supports:

```text
polygonize_raster
```

Example request:

```json
{
  "operation": "polygonize_raster",
  "inputs": {
    "raster": {"path": "/worker/data/classes.tif", "band": 1, "nodata": 0}
  },
  "outputs": {
    "vector": "polygonized_classes.gpkg",
    "metadata_json": "polygonized_classes.metadata.json"
  },
  "options": {
    "value_field": "value",
    "connectivity": 4,
    "max_features": 10000
  }
}
```

## Concept Decision

Polygonization can explode geometry count and output size. The first implementation must include guardrails:

- fixture-sized default tests;
- optional `max_features` or post-run count limit;
- clear warning in metadata;
- no use in the core CDL/Yan/Roy pair-count workflow.

If `godal` does not expose polygonization cleanly, wrap GDAL's polygonize utility through explicit argv rather than writing a custom polygonizer.

## Required Context

Read these files first:

- `cmd/goet-geospatial/main.go`
- `internal/geospatial/raster_info*`
- `containers/goetl-worker-gdal/README.md`
- `docs/concepts/geospatial-worker-plugins/README.md`

## Allowed Production Files

- `internal/geospatial/polygonize_gdal.go`
- `internal/geospatial/vector_info_gdal.go` if needed for output validation
- `cmd/goet-geospatial/main.go`
- `docs/concepts/geospatial-worker-plugins/README.md` only for tracker/status updates

## Allowed Test Files

- `internal/geospatial/polygonize_gdal_test.go`
- tiny raster fixtures with known polygon counts
- container smoke request/expected metadata files

## Out Of Scope

- Using polygonization as a prerequisite for CDL field/crop composition.
- Dissolving polygons across files.
- Topology repair beyond GDAL output validity checks.
- Large real rasters.
- Vector simplification policy.
- Field-boundary inference.

## Acceptance Criteria

- A tiny fixture raster polygonizes into the expected categorical regions.
- Nodata is excluded by default.
- The output vector has a value field containing the source raster value.
- Connectivity policy is explicit: 4-connected or 8-connected.
- `max_features` or equivalent guardrail is enforced or recorded as an issue if GDAL output feature counting cannot be bounded pre-run.
- Metadata reports feature count, value field, connectivity, nodata policy, and source raster evidence.
- The operation writes only under declared artifact paths.
- Default tests do not require GDAL; GDAL tests run in the GDAL container path.

## Notes

Do not let this operation pull the design back toward polygonizing Yan/Roy just to aggregate CDL. The raster-pair count operation is the preferred product path.

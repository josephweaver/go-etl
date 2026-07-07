# Geospatial Worker Plugins Strategic Concept

Status: Proposed  
Cadence: CSxIx  
Revision: 2026-07-07 initial CDL/Yan/Roy raster-product design

## Purpose

Add GDAL-backed Go worker operations for geospatial transformations needed by the CDL-by-Yan/Roy data product, assuming **Data Assets and Materialized Outputs is already implemented**.

The motivating product is:

```text
Yan/Roy field-id raster + USDA CDL crop-code raster
  -> aligned categorical raster pair
  -> field_id,crop_id,count
  -> field/year/crop composition table
  -> dominant-crop and downstream RCI transforms
```

The important design decision is that the Yan/Roy raster already contains `field_id` per pixel and the CDL raster contains `crop_id` per pixel. Therefore the core product should not polygonize Yan/Roy first. The efficient core operation is a raster-to-raster categorical pair count:

```text
for each aligned pixel:
  count[field_id, crop_id] += 1
```

The first reusable output should be:

```csv
field_id,crop_id,count
101,1,817
101,5,29
102,24,1402
```

A Python or R script can then calculate proportions, dominant crop, acres, entropy/mixedness, and other statistical summaries from the compact count table.

## Strategic Decision

Geospatial behavior belongs in **worker operations running inside a GDAL-enabled worker container**, not in GOET controller core.

GOET core owns orchestration, data-asset materialization, artifact promotion, status, and provenance. The geospatial worker plugin owns raster/vector transformation semantics. Data assets provide worker-local input paths. The plugin writes outputs under the attempt artifact directory. GOET records compact manifests and evidence, not raster bytes.

The first implementation should use a dedicated external CLI-style worker plugin, not Go runtime `plugin` loading:

```text
GOET worker assignment
  -> materialized data assets exposed as worker-local paths
  -> goet-geospatial operation request
  -> GDAL/godal operation inside GDAL worker image
  -> output artifact files + compact result manifest
```

This keeps the plugin portable across Docker, Singularity/Apptainer, fake HPCC, real HPCC, and local Linux containers.

## Relationship to Data Assets and Materialized Outputs

This concept assumes Data Assets and Materialized Outputs is complete.

This concept must not reimplement:

- HTTP/local/rclone data providers;
- asset cache policy;
- archive extraction;
- `GOET_DATA_ASSETS_JSON` exposure;
- artifact staging and promotion;
- publish-to-named-location behavior.

Instead, this concept consumes those capabilities:

```text
Data Assets layer:
  cdl_2023.local_path      -> /worker/cache/cdl/2023/cdl.tif
  yanroy_tile.local_path   -> /worker/cache/yanroy/tile_001/fields.tif
  artifact_dir             -> /worker/attempts/<attempt_id>/artifacts

Geospatial plugin layer:
  reads local_path inputs
  writes artifacts under artifact_dir
  emits compact geospatial result metadata
```

## Go/GDAL Library Decision

Use `github.com/airbusgeo/godal` as the primary Go binding where it fits. GDAL command-line utilities may be wrapped through `exec.CommandContext` when the CLI is more stable or complete for a specific operation, especially warp, rasterize, and polygonize paths.

Do not write a new general-purpose GDAL wrapper in this concept.

Because `godal` requires native GDAL headers and cgo, GDAL-dependent Go code must be isolated so default non-GDAL development still works:

```text
normal repo tests:      go test ./...              # must not require GDAL
GDAL plugin tests:      go test -tags gdal ./...   # run inside GDAL worker image
container smoke:        containers/goetl-worker-gdal/test
```

## Product Raster Assumptions

Initial CDL/Yan/Roy assumptions:

- `field_id` is less than 64k.
- `crop_id` fits in `uint16`.
- `0` is the preferred nodata/sentinel value for both rasters unless a fixture explicitly declares otherwise.
- Core pair counting uses `uint16` categorical values and `uint64` counts.
- Categorical reprojection must use nearest-neighbor resampling by default.
- Raster pair counting requires same CRS, transform, resolution, dimensions, and pixel alignment.
- If rasters are not aligned, run `reproject_crs` / `align_to_grid` before pair counting.

## Worker Operation Family

Initial operation names:

| Operation | Purpose | Core CDL path? |
|---|---|---:|
| `raster_info` / `get_bounding_boxes` | Inspect raster metadata, bounds, grid, bands, nodata, dtype. | Yes |
| `reproject_crs` / `align_to_grid` | Reproject or align a categorical raster to a target CRS/grid. | Yes |
| `stack_aligned_rasters` | Combine aligned rasters into a multi-band GeoTIFF. | Optional optimization |
| `raster_pair_value_counts` | Produce `field_id,crop_id,count` from aligned categorical rasters. | Yes |
| `crop_by_polygons` | Chop rasters into polygon/bbox-based pieces. | Situational |
| `polygonize_raster` | Convert contiguous equal-value raster regions to vector polygons. | No, optional/export |
| `aggregate_by_polygons` | Categorical zonal counts from raster values by vector polygon IDs. | Alternative path |

## Target Request Contract

The external plugin should accept a stable JSON request, either by `--request <path>` or stdin. The exact worker integration may evolve, but the request shape should stay stable:

```json
{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "field_raster": {
      "path": "${data.yanroy_field.local_path}",
      "band": 1,
      "nodata": 0
    },
    "value_raster": {
      "path": "${data.cdl_year.local_path}",
      "band": 1,
      "nodata": 0
    }
  },
  "outputs": {
    "counts_csv": "field_crop_counts.csv",
    "metadata_json": "field_crop_counts.metadata.json"
  },
  "options": {
    "require_aligned_grid": true,
    "chunk_rows": 1024,
    "field_dtype": "uint16",
    "value_dtype": "uint16"
  }
}
```

The plugin result should be compact:

```json
{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationResult",
  "operation": "raster_pair_value_counts",
  "artifacts": [
    {
      "name": "counts_csv",
      "path": "field_crop_counts.csv",
      "kind": "table",
      "format": "csv"
    }
  ],
  "summary": {
    "valid_pixels": 224191,
    "skipped_field_nodata": 34,
    "skipped_value_nodata": 12,
    "distinct_pairs": 1482
  }
}
```

## Goals

- Add a GDAL-enabled worker image variant while preserving the current minimal worker image.
- Keep native GDAL/cgo requirements out of default `go test ./...`.
- Add a stable geospatial operation request/result contract.
- Implement fixture-sized geospatial operations using Go plus GDAL/godal where appropriate.
- Prove operations inside the container that will later run under Singularity/Apptainer on HPCC.
- Prefer streaming/windowed raster reads over full-raster memory loading.
- Use deterministic outputs with stable row ordering for count tables.
- Keep all tests small and synthetic.
- Keep real CDL, Yan/Roy, MSU HPCC, LandCore, or private path details out of the reusable repository.
- Preserve the ownership boundary: GOET core orchestrates; plugins transform; workflows bind data; customer data stays external.

## Non-Goals

- Implementing crop science, RCI policy, Bayesian field-boundary probabilities, or crop modeling inside GOET core.
- Downloading full CDL rasters or real Yan/Roy releases in tests.
- Requiring GDAL on a developer's normal Windows/local Go environment.
- Storing raster, vector, Parquet, archive, or large tabular bytes in SQLite.
- Replacing Python/R analytics for post-count statistical summaries.
- Building a general geospatial database, catalog, tile server, or object store.
- Making polygonization part of the core CDL/Yan/Roy count path.
- Supporting arbitrary mixed raster dtypes in the first stacked-raster implementation.

## Implementation Tracker

| Slice | Status | Recommended model | Why |
|---|---|---|---|
| `001-gdal-worker-image-baseline.md` | Proposed | GPT-5.3-Codex-Spark | Container/package/test/doc work with narrow behavior. |
| `002-geospatial-operation-contract.md` | implemented | GPT-5.5 high reasoning | Defines the long-lived plugin boundary. |
| `003-raster-info-and-bounding-boxes.md` | implemented | GPT-5.3-Codex-Spark | Read-only metadata operation with small fixtures. |
| `004-reproject-and-align-raster.md` | implemented | GPT-5.5 high reasoning | CRS/grid/resampling semantics are correctness-sensitive. |
| `005-stack-aligned-rasters.md` | Implemented | GPT-5.3-Codex-Spark | Mechanical after alignment validator exists. |
| `006-raster-pair-value-counts.md` | Implemented | GPT-5.5 high reasoning | Core product algorithm; chunking and numeric invariants matter. |
| `007-crop-by-polygons.md` | Implemented | GPT-5.5 high reasoning | Vector/raster CRS, cutline, and output explosion risks. |
| `008-polygonize-raster.md` | Proposed | GPT-5.5 high reasoning | Geometry explosion and GDAL utility behavior need guardrails. |
| `009-aggregate-by-polygons.md` | Proposed | GPT-5.5 high reasoning | Categorical zonal stats require a precise inclusion policy. |
| `010-cdl-yanroy-fixture-workflow-and-docs.md` | Proposed | GPT-5.3-Codex-Spark after 001-006 pass | Integration/docs only if dependencies are complete. |

## Suggested Implementation Order

Minimum useful path for the CDL data product:

```text
001 GDAL worker image baseline
002 operation contract
003 raster info / bounds
004 reproject / align
006 raster pair value counts
010 fixture workflow and docs
```

Useful optimization:

```text
005 stack aligned rasters
```

Secondary geospatial tools:

```text
007 crop by polygons
008 polygonize raster
009 aggregate by polygons
```

## Model Allocation Guidance

Use GPT-5.3-Codex-Spark when the slice is mostly:

- adding container packages and smoke tests;
- adding docs or examples;
- validating existing contract shapes;
- implementing metadata reads or straightforward file transformations with clear acceptance tests.

Use GPT-5.5 with high reasoning when the slice involves:

- CRS/grid correctness;
- categorical resampling policy;
- chunked raster algorithms;
- vector/raster inclusion rules;
- output explosion risk;
- anything that could silently corrupt the CDL/Yan/Roy product.

## Issues Policy

If an implementation agent finds a major issue, blocker, or unsafe ambiguity, stop and create or append:

```text
docs/concepts/geospatial-worker-plugins/issues.md
```

Do not silently choose geospatial defaults that affect scientific meaning, especially CRS, resampling, nodata handling, pixel inclusion, or polygon/raster alignment.

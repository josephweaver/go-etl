# 010 CDL Yan/Roy Fixture Workflow And Docs

Status: Implemented  
Recommended model: GPT-5.3-Codex-Spark after slices 001-006 pass  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Add a fixture-sized CDL/Yan/Roy workflow demonstration and documentation updates that prove the geospatial worker plugin path using Data Assets and Materialized Outputs.

This slice should not use real CDL or Yan/Roy data.

## Current State

The core geospatial operations exist individually. The repository does not yet have a complete fixture path showing how Data Assets bind local raster inputs, invoke geospatial operations, promote count artifacts, and run downstream tabular summary logic.

## Target State

The repository has a tiny fixture workflow equivalent to:

```text
fixture field-id raster + fixture CDL crop-id raster
  -> raster_info / grid validation
  -> align_to_grid if needed
  -> raster_pair_value_counts
  -> Python/R or small Go summary fixture
  -> field_id,crop_id,count,proportion,dominant flag
```

The fixture should demonstrate the preferred product path:

```text
Yan/Roy raster already has field_id per pixel.
Do not polygonize it before counting CDL crop IDs.
```

## Concept Decision

This slice is an integration and documentation slice. It should not add new geospatial algorithms.

Use fixture data that can run locally and in the GDAL worker image. Do not download national CDL data, use real Yan/Roy releases, or require HPCC.

If Data Assets introduced demo-project conventions outside the reusable repo, place only reusable fixture metadata or documentation in this repo and document where customer-facing workflow examples should live.

## Required Context

Read these files first:

- all implemented files from slices 001-006
- implemented Data Assets and Materialized Outputs docs/code
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/README.md`
- `PROJECT_STATE.md`
- `containers/README.md`
- `docs/concepts/geospatial-worker-plugins/README.md`

## Allowed Production Files

- fixture workflow or example files under the repository's existing demo/example convention
- `scripts/` smoke path if existing conventions use scripts
- `docs/concepts/geospatial-worker-plugins/README.md`
- `docs/concepts/README.md`
- `PROJECT_STATE.md`
- `containers/README.md` if the smoke path changes container docs

## Allowed Test Files

- fixture rasters generated in test setup or tiny committed fixtures if acceptable by repo policy
- smoke script expected-output files
- summary script test fixture files

## Out Of Scope

- New geospatial operations.
- Real CDL downloads.
- Real Yan/Roy archives.
- Real Google Drive, rclone, MSU HPCC, or LandCore data.
- Parquet output unless already supported by Data Assets and available in the fixture environment.
- RCI policy design.

## Acceptance Criteria

- A smoke path runs the fixture data through the core geospatial count path.
- The fixture produces exact expected `field_id,crop_id,count` rows.
- A downstream summary step computes at least `field_id,crop_id,count,total_count,proportion` from the count table.
- The demo makes clear that polygonization is not part of the core Yan/Roy path.
- The fixture uses Data Assets worker-local paths and artifact outputs rather than hard-coded controller-local paths.
- The fixture remains small enough for local and container tests.
- `PROJECT_STATE.md` is updated with the implemented state.
- `docs/concepts/README.md` links this concept in the appropriate section.
- The concept README tracker marks completed slices accurately.

## Notes

If this slice reveals that the Data Assets implementation does not yet expose a clean way to run an external worker operation with materialized input paths, stop and append the issue to `docs/concepts/geospatial-worker-plugins/issues.md`.

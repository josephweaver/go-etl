# 008 CDL Yan/Roy Fixture Pipeline

Status: proposed

## Objective

Add a fixture-sized workflow that proves the CDL/Yan/Roy field-crop-year product shape using tiny raster-like inputs and materialized artifact outputs.

This slice proves the data-product orchestration pattern without downloading real CDL or Yan/Roy data and without adding geospatial libraries to GOET core.

## Current State

GOET's target application is a CDL-oriented agricultural workflow. Dependency-aware workflows will provide staged execution. Earlier slices in this concept provide artifact manifests, Python artifact output, data asset declarations, worker asset materialization, and fake-HPCC artifact smoke coverage.

There is not yet a workflow fixture that resembles:

```text
field-id raster tile + CDL crop-code raster tile -> field_id, crop_type, year
```

## Target State

A tiny fixture pipeline exists in the external demo-project/workflow fixture area. It uses small files that represent raster grids, such as CSV, JSON arrays, or another standard-library-readable format.

Example fixture inputs:

```text
field_ids.csv
1,1,1,0
1,1,2,2
3,3,2,2
3,3,3,2

cdl_2023.csv
5,5,5,0
5,1,1,1
2,2,1,1
2,2,2,1
```

The Python fixture script should:

1. read field-id and crop-code grids;
2. ignore background or nodata field IDs;
3. count pixels by `(field_id, crop_code)`;
4. choose majority crop code per field;
5. map crop code to crop type using a small lookup fixture;
6. write a materialized table artifact;
7. return an artifact manifest with QA metadata.

Expected output shape:

```text
field_id,crop_code,crop_type,year,field_pixel_count,crop_pixel_count,crop_fraction
1,5,corn,2023,5,4,0.8
2,1,soybeans,2023,5,4,0.8
3,2,wheat,2023,5,4,0.8
```

The exact fixture values may differ, but tests and smoke validation must be deterministic.

## Concept Decision

Use plain tiny fixture files for this slice. Do not require GDAL, rasterio, numpy, pandas, or pyarrow to prove GOET core behavior.

A later data-product repository or workflow image can introduce geospatial dependencies for real CDL/Yan/Roy rasters. GOET core should not absorb those dependencies.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/data-assets-and-materialized-outputs/fake-hpcc-artifact-smoke.md` if present
- sibling demo project README and Python fixture layout if available
- `cmd/controller/demo-config.json`
- `cmd/worker/demo-config.json`

Do not read unrelated controller internals unless the smoke path fails and the failure points there.

## Allowed Production Files

In the GOET repository:

- `docs/concepts/data-assets-and-materialized-outputs/cdl-yanroy-fixture-pipeline.md`
- `scripts/cdl-yanroy-fixture-smoke.ps1`
- `scripts/cdl-yanroy-fixture-smoke.sh`
- `PROJECT_STATE.md` after successful validation

In the sibling demo project, if available and intended for workflow fixtures:

- `project.json` only for adding fixture source/data declarations if that is the existing convention
- `workflows/cdl-yanroy-fixture.json`
- `submissions/cdl-yanroy-fixture-local.json`
- `submissions/cdl-yanroy-fixture-fake-hpcc.json`
- `scripts/cdl_yanroy_fixture.py`
- `data/fixtures/cdl-yanroy/*`
- small lookup/config files needed by the fixture

## Allowed Test Files

None in GOET core by default. This is a fixture and smoke slice.

Add Go tests only for bugs in artifact/data-asset/dependency behavior found while running the smoke.

## Out Of Scope

- Real CDL downloads.
- Real Yan/Roy tiles.
- Real MSU HPCC configuration.
- GDAL/rasterio/numpy/pandas/pyarrow dependencies.
- Full CONUS processing.
- Crop science beyond majority crop code by field ID.
- Least-common-area boundary construction.
- Accuracy assessment.
- Multi-year merge beyond a tiny deterministic fixture unless dependency-aware workflow fan-out is already ready and the fixture remains tiny.

## Acceptance Criteria

- A tiny fixture dataset exists outside GOET core runtime code.
- The fixture has at least one field-id grid, one CDL crop-code grid, and one crop-code lookup.
- A Python script computes majority crop code by field ID and writes a materialized table artifact.
- The output includes `field_id`, `crop_type`, and `year`.
- The output also includes QA columns such as counts or fractions.
- The script uses `GOET_ARTIFACT_DIR` and reports artifacts through `GOET_OUTPUT_JSON`.
- A local smoke run proves the fixture through source admission, Python execution, artifact promotion, and controller completion.
- If fake HPCC is available, a fake-HPCC smoke run proves the same fixture through Slurm/Singularity.
- The runbook states that real CDL/Yan/Roy data are intentionally out of scope.
- No generated fixture or smoke output is large.

## Notes

- Keep field IDs numeric and crop codes numeric to mirror raster behavior.
- Keep crop-type labels in the lookup fixture so the script does not hard-code USDA classes.
- A later real-data workflow should replace grid readers with raster window readers while keeping the same output schema and artifact manifest shape.

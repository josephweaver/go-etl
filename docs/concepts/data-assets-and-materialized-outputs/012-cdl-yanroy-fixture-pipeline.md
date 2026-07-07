# 012 CDL Yan/Roy Fixture Pipeline

Status: proposed

## Objective

Add a fixture-sized workflow that proves the CDL-by-Yan/Roy field composition product shape using tiny raster-like bound input assets, optional archive-selected inputs, materialized artifact outputs, and publication to a named fixture data location.

This slice proves the data-product orchestration pattern without downloading real CDL, contacting real Google Drive, extracting real Yan/Roy archives, or adding geospatial libraries to GOET core.

## Current State

GOET's target application is a CDL-oriented agricultural workflow. Dependency-aware workflows will provide staged execution. Earlier slices in this concept provide artifact manifests, Python artifact output, data provider/binding models, worker asset materialization/reference, immutable cache behavior, archive extraction, rclone-backed provider acquisition, data-path command binding, published-asset copying, and fake-HPCC artifact/data smoke coverage.

There is not yet a workflow fixture that resembles:

```text
field-id raster tile + CDL crop-code raster tile
  -> field/year/crop composition table
  -> dominant-crop assignment under a declared policy
  -> published tile dataset
```

## Target State

A tiny fixture pipeline exists in the external demo-project/workflow fixture area. It uses small files that represent raster grids, such as CSV, JSON arrays, or another standard-library-readable format.

The fixture should define provider templates equivalent to:

```json
{
  "data_providers": {
    "cdl_fixture_zip": {
      "provider": "local_file",
      "kind": "raster_fixture_archive",
      "format": "zip_csv_grid",
      "location": {
        "name": "fixture_data",
        "path_template": "archives/cdl_${year}_${tile}.zip"
      },
      "parameters": ["year", "tile"],
      "cache": {
        "strategy": "worker_cache",
        "cache_key_template": "fixtures/cdl/${year}/${tile}/source.zip",
        "immutable": true
      },
      "archive": {
        "type": "zip",
        "select": [
          {
            "member_template": "cdl_${year}_${tile}.csv",
            "as": "cdl.csv",
            "required": true
          }
        ],
        "expose": "selected_path"
      }
    },
    "yanroy_fixture": {
      "provider": "registered_location",
      "kind": "field_id_fixture",
      "format": "csv_grid",
      "location": {
        "name": "fixture_data",
        "path_template": "yanroy/${tile}.csv"
      },
      "parameters": ["tile"],
      "materialization": {"strategy": "reference"}
    },
    "crop_lookup": {
      "provider": "registered_location",
      "kind": "lookup_table",
      "format": "csv",
      "location": {
        "name": "fixture_data",
        "path_template": "lookups/crop_codes.csv"
      },
      "materialization": {"strategy": "reference"}
    },
    "crop_assignment_policy": {
      "provider": "registered_location",
      "kind": "policy",
      "format": "json",
      "location": {
        "name": "fixture_data",
        "path_template": "policies/dominant_share_v1.json"
      },
      "materialization": {"strategy": "reference"}
    }
  },
  "published_data_assets": {
    "field_cdl_composition_tile": {
      "kind": "tabular_dataset",
      "format": "csv",
      "location": {
        "name": "published_data",
        "path_template": "field_cdl_composition/year=${year}/tile=${tile}/field_cdl_composition.csv"
      },
      "parameters": ["year", "tile"],
      "overwrite_policy": "fail_if_exists"
    },
    "field_dominant_crop_tile": {
      "kind": "tabular_dataset",
      "format": "csv",
      "location": {
        "name": "published_data",
        "path_template": "field_dominant_crop/year=${year}/tile=${tile}/field_dominant_crop.csv"
      },
      "parameters": ["year", "tile"],
      "overwrite_policy": "fail_if_exists"
    }
  }
}
```

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

A workflow step should bind data aliases and call the Python script with resolved paths:

```json
{
  "data": {
    "cdl_grid": {
      "provider": "cdl_fixture_zip",
      "parameters": {"year": "${vars.year}", "tile": "${vars.tile}"}
    },
    "field_grid": {
      "provider": "yanroy_fixture",
      "parameters": {"tile": "${vars.tile}"}
    },
    "crop_lookup": {
      "provider": "crop_lookup"
    },
    "assignment_policy": {
      "provider": "crop_assignment_policy"
    }
  },
  "args": [
    "scripts/cdl_yanroy_fixture.py",
    "--cdl", "${data.cdl_grid.local_path}",
    "--fields", "${data.field_grid.local_path}",
    "--lookup", "${data.crop_lookup.local_path}",
    "--policy", "${data.assignment_policy.local_path}",
    "--year", "${vars.year}",
    "--tile", "${vars.tile}",
    "--composition-out", "${artifact_dir}/field_cdl_composition.csv",
    "--dominant-out", "${artifact_dir}/field_dominant_crop.csv"
  ],
  "publish": {
    "field_cdl_composition_tile": {
      "from_artifact": "field_cdl_composition_tile",
      "target": "field_cdl_composition_tile",
      "parameters": {"year": "${vars.year}", "tile": "${vars.tile}"}
    },
    "field_dominant_crop_tile": {
      "from_artifact": "field_dominant_crop_tile",
      "target": "field_dominant_crop_tile",
      "parameters": {"year": "${vars.year}", "tile": "${vars.tile}"}
    }
  }
}
```

The Python fixture script should:

1. read field-id and crop-code grids from CLI paths;
2. read crop-code lookup from a CLI path;
3. read crop-assignment policy from a CLI path;
4. ignore background or nodata field IDs;
5. count pixels by `(field_id, crop_code)`;
6. compute `field_pixel_count`, `crop_pixel_count`, and `crop_fraction`;
7. choose dominant crop code per field according to the declared policy;
8. map crop code to crop type using the lookup fixture;
9. write a composition table with one row per `(field_id, year, crop_code)`;
10. write a dominant-crop table with one row per `(field_id, year)`;
11. return artifact declarations through `GOET_OUTPUT_JSON`;
12. let the worker publish selected artifacts to named `published_data` locations.

Expected composition output shape:

```text
field_id,field_tile_id,year,crop_code,crop_type,field_pixel_count,crop_pixel_count,crop_fraction,is_dominant_crop,dominant_crop_code,dominant_crop_type,dominant_crop_fraction,assignment_policy
1,fixture_tile_001,2023,5,corn,5,4,0.8,true,5,corn,0.8,dominant_share_v1
1,fixture_tile_001,2023,1,soybeans,5,1,0.2,false,5,corn,0.8,dominant_share_v1
2,fixture_tile_001,2023,1,soybeans,5,4,0.8,true,1,soybeans,0.8,dominant_share_v1
2,fixture_tile_001,2023,2,wheat,5,1,0.2,false,1,soybeans,0.8,dominant_share_v1
3,fixture_tile_001,2023,2,wheat,5,4,0.8,true,2,wheat,0.8,dominant_share_v1
3,fixture_tile_001,2023,5,corn,5,1,0.2,false,2,wheat,0.8,dominant_share_v1
```

Expected dominant output shape:

```text
field_id,field_tile_id,year,dominant_crop_code,dominant_crop_type,dominant_crop_fraction,field_pixel_count,assignment_status,assignment_policy
1,fixture_tile_001,2023,5,corn,0.8,5,assigned,dominant_share_v1
2,fixture_tile_001,2023,1,soybeans,0.8,5,assigned,dominant_share_v1
3,fixture_tile_001,2023,2,wheat,0.8,5,assigned,dominant_share_v1
```

The exact fixture values may differ, but tests and smoke validation must be deterministic.

## Concept Decision

Use plain tiny fixture files for this slice. Do not require GDAL, rasterio, numpy, pandas, or pyarrow to prove GOET core behavior.

Keep the first product-facing output as field/CDL composition, not only the majority crop type. The dominant crop is important for RCI, but the full distribution is important for QA, mixed-field detection, boundary issues, and later alternative assignment policies.

A later data-product repository or workflow image can introduce geospatial dependencies for real CDL/Yan/Roy rasters. GOET core should not absorb those dependencies.

The fixture should demonstrate the intended long-term workflow style: plugins operate on data assets through ordinary local paths, while the worker handles provider materialization, cache, archive extraction, artifact promotion, and publication.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/data-assets-and-materialized-outputs/fake-hpcc-data-assets-smoke.md` if present
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

- `project.json` only for adding fixture source/data provider/publish declarations if that is the existing convention
- `workflows/cdl-yanroy-fixture.json`
- `submissions/cdl-yanroy-fixture-local.json`
- `submissions/cdl-yanroy-fixture-fake-hpcc.json`
- `scripts/cdl_yanroy_fixture.py`
- `data/fixtures/cdl-yanroy/*`
- `data/published/.gitkeep` or equivalent empty root marker only if needed
- small lookup/config/policy files needed by the fixture

## Allowed Test Files

None in GOET core by default. This is a fixture and smoke slice.

Add Go tests only for bugs in artifact/data-asset/dependency behavior found while running the smoke.

## Out Of Scope

- Real CDL downloads.
- Real Yan/Roy tiles or `ReleaseData.7z` extraction.
- Real Google Drive access or credentials.
- Real MSU HPCC configuration.
- GDAL/rasterio/numpy/pandas/pyarrow dependencies.
- Full CONUS processing.
- RCI calculation beyond preparing the dominant-crop input table.
- Crop science beyond field/crop composition and declared dominant-crop policy.
- Least-common-area boundary construction.
- Accuracy assessment.
- Multi-year RCI merge beyond a tiny deterministic fixture unless dependency-aware workflow fan-out is already ready and the fixture remains tiny.
- Data catalog registration.

## Acceptance Criteria

- A tiny fixture dataset exists outside GOET core runtime code.
- The fixture has at least one field-id grid, one CDL crop-code grid, one crop-code lookup, and one crop-assignment policy.
- At least one fixture input is a ZIP archive with a selected member to exercise archive extraction.
- The fixture project/workflow defines data providers for CDL grid, Yan/Roy field grid, crop lookup, and crop-assignment policy.
- The workflow step binds concrete data aliases by `year` and `tile`.
- A Python script receives input paths through `${data.<alias>.local_path}` arguments.
- The script computes crop-code distribution by field ID and writes a materialized composition table artifact.
- The composition output includes `field_id`, `crop_code`, `crop_type`, `year`, `field_pixel_count`, `crop_pixel_count`, and `crop_fraction`.
- The script computes a declared dominant crop per field/year and writes a materialized dominant-crop table artifact.
- The dominant output includes `field_id`, `dominant_crop_type`, `year`, `dominant_crop_fraction`, `assignment_status`, and `assignment_policy`.
- The script uses `GOET_ARTIFACT_DIR` and reports both artifacts through `GOET_OUTPUT_JSON`.
- The workflow publishes selected artifacts to named fixture `published_data` locations.
- A local smoke run proves the fixture through source admission, data binding, provider materialization, archive extraction, Python execution, artifact promotion, publication, and controller completion.
- If fake HPCC is available, a fake-HPCC smoke run proves the same fixture through Slurm/Singularity.
- The runbook states that real CDL/Yan/Roy data and real Google Drive access are intentionally out of scope.
- No generated fixture, cache, artifact, or published output is large.

## Notes

- Keep field IDs numeric and crop codes numeric to mirror raster behavior.
- Keep crop-type labels in the lookup fixture so the script does not hard-code USDA classes.
- Keep non-ag and nodata behavior in a declared policy file so the assignment logic is explicit.
- A later real-data workflow should replace grid readers with raster window readers while keeping the same input-provider, output-schema, artifact-manifest, and publication shape.

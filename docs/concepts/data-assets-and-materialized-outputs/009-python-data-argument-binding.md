# 009 Python Data Argument Binding

Status: implemented

## Objective

Let Python work items receive materialized data assets through explicit command argument interpolation such as `${data.cropland_year.local_path}` after the worker has materialized or referenced the bound assets.

This slice adds ergonomic plugin invocation. It does not introduce shell expansion, new work item types, published-asset copying, real data downloads, real Google Drive access, or geospatial processing.

## Current State

The worker can pass `GOET_INPUT_JSON` and, after earlier slices, `GOET_DATA_ASSETS_JSON`. A Python script could read that manifest directly, but the desired plugin shape is often simpler:

```text
field_cdl_composition.py --cdl <local-cdl-path> --yanroy <local-yanroy-path>
```

There is not yet a safe mechanism for a workflow step to say:

```text
--cdl ${data.cropland_year.local_path}
```

## Target State

For Python work items, command/argv fields may contain structured tokens resolved by the worker after data materialization:

```text
${data.<binding_name>.local_path}
${data.<binding_name>}              // optional shorthand for local_path if accepted
${artifact_dir}
```

Example:

```json
{
  "args": [
    "field_cdl_composition.py",
    "--cdl", "${data.cropland_year.local_path}",
    "--yanroy", "${data.field_tile.local_path}",
    "--out", "${artifact_dir}/field_cdl_composition.csv"
  ]
}
```

The worker resolves to ordinary argv entries such as:

```text
field_cdl_composition.py
--cdl
/data/goetl/cache/assets/cdl/2023/30m/extracted/cdl.tif
--yanroy
/shared/readonly/yanroy/tiles/fixture_tile_001.tif
--out
/tmp/goetl/attempt-.../artifacts/field_cdl_composition.csv
```

Resolution order:

```text
1. workflow compiler resolves ordinary variables and fan-out values
2. work item carries concrete bound data assets
3. worker materializes or references those assets
4. worker writes GOET_DATA_ASSETS_JSON
5. worker renders data/artifact tokens in argv fields
6. worker launches Python without invoking a shell
```

## Concept Decision

Use structured token interpolation in argv fields, not shell expansion. Keep the grammar intentionally small.

Avoid nested path-style syntax such as `${data/cdl/${tile}}`. Provider templates and bindings should handle parameterization before the worker sees the command.

`GOET_DATA_ASSETS_JSON` remains the complete manifest. Command interpolation is an ergonomic convenience for scripts that want ordinary CLI arguments.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/python-workitem/README.md`
- `internal/model/data_asset.go`
- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/archive_extractor.go` if created
- `cmd/worker/gdrive_rclone_provider.go` if created
- `cmd/worker/work_python.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/source_bundle.go`

Do not read controller scheduler, transport, Slurm, SSH, or container files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/worker/python_arg_binding.go`
- `cmd/worker/work_python.go`
- `cmd/worker/data_asset_materializer.go` only for narrow manifest shape adjustments
- `internal/model/data_asset.go` only if token names need shared constants

## Allowed Test Files

- `cmd/worker/python_arg_binding_test.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/data_asset_materializer_test.go` only for narrow manifest adjustments

## Out Of Scope

- General template engine.
- Nested interpolation.
- Shell interpolation or environment-variable expansion.
- Resolving arbitrary JSON paths.
- Data asset downloads, archive extraction, or rclone behavior beyond earlier slices.
- Published-asset copying.
- Controller workflow compiler changes unless required to carry argv fields already supported by Python WorkItems.
- Real CDL/Yan/Roy data or real Google Drive access.

## Acceptance Criteria

- A command argument containing `${data.cropland_year.local_path}` resolves to the materialized local path for binding `cropland_year`.
- A command argument containing `${artifact_dir}` resolves to the attempt-local artifact staging directory.
- Multiple tokens across multiple argv entries resolve correctly.
- Unknown data binding names fail before Python starts.
- Unsupported data properties fail before Python starts.
- Nested or malformed token syntax fails with a clear error.
- Resolved arguments are passed as argv entries without invoking a shell.
- Existing Python tests without data tokens still pass.
- A Python fixture can receive `--cdl` and `--yanroy` paths from bound data assets and write a tiny field/CDL composition artifact.
- `GOET_DATA_ASSETS_JSON` is still written when data assets exist.
- `go test ./cmd/worker` passes.

## Notes

- Prefer explicit `.local_path` in documentation even if a shorthand is implemented.
- Do not let command interpolation become a second full workflow variable system.
- This slice is what makes plugin code operate on data assets while still seeing normal local files.

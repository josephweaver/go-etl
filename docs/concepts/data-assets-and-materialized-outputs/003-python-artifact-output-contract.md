# 003 Python Artifact Output Contract

Status: implemented

## Objective

Extend Python WorkItem execution so Python scripts can produce materialized artifacts through `GOET_ARTIFACT_DIR` and declare them in `GOET_OUTPUT_JSON`.

This slice wires the artifact promotion helper into the Python runner. It does not add data asset materialization, data-path command interpolation, published-asset copying, controller persistence schema changes, or HPCC smoke automation.

## Current State

The Python runner stages admitted source, writes `GOET_INPUT_JSON`, runs one Python entrypoint, captures stdout/stderr, validates exactly one `GOET_OUTPUT_JSON` document, canonicalizes it, promotes that compact JSON output into the worker data directory, and reports completion evidence.

There is no script-facing artifact directory and no worker validation of file or directory artifacts produced by Python.

## Target State

For `python_script` work items, the worker creates an attempt-local artifact staging directory and sets:

```text
GOET_ARTIFACT_DIR=<attempt-local artifact staging directory>
```

The Python script may write files or directories under that directory and include an `artifacts` array in its output JSON:

```json
{
  "result": "ok",
  "artifacts": [
    {
      "name": "example_output",
      "kind": "tabular_dataset",
      "format": "csv",
      "path": "example.csv"
    }
  ]
}
```

After the Python subprocess succeeds and the output JSON is parsed, the worker:

1. extracts and validates the optional `artifacts` declaration;
2. promotes declared artifacts through the worker artifact helper;
3. wraps the promoted artifact manifest plus non-artifact script output into the logical output returned to the controller;
4. preserves existing `input_sha256` and `output_sha256` evidence semantics for the final compact logical output.

Python outputs without an `artifacts` array should continue to work as before.

## Concept Decision

Use the existing `GOET_OUTPUT_JSON` boundary for the script to declare artifacts. Do not add a second script-written manifest file unless implementation evidence shows that embedding declarations in output JSON is too awkward.

The script declares staging-relative paths only. The worker owns final path selection, hashing, and manifest rewriting.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/python-workitem/README.md`
- `internal/model/artifact_manifest.go`
- `cmd/worker/work_python.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/artifact_promotion.go`
- `cmd/worker/source_bundle.go`

Do not read controller scheduler, transport, Slurm, SSH, or container files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/worker/work_python.go`
- `cmd/worker/artifact_promotion.go`
- `cmd/worker/evidence.go` only if output hash handling needs a narrow helper

## Allowed Test Files

- `cmd/worker/work_python_test.go`
- `cmd/worker/artifact_promotion_test.go`
- `cmd/worker/evidence_test.go` only for output hash behavior

## Out Of Scope

- New Python package management.
- GDAL/rasterio/numpy/pyarrow dependencies.
- Data asset downloads or data provider bindings.
- `${data.<alias>.local_path}` command interpolation.
- Published-asset copying to named locations.
- Controller persistence schema changes.
- Workflow compiler changes.
- Fake HPCC smoke scripts.
- Real CDL or Yan/Roy data access.

## Acceptance Criteria

- The Python runner sets `GOET_ARTIFACT_DIR` to an attempt-local directory.
- A Python test fixture can write one small file under `GOET_ARTIFACT_DIR` and declare it in `GOET_OUTPUT_JSON`.
- The worker promotes that file to the data root and returns a compact artifact manifest as the logical output.
- The promoted artifact descriptor includes final data-root-relative path, byte count, and SHA-256.
- Python output without `artifacts` preserves existing behavior.
- A declared artifact path outside `GOET_ARTIFACT_DIR` fails the work item.
- A missing declared artifact fails the work item.
- Invalid JSON, trailing JSON, non-zero exit, stdout/stderr capture, and existing output promotion tests still pass.
- `go test ./cmd/worker` passes.

## Notes

- Do not make Python scripts aware of worker `DataDir`.
- Do not allow absolute artifact paths in `GOET_OUTPUT_JSON`.
- Keep fixture files small and standard-library-only.
- Python scripts should receive final input data paths from a later data-binding slice; this slice is only about output artifacts.

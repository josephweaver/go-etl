# 004 Python Subprocess Runner, No Environment Creation

Status: proposed

## Objective

Add worker dispatch for `python_script` work items and run a declared Python entrypoint as a subprocess from staged admitted source.

This slice uses configured or system Python. It does not create virtual environments, install packages, or implement cached Python environments.

## Current State

`cmd/worker/worker.go` dispatches work items by `item.Type`. It supports `write_demo_output` and `summarize_input_file`; unsupported types return an error.

`cmd/worker/work_summary.go` has `stringParameter`, which accepts `string` and `path` parameters and returns a non-empty string.

`cmd/worker/work_demo.go` contains existing output/evidence helper code for writing a work result to `DataDir` and building `WorkEvidence`.

After slice 003, the worker should have a source-bundle staging helper that can create attempt-local `source`, `work`, and `logs` directories for a source-backed work item.

The worker does not yet execute Python subprocesses.

## Target State

`cmd/worker/worker.go` dispatches `model.WorkItemTypePythonScript` to a new worker operation.

The Python runner:

- requires a `python_entrypoint` parameter;
- accepts `python_entrypoint` as `string` or `path`;
- validates that the entrypoint path stays inside staged `source/`;
- optionally accepts `python_environment` as `string` or `path`;
- validates that `python_environment`, when present, stays inside staged `source/`;
- optionally accepts `python_args` as a list of strings;
- writes a structured input JSON file to `work/input.json`;
- sets agreed `GOET_*` environment variables;
- runs the Python executable from configuration or defaults to `python3`;
- sets process working directory to staged `source/`;
- captures stdout to `logs/stdout.log`;
- captures stderr to `logs/stderr.log`;
- fails clearly on missing entrypoint, unsafe entrypoint, unsupported parameter type, non-zero process exit, and missing output JSON.

The worker config may gain an optional JSON field such as:

```go
PythonExecutable string `json:"python_executable,omitempty"`
```

If absent, the runner uses `python3`.

## Concept Decision

This slice adds a Python worker operation concept. It should live in a new file such as:

```text
cmd/worker/work_python.go
```

The existing worker dispatcher should remain the only dispatch entry point. Do not introduce a general plugin registry in this slice.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `internal/model/work_item.go`
- `cmd/worker/worker.go`
- `cmd/worker/config.go`
- `cmd/worker/state.go`
- `cmd/worker/work_demo.go`
- `cmd/worker/work_summary.go`
- `cmd/worker/source_bundle.go`

Do not read controller, scheduler, transport, repository-source internals, or client setup files unless compile or test failures directly require it.

## Allowed Production Files

- `cmd/worker/worker.go`
- `cmd/worker/config.go`
- `cmd/worker/work_python.go`
- `cmd/worker/source_bundle.go`
- `cmd/worker/work_summary.go`

## Allowed Test Files

- `cmd/worker/worker_test.go`
- `cmd/worker/config_test.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/source_bundle_test.go`
- `cmd/worker/work_summary_test.go`

## Allowed Documentation Files

- `PROJECT_STATE.md`

## Out Of Scope

- Virtualenv creation.
- Conda environment creation.
- Package installation.
- Python environment schema validation beyond path presence/safety.
- Controller source-bundle API changes.
- Workflow compilation changes.
- Python SDK/client behavior.
- Worker-side skip/reuse for Python scripts.
- Execution observability framework.
- Artifact upload or log streaming.

## Acceptance Criteria

- Worker dispatch supports `python_script`.
- A `python_script` item with missing `python_entrypoint` fails clearly.
- A `python_script` item with an entrypoint outside staged source fails clearly.
- `python_args`, when present, must be a list of strings or fail clearly.
- The runner writes `GOET_INPUT_JSON` before launching the process.
- The runner sets `GOET_WORK_ITEM_ID`, `GOET_ATTEMPT_ID`, `GOET_INPUT_JSON`, `GOET_OUTPUT_JSON`, `GOET_SOURCE_DIR`, `GOET_WORK_DIR`, `GOET_DATA_DIR`, `GOET_TMP_DIR`, `GOET_LOG_DIR`, `GOET_PYTHON_ENTRYPOINT`, and `GOET_PYTHON_ENVIRONMENT_JSON` when applicable.
- The process working directory is staged `source/`.
- Stdout and stderr are captured to attempt log files.
- Non-zero process exit returns a worker error.
- Missing `GOET_OUTPUT_JSON` returns a worker error.
- Existing worker operations still pass tests.
- `go test ./cmd/worker` passes.

## Notes

- Tests that execute Python may use `exec.LookPath("python3")` and skip only subprocess-execution tests when Python is unavailable. Validation and path-safety tests should not be skipped.
- This slice should not decide final environment semantics. It only passes the optional environment file path through to the script.
- Keep subprocess launch behavior boring and explicit. Do not use shell interpolation.
- Use `exec.Command` with argument slices rather than constructing a shell command string.

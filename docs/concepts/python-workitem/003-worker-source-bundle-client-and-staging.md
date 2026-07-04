# 003 Worker Source Bundle Client and Staging

Status: implemented

Implementation note: `cmd/worker/source_bundle.go` now stages source bundles safely for attempt-local execution, with focused tests in `cmd/worker/source_bundle_test.go`.

## Objective

Add worker code that downloads a controller-provided source bundle for a source-backed work item and safely extracts it into an attempt-local staging directory.

This slice creates the worker staging boundary used by later Python execution. It does not run Python.

## Current State

`cmd/worker/config.go` defines worker `Config` with `log_dir`, `tmp_dir`, `data_dir`, and `controller_url`.

`cmd/worker/worker.go` validates that `LogDir`, `TmpDir`, and `DataDir` exist, then dispatches work by `model.WorkItem.Type`.

`cmd/worker/state.go` already uses `ControllerURL` to fetch work from `/work/next` and report completion/failure.

The worker does not currently download source bundles, create attempt-local source/work/log directories, or safely extract zip files for source-backed work.

## Target State

The worker has a helper that accepts a `model.WorkItem` with `Source`, downloads the source bundle from the controller, and stages it under `TmpDir`.

For attempt ID `attempt-abc`, the staging layout is:

```text
<TmpDir>/attempts/attempt-abc/
  source/
  work/
  logs/
```

The source bundle is extracted under `source/` only.

The helper returns a small staging value such as:

```go
type WorkStaging struct {
    AttemptDir string
    SourceDir  string
    WorkDir    string
    LogDir     string
}
```

The worker rejects unsafe zip entries:

```text
absolute paths
.. traversal
backslashes
Windows drive-qualified paths
duplicate entries
symlink-like entries
paths escaping the staging root
```

The download URL is derived from `Config.ControllerURL` and `WorkItem.Source.RunID`. The worker must not use repository cache paths or source provider paths directly.

## Concept Decision

This slice adds a worker-side staging concept because source-bundle download and safe extraction are separate from Python subprocess execution.

Prefer a new file:

```text
cmd/worker/source_bundle.go
```

if the implementation introduces a helper type and independent safety checks. Keep worker dispatch unchanged unless a minimal hook is needed for tests.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `internal/model/work_item.go`
- `cmd/worker/config.go`
- `cmd/worker/worker.go`
- `cmd/worker/state.go`

Do not read controller, repository-source internals, scheduler, transport, or client setup files unless compile or test failures directly require it.

## Allowed Production Files

- `cmd/worker/source_bundle.go`
- `cmd/worker/config.go`
- `cmd/worker/worker.go`

## Allowed Test Files

- `cmd/worker/source_bundle_test.go`
- `cmd/worker/config_test.go`
- `cmd/worker/worker_test.go`

## Allowed Documentation Files

- `PROJECT_STATE.md`

## Out Of Scope

- Controller source-bundle endpoint implementation.
- Python subprocess execution.
- `python_script` dispatch.
- Python input/output JSON contract.
- Work evidence changes.
- Workflow compilation changes.
- Environment creation.
- Worker artifact upload.
- Cleanup or retention policy for staging directories beyond what tests need.

## Acceptance Criteria

- Worker code can compute the source-bundle URL from `Config.ControllerURL` and `WorkItem.Source.RunID`.
- Worker code can download a zip bundle from an HTTP test server.
- Worker code creates attempt-local `source`, `work`, and `logs` directories under `TmpDir`.
- Worker code extracts safe zip entries into the `source` directory.
- Worker code rejects absolute paths.
- Worker code rejects `..` traversal.
- Worker code rejects backslashes.
- Worker code rejects Windows drive-qualified paths.
- Worker code rejects duplicate entries.
- Worker code rejects symlink-like entries.
- Worker code rejects entries that would escape the staging root after path cleaning.
- Existing worker tests still pass.
- `go test ./cmd/worker` passes.

## Notes

- This slice may be implemented before the real controller endpoint by using `httptest.Server` in worker tests.
- Do not let the worker read or interpret controller repository cache paths.
- Use the attempt ID from the work item when present. If no attempt ID exists, follow the worker's existing attempt-ID convention or fail clearly; do not invent a conflicting attempt identity policy.
- Do not delete staging directories automatically in this slice. Later cleanup can be explicit.

# 007 Python Subprocess Log Emission

Status: Complete

## Objective

Emit Python work-item subprocess stdout and stderr as structured log observations through the worker logging path.

This slice connects the completed Python WorkItem execution path to controller-owned execution observability.

## Current State

`cmd/worker/work_python.go` owns Python work-item execution. The completed Python WorkItem phase stages admitted source, writes `work/input.json`, runs a system Python executable, captures stdout/stderr under an attempt-local log directory, promotes `work/output.json`, and returns wrapper evidence with input/output hashes and optional stdout/stderr hashes.

Slices 004 and 006 added worker log delivery and fallback behavior, but the Python runner does not yet emit subprocess stdout/stderr to that path.

As a result, Python output may exist in worker-local attempt files but is not available through controller-owned logs by `submission_id`.

## Target State

When a Python work item runs, completed stdout and stderr lines are emitted as `internal/model.LogObservation` records through the worker logging path.

Expected observation metadata:

```text
component       python or worker-python
stream          stdout or stderr
level           info for stdout, warn or error for stderr
submission_id   from the assigned work/submission metadata when available
workflow_id     when available
step_id/name    when available
work_item_id    from the assigned work item
attempt_id      from the current assignment/reporting path
worker_id       from worker runtime identity when available
timestamp       time the line is observed or emitted
message         one completed stdout/stderr line
sequence        optional local ordering value
```

The implementation should preserve existing Python WorkItem behavior:

- source staging remains unchanged;
- `work/input.json` behavior remains unchanged;
- `GOET_OUTPUT_JSON` decoding remains unchanged;
- output promotion remains unchanged;
- stdout/stderr attempt-local capture remains available if still needed by current wrapper evidence tests;
- logging delivery failure does not fail the Python work item.

Preferred implementation: tee stdout/stderr through line-oriented writers that both preserve existing capture and emit observations. If the current runner structure makes live teeing too invasive for this slice, replaying completed lines from the existing captured stdout/stderr files after subprocess completion is acceptable, provided line boundaries, stream names, and attempt metadata are preserved.

## Concept Decision

This slice updates the existing Python work-item worker execution concept and the worker logging concept. It should not create a new runtime abstraction unless the current code already has an appropriate boundary.

The subprocess line-to-observation conversion may live in a small helper if that keeps `work_python.go` readable.

## Required Context

Read these files first:

- `docs/concepts/execution-observability/README.md`
- `docs/concepts/execution-observability/001-logging-model.md`
- `docs/concepts/execution-observability/004-worker-logging-client.md`
- `docs/concepts/execution-observability/006-worker-fallback-logging.md`
- `docs/concepts/python-workitem/README.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/worker/README.md`
- `cmd/worker/work_python.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/log_client.go`
- `cmd/worker/fallback_log.go`
- `internal/model/work_item.go`
- `internal/model/log_observation.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/worker/work_python.go`
- `cmd/worker/log_client.go`
- `cmd/worker/fallback_log.go`

## Allowed Test Files

- `cmd/worker/work_python_test.go`
- `cmd/worker/log_client_test.go`

## Out Of Scope

- Controller endpoint changes.
- Controller filesystem sink changes.
- Submission-log read endpoint.
- CLI log command.
- Removing existing stdout/stderr evidence behavior.
- Changing Python source admission or source-bundle staging.
- Changing Python output JSON contract.
- Python environment management.
- Attempt Ledger redesign.
- Execution event generalization.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- Python stdout produces `LogObservation` entries with `stream=stdout`.
- Python stderr produces `LogObservation` entries with `stream=stderr`.
- Observations include work-item ID and attempt ID when available.
- Observations include submission ID when the worker assignment provides it.
- Observations preserve line boundaries.
- Observation delivery failure does not fail the Python work item.
- Existing Python WorkItem tests for output promotion and evidence behavior continue to pass.
- Tests cover stdout observation emission.
- Tests cover stderr observation emission.
- Tests cover logging-delivery failure without Python work-item failure.

## Notes

- Do not require strict global ordering across stdout and stderr. Useful local ordering is enough.
- Do not block indefinitely on log delivery. Logging must not become the critical path for subprocess completion.
- If the current worker assignment does not yet expose `submission_id`, include the best available run/workflow/work-item identifiers and add a test note. Do not redesign submission status in this slice.

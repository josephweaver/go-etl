# 006 Worker Fallback Logging

Status: Complete

## Objective

Add worker-side fallback logging for emergency diagnostics when the worker cannot deliver a log observation to the controller.

This slice provides a limited local fallback path without making workers the normal owner of GOET logs.

## Current State

Slice 004 added worker-side delivery of one `LogObservation` to the controller.

If controller log delivery fails, the worker currently has no explicit emergency diagnostic path for the dropped observation. Existing worker local log directories may exist for runtime or attempt files, but they are not a structured fallback observation store.

The controller must remain the authoritative normal log owner. Worker fallback files are only for controller separation, development debugging, and post-failure inspection.

## Target State

When controller log delivery fails, the worker can write a local fallback JSONL entry under its existing configured log directory.

Expected behavior:

- Fallback logging is attempted only after controller log delivery fails.
- Fallback entries use the same structured `LogObservation` payload when possible.
- The fallback path is stable and obvious, such as:

  ```text
  <worker_log_dir>/fallback-observations.jsonl
  ```

- Fallback write failure becomes a warning or returned logging error only.
- Fallback write failure does not fail work execution.
- Healthy controller-connected logging does not create fallback entries.

## Concept Decision

This slice updates the worker logging client concept and adds a small worker fallback file concept. A new fallback helper file is justified if it keeps local file append behavior separate from HTTP delivery.

Reuse the worker's existing configured log directory if available. Do not add a second worker log-root configuration unless the existing configuration cannot safely represent fallback files.

## Required Context

Read these files first:

- `docs/concepts/execution-observability/README.md`
- `docs/concepts/execution-observability/001-logging-model.md`
- `docs/concepts/execution-observability/004-worker-logging-client.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/worker/README.md`
- `cmd/worker/config.go`
- `cmd/worker/config_test.go`
- `cmd/worker/log_client.go`
- `cmd/worker/log_client_test.go`
- `internal/model/log_observation.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/worker/config.go`
- `cmd/worker/log_client.go`
- `cmd/worker/fallback_log.go`

## Allowed Test Files

- `cmd/worker/config_test.go`
- `cmd/worker/log_client_test.go`
- `cmd/worker/fallback_log_test.go`

## Out Of Scope

- Controller endpoint changes.
- Controller filesystem sink changes.
- Python subprocess stdout/stderr emission.
- Submission-log read endpoint.
- CLI log command.
- Reconciliation or upload of fallback logs into controller-owned logs.
- Attempt Ledger changes.
- Execution event generalization.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- Worker fallback logging writes structured JSONL when controller log delivery fails.
- Fallback logging uses the existing worker log directory or a clearly validated configured fallback path.
- Fallback logging is not used when controller delivery succeeds.
- Fallback logging write failure does not fail work execution.
- Tests cover fallback write after controller delivery failure.
- Tests cover no fallback write after successful controller delivery.
- Tests cover fallback write failure as a non-work failure.
- Fallback logs are documented in code/tests as emergency diagnostics, not authoritative GOET logs.

## Notes

- Do not add retries in this slice unless they are already part of the worker HTTP helper pattern. A retry policy belongs to a separate reliability concept.
- Do not make fallback logs visible through `goet logs`; that command must read controller-owned logs only.
- Keep the fallback format structured JSONL so it can be inspected manually without inventing a different format.

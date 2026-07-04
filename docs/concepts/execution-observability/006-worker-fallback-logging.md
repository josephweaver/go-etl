# 006 Worker Fallback Logging

Status: proposed

## Objective

Add worker-side fallback logging for emergency diagnostics when the Worker cannot reach the Controller logging endpoint. This slice provides a limited local fallback path without making workers the normal owner of GOET logs.

## Required Context

Read these files first:

- docs/epics/execution-observability/README.md
- docs/epics/execution-observability/001-logging-model.md
- docs/epics/execution-observability/002-log-configuration.md
- docs/epics/execution-observability/004-worker-logging-client.md
- docs/ARCHITECTURE_OVERVIEW.md
- cmd/worker/config.go
- cmd/worker/config_test.go
- cmd/worker/worker.go
- cmd/worker/worker_test.go
- internal/model/log_observation.go

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

- cmd/worker/config.go
- cmd/worker/worker.go

## Allowed Test Files

- cmd/worker/config_test.go
- cmd/worker/worker_test.go

## Out Of Scope

- Controller logging endpoint changes.
- Controller filesystem sink changes.
- Subprocess stdout/stderr capture.
- Python work item execution.
- Attempt Ledger changes.
- Execution event generalization.
- Reconciliation of fallback logs into Controller logs.

## Acceptance Criteria

- Worker can be configured with a fallback log path or fallback log root.
- When Controller log submission fails, Worker can write a fallback diagnostic log entry locally.
- Fallback logging is used only when Controller logging is unavailable.
- Fallback logging failure produces a warning/error value only and does not fail work execution.
- Fallback log entries render from structured log observations using a simple stable format.
- Tests cover fallback logging when Controller submission fails.
- Tests cover fallback logging write failure without work-item failure.
- Tests verify normal Controller-connected logging does not write fallback logs.

## Notes

- Worker fallback logs are emergency diagnostics, not a second authoritative logging system.
- During healthy execution, GOET-owned logs should stream to the Controller.
- Logging is best-effort. Failed logging must never fail work execution.
- Reconciliation or upload of fallback logs can be considered later but is not part of this slice.

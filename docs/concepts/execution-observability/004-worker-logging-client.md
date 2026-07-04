# 004 Worker Logging Client

Status: proposed

## Objective

Add worker-side client behavior for submitting structured log observations to the Controller logging endpoint. This slice lets workers emit GOET-owned logs upward to the Controller without implementing subprocess stdout/stderr streaming or durable filesystem sinks.

## Required Context

Read these files first:

- docs/concepts/execution-observability/README.md
- docs/concepts/execution-observability/001-logging-model.md
- docs/concepts/execution-observability/002-log-configuration.md
- docs/concepts/execution-observability/003-controller-logging-endpoint.md
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
- Filesystem log sink implementation.
- Log file creation or writes.
- Stdout/stderr subprocess streaming.
- Worker fallback logging implementation.
- Attempt Ledger changes.
- Execution event generalization.

## Acceptance Criteria

- Worker has a small client/path for submitting one structured log observation to the Controller logging endpoint.
- Worker configuration can identify the Controller logging endpoint using the existing controller URL or a derived path.
- Worker logging submission uses the structured log observation model from `internal/model`.
- Successful Controller responses are treated as successful log delivery.
- Failed log delivery returns or records a warning/error value without failing work execution.
- Tests cover successful log submission.
- Tests cover failed log submission and verify it does not become a work-item failure.
- No subprocess stdout/stderr streaming behavior is added.
- No filesystem sink or durable logging behavior is added.

## Notes

- Logging is best-effort. Failed logging must never fail work execution.
- This slice should establish worker-to-controller logging transport only.
- Later slices will connect this client to actual subprocess stdout/stderr streaming and fallback worker logs.
- Keep the client small and testable; avoid broad worker lifecycle refactors.

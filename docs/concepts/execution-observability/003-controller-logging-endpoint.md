# 003 Controller Logging Endpoint

Status: proposed

## Objective

Add a Controller HTTP endpoint that accepts structured log observations. This slice makes the Controller the authoritative ingestion point for GOET-owned logs without implementing filesystem sinks, worker clients, or subprocess streaming.

## Required Context

Read these files first:

- docs/epics/execution-observability/README.md
- docs/epics/execution-observability/001-logging-model.md
- docs/epics/execution-observability/002-log-configuration.md
- docs/ARCHITECTURE_OVERVIEW.md
- cmd/controller/main.go
- cmd/controller/main_test.go
- internal/model/log_observation.go
- internal/model/log_observation_test.go

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

- cmd/controller/main.go

## Allowed Test Files

- cmd/controller/main_test.go

## Out Of Scope

- Worker logging client.
- Filesystem log sink implementation.
- Log file creation or writes.
- Log configuration parsing changes.
- Stdout/stderr subprocess streaming.
- Worker fallback logging.
- Attempt Ledger changes.
- Execution event generalization.

## Acceptance Criteria

- Controller exposes an HTTP endpoint for submitting one structured log observation.
- Endpoint decodes the request body into the log observation model from `internal/model`.
- Endpoint validates the submitted log observation.
- Valid observations return a successful response.
- Invalid JSON returns a client error.
- Invalid log observations return a client error.
- Logging endpoint failures do not affect work-item execution state.
- No filesystem sink or durable log writing behavior is implemented in this slice.

## Notes

- The endpoint should be the Controller-side ingestion boundary for later worker logging clients.
- This slice may store accepted observations only in a minimal test-visible in-memory path if needed for endpoint testing, but it should not design the durable sink.
- Logging remains best-effort. A failed log submission should not imply work failure.
- Later slices will add worker clients, stdout/stderr streaming, and filesystem sinks.

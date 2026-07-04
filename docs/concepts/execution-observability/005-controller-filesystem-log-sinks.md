# 005 Controller Filesystem Log Sinks

Status: proposed

## Objective

Implement controller-managed filesystem log sinks for structured log observations accepted by the Controller. This slice makes logs durable in physically separated files for controller-wide, workflow/run-level, and attempt-level debugging.

## Required Context

Read these files first:

- docs/epics/execution-observability/README.md
- docs/epics/execution-observability/001-logging-model.md
- docs/epics/execution-observability/002-log-configuration.md
- docs/epics/execution-observability/003-controller-logging-endpoint.md
- docs/ARCHITECTURE_OVERVIEW.md
- cmd/controller/config.go
- cmd/controller/config_test.go
- cmd/controller/main.go
- cmd/controller/main_test.go
- internal/model/log_observation.go

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

- cmd/controller/main.go
- cmd/controller/config.go

## Allowed Test Files

- cmd/controller/main_test.go
- cmd/controller/config_test.go

## Out Of Scope

- Worker logging client changes.
- Subprocess stdout/stderr capture.
- Python work item execution.
- Worker fallback logging.
- Attempt Ledger changes.
- Execution event generalization.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- Controller can route accepted structured log observations to filesystem sinks when enabled by configuration.
- Controller creates parent directories for configured filesystem log paths as needed.
- Controller-wide observations can be written under a controller log path such as `logs/controller/<date-time>.log`.
- Workflow/run observations can be written under a workflow/run log path such as `logs/workflows/<workflow-name>/<run-id>/<date-time>.log`.
- Attempt-level observations can be written under an attempt log path such as `logs/workflows/<workflow-name>/<run-id>/steps/<step-name>/<work-item-id>/<attempt-id>.log`.
- Rendered log lines use the configured line format where available.
- Filesystem write failures produce warnings only and do not fail work execution or log ingestion.
- Tests cover successful controller-wide file logging.
- Tests cover successful workflow/run file logging.
- Tests cover successful attempt-level file logging.
- Tests cover filesystem write failure behavior where practical.

## Notes

- This slice should use the structured log observation model from slice 001 and the logging configuration shape from slice 002.
- This slice should connect to the Controller endpoint from slice 003.
- The sink should remain controller-managed. Workers should not write normal GOET-owned logs directly during healthy controller-connected execution.
- Logging is best-effort. Failed logging must never fail work execution.
- Subprocess stdout/stderr capture belongs to later worker runtime or Python work item slices, not this slice.

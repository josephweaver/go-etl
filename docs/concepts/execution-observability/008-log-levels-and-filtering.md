# 008 Log Levels and Filtering

Status: proposed

## Objective

Implement log-level filtering and sink routing for structured log observations. This slice decides which observations should be written to which configured sinks and resolves any required human-readable names needed for filesystem paths.

## Required Context

Read these files first:

- docs/concepts/execution-observability/README.md
- docs/concepts/execution-observability/001-logging-model.md
- docs/concepts/execution-observability/002-log-configuration.md
- docs/concepts/execution-observability/005-controller-filesystem-log-sinks.md
- docs/ARCHITECTURE_OVERVIEW.md
- cmd/controller/config.go
- cmd/controller/config_test.go
- cmd/controller/main.go
- cmd/controller/main_test.go
- internal/model/log_observation.go

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

- cmd/controller/config.go
- cmd/controller/main.go

## Allowed Test Files

- cmd/controller/config_test.go
- cmd/controller/main_test.go

## Out Of Scope

- Worker logging client changes.
- Worker fallback logging changes.
- Subprocess stdout/stderr capture.
- Python work item execution.
- Attempt Ledger schema redesign.
- Execution event generalization.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- Controller applies configured minimum log levels before writing observations to filesystem sinks.
- Controller can route observations to controller-wide, workflow/run, and attempt-level sinks based on observation metadata.
- Controller does not require strict global log ordering.
- Log filtering failure or metadata resolution failure produces warnings only and does not fail work execution.
- Tests cover filtering out observations below the configured level.
- Tests cover keeping observations at or above the configured level.
- Tests cover routing to the expected sink category.
- Tests cover missing metadata behavior for workflow/run or attempt-level sinks.

## Metadata Resolution

A log observation may carry stable identifiers such as `workflow_id`, `run_id`, `step_id`, `work_item_id`, and `attempt_id`, while filesystem paths may require human-readable names such as workflow name and step name.

This slice should define the controller-side resolution rule for path construction:

- Prefer explicit names already present on the log observation when available.
- Resolve missing workflow or step names from controller-owned state or the ledger/database when available.
- Fall back to stable IDs when names cannot be resolved.
- Never fail logging or work execution because a name cannot be resolved.

## Notes

- Filesystem paths are presentation/storage concerns. The structured observation model should continue to use stable IDs for identity.
- Human-readable names are useful for directory layout, but stable IDs should remain available for unambiguous lookup.
- If the current database does not yet store the required mapping, this slice may document or minimally implement the best available fallback without redesigning the ledger.
- Logging is best-effort. Failed filtering, routing, or name resolution must never fail work execution.

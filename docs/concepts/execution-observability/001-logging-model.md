# 001 Logging Model

Status: proposed

## Objective

Define the internal structured log observation model used by GOET components to transfer log messages. This slice establishes the shared in-memory/log-transfer shape that later controller endpoints, worker clients, subprocess streamers, and filesystem sinks will use.

## Required Context

Read these files first:

- docs/concepts/execution-observability/README.md
- docs/ARCHITECTURE_OVERVIEW.md
- internal/model/work_item.go

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

- internal/model/log_observation.go

## Allowed Test Files

- internal/model/log_observation_test.go

## Out Of Scope

- Controller HTTP endpoints.
- Worker HTTP clients.
- Filesystem log sinks.
- Log configuration parsing.
- Stdout/stderr subprocess streaming.
- Worker fallback logging.
- Changes to the Attempt Ledger.
- Execution event generalization.

## Acceptance Criteria

- Defines a structured log observation type in `internal/model`.
- Includes fields needed for routing and formatting, such as workflow ID, run ID, step name, work-item ID, attempt ID, worker ID, component, level, stream, timestamp, and message.
- Defines allowed log levels.
- Defines allowed stream names for stdout/stderr-style output.
- Provides validation for required fields and known enum-like values.
- Tests cover valid observations and invalid observations.
- No controller, worker, sink, or endpoint behavior is added.

## Notes

- Logging is best-effort; this model should not imply that logging failures can fail work execution.
- The model is for structured transfer. Rendered log-line format belongs to later log configuration and sink slices.
- The model should stay small enough to support early filesystem sinks while leaving room for future log routing or UI display.

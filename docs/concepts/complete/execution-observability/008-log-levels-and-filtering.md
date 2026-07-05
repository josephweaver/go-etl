# 008 Log Levels and Filtering

Status: Complete

## Objective

Apply configured minimum log-level filtering and route observations consistently before durable controller filesystem writes.

This slice turns the log level configured in slice 002 into observable sink behavior.

## Current State

The logging model defines levels. Controller configuration resolves a minimum log level. Controller filesystem sinks can write accepted observations to JSONL files.

The controller does not yet consistently decide which accepted observations should be written durably when their level is below the configured threshold.

Routing rules may also be duplicated between endpoint and sink code unless this slice centralizes them.

## Target State

The controller applies a small, explicit filtering and routing rule before writing to durable log sinks.

Expected filtering behavior:

- Observations below the configured minimum level are accepted by the ingestion endpoint but not written to durable filesystem sinks.
- Observations at or above the configured minimum level are eligible for durable writes.
- Invalid configured levels should already be rejected by startup configuration; runtime code should still handle unexpected values defensively.
- Filtering failures become warnings only and do not fail work execution.

Expected routing behavior:

- Observations without `submission_id` route to controller-wide logs.
- Observations with `submission_id` route to submission-level logs.
- Observations with `submission_id` and `attempt_id` route to attempt-level logs according to the sink strategy chosen in slice 005.
- Missing human-readable names never fail logging. Prefer stable IDs; names are presentation metadata.

## Concept Decision

This slice updates the controller filesystem sink concept. If filtering/routing helpers are large enough to reason about independently, a new controller-local helper file is justified.

Do not move filtering into `internal/model`; model-level helpers may compare levels, but controller configuration decides what gets written.

## Required Context

Read these files first:

- `docs/concepts/complete/execution-observability/README.md`
- `docs/concepts/complete/execution-observability/001-logging-model.md`
- `docs/concepts/complete/execution-observability/002-log-configuration.md`
- `docs/concepts/complete/execution-observability/005-controller-filesystem-log-sinks.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/controller/config.go`
- `cmd/controller/config_test.go`
- `cmd/controller/log_sink.go`
- `cmd/controller/log_sink_test.go`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/model/log_observation.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/config.go`
- `cmd/controller/log_sink.go`
- `cmd/controller/log_filter.go`
- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/config_test.go`
- `cmd/controller/log_sink_test.go`
- `cmd/controller/log_filter_test.go`
- `cmd/controller/main_test.go`

## Out Of Scope

- Worker logging client changes.
- Worker fallback logging changes.
- Python subprocess emission changes.
- Submission-log read endpoint.
- CLI log command.
- Attempt Ledger schema redesign.
- Durable event store.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- Controller filters out observations below the configured minimum level before durable writes.
- Controller writes observations at the configured minimum level.
- Controller writes observations above the configured minimum level.
- Tests cover `debug` filtered by `info` minimum.
- Tests cover `info` retained by `info` minimum.
- Tests cover `warn` and `error` retained by lower minimums.
- Tests cover routing for controller-wide observations.
- Tests cover routing for submission observations.
- Tests cover routing for attempt observations.
- Missing workflow or step names do not fail filtering or routing.
- The ingestion endpoint still treats logging as best-effort.

## Notes

- The controller may accept observations below the durable threshold so workers do not need to know the controller's sink policy.
- Do not introduce per-component or per-stream filtering in this slice unless it is already trivial from existing config. Minimum level is enough.
- Strict global ordering is not required.

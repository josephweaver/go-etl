# 003 Controller Logging Endpoint

Status: Complete

## Objective

Add a controller HTTP endpoint that accepts one structured log observation.

This slice makes the controller the authoritative ingestion boundary for GOET-owned logs without implementing durable filesystem sinks, worker clients, fallback logs, submission-log reads, or Python subprocess emission.

## Current State

`cmd/controller/main.go` owns the controller HTTP API surface. Current endpoints include workflow submission, work assignment, work completion/failure, source-bundle delivery, aggregate or submission status from the previous Strategic Concept, and shutdown.

There is no endpoint that accepts a structured log observation from a worker or controller-adjacent component.

Slice 001 introduced `internal/model.LogObservation` and validation. Slice 002 introduced controller logging configuration and defaults. Neither slice created HTTP behavior.

## Target State

The controller exposes:

```text
POST /observations/logs
```

The endpoint behavior is:

- Accepts `Content-Type: application/json` request bodies.
- Decodes one `internal/model.LogObservation`.
- Rejects invalid JSON with a client error.
- Rejects structurally invalid observations with a client error.
- Accepts valid observations with a success response.
- Enforces the controller's existing maximum request-size protections where applicable.
- Does not mutate work-item, attempt, submission, or queue state.
- Does not write durable logs in this slice.

For testability, the controller may store accepted observations in a small in-memory test sink or call a narrow in-memory handler function. That in-memory behavior is not the durable logging design.

## Concept Decision

This slice updates the existing controller HTTP API concept. The endpoint may be registered from `cmd/controller/main.go`, but request decoding/validation can live in a small new controller file if that keeps `main.go` from growing further.

If this slice introduces a controller-local log receiver interface, keep it narrow:

```go
type logObservationReceiver interface {
    AcceptLogObservation(context.Context, model.LogObservation) error
}
```

A receiver error must not be interpreted as a work-item failure.

## Required Context

Read these files first:

- `docs/concepts/execution-observability/README.md`
- `docs/concepts/execution-observability/001-logging-model.md`
- `docs/concepts/execution-observability/002-log-configuration.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/model/log_observation.go`
- `internal/model/log_observation_test.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/log_observation_endpoint.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/log_observation_endpoint_test.go`

## Out Of Scope

- Durable filesystem log sinks.
- Log-level filtering.
- Submission-log read endpoint.
- Worker logging client.
- Worker fallback logging.
- Python subprocess stdout/stderr emission.
- CLI log command.
- Attempt Ledger changes.
- Execution event generalization.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- `POST /observations/logs` is registered on the controller HTTP server.
- A valid `LogObservation` request returns a success response.
- Invalid JSON returns a client error.
- A structurally invalid `LogObservation` returns a client error.
- The endpoint does not require an active work assignment to accept a valid observation.
- The endpoint does not change work-item completion/failure state.
- Tests verify the endpoint decodes and validates `internal/model.LogObservation`.
- Tests verify invalid payload behavior.
- No filesystem log writing is added.

## Notes

- Use ordinary bounded HTTP request/response handling. Do not implement a long-lived stream.
- The endpoint may return `204 No Content` or another existing success convention if the repository already standardizes one. Keep the response body small.
- Best-effort logging means future sink errors must not fail work execution. This endpoint should avoid introducing any coupling between log acceptance and work state.

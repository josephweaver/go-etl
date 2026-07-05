# 004 Worker Logging Client

Status: Complete

## Objective

Add worker-side client behavior for submitting one structured log observation to the controller logging endpoint.

This slice lets workers emit GOET-owned observations upward to the controller without implementing subprocess stdout/stderr emission, durable controller sinks, CLI log reads, or fallback logging.

## Current State

`cmd/worker` owns worker configuration, controller communication, work retrieval, completion/failure reporting, and work dispatch.

`cmd/worker/state.go` owns HTTP communication with the controller for work assignment and completion/failure reporting. There is no worker-side path for sending `internal/model.LogObservation` to the controller.

Slice 003 added the controller ingestion endpoint:

```text
POST /observations/logs
```

## Target State

The worker package has a small testable client path that submits one `internal/model.LogObservation` to the controller.

Expected behavior:

- Derive the endpoint from the existing worker controller URL configuration.
- Send JSON to:

  ```text
  POST /observations/logs
  ```

- Treat 2xx responses as successful log delivery.
- Treat non-2xx responses, transport errors, and encoding errors as log-delivery errors.
- Return or record delivery errors in a way that the caller can downgrade to a warning.
- Do not allow log-delivery failure to fail a work item.

This slice may introduce a small `LogClient` or helper function in `cmd/worker/log_client.go`.

## Concept Decision

This slice adds a worker logging transport concept. A new file is justified because the behavior is independent from work assignment and completion/failure reporting.

Keep the client inside `cmd/worker` for now. Do not promote it into a reusable public package.

## Required Context

Read these files first:

- `docs/concepts/complete/execution-observability/README.md`
- `docs/concepts/complete/execution-observability/001-logging-model.md`
- `docs/concepts/complete/execution-observability/003-controller-logging-endpoint.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/worker/README.md`
- `cmd/worker/config.go`
- `cmd/worker/config_test.go`
- `cmd/worker/state.go`
- `cmd/worker/state_test.go`
- `cmd/worker/worker.go`
- `cmd/worker/worker_test.go`
- `internal/model/log_observation.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/worker/log_client.go`
- `cmd/worker/state.go`

## Allowed Test Files

- `cmd/worker/log_client_test.go`
- `cmd/worker/state_test.go`

## Out Of Scope

- Controller endpoint changes.
- Controller filesystem log sinks.
- Worker fallback logging.
- Python subprocess stdout/stderr emission.
- Worker lifecycle refactor.
- Attempt Ledger changes.
- Execution event generalization.
- CLI log command.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- Worker code can submit one valid `internal/model.LogObservation` to the controller logging endpoint.
- The logging endpoint URL is derived from the existing controller URL.
- Successful controller responses are treated as successful delivery.
- Non-2xx responses are treated as delivery errors.
- Transport errors are treated as delivery errors.
- Delivery errors do not become work-item execution failures.
- Tests cover successful log delivery against an HTTP test server.
- Tests cover failed log delivery and verify the error remains a logging error, not a work-item failure.
- No subprocess stdout/stderr behavior is added.
- No fallback file logging is added.

## Notes

- Keep this client small. It should not become a general worker event bus.
- Avoid adding background goroutines in this slice.
- Later batching may be introduced, but this slice sends one observation per request.
- Use the shared model validation before sending if doing so keeps errors clearer.

# 001 Logging Model

Status: Ready

## Objective

Define the shared structured log-observation model used by GOET components to transfer execution log lines.

This slice creates the transport type that later controller endpoints, worker clients, subprocess emitters, filesystem sinks, and submission-log readers will use.

## Current State

`internal/model` currently owns shared controller/worker/client transport shapes such as work items, completion/failure reports, and controller status payloads.

There is no shared `LogObservation` type. Existing execution details are represented through unrelated mechanisms:

- work-item assignment payloads;
- completion/failure reports;
- attempt ledger/evidence records;
- Python WorkItem stdout/stderr files under worker attempt-local directories.

Those mechanisms do not provide a role-neutral, structured log line that can move from a worker or subprocess to the controller.

## Target State

`internal/model/log_observation.go` defines a small shared transport model for one completed log line.

The model should be role-neutral and safe to use across controller, worker, internal client, and tests. It should include enough metadata for routing, filtering, and display without making `internal/model` depend on controller or worker internals.

The intended fields are:

```text
observation_id      optional stable/dedup/debug identifier
submission_id       public submission handle when known
workflow_id         workflow identifier when known
workflow_name       human-readable workflow name when known
run_id              workflow run identifier when distinct from submission_id
step_id             stable step identifier when known
step_name           human-readable step name when known
work_item_id        concrete work-item identifier when known
attempt_id          concrete attempt identifier when known
worker_id           worker identifier when known
component           source component, such as controller, worker, runtime, python
stream              stdout, stderr, system, or empty when not stream-specific
level               debug, info, warn, or error
timestamp           RFC3339/RFC3339Nano-compatible timestamp
sequence            optional producer-local monotonic ordering value
message             exactly one completed log message line
```

The model should expose validation that checks at least:

- `level` is one of the allowed levels.
- `stream`, when non-empty, is one of the allowed streams.
- `component` is present.
- `timestamp` is non-zero.
- `message` is present.
- ID-like fields that are later used as path components reject obvious path traversal input when validation can do so generically.

This slice should also define small helper behavior for comparing log levels if that is straightforward and does not pull in controller configuration.

## Concept Decision

This slice adds a new shared transport concept. A new file is justified because log observations have their own struct, constants, validation rules, and tests independent of work-item validation.

Keep the type in `internal/model`; do not create a controller-only or worker-only model for the same payload.

## Required Context

Read these files first:

- `docs/concepts/execution-observability/README.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `internal/model/README.md`
- `internal/model/work_item.go`
- `internal/model/work_item_test.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `internal/model/log_observation.go`

## Allowed Test Files

- `internal/model/log_observation_test.go`

## Out Of Scope

- Controller HTTP endpoints.
- Worker HTTP clients.
- Filesystem log sinks.
- Log configuration parsing.
- Submission-log read APIs.
- CLI log commands.
- Python subprocess stdout/stderr emission.
- Worker fallback logging.
- Attempt Ledger changes.
- Execution event generalization.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- `internal/model` defines a structured `LogObservation` type.
- `internal/model` defines allowed log levels.
- `internal/model` defines allowed stream names.
- `LogObservation` validation accepts representative valid controller, worker, stdout, and stderr observations.
- Validation rejects missing component.
- Validation rejects missing or zero timestamp.
- Validation rejects missing message.
- Validation rejects unknown log levels.
- Validation rejects unknown non-empty streams.
- Tests cover optional routing metadata such as `submission_id`, `work_item_id`, and `attempt_id`.
- No controller, worker, sink, endpoint, CLI, or subprocess behavior is added.

## Notes

- A log observation is a transport record, not a durable attempt outcome.
- Do not make validation require `submission_id` for every observation. Controller-wide logs may not belong to a submission.
- Do not introduce a global logger.
- Keep the model small enough for early JSONL filesystem sinks while leaving room for future events or UI display.
- Prefer simple string constants over a large enum framework.

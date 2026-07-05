# 005 Controller Filesystem Log Sinks

Status: Complete

## Objective

Implement controller-managed JSON Lines filesystem sinks for structured log observations accepted by the controller.

This slice makes accepted observations durable without adding the submission-log read API, CLI log command, Python subprocess emission, or worker fallback logging.

## Current State

Slice 003 added a controller ingestion endpoint for valid `internal/model.LogObservation` payloads. At that point the endpoint can validate and accept observations, but accepted observations are not yet durable.

Slice 002 added or normalized controller logging configuration, including filesystem logging enabled/disabled state and a controller-owned log root path.

There is no controller-owned filesystem sink for observations.

## Target State

The controller has a durable filesystem sink for accepted observations when filesystem logging is enabled.

The durable representation is JSON Lines:

- Each line is one validated `LogObservation` encoded as JSON.
- The sink preserves structured fields for later APIs and SDKs.
- Human-readable rendering is not the durable storage format.

The initial layout should be equivalent to:

```text
<log_root>/
  controller/
    controller.jsonl
  submissions/
    <submission_id>/
      submission.jsonl
      attempts/
        <attempt_id>.jsonl
```

Routing rules for this slice:

- Observations without `submission_id` write to the controller-wide file.
- Observations with `submission_id` write to that submission's `submission.jsonl` file.
- Observations with both `submission_id` and `attempt_id` also write to that attempt's JSONL file, or write only to the attempt file if the implementation documents that attempt files are included by the read path. Prefer avoiding duplicate writes unless later read behavior needs them.
- Submitted IDs must be converted into safe path segments. Raw submitted values must never be concatenated into paths without validation/sanitization.

The sink should create parent directories as needed and serialize concurrent writes enough to avoid corrupting JSONL files.

## Concept Decision

This slice adds a controller-owned filesystem sink concept. A new file is justified because durable log sink behavior has its own responsibilities: path selection, safe segment handling, JSONL append, directory creation, and concurrent write protection.

Keep the sink in `cmd/controller` for now. Do not add a new persistence layer or database schema.

## Required Context

Read these files first:

- `docs/concepts/complete/execution-observability/README.md`
- `docs/concepts/complete/execution-observability/001-logging-model.md`
- `docs/concepts/complete/execution-observability/002-log-configuration.md`
- `docs/concepts/complete/execution-observability/003-controller-logging-endpoint.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/controller/config.go`
- `cmd/controller/config_test.go`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/model/log_observation.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/log_observation_endpoint.go`
- `cmd/controller/log_sink.go`
- `cmd/controller/config.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/log_observation_endpoint_test.go`
- `cmd/controller/log_sink_test.go`
- `cmd/controller/config_test.go`

## Out Of Scope

- Worker logging client changes.
- Worker fallback logging.
- Python subprocess stdout/stderr emission.
- Submission-log read endpoint.
- CLI log command.
- Log retention, cleanup, rotation, compression, or archival.
- Durable database log storage.
- Attempt Ledger schema changes.
- Execution event generalization.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- When filesystem logging is enabled, accepted controller log observations are written to controller-owned JSONL files.
- The sink creates parent directories as needed.
- Controller-wide observations can be written under the controller log path.
- Submission observations can be written under the submission log path.
- Attempt observations can be written under an attempt-level log path when `attempt_id` is available.
- The sink rejects or safely encodes path-unsafe identifiers.
- Filesystem write failures produce warnings or sink errors only and do not fail work execution.
- The ingestion endpoint does not return a work-item failure because a sink write failed.
- Tests cover controller-wide file logging.
- Tests cover submission-level file logging.
- Tests cover attempt-level file logging.
- Tests cover path safety for submitted IDs.
- Tests cover concurrent writes or at least prove writes are serialized through the sink abstraction.
- No log-read API or CLI behavior is added.

## Notes

- JSONL is the durable format. Do not write only human-rendered strings that a later API would need to parse.
- Keep filesystem paths internal to the controller. Public APIs should use `submission_id`, not path names.
- If writing both submission-level and attempt-level files creates duplicate storage complexity, prefer a single authoritative JSONL path and document how later read behavior will aggregate it.
- This slice should not require strict global ordering across workers.

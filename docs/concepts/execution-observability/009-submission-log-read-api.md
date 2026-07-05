# 009 Submission Log Read API

Status: Implemented

## Objective

Add a controller-owned read API for retrieving bounded structured logs for one submission.

This slice exposes execution logs by the public `submission_id` without requiring clients to know controller filesystem paths or worker-local directories.

## Current State

The controller can ingest structured observations and write them to controller-owned JSONL filesystem sinks.

The completed Submission CLI Status concept introduced submission IDs and submission status. A user can ask for status of one submission but cannot yet ask the controller for logs of that submission.

There is no public log-read endpoint.

## Target State

The controller exposes:

```text
GET /submissions/{submission_id}/logs
```

The endpoint returns a bounded JSON response containing structured log entries for the requested submission.

Supported query parameters for this slice:

```text
tail=<positive integer>
level=<debug|info|warn|error>       optional minimum level for returned entries
stream=<stdout|stderr|system>       optional stream filter
attempt_id=<attempt_id>             optional attempt filter
```

Behavior:

- If `tail` is omitted, use `controller_log_read_default_tail_lines`, whose published default is `100` from slice 002.
- If `tail` exceeds `controller_log_read_max_tail_lines`, whose published default is `1000` from slice 002, return a client error. Do not silently clamp in this slice.
- Unknown `submission_id` returns not found if the controller can determine it from submission/status state.
- Known submissions with no logs return an empty `entries` list.
- The endpoint reads controller-owned JSONL logs, not worker fallback logs and not worker attempt-local files.
- The endpoint does not expose filesystem paths.
- The endpoint returns JSON only in this slice. Human-readable formatting belongs to the CLI slice.

Response shape:

```json
{
  "submission_id": "sub_1234",
  "entries": [],
  "tail": 100,
  "truncated": false
}
```

Entries should reuse the structured `LogObservation` shape or a documented response projection that preserves timestamp, level, component, stream, IDs, and message.

## Concept Decision

This slice updates the controller HTTP API concept and adds a submission-log read concept. A new controller-local read helper file is justified because reading bounded JSONL logs has separate responsibilities from ingestion and sinking.

If response structs are shared with `internal/client`, put them in `internal/model` rather than duplicating shapes.

## Required Context

Read these files first:

- `docs/concepts/execution-observability/README.md`
- `docs/concepts/execution-observability/001-logging-model.md`
- `docs/concepts/execution-observability/002-log-configuration.md`
- `docs/concepts/execution-observability/005-controller-filesystem-log-sinks.md`
- `docs/concepts/execution-observability/008-log-levels-and-filtering.md`
- `docs/concepts/submission-cli-status/README.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `cmd/controller/log_sink.go`
- `cmd/controller/log_sink_test.go`
- `internal/model/log_observation.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/log_read.go`
- `cmd/controller/log_sink.go`
- `internal/model/log_observation.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/log_read_test.go`
- `cmd/controller/log_sink_test.go`
- `internal/model/log_observation_test.go`

## Out Of Scope

- CLI log command.
- Internal client log-fetch helper.
- Human-readable log rendering.
- Long-lived streaming, server-sent events, WebSockets, `--follow`, or `--watch` behavior.
- Worker fallback log reconciliation.
- Reading worker attempt-local stdout/stderr files.
- Log retention, cleanup, rotation, compression, or archival.
- Authentication or authorization.
- Metrics, tracing, or monitoring UI.

## Acceptance Criteria

- `GET /submissions/{submission_id}/logs` is registered on the controller HTTP server.
- The endpoint returns JSON for a known submission.
- The JSON response includes `submission_id`, `entries`, `tail`, and `truncated` or equivalent bounded-read metadata.
- The endpoint reads controller-owned structured JSONL logs.
- The endpoint does not expose filesystem paths.
- The endpoint applies the default tail bound of `100` when `tail` is omitted, unless configuration overrides it.
- The endpoint validates invalid `tail` values.
- The endpoint rejects `tail` values greater than the configured maximum, defaulting to `1000`.
- The endpoint can filter by level when requested.
- The endpoint can filter by stream when requested.
- The endpoint can filter by attempt ID when requested.
- Known submissions with no logs return an empty entries list.
- Unknown submissions return a clear not-found response when the controller can determine unknown submission state.
- Tests cover bounded read behavior.
- Tests cover filtering behavior.
- Tests cover malformed query behavior.

## Notes

- This endpoint is the substrate for `goet logs` and future SDK log APIs.
- Do not make the CLI scrape filesystem paths.
- Prefer a simple implementation that reads bounded JSONL files over introducing a database or index.
- The slice 002 defaults are intentional public API defaults: omitted `tail` means `100`, and tail values above `1000` fail unless configuration explicitly raises the maximum.
- If the current submission/status implementation does not expose an easy known-submission check, document the limitation and still avoid filesystem path exposure.

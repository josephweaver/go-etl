# 010 CLI Logs Command

Status: Complete

## Objective

Add `goet logs <submission_id>` so users can retrieve controller-owned logs for one submitted workflow.

This slice connects the submission-log read API to the CLI and internal client boundary created by the completed Submission CLI Status Strategic Concept.

## Current State

The completed Submission CLI Status concept provides `goet submit`, `goet status <submission_id>`, `goet submit --wait`, and `--json` output behavior.

Slice 009 added the controller read API:

```text
GET /submissions/{submission_id}/logs
```

There is no CLI command that calls the log-read endpoint. `cmd/demo-client` and `internal/client` do not yet fetch or render submission logs.

## Target State

`cmd/demo-client` recognizes:

```text
goet logs <submission_id> [--controller-url <url>] [--tail <n>] [--json]
```

Optional filters may also be accepted if they are already implemented by slice 009 and cheap to thread through:

```text
--level <debug|info|warn|error>
--stream <stdout|stderr|system>
--attempt-id <attempt_id>
```

Validation rules:

- `submission_id` is required.
- Extra positional arguments are rejected.
- `--controller-url` defaults to `http://localhost:8080` when omitted.
- `--tail` must be a positive integer when supplied.
- If `--tail` is omitted, the CLI should omit the query parameter and let the controller apply its configured default, which is `100` unless overridden.
- `--json` emits valid JSON to standard output.
- Human-readable default output is compact and stable.
- Diagnostics and errors go to standard error.
- `--watch` is not accepted.
- `--follow` is not accepted.

Expected human-readable output should be line-oriented and include enough metadata to debug one submission, for example:

```text
2026-07-05T11:00:00Z info worker stdout attempt=att_9012 hello from python
```

The exact format may differ, but it should include timestamp, level, component, stream when present, attempt ID when present, and message.

## Concept Decision

This slice updates the existing CLI/client concept from the completed Submission CLI Status work. Reuse existing parser, controller URL, JSON output, and stderr/stdout separation patterns.

Reusable HTTP behavior belongs in `internal/client`; command parsing and presentation belong in `cmd/demo-client`.

## Required Context

Read these files first:

- `docs/concepts/execution-observability/README.md`
- `docs/concepts/execution-observability/001-logging-model.md`
- `docs/concepts/execution-observability/009-submission-log-read-api.md`
- `docs/concepts/submission-cli-status/README.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/demo-client/README.md`
- `cmd/demo-client/main.go`
- `cmd/demo-client/main_test.go`
- `internal/client/README.md`
- `internal/client/controller_client.go`
- `internal/client/controller_client_test.go`
- `internal/model/log_observation.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/demo-client/main.go`
- `internal/client/controller_client.go`

## Allowed Test Files

- `cmd/demo-client/main_test.go`
- `internal/client/controller_client_test.go`

## Out Of Scope

- Controller log-read endpoint changes.
- Controller filesystem sink changes.
- Worker logging client changes.
- Python subprocess logging changes.
- Built-in `--watch`.
- Built-in `--follow` or long-lived tailing.
- Artifact browsing or attempt detail commands.
- Python SDK or R SDK.
- Authentication or authorization.

## Acceptance Criteria

- `cmd/demo-client` recognizes `logs` as a top-level command.
- `logs` requires exactly one `submission_id` positional argument.
- `logs` accepts `--controller-url` and defaults to `http://localhost:8080` when omitted.
- `logs` accepts positive `--tail` values.
- `logs` rejects invalid `--tail` values.
- `logs` accepts `--json`.
- `logs` rejects `--watch`.
- `logs` rejects `--follow`.
- `internal/client` can call `GET /submissions/{submission_id}/logs`.
- Human-readable output includes timestamp, level, component, stream when present, attempt ID when present, and message.
- `--json` output is valid JSON and contains no human-readable extra text on standard output.
- CLI diagnostics and errors go to standard error.
- Tests cover parser validation.
- Tests cover client request path and query encoding.
- Tests cover human-readable rendering.
- Tests cover JSON output separation.

## Notes

- Keep `goet logs` as a bounded read. Users who want repeated display can compose with OS tools such as `watch` where available.
- Do not let the CLI infer logs from worker directories or controller filesystem paths.
- Preserve the existing CLI/status JSON conventions from the previous Strategic Concept.

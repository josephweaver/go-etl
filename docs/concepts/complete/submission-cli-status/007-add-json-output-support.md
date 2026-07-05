# 007 Add JSON Output Support

Status: Complete

## Objective

Add machine-readable JSON output to the GOET CLI.

Human-readable output remains the default. `--json` becomes the stable automation interface for shell scripts, CI jobs, and future Python/R wrappers.

## Current State

Before this slice:

- `goet submit` can submit a workflow and print a human-readable acknowledgement.
- `goet status <submission_id>` can print a human-readable status summary.
- `goet submit ... --wait` can print final human-readable status after waiting.
- The parser accepts `--json`, but the CLI does not yet guarantee valid JSON-only standard output.
- The shared submission acknowledgement and submission status JSON shapes exist in `internal/model`.

## Target State

Support:

```text
goet submit ... --json
```

and:

```text
goet status <submission_id> --json
```

When `--json` is specified:

- Standard output contains only valid JSON.
- Human-readable text is not mixed into standard output.
- Diagnostic messages and errors are written to standard error.
- Process exit codes retain the same success/failure meaning as human-readable mode.

### Submit JSON output

Successful submission should produce JSON matching the shared acknowledgement model:

```json
{
  "submission_id": "sub_1234",
  "workflow_id": "annual-report",
  "initial_work_item_count": 47
}
```

### Status JSON output

Successful status queries should produce JSON matching the shared status model:

```json
{
  "submission_id": "sub_1234",
  "workflow_id": "annual-report",
  "status": "running",
  "known_work_items": 47,
  "queued": 20,
  "running": 4,
  "completed": 23,
  "failed": 0,
  "skipped": 0
}
```

### Wait JSON output

For `goet submit ... --wait --json`, output should be one valid JSON document representing the final observed status, or a wrapper object that includes both the acknowledgement and final status. Choose the smaller implementation that is clear and tested.

Recommended shape:

```json
{
  "submission": {
    "submission_id": "sub_1234",
    "workflow_id": "annual-report",
    "initial_work_item_count": 47
  },
  "final_status": {
    "submission_id": "sub_1234",
    "workflow_id": "annual-report",
    "status": "completed",
    "known_work_items": 47,
    "queued": 0,
    "running": 0,
    "completed": 47,
    "failed": 0,
    "skipped": 0
  }
}
```

If the implementation chooses a different shape, it must be documented in tests and docs.

## Concept Decision

This slice updates the CLI presentation concept. Keep JSON serialization close to the CLI output boundary, using shared `internal/model` response structs rather than building ad hoc maps when a typed model already exists.

Do not change controller API JSON solely for display preference. The CLI should mirror controller/client models whenever practical.

## Required Context

Read these files first:

- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/submission-cli-status/001-cli-client-contract.md`
- `docs/concepts/submission-cli-status/003-return-submission-ack.md`
- `docs/concepts/submission-cli-status/004-add-submission-status-api`
- `docs/concepts/submission-cli-status/005-add-cli-status-command.md`
- `docs/concepts/submission-cli-status/006-add-wait-support.md`
- `docs/CUSTOMER_API.md`
- `cmd/demo-client/README.md`
- `cmd/demo-client/main.go`
- `cmd/demo-client/main_test.go`
- `internal/client/controller_client.go`
- `internal/client/controller_client_test.go`
- `internal/model/submission.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/demo-client/main.go`
- `internal/client/controller_client.go`
- `internal/model/submission.go`

## Allowed Test Files

- `cmd/demo-client/main_test.go`
- `internal/client/controller_client_test.go`
- `internal/model/submission_test.go`

## Out Of Scope

- Changing controller APIs.
- Implementing new status fields.
- Implementing hierarchical workflow or step output.
- Pretty terminal formatting.
- Streaming status updates.
- Implementing or accepting `--watch`.
- Python or R SDKs.
- Authentication or authorization.
- Artifact output.
- Attempt output.
- Retry behavior.
- Versioned public JSON schemas.

## Acceptance Criteria

- `goet submit --json` produces valid JSON.
- `goet status <submission_id> --json` produces valid JSON.
- `goet submit ... --wait --json` produces valid JSON for the final wait result.
- Human-readable output remains the default when `--json` is not supplied.
- Errors and diagnostics are written to standard error.
- JSON output contains no additional human-readable text on standard output.
- Unit tests verify JSON serialization for submit, status, and wait output.
- Unit tests verify that errors do not pollute JSON standard output.
- The output format is suitable for shell pipelines and future SDK wrappers.

## Notes

- Human-readable output is the primary interactive interface.
- JSON output is the primary automation interface.
- The CLI should never mix progress text with JSON on standard output.
- If wait mode eventually needs progress events, that should be a different output mode, not silent mutation of `--json` semantics.


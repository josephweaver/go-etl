# 007 Add JSON Output Support

Status: Proposed

## Objective

Add machine-readable JSON output to the GOET CLI.

The CLI should continue to produce human-readable output by default while supporting `--json` for automation, scripting, and future SDK integration.

This slice establishes JSON as the stable machine interface for CLI output.

## Required Context

Read these files first:

* docs/concepts/submission-cli-status/README.md
* docs/concepts/submission-cli-status/001-upgrade-demo-client-cli-arguments.md
* docs/concepts/submission-cli-status/003-return-submission-acknowledgement.md
* docs/concepts/submission-cli-status/004-add-submission-status-api.md
* docs/concepts/submission-cli-status/005-add-cli-status-command.md
* docs/CUSTOMER_API.md
* cmd/demo-client/main.go
* internal/client/local_controller.go

Do not read unrelated files unless test failures require them.

## Allowed Production Files

* cmd/demo-client/main.go
* internal/client/local_controller.go

## Allowed Test Files

* cmd/demo-client/main_test.go
* internal/client/local_controller_test.go

## Required Behavior

Support:

```text
goet submit ... --json
```

and

```text
goet status <submission_id> --json
```

When `--json` is specified, the CLI should emit only valid JSON to standard output.

Diagnostic messages and errors should continue to be written to standard error.

### Submit Output

Successful submission should produce JSON similar to:

```json
{
    "submission_id": "sub_1234",
    "workflow_id": "annual-report",
    "initial_work_item_count": 47
}
```

### Status Output

Successful status queries should produce JSON similar to:

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

The JSON field names should remain stable whenever practical.

## Out Of Scope

* Changing controller APIs.
* Implementing new status information.
* Implementing hierarchical workflow or step output.
* Pretty-print terminal formatting.
* Python or R SDKs.
* Authentication or authorization.
* Artifact output.
* Retry behavior.
* Versioned JSON schemas.

## Acceptance Criteria

* `goet submit --json` produces valid JSON.
* `goet status --json` produces valid JSON.
* Human-readable output remains the default.
* Errors continue to be written to standard error.
* JSON output contains no additional human-readable text.
* Unit tests verify JSON serialization.
* The output format is suitable for shell pipelines and future SDKs.

## Notes

* Human-readable output is the primary interactive interface.
* JSON output is the primary automation interface.
* The CLI should never mix human-readable text with JSON on standard output.
* The JSON output should be stable enough to serve as the initial machine contract for future Python and R wrappers.

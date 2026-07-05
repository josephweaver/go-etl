# 005 Add CLI Status Command

Status: Complete

## Objective

Implement `goet status <submission_id>` in `cmd/demo-client`.

This slice connects the CLI to the submission-scoped controller endpoint added in slice 004 and prints a simple human-readable status summary.

## Current State

This slice is implemented in `cmd/demo-client/main.go` and
`internal/client/controller_client.go`.

Before this slice:

- The CLI parses `status <submission_id>` and validates the optional `--controller-url` flag.
- The controller exposes `GET /submissions/{submission_id}/status`.
- `internal/model/submission.go` defines the shared submission status response shape.
- `internal/client.ControllerClient` can call aggregate `GET /status`, but it does not yet expose a helper for submission-scoped status.
- `goet status` does not yet call the controller.

## Target State

The CLI implements:

```text
goet status <submission_id> [--controller-url <url>]
```

Behavior:

- `submission_id` is required.
- If `--controller-url` is omitted, default to:

  ```text
  http://localhost:8080
  ```

- The client calls:

  ```text
  GET /submissions/{submission_id}/status
  ```

- Successful responses are printed in human-readable form.
- Unknown submission IDs produce useful user-facing errors.
- Controller connection failures produce useful user-facing errors.

Expected human-readable output may be simple:

```text
Submission: sub_1234
Workflow: annual-report
Status: Complete
Known work items: 47
Queued: 20
Running: 4
Completed: 23
Failed: 0
Skipped: 0
```

`--json` remains parser-only until slice 007. Human-readable output remains the default.

## Concept Decision

This slice updates the existing `internal/client` controller client concept by adding submission-scoped status retrieval.

Add the HTTP helper in `internal/client/controller_client.go` unless the file becomes too mixed. Do not make `cmd/demo-client/main.go` hand-code HTTP requests to the controller. The CLI should call reusable client behavior.

## Required Context

Read these files first:

- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/submission-cli-status/001-cli-client-contract.md`
- `docs/concepts/submission-cli-status/004-add-submission-status-api`
- `docs/CUSTOMER_API.md`
- `cmd/demo-client/README.md`
- `cmd/demo-client/main.go`
- `cmd/demo-client/main_test.go`
- `internal/client/README.md`
- `internal/client/controller_client.go`
- `internal/client/controller_client_test.go`
- `internal/model/submission.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/demo-client/main.go`
- `internal/client/controller_client.go`

## Allowed Test Files

- `cmd/demo-client/main_test.go`
- `internal/client/controller_client_test.go`

## Out Of Scope

- Implementing `--wait`.
- Implementing `--json` output.
- Implementing or accepting `--watch`.
- Creating new controller endpoints.
- Changing submission status semantics.
- Implementing hierarchical workflow, step, or work-item rendering.
- Implementing client-side remembered state.
- Remembering previous submissions or controller URLs.
- Python or R SDKs.
- Authentication or authorization.
- Artifact reporting.
- Attempt reporting.
- Retry behavior.

## Acceptance Criteria

- `goet status <submission_id>` calls the submission-scoped controller status endpoint.
- `goet status` requires a submission ID.
- `goet status` defaults to `http://localhost:8080` when no controller URL is supplied.
- Successful responses are displayed in human-readable form.
- Unknown submission IDs return useful errors.
- Controller connection failures return useful errors.
- Unit tests cover argument validation, client response decoding, HTTP error handling, and human-readable rendering.
- The implementation does not add client-side state.

## Notes

- Keep output simple and stable.
- Do not add a separate `wait` command in this slice.
- Do not implement continuous display.
- This slice makes status lookup usable before adding wait or JSON output.


# 001 CLI Client Contract

Status: Implemented

## Objective

Add the first command-shaped CLI contract to `cmd/demo-client`.

This slice replaces the current positional-only demo behavior with explicit top-level parsing for:

```text
goet submit
goet status
```

The goal is to establish the public command and flag shape without implementing submission acknowledgement, submission status retrieval, wait behavior, JSON output, or a built-in watch mode.

## Current State

`cmd/demo-client/main.go` is a demo executable. It currently:

- builds demo controller variables in `demoResolver()`;
- creates `client.NewLocalControllerStarter` and `client.NewControllerClientWithStarter`;
- submits a workflow-run source-reference file selected by `demoWorkflowRunPath(os.Args)`;
- waits for aggregate controller idle state with `ShutdownWhenIdle(60)`;
- requests controller shutdown through `internal/client`;
- prints `formatFinalStatus(status)`.

`cmd/demo-client/main_test.go` currently verifies:

- the default sibling demo submission file path;
- the custom positional submission file path;
- aggregate final status formatting.

There are no `submit` or `status` subcommands. The first non-program argument is interpreted as a workflow-run submission file path. There is no parser validation for controller, project, workflow, wait, JSON, or submission status flags.

## Target State

`cmd/demo-client` recognizes two top-level commands:

```text
submit
status
```

For this slice, the executable may still be run as `go run ./cmd/demo-client ...`; the command text in help/errors should use `goet` as the long-term public command name.

### Submit command shape

```text
goet submit \
  --controller <controller.json> \
  --project <project.json> \
  --workflow <workflow.json>
```

```text
goet submit \
  --controller-url <url> \
  --project <project.json> \
  --workflow <workflow.json>
```

Accepted flags:

- `--controller <path>`
- `--controller-url <url>`
- `--project <path>`
- `--workflow <path>`
- `--wait`
- `--json`

Validation rules:

- `submit` is the primary workflow-submission command.
- Exactly one of `--controller` or `--controller-url` is required.
- Supplying both `--controller` and `--controller-url` is an error.
- `--project` is required.
- `--workflow` is required.
- `--wait` is accepted by the parser, but full wait behavior is deferred to slice 006.
- `--json` is accepted by the parser, but final machine-readable output behavior is deferred to slice 007.
- `--watch` is not accepted.

### Status command shape

```text
goet status <submission_id> [--controller-url <url>] [--json]
```

Validation rules:

- `status` is accepted as a top-level command.
- `submission_id` is required.
- Extra positional arguments are rejected.
- If `--controller-url` is omitted, the parsed command defaults to:

  ```text
  http://localhost:8080
  ```

- `--json` is accepted by the parser, but final machine-readable output behavior is deferred to slice 007.
- `--watch` is not accepted.
- Full controller-backed submission status retrieval is deferred until the controller status model exists.

### Compatibility path

The current zero-argument demo path may remain as an explicit compatibility path for local smoke use during this slice. Do not advertise the compatibility path as the long-term public API.

## Concept Decision

This slice updates the existing `cmd/demo-client` concept. It should not create a new production package or move reusable client behavior yet.

The parser/validation logic may be implemented as small helper functions in `cmd/demo-client/main.go` so later slices can attach real loading/submission/status behavior without duplicating argument parsing.

## Required Context

Read these files first:

- `docs/concepts/submission-cli-status/README.md`
- `docs/CUSTOMER_API.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/demo-client/README.md`
- `cmd/demo-client/main.go`
- `cmd/demo-client/main_test.go`
- `internal/client/README.md`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/demo-client/main.go`

## Allowed Test Files

- `cmd/demo-client/main_test.go`

## Out Of Scope

- Loading `controller.json`, `project.json`, or `workflow.json`.
- Submitting the new CLI input shape to the controller.
- Returning or printing a submission acknowledgement.
- Creating first-class controller Submission storage.
- Creating new controller HTTP endpoints.
- Calling a submission status endpoint.
- Implementing final `--wait` behavior.
- Implementing final `--json` output.
- Implementing or accepting `--watch`.
- Renaming the executable outside `cmd/demo-client`.
- Python or R SDKs.
- Authentication or authorization.
- Artifact or attempt commands.
- Retry behavior.
- Durable queue redesign.

## Acceptance Criteria

- `cmd/demo-client` recognizes `submit` as a top-level command.
- `cmd/demo-client` recognizes `status` as a top-level command.
- `submit` validates that exactly one of `--controller` or `--controller-url` is supplied.
- `submit` requires `--project`.
- `submit` requires `--workflow`.
- `submit` accepts `--wait`.
- `submit` accepts `--json`.
- `submit` rejects `--watch`.
- `status` requires exactly one `submission_id` positional argument.
- `status` defaults to `http://localhost:8080` when `--controller-url` is omitted.
- `status` accepts `--json`.
- `status` rejects `--watch`.
- Invalid argument combinations return useful errors.
- The existing zero-argument demo path remains usable or is replaced by an explicit compatibility path covered by tests.
- Unit tests cover argument parsing and validation behavior.

## Notes

- Keep the CLI thin. The CLI should not compile workflows, inspect worker state, or own submission progress.
- Use standard library parsing unless there is a strong reason to add a dependency.
- Prefer a small parsed command structure that later slices can pass to loader/submission/status helpers.
- The public docs should not mention `--watch`; later documentation should show OS-level composition such as `watch -n 5 goet status <submission_id>`.

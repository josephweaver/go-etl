# 002 Deserialize CLI JSON Inputs

Status: Ready

## Objective

Add the first CLI-side loader for explicit `goet submit` JSON inputs and wire `goet submit` to the existing controller workflow submission path.

This slice should let the CLI read:

```text
controller.json
project.json
workflow.json
```

from paths supplied by the slice 001 parser. It should submit the loaded workflow through the existing `/workflow` controller path while that path still uses the pre-acknowledgement success behavior.

## Current State

After slice 001, `cmd/demo-client` has a command-shaped parser for `submit` and `status`, but `goet submit --controller/--controller-url --project --workflow` does not yet have a real input loading boundary.

Current nearby implementation facts:

- `internal/client/controller_client.go` already owns controller reachability checks and `POST /workflow` submission.
- `ControllerClient.SubmitWorkflowRunFile(path)` loads a single source-reference workflow-run submission envelope and submits it.
- `ControllerClient.SubmitWorkflowFile(path)` loads the legacy inline workflow submission shape and submits it.
- `ControllerClient.submitWorkflowPayload` currently treats `204 No Content` from `POST /workflow` as success.
- `cmd/demo-client/main.go` currently builds a hard-coded demo resolver rather than loading controller selection from CLI inputs.
- `project.json` is not yet used by the CLI as `project_config` input.

## Target State

The CLI has a bounded input-loading helper that can read user-supplied controller, project, and workflow files.

### Controller input

When `--controller <path>` is supplied:

- The file is read from disk.
- Invalid JSON returns a useful error that identifies the controller file path.
- The input helper preserves the controller path so the local controller starter can be configured to start:

  ```text
  go run ./cmd/controller --config <controller path>
  ```

- The controller URL may default to `http://localhost:8080` for this slice unless the implementation can read `controller_config.controller_url` without duplicating controller-owned validation.
- Full controller document validation remains owned by `cmd/controller/config.go`, not by the CLI.

When `--controller-url <url>` is supplied:

- No controller config file is required.
- The client submits to the already-running controller URL.
- The client should not create a local controller starter.

### Project input

The project file is read from disk and treated as project-scoped data.

For this slice, convert supported top-level JSON fields into `project_config` variables:

```json
{
  "id": "go-etl-demo",
  "name": "GO ETL Demo Project"
}
```

becomes conceptually:

```text
project_config.id = "go-etl-demo"
project_config.name = "GO ETL Demo Project"
```

Supported literal JSON values should map onto the existing typed-variable model where practical:

- JSON string -> `variable.TypeString`
- JSON integer -> `variable.TypeInt`
- JSON boolean -> `variable.TypeBool`
- JSON object -> `variable.TypeObject` when all child fields can be represented
- JSON array -> `variable.TypeList` when all child values can be represented

Unsupported values should return useful errors instead of silently creating a second project configuration system. Null handling may be rejected for this slice.

### Workflow input

The workflow file is read from disk and deserialized using the existing workflow submission shape where practical.

The loader should support the current wrapper form already used by demo workflow files:

```json
{
  "workflow": {},
  "source_manifest": {},
  "variables": []
}
```

The loader should preserve workflow-scope variables inside the workflow object and preserve top-level submitted variables as submission variables.

### Submit behavior

`goet submit` should use the loaded inputs to call the current controller workflow submission path.

For this slice:

- A successful current controller response may still be `204 No Content`.
- The command may print a simple human-readable success message without `submission_id`.
- Structured acknowledgement is deferred to slice 003.
- `--wait` remains parser-only until slice 006.
- `--json` remains parser-only until slice 007.

## Concept Decision

This slice adds a new CLI input-loading concept under `internal/client` because the logic is reusable client behavior, not demo executable wiring.

Create `internal/client/cli_inputs.go` for the input model and loader if that keeps file responsibilities clearer. Do not put all JSON file parsing into `cmd/demo-client/main.go`.

This slice updates `cmd/demo-client/main.go` only enough to call the input loader and existing client submission path.

## Required Context

Read these files first:

- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/submission-cli-status/001-cli-client-contract.md`
- `docs/CUSTOMER_API.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/demo-client/README.md`
- `cmd/demo-client/main.go`
- `cmd/demo-client/main_test.go`
- `internal/client/README.md`
- `internal/client/controller_client.go`
- `internal/client/controller_client_test.go`
- `internal/client/local_controller.go`
- `internal/workflow/workflow.go`
- `internal/variable/variable.go`
- `internal/variable/namespace.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/demo-client/main.go`
- `internal/client/controller_client.go`
- `internal/client/local_controller.go`
- `internal/client/cli_inputs.go`

## Allowed Test Files

- `cmd/demo-client/main_test.go`
- `internal/client/controller_client_test.go`
- `internal/client/local_controller_test.go`
- `internal/client/cli_inputs_test.go`

## Out Of Scope

- Returning a submission acknowledgement.
- Introducing `submission_id`.
- Creating a submission status endpoint.
- Implementing `goet status` against the controller.
- Implementing final `--wait` behavior.
- Implementing final `--json` output.
- Implementing or accepting `--watch`.
- Full public JSON schema versioning.
- Schema migration.
- Secrets management.
- Project persistence.
- Durable workflow persistence redesign.
- Redesigning the variable resolver.
- Redesigning workflow compilation.
- Redesigning controller configuration validation.
- Changing worker execution behavior.
- Python or R SDKs.

## Acceptance Criteria

- `goet submit --controller <file> --project <file> --workflow <file>` reads all three files from disk.
- `goet submit --controller-url <url> --project <file> --workflow <file>` reads project/workflow files and does not require a controller file.
- Invalid controller JSON returns a useful error identifying the controller file.
- Invalid project JSON returns a useful error identifying the project file.
- Invalid workflow JSON returns a useful error identifying the workflow file.
- Missing required files return useful errors.
- Project fields are represented through `project_config` variables in the existing variable model.
- Workflow input is represented through the existing workflow submission/workflow model where practical.
- `goet submit` can call the existing controller workflow submission path.
- Existing demo-client compatibility behavior remains usable or is intentionally covered by updated tests.
- Unit tests cover controller/project/workflow input loading and submit-path validation.
- The implementation does not introduce a separate project configuration authority outside the variable model.

## Notes

- Keep the input structures intentionally minimal.
- Do not overfit the JSON schema before the submission model exists.
- Do not implement hidden client state.
- Do not infer orchestration state from input files.
- This slice creates the typed input boundary that later acknowledgement, wait, and JSON slices will use.

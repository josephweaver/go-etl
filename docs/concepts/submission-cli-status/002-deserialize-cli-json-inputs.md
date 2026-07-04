# 002 Deserialize CLI JSON Inputs

Status: Proposed

## Objective

Add the first CLI-side JSON input loader for the long-term GOET client.

This slice should deserialize:

* `controller.json`
* `project.json`
* `workflow.json`

into Go structures that can later be passed into the submit path.

The goal is to establish the client-side input boundary for `goet submit` without yet implementing full controller submission behavior.

## Required Context

Read these files first:

* docs/epics/submission-cli-status/README.md
* docs/epics/submission-cli-status/001-upgrade-demo-client-cli-arguments.md
* docs/CUSTOMER_API.md
* docs/ARCHITECTURE_OVERVIEW.md
* cmd/demo-client/main.go
* cmd/controller/config.go
* internal/workflow/workflow.go
* internal/variable/variable.go
* internal/variable/namespace.go

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

* cmd/demo-client/main.go
* internal/client/local_controller.go
* internal/client/cli_inputs.go

## Allowed Test Files

* cmd/demo-client/main_test.go
* internal/client/cli_inputs_test.go
* internal/client/local_controller_test.go

## Required Behavior

Add client-side support for reading three JSON files:

```text
controller.json
project.json
workflow.json
```

### controller.json

The controller file describes controller startup and execution configuration.

For this slice, it may reuse the existing controller config loading shape if practical.

The loader should preserve the file contents in a structured form suitable for later controller startup and submission work.

### project.json

The project file represents variables in the `project_config` namespace.

The loader should deserialize project variables into the existing variable model rather than inventing a second project configuration system.

For this slice, project JSON may be minimal, but it must support a collection of project-scoped variables.

### workflow.json

The workflow file describes reusable workflow work.

The loader should deserialize workflow JSON into the existing workflow model where practical.

The expected workflow shape is:

* workflow ID
* workflow variables
* ordered steps

The loader should not execute or compile the workflow in this slice unless existing helper behavior makes that unavoidable.

## Out Of Scope

* Submitting the deserialized objects to the controller.
* Creating a new controller HTTP endpoint.
* Implementing the first-class Submission model.
* Implementing submission acknowledgement.
* Implementing submission status.
* Implementing final schema versioning.
* Implementing schema migration.
* Implementing Python or R SDKs.
* Implementing secrets management.
* Implementing project persistence.
* Implementing durable workflow persistence.
* Redesigning the variable resolver.
* Redesigning workflow compilation.
* Redesigning controller configuration.
* Changing worker execution behavior.

## Acceptance Criteria

* The client can read a controller JSON file from disk.
* The client can read a project JSON file from disk.
* The client can read a workflow JSON file from disk.
* Invalid JSON returns a useful error identifying the failing file.
* Missing required files return useful errors.
* Project variables are represented through the existing variable model.
* Workflow input is represented through the existing workflow model where practical.
* The loader is covered by unit tests.
* The implementation does not introduce a separate configuration authority outside the variable model.
* Existing demo-client behavior remains usable.

## Notes

* Prefer a small `internal/client/cli_inputs.go` helper rather than placing all parsing logic directly in `cmd/demo-client/main.go`.
* Keep the input structures intentionally minimal.
* Do not overfit the JSON schema before the submission model exists.
* Do not implement hidden client state.
* This slice creates the typed input boundary that later slices will use for submission.

# 001 WorkItem Source and Python Operation Contract

Status: proposed

## Objective

Add the shared work-item model contract needed for source-backed Python execution.

This slice adds a `python_script` work-item type and an optional controller-generated `WorkItemSource` locator on `model.WorkItem`. It does not add controller source-bundle behavior, worker staging, Python subprocess execution, or workflow compilation behavior.

## Current State

`internal/model/work_item.go` defines two work-item types:

```text
write_demo_output
summarize_input_file
```

`model.WorkItem` currently carries identity, attempt ID, work type, output filename, parameters, reuse candidates, workflow metadata, fingerprints, and code version. It does not carry a source locator.

`WorkItem.Validate()` currently checks required `ID`, required `Type`, required basename-only `OutputFilename`, and parameter shape. It currently treats an unknown work-item type as structurally valid.

`internal/model/work_item_test.go` already tests basic validation, JSON round-tripping for runtime metadata, work completion JSON, work failure JSON, controller status JSON, and work skip validation.

## Target State

`internal/model/work_item.go` defines:

```go
type WorkItemSource struct {
    Schema       string `json:"schema,omitempty"`
    RunID        string `json:"run_id"`
    ManifestPath string `json:"manifest_path"`
}
```

`model.WorkItem` has:

```go
Source *WorkItemSource `json:"source,omitempty"`
```

The model defines:

```go
WorkItemTypePythonScript WorkItemType = "python_script"
```

`WorkItem.Validate()` preserves compatibility for existing work items while requiring `Source` for `python_script` items.

A `WorkItemSource.Validate()` helper, or equivalent local validation logic, rejects missing `run_id` and missing `manifest_path`. `schema` is optional in this slice, but when present it should be non-empty after trimming whitespace.

Tests prove:

- existing valid work items remain valid;
- unknown work-item types remain structurally valid unless a later slice deliberately changes that policy;
- `python_script` with valid `Source` validates;
- `python_script` without `Source` fails validation;
- `python_script` with empty `Source.RunID` fails validation;
- `python_script` with empty `Source.ManifestPath` fails validation;
- JSON round-trip preserves `Source`.

## Concept Decision

This slice updates the existing `model.WorkItem` concept because `WorkItemSource` is part of the shared worker payload contract.

A new production file is not required unless keeping `WorkItemSource` in `work_item.go` makes the model package harder to read. If Codex creates a new file, it must remain inside `internal/model` and own only source-locator model behavior.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `internal/model/work_item.go`
- `internal/model/work_item_test.go`

Do not read unrelated controller, worker, scheduler, transport, or repository-source files unless compile or test failures directly require it.

## Allowed Production Files

- `internal/model/work_item.go`
- `internal/model/work_item_source.go`

## Allowed Test Files

- `internal/model/work_item_test.go`
- `internal/model/work_item_source_test.go`

## Allowed Documentation Files

- `PROJECT_STATE.md`

## Out Of Scope

- Controller source-bundle API.
- Worker source-bundle download.
- Worker staging directories.
- Python subprocess execution.
- Python output/evidence contract.
- Workflow compilation changes.
- Source-manifest role validation.
- Python SDK or client behavior.
- Requiring `Source` for non-`python_script` work items.
- Rejecting unknown work-item types globally.

## Acceptance Criteria

- `WorkItemTypePythonScript` exists with JSON/string value `python_script`.
- `model.WorkItem` can carry an optional `source` object in JSON.
- `WorkItemSource` carries `schema`, `run_id`, and `manifest_path` JSON fields.
- `WorkItem.Validate()` requires a valid source locator for `python_script` work items.
- Existing tests for demo and summary work items still pass.
- New tests cover valid and invalid `python_script` source-locator cases.
- New tests cover JSON round-trip behavior for `WorkItem.Source`.
- `go test ./internal/model` passes.

## Notes

- The controller, not the workflow author, will generate `WorkItem.Source` in a later slice.
- `manifest_path` should identify the admitted source manifest or manifest reference known to the controller. Do not make the worker interpret controller cache filesystem paths.
- Keep validation structural. Do not validate whether the run ID or manifest exists in this slice.
- Update `PROJECT_STATE.md` only if this model change should be recorded as current implementation state. If no update is needed, the implementation report should say why.

# 006 Workflow Compilation Integration for Python Source WorkItems

Status: proposed

## Objective

Update source-reference workflow admission and workflow compilation so `python_script` work items are validated against the admitted `source_manifest` and receive a controller-generated `WorkItem.Source` locator before queue insertion.

This slice connects Python work-item model/staging/execution to source-reference workflow admission. It does not redesign source admission.

## Current State

Source-reference `/workflow` admission already reads project, workflow, and workflow-declared supplemental files through `internal/reposource`, publishes admitted files into the repository cache, and compiles workflow work from verified cached workflow bytes.

Workflow source documents can declare supplemental source files with roles such as:

```text
python_entrypoint
python_environment
support_file
```

`internal/reposource` defines corresponding file roles.

`model.WorkItem` should have `Source *WorkItemSource` and `WorkItemTypePythonScript` after slice 001.

The worker should be able to stage source bundles and execute `python_script` after slices 003 through 005.

The compiler/admission path does not yet validate Python operation source paths against admitted manifest roles or attach source locators to Python work items.

## Target State

During source-reference workflow admission, the controller validates each `python_script` work item before insertion.

Rules:

- `python_entrypoint` is required.
- `python_entrypoint` must be a `string` or `path` parameter.
- `python_entrypoint` must match an admitted manifest file with role `python_entrypoint`.
- `python_environment`, when present, must be a `string` or `path` parameter.
- `python_environment`, when present, must match an admitted manifest file with role `python_environment`.
- Support files are allowed only when declared in the admitted manifest as `support_file`.
- Undeclared source paths fail before work-item insertion.
- Wrong-role paths fail before work-item insertion.
- Non-Python work items remain unchanged.

The controller attaches a generated source locator to each compiled Python work item:

```json
"source": {
  "schema": "goet/work-item-source/v1",
  "run_id": "...",
  "manifest_path": "..."
}
```

The workflow author does not provide this source locator.

## Concept Decision

This slice updates the controller workflow-compilation/admission concept. The controller remains responsible for validating source-role requirements before worker assignment.

Prefer small helper functions in `cmd/controller` if `main.go` would otherwise grow unclear. Do not add a broad compiler package unless the existing code already has a clear package boundary for workflow compilation.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/source-control-resolution-and-cache/README.md`
- `internal/model/work_item.go`
- `internal/reposource/model.go`
- `internal/reposource/source_declaration.go`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`

Do not read worker subprocess code, scheduler, transport, SSH, Docker, or client setup files unless compile or test failures directly require it.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/python_workitem.go`
- `cmd/controller/source_control.go`
- `internal/reposource/source_declaration.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/python_workitem_test.go`
- `internal/reposource/source_declaration_test.go`

## Allowed Fixture Files

- `../go-etl-demo-project/project.json`
- `../go-etl-demo-project/workflows/*.json`
- `../go-etl-demo-project/submissions/*.json`
- `../go-etl-demo-project/scripts/*.py`
- `../go-etl-demo-project/scripts/**/*.py`
- `../go-etl-demo-project/environments/*.json`

## Allowed Documentation Files

- `PROJECT_STATE.md`

## Out Of Scope

- Source admission redesign.
- Provider rereads.
- Controller source-bundle endpoint changes.
- Worker staging or execution changes.
- Python environment creation.
- Dependency-aware workflow scheduling.
- Python SDK/client behavior.
- Multi-repository or multi-source workflow admissions.
- Sensitive variable propagation.

## Acceptance Criteria

- Source-reference workflow admission attaches `WorkItem.Source` to compiled `python_script` items.
- `WorkItem.Source.RunID` matches the admitted workflow run identity.
- `WorkItem.Source.ManifestPath` identifies the admitted manifest reference used by the controller.
- Admission rejects `python_script` items with missing `python_entrypoint`.
- Admission rejects `python_script` items whose `python_entrypoint` is undeclared.
- Admission rejects `python_script` items whose `python_entrypoint` has the wrong source-manifest role.
- Admission rejects `python_environment` paths that are undeclared or wrong-role when the parameter is present.
- Non-Python work items are still compiled as before.
- Existing source-reference workflow tests still pass.
- `go test ./cmd/controller` passes.

## Notes

- This slice assumes slices 001 through 005 have landed, or at minimum that `model.WorkItem.Source` and `model.WorkItemTypePythonScript` exist.
- The controller should validate paths against admitted source facts, not against the live provider repository.
- Do not let user-authored workflow JSON provide the source locator. The source locator is controller-generated.
- If adding fixtures in `../go-etl-demo-project` is too broad for this slice, report that and leave demo fixture creation for slice 007.

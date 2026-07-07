# 004 Stamp Work Items With Step Instance Metadata

Status: Complete

## Objective

Ensure every dependency-aware compiled work item carries or is associated with workflow, stage, step, and item-order metadata before queue insertion.

## Current State

Slices 001 through 003 are treated as complete. Slice 004 is currently running on the branch.

The active work is the boundary between the 002 stage compiler and the 003 dependency-state owner. The implementation should take one compiled stage result, produce queue-ready work-item payloads, and produce membership/state records that let completion/failure handlers find the owning workflow, stage, step, and fan-out item index later.

Do not start slice 005 until 004 has stable helper names and tests.

## Target State

The controller has one narrow helper for stamping compiled stage results into queue-ready work items and dependency membership records.

The helper should associate each generated work item with:

```text
submission_id
workflow_instance_id or run_id
workflow_definition_id
stage_index
step_index
step_definition_id
step_instance_id
work_item_index within the step
work_item_id
```

The exact transport shape may differ. Metadata that workers do not need can remain in controller-owned membership state rather than in the worker payload. Metadata that the completion/failure endpoints need to validate may be copied into `model.WorkItem` or looked up by assigned `attempt_id`/work-item ID.

The helper should not compile stages. It should only transform an already compiled stage result into queue and membership records.

The 004 implementation report should name the helper(s), the store owner it writes to, and any field names that downstream slices 005-011 must use.

## Concept Decision

This slice updates the controller workflow-submission concept and the shared work-item identity concept.

Prefer controller-owned membership state for dependency-only metadata such as `stage_index` and `work_item_index`. Add fields to `internal/model.WorkItem` only when the worker payload or completion contract genuinely needs them.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/002-compile-single-workflow-stage.md`
- `docs/concepts/dependency-aware-workflows/003-persist-workflow-stage-state.md`
- the actual stage compiler file created by 002, such as `internal/workflow/compile_stage.go`
- the actual dependency-state owner created by 003, such as `cmd/controller/workflow_dependency_store.go` or an `internal/persistence` owner
- `internal/model/work_item.go`
- `internal/model/work_item_test.go`
- `cmd/controller/main.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/workflow_stage_queue.go`
- the actual dependency-state owner created by 003
- `cmd/controller/main.go`
- `internal/model/work_item.go`

If 004 is already running and created a differently named controller helper file, continue in that owner and report the substitution.

## Allowed Test Files

- `cmd/controller/workflow_stage_queue_test.go`
- the actual dependency-state owner tests created by 003
- `cmd/controller/main_test.go`
- `internal/model/work_item_test.go`

## Out Of Scope

- Changing live workflow submission to queue only stage 0.
- Completion/failure handling.
- JIT compilation.
- Output parsing.
- Status APIs.
- Worker behavior changes.
- Resource constraints.

## Acceptance Criteria

- A controller helper can take a stage compile result and produce queue-ready work items plus membership records.
- Every membership record includes `submission_id`, `stage_index`, `step_index`, `work_item_id`, and `work_item_index`.
- Every work item has stable workflow/step identity fields populated when those fields already exist in `model.WorkItem`.
- `step_instance_id` is deterministic within a submission and differs across submissions.
- Work-item index preserves fan-out generation order.
- Tests do not rely on lexicographic work-item ID sorting for fan-out order.
- Existing work-item validation still passes for supported work-item types.
- No live endpoint behavior changes in this slice.

## Notes

- A deterministic `step_instance_id` such as `<submission_id>:step:<step_index>` is acceptable if it follows existing ID style.
- Do not expose `parallel_with` labels as step identities.

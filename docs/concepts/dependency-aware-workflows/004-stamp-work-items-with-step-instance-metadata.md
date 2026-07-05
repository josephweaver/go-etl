# 004 Stamp Work Items With Step Instance Metadata

Status: Ready

## Objective

Ensure every dependency-aware compiled work item carries or is associated with workflow, stage, step, and item-order metadata before queue insertion.

## Current State

`internal/model.WorkItem` already has workflow and step identity fields such as `WorkflowDefinitionID`, `WorkflowInstanceID`, `StepDefinitionID`, `StepInstanceID`, and fingerprint fields.

Stage-scoped compilation from slice 002 can report each compiled work item's `step_index`, `step_id`, and `work_item_index`, but the queued work item or controller membership state does not yet consistently connect these values to a concrete submission.

Later slices need stable metadata to update the owning step when a worker completes or fails a work item.

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

## Concept Decision

This slice updates the controller workflow-submission concept and the shared work-item identity concept.

Prefer controller-owned membership state for dependency-only metadata such as `stage_index` and `work_item_index`. Add fields to `internal/model.WorkItem` only when the worker payload or completion contract genuinely needs them.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/002-compile-single-workflow-stage.md`
- `docs/concepts/dependency-aware-workflows/003-persist-workflow-stage-state.md`
- `internal/model/work_item.go`
- `internal/model/work_item_test.go`
- `cmd/controller/main.go`
- `cmd/controller/workflow_dependency_store.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/workflow_stage_queue.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/main.go`
- `internal/model/work_item.go`

If the previous concepts already split submission/queue helpers into another controller file, modify that owner instead of creating `workflow_stage_queue.go` and report the substitution.

## Allowed Test Files

- `cmd/controller/workflow_stage_queue_test.go`
- `cmd/controller/workflow_dependency_store_test.go`
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

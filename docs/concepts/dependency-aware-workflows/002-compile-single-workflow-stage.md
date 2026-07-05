# 002 Compile Single Workflow Stage

Status: Ready

## Objective

Add a workflow compiler entry point that compiles one normalized stage at a time instead of requiring the controller to compile the whole workflow.

## Current State

`internal/workflow.CompileWorkflowResult` compiles every step in the workflow and returns all generated work items. That shape forces workflow submission to produce all upstream and downstream work items at once.

Slice 001 introduced a normalized stage plan, but the compiler still needs a way to compile only the selected stage.

## Target State

`internal/workflow` exposes a stage-scoped compile function, such as:

```go
CompileWorkflowStage(resolver variable.Resolver, workflow Workflow, plan WorkflowPlan, stageIndex int) (CompileStageResult, error)
```

The exact name may differ, but the result should identify:

```text
workflow_id
stage_index
compiled steps in workflow-definition order
compiled work items in deterministic fan-out order
per-work-item membership metadata needed by later slices:
  step_index
  step_id
  work_item_index within that step
```

Behavior:

- The function compiles only the requested stage.
- It validates the stage index.
- It preserves the existing fan-out behavior for each step.
- It preserves deterministic ordering from `CompileFanOutWorkItems`.
- It detects duplicate generated work-item IDs within the compiled stage.
- It reports enough context in errors to identify the stage and step that failed to compile.

`CompileWorkflowResult` may continue to exist for tests or compatibility, but dependency-aware controller code should be able to use the new stage-scoped function.

## Concept Decision

This slice updates the existing workflow compiler concept. Keep the stage compile function in `internal/workflow`; do not duplicate compiler logic in the controller.

A new file such as `internal/workflow/compile_stage.go` is acceptable if it keeps stage compilation separate from legacy full-workflow compilation.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/001-normalize-workflow-stages.md`
- `internal/workflow/stage.go`
- `internal/workflow/workflow.go`
- `internal/workflow/step.go`
- `internal/workflow/fanout.go`
- `internal/workflow/fanout_test.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `internal/workflow/compile_stage.go`
- `internal/workflow/stage.go`
- `internal/workflow/workflow.go`
- `internal/workflow/fanout.go`

## Allowed Test Files

- `internal/workflow/compile_stage_test.go`
- `internal/workflow/workflow_test.go`
- `internal/workflow/fanout_test.go`

## Out Of Scope

- Controller submission changes.
- Persistent workflow instance state.
- Queue insertion.
- Completion handling.
- Output aggregation.
- Status APIs.
- Observability.
- Worker changes.

## Acceptance Criteria

- A stage-scoped compile function exists.
- Compiling stage 0 of a sequential workflow returns only step 0 work items.
- Compiling stage 1 of a sequential workflow returns only step 1 work items.
- Compiling a parallel stage returns work items for every step in that stage.
- Work items in a parallel stage are ordered first by workflow step order, then by fan-out order within each step.
- The result includes the step index, step ID, and work-item index for every compiled work item.
- Invalid stage indexes return clear errors.
- Duplicate generated work-item IDs inside one compiled stage return clear errors.
- No controller or worker behavior changes in this slice.

## Notes

- Later slices will check duplicate work-item IDs against previously queued or completed items from the same submission.
- Do not use completion order as an output-ordering source.

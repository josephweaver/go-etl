# 002 Compile Single Workflow Stage

Status: Complete

## Objective

Preserve and verify the completed stage-scoped compiler entry point so controller slices can compile exactly one normalized stage at a time.

## Current State

This slice is visible as implemented on `docs/concepts/dependency-aware-workflows`.

Expected completed artifacts include:

- a stage-scoped compile entry point in `internal/workflow`, such as `CompileWorkflowStage`;
- a compile result that identifies workflow ID, stage index, step index, step ID, and work-item index;
- preservation of existing fan-out behavior and deterministic fan-out ordering;
- duplicate generated work-item ID validation within the compiled stage;
- tests for sequential stages, parallel stages, invalid stage indexes, and duplicate generated IDs.

Later controller slices should call this stage compiler directly instead of compiling the whole workflow or duplicating compiler logic.

## Target State

No new target behavior is expected beyond preserving the completed 002 contract.

The compiler should remain persistence-agnostic. It produces compiled stage data and membership metadata; controller/store slices decide when and how those results become queue records.

## Concept Decision

Keep stage-scoped compilation in `internal/workflow`.

Do not add controller-local fan-out or workflow compilation logic in later slices. If the actual function or result names differ from examples here, update downstream slice references after 004 stabilizes.

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

No production changes are expected when verifying this completed slice.

Only modify these files if review finds a narrow 002 regression:

- `internal/workflow/compile_stage.go`
- `internal/workflow/stage.go`
- `internal/workflow/workflow.go`
- `internal/workflow/fanout.go`

## Allowed Test Files

No test changes are expected unless verification reveals a missing regression case.

Allowed verification-test files:

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

## Verification Criteria

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

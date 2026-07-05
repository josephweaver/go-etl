# 008 Compile Next Ready Stage

Status: Ready

## Objective

After a stage completes successfully, compile the next stage just in time using retained workflow context and completed predecessor outputs, then queue the newly ready work items.

## Current State

Submission queues only stage 0. Terminal work-result handling can now mark steps and stages completed. Step outputs can be captured and exposed as a generated `workflow.step` scope.

No controller path yet activates stage 1 after stage 0 completes. A multi-stage workflow therefore stops after the first stage.

## Target State

When completion handling transitions stage `N` to completed, the controller attempts to activate stage `N+1` if it exists.

Activation should:

```text
check workflow is still running
check stage N is completed
check stage N+1 is currently blocked
assemble resolver scopes from retained submission/workflow context plus generated workflow.step outputs
compile stage N+1 only
stamp compiled work items with dependency metadata
insert membership records
queue compiled work items
mark stage N+1 active/ready according to existing state names
update submission status
notify or reuse the existing worker-scaling/reconciliation path if one exists
```

If `N` is the final stage, the workflow/submission should become completed after stage `N` completes.

Activation must be idempotent. If the completion handler runs twice, stage `N+1` must not be queued twice.

## Concept Decision

This slice updates the controller completion and workflow-submission concepts. It uses the stage compiler from `internal/workflow`; it must not reimplement workflow compilation in controller code.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/002-compile-single-workflow-stage.md`
- `docs/concepts/dependency-aware-workflows/005-submit-only-initial-ready-stage.md`
- `docs/concepts/dependency-aware-workflows/006-record-terminal-work-item-state.md`
- `docs/concepts/dependency-aware-workflows/007-capture-typed-step-outputs.md`
- `cmd/controller/main.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_completion.go`
- `cmd/controller/workflow_outputs.go`
- `cmd/controller/workflow_stage_queue.go`
- `internal/workflow/compile_stage.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/workflow_completion.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_outputs.go`
- `cmd/controller/workflow_stage_queue.go`
- `internal/workflow/compile_stage.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/workflow_completion_test.go`
- `cmd/controller/workflow_dependency_store_test.go`
- `cmd/controller/workflow_outputs_test.go`
- `cmd/controller/workflow_stage_queue_test.go`

## Out Of Scope

- Empty fan-out auto-advance beyond not creating synthetic work.
- Failure-propagation refinement beyond existing failed state.
- Resource constraints.
- Worker changes.
- CLI command changes.
- New expression language features.
- Cross-workflow dependencies.

## Acceptance Criteria

- Completing stage 0 of a two-stage sequential workflow queues stage 1 work items.
- Stage 1 is not queued before stage 0 completes.
- Completing stage 0 twice does not queue duplicate stage 1 work items.
- A downstream stage can reference `workflow.step[0]` and receive the completed prior step output.
- A downstream stage that references an unavailable future step fails compilation with a clear error and fails the workflow/submission.
- Completing the final stage transitions the workflow/submission to completed.
- Stage activation does not occur when the workflow/submission is already failed.
- Newly queued downstream work appears through the existing work assignment path.
- Existing worker-scaling/reconciliation hooks are invoked or left in a state where the existing scaler observes queued demand.

## Notes

- If the current controller completion endpoint cannot safely do all activation in one database transaction, make each transition idempotent and test duplicate handling.
- Do not compile every future stage to discover later errors early; dependency-aware compilation is intentionally just in time.

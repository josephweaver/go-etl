# 003 Persist Workflow Stage State

Status: Implemented

## Objective

Add controller-owned dependency state records for workflow instances, stages, steps, and compiled work-item membership without changing submission or assignment behavior yet.

## Current State

The controller already owns workflow submission, work assignment, completion/failure endpoints, and direct database access. After the Submission CLI Status concept, it also owns submission-scoped status records keyed by `submission_id`.

The controller does not yet have dependency-stage state that can say:

- which normalized stages belong to a submission;
- which steps belong to each stage;
- which concrete work items belong to each step;
- whether a stage is blocked, active, completed, or failed;
- whether a step has enough terminal work results to complete.

## Target State

The controller has a narrow persistence/state layer for dependency-aware workflow execution.

Use the existing controller database/store pattern. If the previous Submission CLI Status concept introduced a workflow/submission store file, extend that owner. If not, create a focused controller file such as `cmd/controller/workflow_dependency_store.go`.

The state layer should support at least:

```text
CreateWorkflowDependencyPlan(submission_id, workflow/run ids, normalized stages, retained workflow metadata)
ListWorkflowStages(submission_id)
ListWorkflowSteps(submission_id)
RecordCompiledWorkItemMembership(submission_id, stage_index, step_index, work_item_id, work_item_index)
ReadStepState(submission_id, step_index)
ReadStageState(submission_id, stage_index)
```

State enums may be strings, but must be centrally defined and tested. The minimum states are:

```text
workflow: running | completed | failed
stage: blocked | ready | active | completed | failed
step: blocked | ready | active | completed | failed
work item membership: queued | running | completed | failed | skipped
```

This slice should not wire the state layer into live submission behavior yet. It only creates the durable/owned state concept and tests it directly.

## Concept Decision

This slice adds a controller-owned dependency state concept. A new controller file is justified if no existing store file owns workflow-stage state.

Do not add this state to `internal/ledger`; the attempt ledger records attempts and evidence, while dependency readiness is controller orchestration state.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/001-normalize-workflow-stages.md`
- `docs/concepts/submission-cli-status/README.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/workflow/stage.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/main.go`
- `internal/model/workflow_dependency.go`

If the previous concepts already created a controller store file that owns submission/workflow persistence, modify that file instead of creating `workflow_dependency_store.go` and report the substitution.

## Allowed Test Files

- `cmd/controller/workflow_dependency_store_test.go`
- `cmd/controller/main_test.go`
- `internal/model/workflow_dependency_test.go`

## Out Of Scope

- Changing `POST /workflow` behavior.
- Changing `GET /work/next` behavior.
- Compiling only stage 0.
- Completion/failure endpoint changes.
- Step-output parsing.
- JIT compiling later stages.
- CLI changes.
- Worker changes.
- Observability.

## Acceptance Criteria

- Controller code defines workflow, stage, step, and work-item membership state records or equivalent persisted records.
- State records are keyed by `submission_id` and stage/step indexes.
- Stage and step state can be inserted and read back in deterministic order.
- Work-item membership can be inserted and read back with its original work-item index.
- Invalid state strings are rejected by validation or impossible through typed constants.
- The state layer prevents duplicate stage or step records for the same submission/index.
- The state layer prevents duplicate work-item membership for the same submission/work-item ID.
- Direct state-layer tests pass without relying on live HTTP submission.
- Existing controller tests still pass.

## Notes

- Do not rely on process-local maps for dependency correctness if the existing controller store is already database-backed.
- This slice does not need full controller restart recovery; it does need state that later slices can update idempotently.

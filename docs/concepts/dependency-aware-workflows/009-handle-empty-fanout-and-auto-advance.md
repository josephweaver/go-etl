# 009 Handle Empty Fan-Out And Auto-Advance

Status: Implemented

## Objective

Make zero-work-item stages complete successfully without synthetic attempts, then continue dependency activation until queued work, workflow completion, or failure is reached.


## Implementation Handoff Note

Use the actual file names and helper/store owners introduced by slices 001-004. Where this document names example files such as `workflow_dependency_store.go`, `workflow_completion.go`, or `workflow_stage_queue.go`, treat those as placeholders if the branch implementation chose different owners.

## Current State

A fan-out step can compile to zero work items when its fan-out expression resolves to an empty list. Stage-scoped compilation can represent an empty result, and submission/JIT compilation avoid creating synthetic work items.

Without explicit auto-advance behavior, a workflow can become stuck when stage 0 or a later stage produces no work items.

## Target State

When a compiled step produces zero work items:

```text
step state -> completed
step output -> []
no work-item membership records are inserted for that step
no attempt is created
no synthetic work item is queued
```

The empty output is a dependency **step** output. Store it where OS 007 stores logical step outputs so later compilation can resolve `workflow.step[index]` as an empty list. Do not store an empty-fanout output in `workflow_stages.output_json`, and do not create a stage-level wrapper for mixed parallel stages.

When every step in a stage has completed this way, the stage completes and activation continues to the next stage.

The same behavior must work for:

- empty stage 0 during initial submission;
- empty later stages during JIT activation;
- mixed parallel stages where one step produces zero work items and another step produces assignable work;
- chains of multiple empty stages.

Auto-advance must be bounded. It should not recurse indefinitely without a clear loop over finite normalized stages.

## Concept Decision

This slice updates the controller stage activation concept. Empty fan-out is a normal successful stage outcome, not an exceptional queue condition.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/002-compile-single-workflow-stage.md`
- `docs/concepts/dependency-aware-workflows/005-submit-only-initial-ready-stage.md`
- `docs/concepts/dependency-aware-workflows/007-capture-typed-step-outputs.md`
- `docs/concepts/dependency-aware-workflows/008-compile-next-ready-stage.md`
- `cmd/controller/workflow_completion.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_stage_queue.go`
- `cmd/controller/workflow_outputs.go`
- `internal/workflow/fanout.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/workflow_completion.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_stage_queue.go`
- `cmd/controller/workflow_outputs.go`
- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/workflow_completion_test.go`
- `cmd/controller/workflow_dependency_store_test.go`
- `cmd/controller/workflow_stage_queue_test.go`
- `cmd/controller/workflow_outputs_test.go`
- `cmd/controller/main_test.go`

## Out Of Scope

- New expression functions for list transformation.
- Synthetic attempts or synthetic work-item records.
- Worker changes.
- CLI changes.
- Resource constraints.
- Cross-workflow dependencies.

## Acceptance Criteria

- Submitting a workflow whose stage 0 fan-out is empty does not queue any work item for that step.
- The empty fan-out step completes with logical output `[]`.
- An empty stage 0 auto-advances to stage 1 when stage 1 exists.
- A chain of empty stages auto-advances until a non-empty stage is queued or the workflow completes.
- A parallel stage with one empty fan-out step and one non-empty fan-out step waits for the non-empty step before advancing.
- No synthetic work item is assigned to a worker for an empty fan-out step.
- No synthetic attempt is inserted for an empty fan-out step.
- Auto-advance is idempotent and does not queue duplicate downstream work.

## Notes

- Test empty fan-out using a resolver expression that returns an empty list.
- Keep auto-advance in the controller. Do not make workers aware of empty-stage transitions.
- When auto-advance reaches a later stage, generated `workflow.step` scope must include prior empty-step outputs as `[]` if those steps are before the stage being compiled.

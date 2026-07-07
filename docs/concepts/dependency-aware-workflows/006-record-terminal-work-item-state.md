# 006 Record Terminal Work Item State

Status: Complete

## Objective

Update dependency state when a worker reports work completion or failure, without yet compiling downstream stages.


## Implementation Handoff Note

Use the actual file names and helper/store owners introduced by slices 001-004. Where this document names example files such as `workflow_dependency_store.go`, `workflow_completion.go`, or `workflow_stage_queue.go`, treat those as placeholders if the branch implementation chose different owners.

## Current State

After slice 005, submission queues only stage 0 and records dependency membership for those queued work items.

The controller already has completion and failure endpoints such as `POST /work/complete` and `POST /work/fail`. These endpoints record attempt/evidence state, remove assigned work, and update existing submission status.

The dependency state introduced in slice 003 records which stage and step own each queued work item, but terminal work reports do not yet update that dependency state.

## Target State

When a work item reports completion:

```text
membership state -> completed or skipped
owning step completed count increments idempotently
owning step becomes completed when all expected work items are completed/skipped
owning stage becomes completed when all steps in that stage are completed
workflow remains running unless the completed stage was the last stage
```

When a work item reports failure:

```text
membership state -> failed
owning step -> failed
owning stage -> failed
workflow/submission -> failed
```

This slice should stop after recording dependency terminal state. It should not compile the next stage yet; that is slice 008.

The update should be idempotent. A duplicate terminal report for the same attempt/work-item should not double-increment counts or double-transition a step.

## Concept Decision

This slice updates the controller completion/failure concept. Dependency state remains controller-owned; workers still report only terminal work results.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/003-persist-workflow-stage-state.md`
- `docs/concepts/dependency-aware-workflows/004-stamp-work-items-with-step-instance-metadata.md`
- `docs/concepts/dependency-aware-workflows/005-submit-only-initial-ready-stage.md`
- `docs/concepts/submission-cli-status/README.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- the actual dependency-state owner created by 003
- the actual queue/membership helper created by 004
- `internal/model/work_item.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_completion.go`
- `internal/model/work_item.go`

If completion handling already lives in a named controller file after previous concepts, modify that owner instead of creating `workflow_completion.go` and report the substitution.

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/workflow_dependency_store_test.go`
- `cmd/controller/workflow_completion_test.go`

## Out Of Scope

- Parsing logical output JSON into typed step outputs.
- JIT compiling the next stage.
- Empty fan-out auto-advance.
- Worker changes.
- CLI changes.
- New status fields except minimal internal assertions needed by tests.
- Observability.
- Resource constraints.

## Acceptance Criteria

- Completing the only work item in a one-step workflow marks the step completed.
- Completing all work items in a fan-out step marks the step completed only after the last required item completes.
- Completing only some fan-out items leaves the step active or incomplete.
- Completing every step in a stage marks the stage completed.
- A duplicate completion report does not double-count the same work item.
- Failing a work item marks the owning work-item membership failed.
- Failing a work item marks the owning step and stage failed.
- Failing a work item marks the submission/workflow failed through the existing status model.
- No downstream stage is compiled or queued by this slice.

## Notes

- If completion and failure endpoint code is too large, create small helper functions and test them directly.
- Do not use pending queue emptiness as proof that a step completed; use recorded membership state.

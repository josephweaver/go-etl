# 005 Submit Only Initial Ready Stage

Status: Ready

## Objective

Change workflow submission so it normalizes the workflow, persists dependency state, compiles stage 0, and queues only stage 0 work items.

## Current State

Before this slice, workflow submission still compiles all workflow steps and queues all generated work items during `POST /workflow` or the equivalent submission path used by `goet submit`.

Slices 001 through 004 added stage normalization, stage-scoped compilation, dependency state records, and metadata stamping, but the live submission path has not yet been changed to use them.

## Target State

The live workflow submission path becomes dependency-aware at admission time:

```text
receive submission
  -> build workflow/project/submission resolver scopes as before
  -> normalize stages
  -> persist workflow/stage/step plan
  -> compile stage 0 only
  -> stamp stage 0 work items and membership records
  -> queue stage 0 work items only
  -> leave later stages blocked and uncompiled
  -> return the existing submission acknowledgement
```

If stage 0 compilation returns zero work items, this slice may leave the workflow with no queued work and allow slice 009 to add auto-advance behavior. It should not synthesize a fake work item.

Submission validation must reject invalid stage plans before queue mutation.

## Concept Decision

This slice updates the controller's existing workflow submission concept. Do not create a second submission endpoint.

The public API shape created by Submission CLI Status remains unchanged. The change is internal controller behavior plus status counts/state.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/001-normalize-workflow-stages.md`
- `docs/concepts/dependency-aware-workflows/002-compile-single-workflow-stage.md`
- `docs/concepts/dependency-aware-workflows/003-persist-workflow-stage-state.md`
- `docs/concepts/dependency-aware-workflows/004-stamp-work-items-with-step-instance-metadata.md`
- `docs/concepts/submission-cli-status/README.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/workflow/stage.go`
- `internal/workflow/compile_stage.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_stage_queue.go`
- `internal/workflow/stage.go`
- `internal/workflow/compile_stage.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/workflow_submission_test.go`
- `cmd/controller/workflow_stage_queue_test.go`
- `internal/workflow/compile_stage_test.go`

If controller submission tests already live under different file names, modify those tests instead and report the substitution.

## Out Of Scope

- Completion/failure endpoint changes.
- JIT compiling stage 1 after stage 0 completion.
- Output capture.
- Empty fan-out auto-advance.
- Status response enrichment beyond whatever existing tests require.
- CLI changes.
- Worker changes.
- Observability changes.

## Acceptance Criteria

- Submitting a two-step sequential workflow queues only stage 0 work items.
- Stage 1 work items from the same workflow are not present in the assignable queue immediately after submission.
- Submitting a parallel stage 0 queues work items for all steps in stage 0.
- Submitting an invalid non-contiguous `parallel_with` workflow returns a client-visible validation error before any dependency state or queue records are committed.
- Submission acknowledgement still returns the same public `submission_id` shape created by Submission CLI Status.
- Queue/status counts do not count blocked future-stage work as assignable pending work.
- Existing one-step workflow submissions still work.
- Existing source-admission and Python-workitem validation still run for queued stage 0 work items.

## Notes

- It is acceptable for later stages to remain uncompiled in this slice.
- Do not compile future stages merely to inspect their generated work-item IDs.
- If a test needs a two-step workflow fixture, keep it small and local to controller tests unless a later smoke slice moves it into demo docs.

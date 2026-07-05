# 010 Propagate Step And Workflow Failure

Status: Ready

## Objective

Make dependency failure transitions terminal and ensure downstream stages never activate after a work-item, step, stage, or output-capture failure.

## Current State

A work-item failure can mark dependency state failed. Stage activation can compile downstream stages after predecessor completion. Output capture and downstream compilation can also fail after a terminal work result.

The controller needs a single tested failure contract so late sibling completions, duplicate reports, and JIT compilation errors cannot reactivate a failed workflow.

## Target State

Any failure in a dependency-aware workflow produces a terminal failed submission/workflow state.

Failure sources include:

- worker-reported work-item failure;
- output JSON missing or invalid for a step that must expose output;
- downstream stage compilation failure;
- source-admission validation failure during downstream stage compilation;
- queue/membership conflict that prevents safe downstream activation.

Once a workflow is failed:

- no blocked future stage can become active;
- no downstream stage can be compiled;
- already-running sibling work items may report completion or failure;
- late sibling reports update their own terminal membership when safe, but do not change the workflow terminal state;
- `goet submit ... --wait` and status semantics treat the submission as failed.

## Concept Decision

This slice updates the dependency state machine and completion handling. It should centralize failure transition logic instead of scattering direct state updates across unrelated handlers.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/006-record-terminal-work-item-state.md`
- `docs/concepts/dependency-aware-workflows/007-capture-typed-step-outputs.md`
- `docs/concepts/dependency-aware-workflows/008-compile-next-ready-stage.md`
- `docs/concepts/submission-cli-status/README.md`
- `cmd/controller/main.go`
- `cmd/controller/workflow_completion.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_outputs.go`
- `cmd/controller/workflow_stage_queue.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/workflow_completion.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_outputs.go`
- `cmd/controller/workflow_stage_queue.go`
- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/workflow_completion_test.go`
- `cmd/controller/workflow_dependency_store_test.go`
- `cmd/controller/workflow_outputs_test.go`
- `cmd/controller/workflow_stage_queue_test.go`
- `cmd/controller/main_test.go`

## Out Of Scope

- Cancelling already-running sibling work items.
- Retrying failed work items.
- Attempt liveness recovery.
- Resource cleanup.
- New CLI commands.
- Rich failure UIs.
- Cross-workflow failure propagation.

## Acceptance Criteria

- A failed work item fails the owning step, stage, workflow, and submission.
- After a workflow is failed, completing another already-running sibling work item does not activate the next stage.
- After a workflow is failed, duplicate completion reports do not change the terminal failed state.
- Invalid output JSON during step-output capture fails the workflow and prevents downstream activation.
- Downstream stage compilation errors fail the workflow and prevent later retries from queueing partial downstream work.
- Failure reasons are recorded in the existing status model or dependency state so users can diagnose the failed step.
- `goet submit ... --wait` observes the failed terminal state through the existing submission status API.
- Existing success-path tests from earlier slices still pass.

## Notes

- If queue insertion partially succeeds before a compilation/activation error, prefer transaction rollback. If the current store cannot rollback, detect and handle partial state explicitly before marking failure.
- Do not mark an already-completed workflow failed due to a duplicate late failure for work that is not part of the active workflow state.

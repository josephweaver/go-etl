# 003 Persist Workflow Stage State

Status: Implemented on visible branch — preserve as regression checklist

## Objective

Preserve and verify the completed dependency-state persistence layer for workflow instances, stages, steps, and compiled work-item membership.

## Current State

This slice is visible as implemented on `concept/dependency-aware-workflows`.

Expected completed behavior includes a controller-owned state/persistence owner that can record:

- which normalized stages belong to a submission or workflow run;
- which steps belong to each stage;
- which concrete work items belong to each step;
- stage, step, and work-item membership state;
- enough state to evaluate completion idempotently after terminal work reports.

The actual owner may be `cmd/controller/workflow_dependency_store.go`, an `internal/persistence` package, or another file introduced by 003. Later slices must reuse that owner and should not create a duplicate state layer.

## Target State

No new target behavior is expected beyond preserving the completed 003 contract.

Before implementing 004 or later slices, verify that the state layer can represent both:

- a sequential stage containing one step;
- a `parallel_with` stage containing multiple steps, with per-step work-item membership and original fan-out order.

If the completed 003 implementation models only one step per stage, extend the existing store owner before 005 changes live submission behavior.

## Concept Decision

Dependency readiness remains controller-owned orchestration state.

Do not add this state to `internal/ledger`; the attempt ledger records attempts and evidence, while dependency readiness is controller orchestration state. Do not create a second controller store if 003 already introduced an owner.

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

No production changes are expected when verifying this completed slice.

Only modify the actual 003 state owner if review finds a gap. Expected candidate files include, but are not limited to:

- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/main.go`
- `internal/model/workflow_dependency.go`
- `internal/persistence/*.go` if 003 chose an internal persistence owner

## Allowed Test Files

No test changes are expected unless verification reveals a missing regression case.

Allowed verification-test files include the actual 003 store tests, for example:

- `cmd/controller/workflow_dependency_store_test.go`
- `cmd/controller/main_test.go`
- `internal/model/workflow_dependency_test.go`
- `internal/persistence/*_test.go` if 003 chose an internal persistence owner

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

## Verification Criteria

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

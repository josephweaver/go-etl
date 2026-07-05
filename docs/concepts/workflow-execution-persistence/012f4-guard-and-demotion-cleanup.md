# 012f4 Epic Closure And Boundary Cleanup

Status: implemented

## Objective

Close the workflow-execution-persistence epic by reconciling the queue-authority
documentation with the implemented source-reference controller path, replacing
the remaining skipped legacy inline-workflow tests where practical, and naming
the work that now belongs to follow-up epics.

When `Controller.workflowStore` is configured, the database is the queue
authority:

```text
queued_work     pending work
running_work    assigned work
completed_work  completed or skipped work
failed_work     failed work
```

The former in-memory fields `pending`, `assigned`, and `failed` have been
removed from `Controller`. No-store queue behavior is no longer a supported
fallback.

## Background

The earlier 012f cleanup plan named these atoms:

```text
012f-a Define workflow admission payload and provenance bridge
012f-b Persist admitted workflow run and initially ready compiled work
012f-c Make persisted workflow scaling demand derive from queued/running store counts
012f-d Add guard tests proving persisted paths do not mutate pending/assigned/failed
012f-e Remove or demote in-memory queue authority after no live store path uses it
```

The 012f3 source-reference admission work has now implemented the practical
equivalents of `012f-a`, `012f-b`, and `012f-c`.

This slice owned the remaining guard/demotion work.

## Required Context

Read these files first:

- `docs/concepts/workflow-execution-persistence/012f-remove-in-memory-queue-authority.md`
- `docs/concepts/workflow-execution-persistence/012f3-controller-source-reference-workflow-admission.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/persistence/store.go`

## Design Decision

The in-memory queue fields have already been removed. Closure work should not
reintroduce a no-store queue fallback or add a replacement in-memory helper.

The current controller contract is:

```text
workflowStore != nil = persisted queue authority
workflowStore == nil = queue endpoints unavailable
```

## Implementation Shape

Implement as small atoms:

```text
012f4-a Remove in-memory queue fields from Controller [implemented]
012f4-b Replace skipped legacy inline workflow tests with source-reference coverage [implemented]
012f4-c Reconcile epic and slice statuses [implemented]
012f4-d Document moved/deferred follow-up epics [implemented]
```

Each atom should leave `go test ./cmd/controller` passing. Run `go test ./...`
after the final atom.

## 012f4-a Remove In-Memory Queue State

Remove `pending`, `assigned`, and `failed` from `Controller`. The controller no
longer has a process-local queue fallback; it requires the workflow-execution
store for queue endpoints.

Acceptance criteria:

- The controller has no `pending`, `assigned`, or `failed` fields.
- Store-configured paths are the only active queue implementation.
- Queue endpoints return service unavailable when `workflowStore` is absent.

Implementation note:

- The initial conservative label-only implementation was replaced by direct
  removal after review.
- Tests that still described legacy inline `/workflow` submission were skipped
  with explicit notes. They should be replaced by source-reference based worker
  startup and scaling coverage.

## 012f4-b Replace Skipped Inline Workflow Tests

Replace skipped tests that still describe legacy inline `/workflow` JSON with
tests for the current source-reference contract.

Prioritize the smallest tests first:

- malformed or incomplete source-reference submissions;
- configured execution-environment startup from persisted demand;
- submitted worker-scale variables carried through source-reference admission;
- duplicate generated work-item IDs through source-loaded workflow fixtures.

Acceptance criteria:

- Each replaced test exercises source-reference `/workflow` admission with a
  configured `workflowStore`.
- Tests no longer depend on `Controller.pending`, `Controller.assigned`, or
  `Controller.failed`.
- Tests that remain skipped include a specific reason and a named replacement
  path.

Implementation note:

- `TestSubmitWorkflowHandlerRejectsInvalidPayload` now covers an incomplete
  source-reference payload instead of skipped legacy inline JSON.
- The first worker startup/scaling cleanup converted these legacy inline tests
  to source-reference fixtures backed by `LocalSourceControlAdapter`:
  `TestSubmitWorkflowHandlerStartsConfiguredWorker`,
  `TestSubmitWorkflowHandlerUsesConfiguredSlurmJob`,
  `TestSubmitWorkflowHandlerStartsPlannedWorkerCount`,
  `TestSubmitWorkflowHandlerUsesSubmittedWorkerScaleConfig`, and
  `TestSubmitWorkflowHandlerWaitsForWorkerClaimBeforeOrganicScaleUp`.
- The two old skipped ledger-handler tests were retired instead of replaced.
  `TestCompleteWorkHandlerCompletesPersistedAttemptWhenWorkflowStoreConfigured`
  now covers the active persisted terminal-attempt write path, and
  `TestStatusHandlerReportsPersistedCountsWhenWorkflowStoreConfigured` covers
  the active status authority. Handler-written ledger attempt-variable counts
  are no longer part of controller behavior.
- The final inline cleanup converted the remaining skipped tests for general
  workflow submission, submitted code version, Singularity runtime, invalid
  worker scale config, and duplicate generated IDs. `cmd/controller/main_test.go`
  now has no skipped tests.

## 012f4-c Reconcile Epic And Slice Statuses

Update the epic status trail so it matches the implementation evidence.

Acceptance criteria:

- The README no longer presents GitHub/cache implementation as part of this
  epic's remaining scope.
- Implemented slices are marked consistently.
- Moved slices identify their destination epic.
- Any remaining incomplete work is either a small closure item or a named
  follow-up epic.

## 012f4-d Document Moved And Deferred Follow-ups

Record the boundary that prevents this epic from continuing to absorb adjacent
work.

Moved or deferred work:

- source-control resolution, GitHub behavior, local cache layout, cache pins,
  and materialization belong to the `Repository Source Resolution and Cache`
  Strategic Concept;
- heartbeat, caretaker recovery, abandoned-attempt requeue, and stale report
  fencing belong to `attempt-liveness-recovery`;
- sequential stage readiness, JIT downstream compilation, and typed step
  outputs belong to `dependency-aware-workflows`;
- terminal-row cleanup, archives, and retention policy belong to
  `controller-retention-cleanup`;
- broader plugin/source semantic fingerprints belong to the fingerprint and
  workflow-compilation follow-up designs.

Acceptance criteria:

- The closure docs name the next recommended epic.
- The persistence epic stops accepting new feature slices except narrowly scoped
  fixes required to close the documented boundary.

## Out Of Scope

- Removing all in-memory tests in one pass.
- Removing the old attempt ledger.
- Changing the database schema.
- Adding transaction bundling for source-reference admission.
- Implementing GitHub/cache behavior.
- Retry/requeue policy.
- Worker heartbeat or abandoned-attempt recovery.

## Closure Recommendation

The skipped legacy inline workflow tests have been replaced or explicitly
retired, so this Strategic Concept is ready for implementation review. The
recommended next Strategic Concept is `Repository Source Resolution and Cache`,
because current live admission
still depends on the temporary local source adapter and hard-coded demo mapping.

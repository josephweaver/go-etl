# 012f4 Guard And Demotion Cleanup

Status: partially implemented

## Objective

Make the controller's queue authority boundary explicit after source-reference
workflow admission moved onto the workflow-execution store.

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

This slice owns the remaining guard/demotion work.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/012f-remove-in-memory-queue-authority.md`
- `docs/epics/workflow-execution-persistence/012f3-controller-source-reference-workflow-admission.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/persistence/store.go`

## Design Decision

Do not remove `pending`, `assigned`, and `failed` in this slice.

Reason:

- no-store behavior still exists and is useful for small controller tests;
- many legacy tests construct controllers by seeding in-memory state;
- removing the fields would force broad test rewrites and distract from the
  persistence authority boundary.

Instead, demote them explicitly:

```text
workflowStore != nil = persisted queue authority
workflowStore == nil = queue endpoints unavailable
```

## Implementation Shape

Implement as small atoms:

```text
012f4-a Remove in-memory queue fields from Controller [implemented]
012f4-b Replace skipped legacy inline workflow tests with source-reference coverage
012f4-c Split status helpers by authority where still useful
012f4-d Document remaining removal criteria
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

## 012f4-b Persisted-Path Guard Tests

Add or consolidate guard tests proving store-configured controller paths leave
legacy memory queue state unchanged.

Coverage target:

```text
POST /workflow
POST /work
GET  /work/next
POST /work/complete
POST /work/fail
GET  /status
```

Guard setup:

- create a controller with `workflowStore` configured;
- seed legacy memory state with sentinel entries:

```text
pending:  memory-pending
assigned: memory-assigned
failed:   memory-failed
```

- perform the persisted endpoint operation;
- assert sentinel state is unchanged;
- assert persisted store state changed or was read as expected.

Acceptance criteria:

- Every persisted endpoint listed above has guard coverage.
- The tests fail if a store-configured endpoint appends to `pending`, assigns in
  `assigned`, deletes from `assigned`, or writes to `failed`.
- Status guard proves `GET /status` reports persisted counts, not sentinel
  memory counts.

## 012f4-c Split Status Helpers By Authority

Current status behavior already branches on `workflowStore`, but the authority
boundary should be easy to read.

Preferred implementation:

```go
func (c *Controller) status(ctx context.Context) (model.ControllerStatus, error) {
    if c.workflowStore != nil {
        return c.persistedStatus(ctx)
    }
    return c.legacyMemoryStatus(ctx)
}
```

Names are candidates. The important part is that the top-level status path
clearly delegates to persisted or legacy authority.

Acceptance criteria:

- Store-configured status code path does not inspect legacy memory queue state.
- No-store status code path remains unchanged behaviorally.
- Tests cover both branches.

## 012f4-d Remaining Removal Criteria

Document the conditions required to remove the in-memory fallback entirely.

Candidate criteria:

- all controller endpoint tests can use persisted fixtures or explicit no-store
  helper tests;
- old attempt ledger skip/reuse paths are either migrated or marked as separate
  legacy behavior;
- raw work admin/testing path has a persisted fixture helper;
- no production startup path constructs a no-store controller.

Acceptance criteria:

- `012f-remove-in-memory-queue-authority.md` records whether the end state is
  "temporarily demoted" or "ready for removal."
- Any remaining references to `pending`, `assigned`, and `failed` are either:
  - inside no-store fallback helpers;
  - inside tests that explicitly test no-store fallback; or
  - inside guard tests that verify persisted paths ignore them.

## Out Of Scope

- Removing all in-memory tests in one pass.
- Removing the old attempt ledger.
- Changing the database schema.
- Adding transaction bundling for source-reference admission.
- Implementing GitHub/cache behavior.
- Retry/requeue policy.
- Worker heartbeat or abandoned-attempt recovery.

## Ambiguity To Review

The main ambiguity is whether to rename the fields now or only comment them.

Recommendation:

- If the diff stays localized, introduce `legacyMemoryQueue`.
- If the rename fans out across too many tests, first add comments and guard
  tests, then do the struct rename as a separate cleanup slice.

The second ambiguity is whether `GET /status` should include legacy sentinel
counts for diagnostics when a store is configured. Recommendation: no. Once
`workflowStore` is configured, status should report persisted authority only.

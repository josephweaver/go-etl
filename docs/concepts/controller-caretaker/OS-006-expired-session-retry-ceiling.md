# OS-006: Expired-Session Retry Ceiling

Status: implemented

Current implementation:

- `expiredSessionRequeueLimit` permits three expired-session requeues per work
  item.
- `requeueExpiredSessionWork` counts matching abandonment reasons inside the
  recovery transaction and inserts `failed_work` instead of `queued_work` when
  the ceiling is exhausted.
- graceful `worker_stopped` recovery continues through the uncapped
  `requeueAbandonedWork` path.
- focused persistence tests cover three permitted requeues, terminal failure on
  the fourth expiry, terminal-query visibility, and graceful-stop requeue after
  three expiry abandonments.

Verification:

```text
go test ./internal/persistence -count=1
```

## Objective

Prevent a work item from being requeued forever when every assigned worker
session expires. Allow at most three `heartbeat_expired` requeues per work item,
then preserve the final abandonment as a terminal failed outcome.

## Current State

`internal/persistence/store.go` implements
`Store.RecoverExpiredWorkerSessions`. For every expired session assignment, the
transaction inserts an `abandoned_work` row, deletes the `running_work` row,
and unconditionally calls `requeueAbandonedWork`.

Because `abandoned_work` history is never consulted before that requeue, an
invalid work item can cycle through claim, worker exit, heartbeat expiry, and
requeue without a bound. The production `run-630e4bf8eca7e030a20416f381fd87ea`
demonstrated this with 16 work items and 6,373 abandoned attempts.

`Store.StopWorkerSessionAndRecoverWork` uses the same requeue helper for
graceful worker stops. A graceful stop is not lost-worker retry exhaustion and
must retain its existing requeue behavior.

## Target State

`Store.RecoverExpiredWorkerSessions` counts prior `abandoned_work` rows for the
same work item and expiry reason inside its existing transaction. It permits
three requeues caused by expired sessions. On the fourth expired-session
abandonment, it does not insert `queued_work`; it inserts a `failed_work` row
for the current attempt with a stable retry-ceiling error instead.

The final attempt remains in `abandoned_work` to preserve why assignment
ownership was fenced, and also appears in `failed_work` so workflow terminal
queries can observe that retry recovery is exhausted.

`worker_stopped` recovery remains uncapped and continues to requeue normally.

## Concept Decision

This slice updates the existing abandonment/requeue persistence concept owned
by `internal/persistence/store.go`. The retry check belongs in the same database
transaction as abandonment and requeue, so it does not require a new production
file or schema migration.

## Required Context

Read these files first:

- `docs/concepts/controller-caretaker/README.md`
- `docs/concepts/controller-caretaker/OS-003-assignment-fencing-abandonment-and-requeue.md`
- `internal/persistence/store.go`
- `internal/persistence/store_test.go`

Do not read unrelated files unless focused test failures directly require it.

## Allowed Production Files

- `internal/persistence/store.go`

## Allowed Test Files

- `internal/persistence/store_test.go`

## Out Of Scope

- Making the ceiling configurable in controller startup variables.
- Changing worker heartbeat timing or worker fetch-error shutdown behavior.
- Repairing or resubmitting the malformed production workflow.
- Modifying graceful `worker_stopped` requeue behavior.
- Automatically deploying the fix or changing production database records.

## Acceptance Criteria

- The first three expired-session abandonments for a work item may requeue it.
- The fourth expired-session abandonment is recorded but the work item is not
  requeued.
- The fourth attempt receives a `failed_work` record whose error states that
  the expired-session retry ceiling was reached.
- The final abandonment, running-work deletion, terminal failure, and absence
  of requeue commit atomically.
- A graceful stopped session can still requeue its owned work.
- Existing recovery tests and the new focused ceiling test pass.

## Notes

- The ceiling is three retries, meaning up to four total attempts: the initial
  attempt plus three requeued attempts.
- Counting only matching expiry abandonment reasons prevents planned worker
  stops from consuming the lost-worker retry budget.
- A configurable ceiling should be a later slice because it requires controller
  configuration and CareTaker adapter changes outside this emergency
  one-production-file boundary.

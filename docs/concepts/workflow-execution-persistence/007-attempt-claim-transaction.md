# 007 Attempt Claim Transaction

Status: proposed

## Objective

Replace in-memory assignment authority with a durable claim transaction. The
controller should be able to atomically move the oldest queued work item into an
active attempt, preserve the queue eligibility timestamp, and return the claimed
payload only after the database commit succeeds.

This feature will likely require multiple Review Atoms. Implement exactly one
Review Atom per implementation cycle until the acceptance criteria below are
fully implemented and reviewed.

## Required Context

Read these files first:

- `docs/concepts/AGENT_INSTRUCTIONS.md`
- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/concepts/workflow-execution-persistence/006-work-item-and-queue-persistence-methods.md`
- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/db_adapter_sqlite_test.go`
- `internal/persistence/store.go`
- `internal/persistence/store_test.go`

Do not read controller files unless compile or test failures directly require
it.

## Feature Acceptance Criteria

- `running_work` preserves the `queued_at` value from the claimed
  `queued_work` row.
- `Store` exposes a method to claim the oldest queued work item.
- Claiming work creates one `work_item_attempts` row.
- Claiming work creates one `running_work` row.
- Claiming work deletes the selected `queued_work` row.
- Claiming work returns the claimed work item payload and attempt identity only
  after commit succeeds.
- Claiming an empty queue returns a distinguishable no-work result.
- Claiming uses deterministic order: `queued_at`, then `work_item_id`.
- Claiming is atomic; failed insert/delete work must leave the queue unchanged.
- A work item cannot have two active `running_work` rows.
- Claim behavior is tested without controller HTTP wiring.

## First Review Atom

Selected Review Atom:
Add `queued_at` to `running_work` schema and schema tests.

Purpose:
Prepare the database for a later claim method that copies queue eligibility
evidence from `queued_work` into `running_work`.

In Scope:

- Add `queued_at TEXT NOT NULL` to the `running_work` schema.
- Update schema tests to verify the column exists.
- Keep existing running-work count behavior passing.
- Update focused documentation if the implemented schema differs from older
  notes.

Out of Scope:

- Claiming work.
- Creating attempts.
- Deleting `queued_work`.
- Returning claimed payloads.
- Worker registration.
- Completion, failure, retry, or terminal timestamp propagation.
- Controller startup, HTTP handler, or in-memory queue replacement.

Allowed Production Files:

- `internal/persistence/db_adapter_sqlite.go`

Allowed Test Files:

- `internal/persistence/db_adapter_sqlite_test.go`

Documentation Files:

- `docs/concepts/workflow-execution-persistence/007-attempt-claim-transaction.md`
- `epi_ctl/20260703.md`

## Later Review Atoms

Later atoms should be selected one at a time after reviewing the previous atom.
Likely candidates include:

- Add claim input/output structs and request validation without claim behavior.
- Add the claim transaction for the oldest queued row.
- Add empty-queue behavior.
- Add conflict/rollback tests for active-running constraints.

These candidates are planning notes, not authorization to implement multiple
atoms in one cycle.

## Notes

- The claim method should accept caller-supplied IDs and timestamps initially;
  UUIDv7 generation and clock ownership can remain outside this slice unless a
  later atom explicitly selects them.
- Worker identity should remain optional until worker registration is designed.
  The current schema permits controller-owned attempts through
  `executor_type = 'controller'`.
- No worker should receive a payload until the claim transaction has committed.

## Review Atom Notes

- The first Review Atom adds `queued_at` to `running_work`.
- The second Review Atom defines the claim request/result structs and validates
  caller-supplied attempt ID, executor type, and start timestamp without adding
  the claim transaction.
- The third Review Atom adds the `ClaimNextWork` method boundary, including
  request validation and an empty-queue no-work result, without adding the
  successful queue-to-running transition.
- The fourth Review Atom implements the successful claim transition for one
  queued row: insert attempt, insert running placement with copied `queued_at`,
  delete queue placement, commit, then return the claimed payload.
- The fifth Review Atom adds rollback/conflict coverage showing that a duplicate
  attempt ID fails the claim and leaves the selected work item queued with no
  running placement.
- The sixth Review Atom adds coverage for claiming with an existing worker ID,
  proving the claim records that worker on both the attempt and running
  placement without designing worker registration.

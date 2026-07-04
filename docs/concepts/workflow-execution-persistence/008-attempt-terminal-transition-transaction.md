# 008 Attempt Terminal Transition Transaction

Status: implemented

## Objective

Add persistence methods that terminate one active attempt atomically. Completion
records successful terminal evidence, failure records unsuccessful terminal
evidence, and both transitions remove the active `running_work` placement only
after the terminal row is durably written.

This feature completes the database lifecycle that 007 started:

```text
queued_work -> running_work -> completed_work
queued_work -> running_work -> failed_work
```

## Required Context

Read these files first:

- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/concepts/workflow-execution-persistence/007-attempt-claim-transaction.md`
- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/db_adapter_sqlite_test.go`
- `internal/persistence/store.go`
- `internal/persistence/store_test.go`

Do not read controller files unless compile or test failures directly require
it.

## Allowed Production Files

- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/store.go`

## Allowed Test Files

- `internal/persistence/db_adapter_sqlite_test.go`
- `internal/persistence/store_test.go`

## Documentation Files

- `docs/concepts/workflow-execution-persistence/008-attempt-terminal-transition-transaction.md`
- `epi_ctl/20260703.md`

## Acceptance Criteria

- Terminal tables preserve the timing evidence needed after `running_work` is
  removed.
- `completed_work` stores copied `queued_at`, copied `started_at`, and
  completion timestamp.
- `failed_work` stores copied `queued_at`, copied `started_at`, and failure
  timestamp.
- `Store` exposes a method to complete an active attempt.
- Completing an active attempt inserts one `completed_work` row.
- Completing an active attempt deletes the matching `running_work` row.
- Completion records `output_json`, `output_json_sha256`,
  `pre_state_sha256`, and `post_state_sha256`.
- Completion may record `skipped_parent_id` when the terminal row represents
  reuse of a prior completed result.
- Repeating an identical completion report for an already completed attempt is
  idempotent.
- Repeating a conflicting completion report for an already completed attempt
  fails.
- Completing a missing or non-running attempt returns a distinguishable failure.
- `Store` exposes a method to fail an active attempt.
- Failing an active attempt inserts one `failed_work` row.
- Failing an active attempt deletes the matching `running_work` row.
- Failure records an error string and failure timestamp.
- Repeating an identical failure report for an already failed attempt is
  idempotent.
- Repeating a conflicting failure report for an already failed attempt fails.
- Failing a missing or non-running attempt returns a distinguishable failure.
- Terminal transitions are atomic; failed terminal insertion must leave
  `running_work` unchanged.
- Terminal transition behavior is tested without controller HTTP wiring.

## Out Of Scope

- Retry and requeue behavior.
- Stage completion or failure state updates.
- Publishing downstream ready work.
- Worker liveness, heartbeat, or abandoned-attempt recovery.
- Controller HTTP handler integration.
- Artifact storage.
- Source-control cache behavior.
- UUIDv7 generation.
- Canonical JSON computation.
- Retention cleanup.

## Notes

- The epic README is the implementation authority when older working notes
  differ from this slice.
- Keep the current `completed_at` and `failed_at` column names unless the
  implementation exposes a concrete reason to rename them.
- `queued_at` and `started_at` must be copied from `running_work` before the
  running row is deleted.
- Completion/failure methods should accept caller-supplied timestamps and
  hashes. Timestamp generation and canonical hashing are owned by caller-side
  lifecycle code for now.
- This feature should not decide whether a failed attempt is retryable. Retry
  and requeue policy belongs in a later feature.

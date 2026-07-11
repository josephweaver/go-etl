# OS-003: Assignment Fencing, Abandonment, and Requeue

## Status

Implementation in progress.

Current implementation:

- `/work/next` receives worker/session identity from OS-002 and now passes a heartbeat cutoff into persistence.
- `ClaimNextWork` validates the matching worker session is active, not expired relative to the supplied cutoff, and not already assigned running work before selecting or mutating queued work.
- Controller claim failures for inactive/expired sessions and busy sessions return `409 Conflict`.
- completion and failure payloads carry worker/session identity, worker clients fill those fields from the active session, and persistence rejects mismatched, stopped, dead, or expired assignment owners before inserting terminal records.
- `RecoverExpiredWorkerSessions` atomically marks expired active sessions dead, records owned running attempts as abandoned, removes running assignments, and requeues abandoned work items.
- `StopWorkerSessionAndRecoverWork` atomically marks an active session stopped, records any owned running attempts as abandoned with `worker_stopped`, removes running assignments, and requeues the work items.

Remaining OS-003 work:

- deterministic race tests.

## Minimum capable model

Use **GPT-5.5** with **Extra High** reasoning.

This is the highest-risk correctness slice. It changes claim ownership and creates races among heartbeat, expiry, completion, failure, and stop. Do not assign the primary implementation to a small model.

Use **GPT-5.3-codex-spark** only for isolated additional tests after transaction semantics are established.

## Goal

Bind every worker assignment to a live worker session and guarantee that a dead, stopped, expired, or mismatched session cannot claim or report outcomes.

When a session expires or stops while owning work, atomically:

1. terminate the session;
2. preserve the attempt as abandoned;
3. remove its running assignment; and
4. return the work item to `queued_work`.

## Scope

### In scope

- Worker/session identity on `/work/next`.
- Worker/session identity in completion and failure payloads.
- Live-session validation during claim.
- Assignment ownership validation during complete/fail.
- Atomic expiry + abandonment + requeue persistence operation.
- Atomic stop + abandonment + requeue extension.
- Late-outcome 409 behavior.
- Worker handling of assignment-not-owned.
- Transaction and race tests.

### Out of scope

- CareTaker goroutine.
- Dynamic wake timers.
- Automatic worker launch.
- API scheduler cutover.
- Multiple concurrent assignments per worker.
- Arbitrary task cancellation.
- Retry-budget policy redesign.

## Preferred file budget

```text
internal/persistence/store.go
internal/persistence/store_test.go
cmd/controller/main.go
cmd/controller/worker_lifecycle.go
cmd/controller/worker_assignment.go
cmd/controller/*_test.go
cmd/worker/state.go
cmd/worker/main.go
internal/model/*
```

Create focused files for ownership/recovery rather than adding all logic to `main.go`.

## Claim identity contract

Keep the current route for this slice:

```http
GET /work/next
```

Require:

```text
X-GORC-Worker-ID
X-GORC-Worker-Session-ID
```

The worker client sets both headers from its registration.

Missing identity:

```text
400 Bad Request
worker_identity_required
```

Unknown, mismatched, stopped, dead, or expired session:

```text
409 Conflict
worker_session_not_active
```

The claim transaction must validate live state using controller current time and configured `worker_dead_after`.

Do not only check `status = active`; a session past its heartbeat cutoff is not permitted to claim even if the CareTaker has not marked it dead yet.

## Claim transaction

Extend `ClaimNextWork` or add a worker-specific claim method that performs one transaction:

```text
1. Verify worker/session identity matches.
2. Verify status=active.
3. Verify last_heartbeat_at >= cutoff.
4. Verify the session owns no current running assignment.
5. Select one claimable queued work item.
6. Insert work_item_attempts with worker_id and worker_session_id.
7. Delete queued_work row.
8. Insert running_work with worker_id and worker_session_id.
9. Commit.
```

If no work is claimable, return no work without ending the session. The worker then follows OS-002 graceful-stop behavior.

The transaction must preserve existing resource-constraint claim semantics.

## Completion/failure wire changes

Add ownership fields:

```go
type WorkCompletion struct {
    // existing fields...
    WorkerID        string `json:"worker_id"`
    WorkerSessionID string `json:"worker_session_id"`
}

type WorkFailure struct {
    // existing fields...
    WorkerID        string `json:"worker_id"`
    WorkerSessionID string `json:"worker_session_id"`
}
```

The worker client fills them from the active registration, not from workflow parameters.

## Outcome transaction predicate

Completion and failure must validate inside their terminal-state transaction:

```text
running_work.attempt_id = request.attempt_id
running_work.work_item_id = request.work_item_id
running_work.worker_id = request.worker_id
running_work.worker_session_id = request.worker_session_id
worker_sessions.status = active
worker_sessions.last_heartbeat_at >= cutoff
```

Then:

- completion moves running -> completed;
- failure moves running -> failed.

If the predicate does not match, distinguish:

```text
not found / no longer owned -> 409 assignment_no_longer_owned
malformed request -> 400
persistence fault -> 500
```

Do not reveal another worker's identity in the error response.

## Atomic dead-session recovery

Add one store operation similar to:

```go
type RecoverExpiredWorkerSessionsRequest struct {
    Cutoff      string
    RecoveredAt string
    Reason      string
}

type RecoverExpiredWorkerSessionsResult struct {
    ExpiredSessions  int
    AbandonedAttempts int
    RequeuedWorkItems int
}

func (s *Store) RecoverExpiredWorkerSessions(
    ctx context.Context,
    request RecoverExpiredWorkerSessionsRequest,
) (RecoverExpiredWorkerSessionsResult, error)
```

One transaction:

```text
expired = active sessions with last_heartbeat_at < cutoff

for each expired session:
    UPDATE worker_sessions
       SET status='dead', ended_at=recovered_at, end_reason=reason
     WHERE session_id=...
       AND status='active'
       AND last_heartbeat_at < cutoff

    only if update affected one row:
        for each running assignment owned by session:
            INSERT abandoned_work(...)
            DELETE running_work WHERE attempt_id=... AND session_id=...
            INSERT queued_work(work_item_id, queued_at=recovered_at)
                ON CONFLICT(work_item_id) DO NOTHING
```

Commit after every selected session and assignment has been processed, preferably as one transaction for the recovery batch.

If any insert/delete fails, roll back the entire batch.

Use a stable reason:

```text
heartbeat_expired
```

Do not insert into `failed_work`.

## Atomic graceful stop with work

Extend stop persistence with a transaction:

```go
StopWorkerSessionAndRecoverWork(...)
```

Normal no-work stop produces:

```text
active -> stopped
abandoned=0
requeued=0
```

If a stop request arrives while the session owns work:

```text
active -> stopped
running -> abandoned
work item -> queued
reason=worker_stopped
```

This makes explicit stop safe under shutdown races.

A duplicate stop after the first transaction is idempotent and does not duplicate abandoned history or queue rows.

## Fencing behavior

### Dead/stopped session heartbeat

Return 409. Never reactivate.

### Dead/stopped/expired session claim

Return 409 before changing the queue.

### Late completion/failure

Return 409 and preserve:

```text
abandoned_work exists
queued_work exists or a newer attempt owns the work
completed_work does not contain stale attempt
failed_work does not contain stale attempt
```

### Worker behavior on 409 outcome

The worker logs that assignment ownership was lost, cancels heartbeat, does not retry the same outcome indefinitely, and exits nonzero or with a distinct fenced result.

Do not automatically claim new work with the stale session.

## Race requirements

### Heartbeat wins before expiry

1. heartbeat transaction updates `last_heartbeat_at`;
2. expiry transaction's strict `< cutoff` predicate does not match;
3. session remains active;
4. assignment remains running.

### Expiry wins before heartbeat

1. expiry marks session dead and recovers work;
2. heartbeat update predicate `status=active` affects zero rows;
3. heartbeat returns 409;
4. session remains dead.

### Completion wins before expiry

1. completion validates live ownership and removes running row;
2. expiry may mark session dead later;
3. no running assignment exists to abandon;
4. completed result remains valid.

### Expiry wins before completion

1. expiry moves running -> abandoned + queued;
2. completion ownership predicate finds no running assignment;
3. completion returns 409;
4. no stale completed row is inserted.

### Stop versus completion

The same transaction ordering rule applies. Exactly one terminal transition succeeds.

## Retry/attempt semantics

Abandonment creates a historical attempt but does not mark the work item failed.

A later claim creates a new attempt ID.

Example:

```text
work item W
  attempt A1: running -> abandoned (session S1 dead)
  attempt A2: running -> completed (session S2)
```

Do not reuse `A1`.

If a future infrastructure retry limit is added, abandoned attempts may feed that policy, but this slice does not invent a new limit.

## Controller integration without CareTaker

Add a narrow controller method for test/manual invocation:

```go
RecoverExpiredWorkerSessions(ctx, now)
```

It resolves `worker_dead_after`, computes cutoff, calls the atomic store method, and returns the summary.

Do not start a polling goroutine in this slice. OS-004 will call it.

After a recovery or stop transaction requeues work, invoke the state-change signal seam after commit.

## Tests

### Persistence transaction tests

```text
TestClaimRequiresLiveMatchingWorkerSession
TestClaimRejectsExpiredSessionBeforeCaretakerMarksDead
TestClaimEnforcesOneRunningAssignmentPerSession
TestClaimRecordsWorkerAndSessionOnAttempt
TestCompleteRequiresMatchingAssignmentOwner
TestFailRequiresMatchingAssignmentOwner
TestRecoverExpiredSessionMarksDead
TestRecoverExpiredSessionRecordsAbandonedAttempt
TestRecoverExpiredSessionRemovesRunningWork
TestRecoverExpiredSessionRequeuesWork
TestRecoverExpiredSessionRollsBackAllChangesOnFailure
TestRecoverExpiredSessionIsIdempotent
TestStopSessionWithRunningWorkAbandonsAndRequeues
TestDuplicateStopDoesNotDuplicateAbandonment
```

### Controller tests

```text
TestNextWorkRequiresWorkerIdentityHeaders
TestNextWorkRejectsDeadSession
TestNextWorkRejectsExpiredSession
TestCompleteReturnsConflictForAbandonedAttempt
TestFailReturnsConflictForAbandonedAttempt
TestLateOutcomeDoesNotCreateCompletedOrFailedRecord
TestRecoverySignalsStateChangeAfterCommit
```

### Race tests

Use coordinated transactions/fakes where possible:

```text
TestHeartbeatBeforeExpiryKeepsSessionActive
TestExpiryBeforeHeartbeatRejectsHeartbeat
TestCompletionBeforeExpiryIsNotAbandoned
TestExpiryBeforeCompletionRejectsCompletion
TestStopAndCompletionHaveSingleWinner
```

Do not rely on arbitrary sleeps for race ordering.

### Worker tests

```text
TestWorkerAddsSessionIdentityToClaim
TestWorkerAddsSessionIdentityToCompletion
TestWorkerAddsSessionIdentityToFailure
TestWorkerExitsWhenAssignmentOwnershipLost
```

## Implementation sequence

1. Add session identity headers to worker claim requests.
2. Extend claim request/store records.
3. Add live-session validation to claim transaction.
4. Add worker/session fields to outcome models.
5. Add ownership predicates to completion/failure transactions.
6. Add abandoned-work insert helpers private to the transaction.
7. Implement expired-session recovery transaction.
8. Extend stop to use the same recovery primitive.
9. Add stable conflict errors and handler mapping.
10. Add deterministic transaction/race tests.
11. Run all persistence, controller, and worker tests.

## Acceptance criteria

1. Only a live matching session can claim work.
2. One session cannot own more than one running assignment.
3. Claims persist worker and session identity.
4. Only the live owner can complete or fail an assignment.
5. Expired/stopped/dead sessions cannot report outcomes.
6. Expiry atomically marks dead, records abandonment, removes running state, and requeues work.
7. Stop with work uses the same atomic recovery behavior.
8. Recovery is idempotent.
9. A late completion or failure returns 409 and creates no stale terminal record.
10. A requeued item receives a new attempt ID on its next claim.
11. No polling CareTaker or scheduler cutover is included yet.

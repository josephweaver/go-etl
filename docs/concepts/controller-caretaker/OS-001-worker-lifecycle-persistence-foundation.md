# OS-001: Worker Lifecycle Persistence Foundation

## Status

Implemented pending review.

## Minimum capable model

Use **GPT-5.5** with **High** reasoning.

This slice is schema- and transaction-sensitive but is contained within persistence. A test-only follow-up may use **GPT-5.3-codex-spark** with **Medium** reasoning.

## Goal

Create the durable persistence foundation for:

- logical workers;
- worker sessions;
- live-session queries;
- session terminal states;
- assignment session ownership; and
- abandoned-attempt history.

This slice does not add HTTP endpoints, a heartbeat goroutine, dead-worker reconciliation, or the CareTaker loop.

## User-visible outcome

None yet. The repository gains an implementation-ready schema and persistence API that later slices can expose safely.

## Scope

### In scope

- Advance the persistence schema version once.
- Add `worker_sessions`.
- Add `worker_session_id` to worker attempts and running assignments.
- Add `abandoned_work`.
- Add indexes and SQLite constraints.
- Add record/request/result types.
- Add persistence methods needed by later slices.
- Add focused schema and store tests.
- Define old-schema behavior explicitly.

### Out of scope

- Controller routes.
- Worker client changes.
- Heartbeat scheduling.
- Work claim fencing.
- Completion/failure fencing.
- Expiry/requeue transaction.
- CareTaker logic.
- Worker launch logic.

## Preferred file budget

```text
internal/persistence/db_adapter_sqlite.go
internal/persistence/store.go
internal/persistence/db_adapter_sqlite_test.go
internal/persistence/store_test.go
```

A narrowly named additional persistence test file is allowed.

Do not modify controller handlers or worker code in this slice.

## Schema version

Advance:

```text
SupportedSchemaVersion: 5 -> 6
```

Do not silently accept a version-5 core schema as version 6.

Use one of the repository-approved behaviors:

1. explicit version-5-to-6 migration; or
2. fail closed with a clear development database rebuild instruction.

Do not destructively rebuild a populated core schema without an explicit test and documented policy.

## Proposed schema

### Preserve logical workers

Keep the existing `workers` table as the logical identity root.

The existing nullable `run_id` should not be used to limit a worker to one workflow run. Shared workers can process work across runs.

### Add worker sessions

```sql
CREATE TABLE worker_sessions (
    worker_session_id TEXT PRIMARY KEY,
    worker_id TEXT NOT NULL,
    status TEXT NOT NULL
        CHECK (status IN ('active', 'stopped', 'dead')),
    registered_at TEXT NOT NULL,
    last_heartbeat_at TEXT NOT NULL,
    ended_at TEXT,
    end_reason TEXT,
    execution_handle TEXT,

    FOREIGN KEY (worker_id) REFERENCES workers(worker_id),

    CHECK (
        (status = 'active' AND ended_at IS NULL)
        OR
        (status IN ('stopped', 'dead') AND ended_at IS NOT NULL)
    )
);
```

Recommended index:

```sql
CREATE INDEX idx_worker_sessions_status_heartbeat
ON worker_sessions(status, last_heartbeat_at);
```

Recommended worker-history index:

```sql
CREATE INDEX idx_worker_sessions_worker_registered
ON worker_sessions(worker_id, registered_at);
```

### Bind attempts to sessions

Add:

```text
work_item_attempts.worker_session_id TEXT
running_work.worker_session_id TEXT
```

Both reference `worker_sessions(worker_session_id)`.

For worker-executed attempts, enforce:

```text
worker_id is not null
worker_session_id is not null
```

For controller-executed attempts, allow both to be null.

If SQLite table-level checks are required, encode the executor-type relationship explicitly.

### Enforce one running assignment per session

Add a unique index:

```sql
CREATE UNIQUE INDEX idx_running_work_one_per_worker_session
ON running_work(worker_session_id)
WHERE worker_session_id IS NOT NULL;
```

This expresses the initial one-assignment-per-worker contract.

### Add abandoned attempt history

```sql
CREATE TABLE abandoned_work (
    attempt_id TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL,
    worker_id TEXT NOT NULL,
    worker_session_id TEXT NOT NULL,
    queued_at TEXT NOT NULL,
    started_at TEXT NOT NULL,
    abandoned_at TEXT NOT NULL,
    reason TEXT NOT NULL,

    FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
    FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
    FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
    FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id)
);
```

Recommended index:

```sql
CREATE INDEX idx_abandoned_work_item_time
ON abandoned_work(work_item_id, abandoned_at, attempt_id);
```

An abandoned attempt is history. The work item's current state may simultaneously be queued again.

## Persistence types

Add types similar to:

```go
type WorkerRecord struct {
    ID              string
    ExecutionHandle string
    CreatedAt       string
}

type WorkerSessionRecord struct {
    ID              string
    WorkerID        string
    Status          string
    RegisteredAt    string
    LastHeartbeatAt string
    EndedAt         string
    EndReason       string
    ExecutionHandle string
}

type RegisterWorkerSessionRequest struct {
    WorkerID        string
    SessionID       string
    RegisteredAt    string
    ExecutionHandle string
}

type HeartbeatWorkerSessionRequest struct {
    WorkerID     string
    SessionID    string
    HeartbeatAt  string
}

type EndWorkerSessionRequest struct {
    WorkerID  string
    SessionID string
    Status    string
    EndedAt   string
    Reason    string
}

type AbandonedWorkRecord struct {
    AttemptID      string
    WorkItemID     string
    WorkerID       string
    WorkerSessionID string
    QueuedAt       string
    StartedAt      string
    AbandonedAt    string
    Reason         string
}
```

Use repository naming conventions even if exact field names differ.

Update:

```go
type ClaimWorkRequest struct {
    AttemptID       string
    WorkerID        string
    WorkerSessionID string
    ExecutorType    string
    StartedAt       string
}
```

Also surface `WorkerSessionID` through claimed/running records.

Do not yet change completion/failure requests in this slice unless needed to keep persistence record shapes consistent.

## Required persistence methods

Add narrow methods that later slices can compose.

### Register

```go
RegisterWorkerSession(
    ctx context.Context,
    request RegisterWorkerSessionRequest,
) (WorkerSessionRecord, error)
```

Required behavior:

- validate IDs and timestamp;
- insert the logical worker if it does not exist;
- reject reuse of an existing session ID with different values;
- insert active session;
- set `registered_at` and `last_heartbeat_at` to the same controller-provided time;
- commit atomically.

### Heartbeat update

```go
HeartbeatWorkerSession(
    ctx context.Context,
    request HeartbeatWorkerSessionRequest,
) (updated bool, error)
```

Required SQL predicate:

```text
worker_id matches
session_id matches
status = active
```

Do not update dead/stopped sessions.

### Read one session

```go
GetWorkerSession(
    ctx context.Context,
    workerID string,
    sessionID string,
) (WorkerSessionRecord, found bool, error)
```

### List/count live sessions

```go
ListLiveWorkerSessions(
    ctx context.Context,
    cutoff time.Time,
) ([]WorkerSessionRecord, error)
```

or equivalent count + deadline queries:

```go
CountLiveWorkerSessions(ctx, cutoff)
EarliestActiveWorkerHeartbeat(ctx)
```

Live predicate:

```text
status = active
and last_heartbeat_at >= cutoff
```

### List expired sessions

```go
ListExpiredWorkerSessions(
    ctx context.Context,
    cutoff time.Time,
) ([]WorkerSessionRecord, error)
```

Expired predicate:

```text
status = active
and last_heartbeat_at < cutoff
```

This query is observational. OS-003 will add the atomic state transition.

### End session

```go
EndWorkerSession(
    ctx context.Context,
    request EndWorkerSessionRequest,
) (changed bool, error)
```

For this slice, it may only end sessions that have no running assignment. OS-003 will replace/extend this with atomic assignment recovery.

Allowed terminal status:

```text
stopped
dead
```

Never update a terminal session back to active.

### List abandoned history

```go
ListAbandonedWorkForItem(
    ctx context.Context,
    workItemID string,
) ([]AbandonedWorkRecord, error)
```

A direct insert method may be omitted until OS-003 so callers cannot create non-atomic abandonment.

## Validation rules

- worker ID required;
- session ID required;
- timestamps parse as RFC3339/RFC3339Nano according to repository convention;
- session status is only active/stopped/dead;
- active session has no terminal time;
- terminal session has terminal time and reason;
- worker attempts require matching worker/session IDs;
- controller attempts do not invent worker/session IDs;
- empty execution handle is allowed;
- session IDs are never reused.

## Data-state examples

### Registration

Before:

```text
workers: no worker-1
worker_sessions: no session-1
```

After one transaction:

```text
workers:
  worker-1

worker_sessions:
  session-1
  worker=worker-1
  status=active
  registered_at=t0
  last_heartbeat_at=t0
```

### Heartbeat

Before:

```text
session-1 active last_heartbeat=t0
```

After:

```text
session-1 active last_heartbeat=t1
```

No new session or worker row is created.

### Terminal session

Before:

```text
session-1 active
```

After:

```text
session-1 stopped
ended_at=t2
end_reason=no_work
```

A later heartbeat update affects zero rows.

## Tests

### Schema tests

```text
TestSQLiteSchemaVersionSix
TestSQLiteSchemaContainsWorkerSessions
TestSQLiteSchemaContainsAbandonedWork
TestWorkerSessionStatusConstraint
TestActiveWorkerSessionRequiresNullEndedAt
TestTerminalWorkerSessionRequiresEndedAt
TestRunningWorkAllowsOnlyOneAssignmentPerSession
TestWorkerAttemptRequiresSessionForWorkerExecutor
TestControllerAttemptAllowsNullWorkerSession
```

### Store tests

```text
TestRegisterWorkerSessionCreatesWorkerAndActiveSession
TestRegisterWorkerSessionRejectsSessionIDReuse
TestHeartbeatWorkerSessionUpdatesActiveSession
TestHeartbeatWorkerSessionDoesNotReviveDeadSession
TestHeartbeatWorkerSessionDoesNotReviveStoppedSession
TestListLiveWorkerSessionsUsesInclusiveCutoff
TestListExpiredWorkerSessionsUsesStrictCutoff
TestEndWorkerSessionIsIdempotentForSameTerminalState
TestEndWorkerSessionDoesNotRewriteDeadAsStopped
TestListAbandonedWorkReturnsAttemptHistory
```

### Old schema behavior

Add one test proving the selected policy:

```text
TestOpenVersionFiveStoreMigratesToSix
```

or:

```text
TestOpenVersionFiveStoreFailsWithRebuildInstruction
```

Do not leave old-version behavior implicit.

## Implementation sequence

1. Add schema statements and indexes.
2. Advance the schema version.
3. Add persistence record/request types.
4. Update claim/running record shapes with session ID.
5. Implement register/read/heartbeat queries.
6. Implement live/expired queries.
7. Implement terminal-session update for sessions without running work.
8. Add abandoned-history read shape.
9. Add constraints and old-schema tests.
10. Run all persistence tests.

## Acceptance criteria

1. Schema version 6 has durable worker sessions.
2. Live and expired sessions are distinguished by controller-provided cutoff.
3. Dead/stopped sessions cannot be heartbeated back to active.
4. Worker attempts and running assignments can record session ownership.
5. One worker session cannot own two running assignments.
6. Abandoned attempts have a durable history table.
7. Version-5 schema handling is explicit and tested.
8. No controller route or worker behavior changes are included.

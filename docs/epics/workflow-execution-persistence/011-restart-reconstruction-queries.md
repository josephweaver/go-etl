# 011 Restart Reconstruction Queries

Status: proposed

## Objective

Add read-only persistence queries that expose the durable execution views needed
after controller restart. The controller should be able to inspect active runs,
queued work, running attempts, terminal attempts, and aggregate run state from
the database without rebuilding or trusting a separate in-memory queue.

This slice does not perform controller startup recovery. It only adds the store
query boundaries that later restart and controller-integration code can call.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/README.md`
- `docs/epics/workflow-execution-persistence/006-work-item-and-queue-persistence-methods.md`
- `docs/epics/workflow-execution-persistence/007-attempt-claim-transaction.md`
- `docs/epics/workflow-execution-persistence/008-attempt-terminal-transition-transaction.md`
- `docs/epics/workflow-execution-persistence/009-stage-completion-and-ready-work-publication.md`
- `internal/persistence/store.go`
- `internal/persistence/store_test.go`

Do not read controller HTTP files unless compile or test failures directly
require it.

## Allowed Production Files

- `internal/persistence/store.go`

## Allowed Test Files

- `internal/persistence/store_test.go`

## Documentation Files

- `docs/epics/workflow-execution-persistence/011-restart-reconstruction-queries.md`
- `PROJECT_STATE.md`
- `epi_ctl/20260703.md`

## Proposed API Shape

The store already has some reconstruction pieces:

```go
ListActiveWorkflowRuns(ctx)
ListQueuedWorkItems(ctx)
CountWorkItemsForStage(ctx, runID, stageIndex)
GetWorkflowStage(ctx, runID, stageIndex)
GetWorkItem(ctx, workItemID)
```

This slice should add the missing durable views:

```go
type RunningWorkRecord struct {
    AttemptID    string
    WorkItem     WorkItemRecord
    WorkerID     string
    ExecutorType string
    QueuedAt     string
    StartedAt    string
}

type TerminalAttemptRecord struct {
    AttemptID        string
    WorkItem         WorkItemRecord
    TerminalState    string
    WorkerID         string
    ExecutorType     string
    QueuedAt         string
    StartedAt        string
    FinishedAt       string
    Error            string
    SkippedParentID  string
    OutputJSON       string
    OutputJSONSHA256 string
    PreStateSHA256   string
    PostStateSHA256  string
}

type RunWorkStatusCounts struct {
    Queued    int
    Running   int
    Completed int
    Failed    int
}

func (s *Store) ListRunningWork(ctx context.Context) ([]RunningWorkRecord, error)
func (s *Store) GetRunningWork(ctx context.Context, attemptID string) (RunningWorkRecord, bool, error)
func (s *Store) ListTerminalAttemptsForRun(ctx context.Context, runID string) ([]TerminalAttemptRecord, error)
func (s *Store) CountWorkItemsForRun(ctx context.Context, runID string) (RunWorkStatusCounts, error)
```

Names are candidates. The implementation should choose names that fit the
existing store style.

## Ordering Rules

Restart views should be deterministic:

- queued work remains ordered by `queued_at`, then `work_item_id`;
- running work should be ordered by `started_at`, then `attempt_id`;
- terminal attempts should be ordered by terminal timestamp, then `attempt_id`;
- run counts should be derived from placement and terminal tables, not stored.

## Acceptance Criteria

- `Store` exposes a method to list all running work with work-item payload,
  attempt identity, executor type, worker ID, copied `queued_at`, and
  `started_at`.
- Running-work listing is deterministic.
- `Store` exposes a method to get one running attempt by `attempt_id`.
- Missing running-attempt lookup returns a distinguishable not-found result.
- `Store` exposes a method to list terminal attempts for a run.
- Terminal attempt rows include enough information to distinguish completed and
  failed attempts.
- Completed terminal rows include output JSON/hash, pre-state hash,
  post-state hash, optional skipped parent, copied `queued_at`, copied
  `started_at`, and finished timestamp.
- Failed terminal rows include error, copied `queued_at`, copied `started_at`,
  and finished timestamp.
- Terminal attempt listing is deterministic.
- `Store` exposes a method to count queued, running, completed, and failed work
  items for an entire run.
- Run-level counts are derived from `queued_work`, `running_work`,
  `completed_work`, and `failed_work`.
- Query behavior is tested without controller HTTP wiring.

## Out Of Scope

- Mutating persisted state during restart.
- Requeueing abandoned running work.
- Worker heartbeat or liveness policy.
- Fencing tokens.
- Cache pin reconstruction.
- Source-control cache reload.
- Controller startup integration.
- Controller HTTP handler integration.
- Worker scaling integration.
- Retry or max-retry policy.
- Stage failure transition policy.

## Ambiguity To Review

The terminal attempt view could either use two separate records
(`CompletedWorkRecord` and `FailedWorkRecord`) or one union record with a
`TerminalState` field. A single union record is more convenient for restart and
status views, while separate records are stricter and avoid empty fields. This
slice should choose one and document why.

The relationship between running attempts and worker liveness is intentionally
undefined here. A running row after restart is not automatically abandoned; it
is only durable evidence that an attempt was active when the controller stopped
or lost track of it. Liveness recovery belongs to the `attempt-liveness-recovery`
epic.

## Notes

- This slice should not introduce a separate `ReconstructExecutionState` object
  unless it removes real duplication. Focused query methods are easier to review
  and compose.
- The controller remains the only process that talks to the database.
- These queries are a prerequisite for controller integration cutover, but they
  should not alter current controller behavior.

# 012 Controller Integration Cutover

Status: proposed

## Objective

Move controller runtime authority from the in-memory `pending`, `assigned`, and
`failed` collections onto the workflow-execution persistence store.

The long-term cutover target is:

```text
/workflow       -> persist run, stages, compiled work, and queued work
/work/next      -> ClaimNextWork
/work/complete  -> CompleteAttempt, then CompleteStageIfReady when applicable
/work/fail      -> FailAttempt, later retry/requeue policy
/status         -> persisted queue, running, terminal, and run counts
scaling         -> persisted queued/running demand
restart         -> persisted reconstruction queries
```

This feature should not be implemented as one large code change. It should be
decomposed into smaller controller-integration slices after review.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/README.md`
- `docs/epics/workflow-execution-persistence/006-work-item-and-queue-persistence-methods.md`
- `docs/epics/workflow-execution-persistence/007-attempt-claim-transaction.md`
- `docs/epics/workflow-execution-persistence/008-attempt-terminal-transition-transaction.md`
- `docs/epics/workflow-execution-persistence/009-stage-completion-and-ready-work-publication.md`
- `docs/epics/workflow-execution-persistence/011-restart-reconstruction-queries.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/persistence/store.go`
- `internal/model/work_item.go`

## Current Controller State

The controller currently keeps operational work state in memory:

```go
pending  []model.WorkItem
assigned map[string]model.WorkItem
failed   map[string]model.WorkFailure
```

The live startup path opens the main database as an older ledger-style
`*sql.DB` handle and stores it on `Controller.ledger`. The newer
`internal/persistence.Store` exists, but the live HTTP handlers do not use it.

The current `/status` endpoint reports in-memory pending/assigned/failed counts
plus older attempt-ledger counts. The current `/work/next` endpoint mutates
`pending` and `assigned`. The current `/work/complete` and `/work/fail`
endpoints mutate `assigned` and `failed`, while completion may also write to the
older attempt ledger.

## Cutover Strategy

Use an incremental strangler approach. Do not replace all endpoints at once.

Recommended sequence:

```text
012a Controller Store Handle Wiring
012b Persistence-backed Status Read Model
012c Persistence-backed Raw Work Submission
012d Persistence-backed Work Claim Endpoint
012e Persistence-backed Completion/Failure Endpoints
012f Remove In-Memory Queue Authority
```

Each implementation slice should be separately reviewed and committed.

## 012a Candidate: Controller Store Handle Wiring

Add a `*persistence.Store` handle to `Controller` and startup assembly without
changing endpoint behavior.

Acceptance criteria:

- Controller startup opens the workflow-execution persistence store through the
  same configured database driver and connection string.
- `Controller` can hold the store handle separately from the older ledger
  handle during transition.
- Tests can construct a controller with an injected persistence store.
- Existing HTTP behavior remains unchanged.
- Existing tests continue to pass.

Out of scope:

- Changing `/status`.
- Changing `/workflow`.
- Changing `/work/next`, `/work/complete`, or `/work/fail`.
- Removing `pending`, `assigned`, or `failed`.
- Removing the old ledger.

## 012b Candidate: Persistence-backed Status Read Model

Change `/status` or an internal status helper to read queue/running/terminal
counts from `internal/persistence` when a store is configured.

Acceptance criteria:

- Status can derive pending/assigned/failed-equivalent counts from persisted
  queued/running/failed rows.
- Existing legacy status behavior remains available when no persistence store is
  configured.
- Worker-scaling demand can be computed from persisted queued/running counts in
  a later slice.

Out of scope:

- Assignment.
- Completion/failure mutation.
- Workflow submission persistence.

## 012c Candidate: Persistence-backed Raw Work Submission

Add a narrow path for persisting raw/admin work items into `work_items` and
`queued_work`.

This is not the final workflow submission path. It is a small bridge that lets
assignment be cut over before source-control-backed workflow submission is
complete.

Acceptance criteria:

- Raw work submission can insert a work item and queue row through
  `persistence.Store`.
- Existing in-memory raw-work behavior remains available during transition or is
  explicitly replaced with tests.
- The slice defines how `model.WorkItem` maps to `persistence.WorkItemRecord`
  and `worker_payload_json`.

Out of scope:

- Source-control project/workflow provenance.
- Full `/workflow` persistence.
- Dependency-aware compilation.

## 012d Candidate: Persistence-backed Work Claim Endpoint

Move `/work/next` assignment from `pending`/`assigned` mutation to
`Store.ClaimNextWork`.

Acceptance criteria:

- The handler claims one queued row through the persistence store.
- No payload is returned until the claim transaction succeeds.
- Empty persisted queue returns `204 No Content`.
- Claimed work is translated back to the existing worker assignment response
  shape.
- Existing worker clients continue to work.

Open issue:

- `persistence.WorkItemRecord.WorkerPayloadJSON` and `model.WorkItem` are not
  the same shape. This slice must define the conversion before implementation.

## 012e Candidate: Persistence-backed Completion/Failure Endpoints

Move `/work/complete` and `/work/fail` from in-memory `assigned` mutation to
`CompleteAttempt` and `FailAttempt`.

Acceptance criteria:

- Completion reports terminate running attempts through `CompleteAttempt`.
- Failure reports terminate running attempts through `FailAttempt`.
- Duplicate identical reports are accepted idempotently.
- Conflicting duplicate reports fail.
- Completion can trigger `CompleteStageIfReady` when the stage is fully done.

Open issue:

- Current worker completion payloads were designed for the old ledger path.
  The endpoint contract must carry or derive attempt ID, output JSON/hash,
  pre-state hash, post-state hash, and terminal timestamp.

## 012f Candidate: Remove In-Memory Queue Authority

After status, submission, assignment, and terminal reports use persistence, the
controller can remove or demote `pending`, `assigned`, and `failed`.

Acceptance criteria:

- The controller no longer uses in-memory collections as queue authority.
- `/status`, scaling, assignment, completion, and failure derive state from the
  persistence store.
- Tests no longer seed queue state by mutating controller internals directly.

## Out Of Scope For 012

- Source-control implementation.
- GitHub or local cache behavior.
- Full dependency-aware workflow execution.
- Worker liveness, heartbeat, abandoned-attempt recovery, or fencing.
- Retry/max-retry policy.
- Retention cleanup.
- Python API changes.

## Ambiguity To Review

Full `/workflow` cutover may be blocked until source-control resolution exists.
The persistence schema expects project/workflow source identities, but the live
controller currently compiles submitted JSON directly from the HTTP payload.
We can either:

- implement a temporary persisted submission path with synthetic/local source
  identities; or
- leave `/workflow` on the current in-memory path until the source-control epic
  can provide real pinned project/workflow identities.

The second option is cleaner architecturally, but it delays full cutover.

There is also a model conversion ambiguity between `internal/model.WorkItem`
and `persistence.WorkItemRecord`. The persistence model stores compact
`worker_payload_json`, while existing worker endpoints expect the current
`model.WorkItem` transport shape. The first claim-integration slice must define
whether `worker_payload_json` stores the whole existing `model.WorkItem`, only
plugin parameters, or a transitional wrapper.

## Recommended First Implementation Slice

Start with `012a Controller Store Handle Wiring`.

Reason:

- It introduces the dependency boundary without changing HTTP behavior.
- It gives tests a place to inject `*persistence.Store`.
- It keeps the old ledger and the new workflow-execution store side by side
  until endpoint cutover is explicit.
- It exposes any startup/database ownership conflict before queue semantics are
  changed.

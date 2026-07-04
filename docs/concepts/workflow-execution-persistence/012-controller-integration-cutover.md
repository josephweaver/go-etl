# 012 Controller Integration Cutover

Status: in progress

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

- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/concepts/workflow-execution-persistence/006-work-item-and-queue-persistence-methods.md`
- `docs/concepts/workflow-execution-persistence/007-attempt-claim-transaction.md`
- `docs/concepts/workflow-execution-persistence/008-attempt-terminal-transition-transaction.md`
- `docs/concepts/workflow-execution-persistence/009-stage-completion-and-ready-work-publication.md`
- `docs/concepts/workflow-execution-persistence/011-restart-reconstruction-queries.md`
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
012e2 Worker-observed Skip Evidence
012f2 Client Source-reference Workflow Submission
012f3 Controller Source-reference Workflow Admission
012f Remove In-Memory Queue Authority
```

Each implementation slice should be separately reviewed and committed.

## 012a Candidate: Controller Store Handle Wiring

Add a `*persistence.Store` handle to `Controller` and startup assembly without
changing endpoint behavior.

Acceptance criteria:

- Controller startup opens the workflow-execution persistence store through the
  configured database driver and connection string.
- `Controller` can hold the store handle separately from the older ledger
  handle during transition.
- Tests can construct a controller with an injected persistence store.
- Existing HTTP behavior remains unchanged.
- Existing tests continue to pass.

Implemented 012a note:

- Live startup opens the workflow-execution store as the configured main
  database. The older attempt ledger remains in code for legacy helper tests and
  old skip/reuse paths, but it is not opened by live startup.

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

## 012e2 Candidate: Worker-observed Skip Evidence

Revise the completion contract so workers can report observed input/output and
state hashes, mark an attempt as skipped, and point to the prior completed
attempt that made the local skip safe.

Acceptance criteria:

- Workers can use the shared `internal/fingerprint` helper boundary.
- Completion reports can carry worker-observed input, output, pre-state, and
  post-state hashes.
- Completion reports can mark `skipped=true`, carry `skipped_parent_id`, and
  include a skip reason.
- Persisted skipped reports remain completed terminal attempts rather than
  failures.
- `/work/next` can include prior completed attempt candidates selected by a
  composite execution fingerprint.

Open issue:

- The exact definitions of `controller_sha256`, `plugin_sha256`,
  `input_sha256`, and `output_sha256` must be tightened before implementation.
- The persistence schema may need explicit columns for worker-observed input
  and output hashes rather than hiding them in `output_json`.

## 012f2 Candidate: Client Source-reference Workflow Submission

Update `internal/client` and `cmd/demo-client` so clients submit project and
workflow source references to `/workflow`, not inline workflow JSON.

Acceptance criteria:

- The client submission envelope contains project/workflow repository, ref, and
  path fields.
- The demo client loads a workflow-run submission file containing source
  references.
- The demo client no longer calls the inline workflow-file submission method.
- Controller startup, reachability checks, status polling, and shutdown behavior
  remain unchanged.
- Inline workflow JSON submission is not the normal client/demo path.

Out of scope:

- Controller-side source-reference admission.
- GitHub/cache implementation.
- Ref resolution to immutable commits.

## 012f3 Candidate: Controller Source-reference Workflow Admission

Implement persisted `/workflow` admission for project/workflow source
references. The controller should load referenced project/workflow JSON through
a source-control adapter boundary, persist source identity and canonical hashes,
create a workflow run, compile initially ready work, and enqueue it without
mutating `Controller.pending`.

Acceptance criteria:

- `/workflow` accepts the source-reference envelope used by the client.
- `/workflow` rejects legacy inline workflow JSON when `workflowStore` is
  configured.
- A first `local` source-control adapter can resolve fixture/local repository
  references when the controller has local filesystem access.
- Project/workflow source identity and canonical SHA-256 values are persisted.
- A workflow run and initially ready queued work are persisted.
- Store-configured `/workflow` derives scaling demand from persisted
  queued/running state.

Out of scope:

- GitHub source-control adapter.
- Full source-control cache retention.
- Full project semantic model.

## 012f Candidate: Remove In-Memory Queue Authority

After status, submission, assignment, and terminal reports use persistence, the
controller can remove or demote `pending`, `assigned`, and `failed`.

Acceptance criteria:

- The controller no longer uses in-memory collections as queue authority.
- `/status`, scaling, assignment, completion, and failure derive state from the
  persistence store.
- Tests no longer seed queue state by mutating controller internals directly.

Implementation should proceed in smaller atoms:

```text
012f2 Client Source-reference Workflow Submission
012f3 Controller Source-reference Workflow Admission
012f-c Make persisted workflow scaling demand derive from queued/running store counts
012f-d Add guard tests proving persisted paths do not mutate pending/assigned/failed
012f-e Remove or demote in-memory queue authority after no live store path uses it
```

`/workflow` remains the client API for submitting a project/workflow run. It
should accept source-control references to project/workflow JSON documents. It
must not be reframed as client-submitted work items or direct inline JSON
submission. Compiled work items are controller generated after the workflow run
is admitted.

## Out Of Scope For 012

- Source-control implementation.
- GitHub or local cache behavior.
- Full dependency-aware workflow execution.
- Worker liveness, heartbeat, abandoned-attempt recovery, or fencing.
- Retry/max-retry policy.
- Retention cleanup.
- Python API changes.

## Ambiguity To Review

Full `/workflow` cutover depends on a source-reference admission boundary. The
persistence schema expects project/workflow source identities, while the live
controller currently compiles submitted JSON directly from the HTTP payload.
012f should either define the narrow source-reference loader it needs or split
that source-reference admission work into a preceding slice. It should not add a
new persisted inline-JSON workflow submission path.

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

# 006 Checkpoint-Generation and Pending-Resume Store Lifecycle

Status: implemented

## Objective

Extend the SQLite persistence store so one owned running attempt can confirm
multiple immutable `goet/resume-artifact/v1` generations without stopping.
Each accepted periodic generation becomes an eligible recovery point while the
attempt remains running. Each manifest represents one adapter-validated bundle
of process/application state plus immutable copies of registered mutable
outputs taken at the same quiesced boundary. A quantum-yield or final
confirmation, or an explicit fallback to the latest accepted generation,
atomically records suspended history and returns the logical work item to the
existing FIFO queue for resume.

A later claim creates a new fenced attempt linked to the accepted artifact,
its producing attempt, execution lineage, and per-artifact resume-attempt
number.

## Pre-Implementation State

SQLite schema version 6 is defined in
`internal/persistence/db_adapter_sqlite.go`. It contains:

- immutable `work_items`;
- one-row-per-item `queued_work` with only `work_item_id` and `queued_at`;
- `work_item_attempts` with the attempt, work item, owner, executor type, and
  start time;
- one active assignment per work item in `running_work`;
- historical `abandoned_work`, `completed_work`, and `failed_work`; and
- session ownership and resource-constraint indexes/views.

`internal/persistence/store.go` owns the transactions:

- `ClaimNextWork` selects the oldest resource-eligible queue row, creates an
  attempt and `running_work` row, then removes the queue row;
- `CompleteAttempt` and `FailAttempt` validate current worker/session ownership,
  write terminal history, and remove `running_work`; and
- worker stop or expiry inserts `abandoned_work`, removes `running_work`, and
  requeues the logical work item at recovery time.

Completion and failure from an abandoned owner return
`ErrAssignmentNoLongerOwned`. Fresh and recovered work share the same queue,
FIFO order, and resource checks.

OS-005 defines and validates the role-neutral
`model.ResumeArtifactManifest` and `model.ResumeArtifactReference`. No
persistence transaction consumes those values. There is no durable
representation for:

- an accepted checkpoint generation;
- multiple generations produced by one running attempt;
- an execution lineage or its latest accepted recovery point;
- a nonterminal checkpoint confirmation;
- a suspended attempt or pending resume;
- a resume predecessor; or
- a per-artifact resume-attempt count.

The OS-005 common `files` inventory can include DMTCP/state files, an
adapter-owned versioned output-snapshot index, and immutable registered-output
copies. It validates path, size, and digest metadata but deliberately does not
classify completed workflow outputs. The worker adapter, not SQLite, owns
quiescing the payload, copying output, and validating the bytes before
confirmation.

The schema initializer accepts only the exact supported schema version. It has
no migration from version 6, so only incrementing `SupportedSchemaVersion`
would prevent existing controller databases from opening.

## Target State

SQLite schema version 7 adds accepted checkpoint generations and resume
lifecycle state without adding a second queue or storing checkpoint bytes.

### Schema

`resume_artifacts` stores every controller-accepted immutable manifest:

```text
resume_artifact_id primary key
work_item_id
producing_attempt_id
execution_lineage_id
resume_generation
capture_kind                 periodic | quantum | final
pause_strategy
manifest_json
manifest_sha256
storage_scope
manifest_relative_path
created_at
accepted_at
```

The table requires valid JSON, positive generations, the OS-005 strategy
values, `shared_tmp` storage, and foreign keys to the work item and producing
attempt. It enforces uniqueness of
`(execution_lineage_id, resume_generation)`.

`producing_attempt_id` is deliberately **not unique**: a running attempt may
produce and confirm many generations. It is indexed with generation for
attempt-level lookup. Checkpoint/state bytes remain in protected shared
storage; SQLite stores only the exact small manifest and reference facts.

`suspended_work` stores the terminal ownership transition for an explicitly
suspended attempt:

```text
attempt_id primary key
work_item_id
resume_artifact_id
worker_id
worker_session_id
queued_at
started_at
suspended_at
suspend_reason               quantum | shutdown
```

The selected artifact may be:

- a quantum generation produced when the attempt yields;
- a final generation produced by that attempt;
- the latest periodic generation produced by that attempt; or
- for a resumed attempt that made no newer generation, the accepted artifact
  from which it started.

`queued_work` gains nullable `resume_artifact_id`. Null means fresh pending
work. Non-null means pending resume. Both retain ordering by `queued_at`, then
`work_item_id`, and the same resource eligibility checks.

`work_item_attempts` gains nullable:

```text
resumed_from_attempt_id
resume_artifact_id
execution_lineage_id
resume_attempt_number
```

Fresh attempts initially have all four fields null. The first accepted
checkpoint assigns the manifest lineage to the producing attempt. Resume
claims populate all four fields at creation.

Indexes support:

- artifact generations by producing attempt and lineage;
- selection of the newest accepted lineage generation;
- suspended history by work item and time;
- attempts by consumed artifact and resume-attempt number; and
- pending-resume lookup by artifact.

`internal/persistence/testdata/schema-v6.sql` is an immutable test-only version
6 fixture. Opening a representative version 6 database performs one explicit,
transactional, non-destructive migration to version 7. Existing work items,
attempts, queue rows, running state, and terminal history remain unchanged.
Unsupported versions and malformed/incomplete version 6 schemas fail closed.

### Store Records and Operations

`internal/persistence/store.go` adds records and typed values equivalent to:

- `CheckpointCaptureKind` with `periodic`, `quantum`, and `final`;
- `CheckpointDisposition` with `continue` and `suspend`;
- `SuspendReason` with `quantum` and `shutdown`;
- `ResumeArtifactRecord`;
- `SuspendedWorkRecord`;
- `ConfirmCheckpointRequest` and `ConfirmCheckpointResult`;
- `SuspendFromLatestCheckpointRequest` and result;
- `Store.ConfirmCheckpoint`;
- `Store.SuspendFromLatestCheckpoint`;
- `Store.GetResumeArtifact`;
- `Store.GetLatestAcceptedCheckpoint`;
- `Store.ListSuspendedWorkForItem`; and
- `ErrResumeAttemptLimitExceeded`.

`QueuedWorkRecord` carries an optional `ResumeArtifactID`.
`ClaimedWorkRecord` additionally exposes, for resume claims:

```text
resumed_from_attempt_id
execution_lineage_id
resume_attempt_number
resume_artifact
```

The returned artifact contains the validated manifest JSON and
controller-facing reference needed by a later handler to construct work
assignment transport. This slice does not modify `model.WorkItem` or HTTP
responses.

### Checkpoint Bundle Boundary

The store treats the exact manifest as the registration document for the whole
generation. Its `files` inventory includes every file needed for resume,
including the strategy checkpoint/state, output-snapshot index, and registered
output copies. The controller stores and returns that complete document; it
does not promote the copied outputs into completed workflow artifacts.

This slice validates manifest structure and reference identity but does not
read shared-storage files or prove their live hashes. The later worker adapter
must do that before confirmation and again before resume. The adapter's
versioned output index maps immutable artifact copies back to the narrowly
registered execution-workspace paths and records absence where required.

### Common Checkpoint Validation

`ConfirmCheckpoint` receives current attempt/worker/session ownership, exact
manifest JSON, its `ResumeArtifactReference`, capture kind, disposition, a
live-session cutoff, and a controller-selected acceptance time for a new row.
Only `periodic/continue`, `quantum/suspend`, and `final/suspend` are valid
pairs. Quantum suspension records reason `quantum`; final suspension records
reason `shutdown`.

Before writing, the store:

1. decodes and validates `ResumeArtifactManifest`;
2. validates the reference;
3. hashes the exact supplied manifest JSON bytes and requires the digest to
   match `manifest_sha256`;
4. requires manifest/reference artifact identity and storage scope to match;
5. requires the reference manifest path to be inside the manifest's declared
   storage-relative directory;
6. requires manifest work item and producing attempt to match the current
   running assignment;
7. applies the existing live worker/session ownership fence; and
8. validates lineage and generation.

The exact accepted manifest, not a separately submitted output list, is the
controller registration for the state/output consistency bundle. SQLite never
accepts a later mutation to add output files to an already accepted artifact.

The first checkpoint of a fresh attempt must be generation `1`. The next
checkpoint from that attempt must be exactly one greater than the lineage's
latest accepted generation. A resumed attempt consuming generation N must use
the same lineage and next produce generation N+1. Multiple generations from
one producing attempt are valid; reuse or skipping of a lineage generation is
not.

### Confirm and Continue

For `disposition=continue`, one transaction:

1. inserts the accepted `resume_artifacts` row;
2. records the execution lineage on the producing attempt when first needed;
3. commits the new latest accepted generation; and
4. leaves `running_work` and `queued_work` unchanged.

The result identifies the exact accepted artifact and generation so the later
controller route can acknowledge it. The attempt remains owned and running.
The store does not schedule the next interval.

An exact replay of an already accepted generation is idempotent, including
after an ambiguous HTTP response. Reusing an artifact ID or lineage generation
with different manifest bytes, reference, capture kind, work item, attempt, or
identity facts fails as a conflict. A replay returns the original persisted
`accepted_at` rather than replacing it with the caller's newly proposed
controller time. A failed transaction leaves the previous latest accepted
generation unchanged.

### Confirm and Suspend

For `capture_kind=quantum|final` and `disposition=suspend`, the common
validation runs and one transaction:

1. inserts the final `resume_artifacts` row;
2. records the producing attempt lineage;
3. inserts `suspended_work` selecting that artifact;
4. removes `running_work`; and
5. inserts `queued_work` at `suspended_at` with the artifact ID.

The queue timestamp is suspension time, not the original queue timestamp.
Quantum-yield work therefore moves to ordinary FIFO tail and receives no
priority over already waiting work. Exact replay is idempotent; a conflicting
replay fails.

Once suspended, completion or failure from the former owner returns
`ErrAssignmentNoLongerOwned`.

### Suspend from the Latest Accepted Checkpoint

`SuspendFromLatestCheckpoint` is a separate owner-fenced transaction used when
a quantum or final generation cannot be created or confirmed but a prior
accepted recovery point exists. Its request carries the `quantum` or
`shutdown` suspend reason. It:

1. validates the current running assignment and live session;
2. selects the greatest accepted generation in the attempt's execution
   lineage, including the artifact consumed by a resumed attempt;
3. inserts `suspended_work` referencing that existing artifact;
4. removes `running_work`; and
5. queues pending resume at `suspended_at`.

It never accepts local unconfirmed files. When the running attempt has no
accepted checkpoint or consumed artifact, it returns a typed no-checkpoint
result without mutating ownership; later worker/session loss retains the
existing fresh lost-work recovery.

### Resume Claim and Lost-Work Recovery

`ClaimWorkRequest` gains `ResumeAttemptLimit`, consulted only when the selected
queue row has a resume artifact.

For pending resume, `ClaimNextWork`:

1. loads and revalidates the accepted artifact;
2. counts prior attempts consuming that exact artifact;
3. rejects the claim with `ErrResumeAttemptLimitExceeded` when the next number
   exceeds the positive supplied limit, leaving queue state unchanged;
4. creates the caller-supplied new `attempt_id`;
5. records the artifact's producing attempt as predecessor, artifact ID,
   lineage, and next resume-attempt number;
6. creates the ordinary session-owned `running_work` row;
7. removes the FIFO queue row; and
8. returns resume metadata with the claim.

Fresh claims remain unchanged and do not require a positive
`ResumeAttemptLimit`.

Worker stop or session expiry still records the current attempt as abandoned.
Before requeue, recovery selects the newest accepted generation available to
the attempt's lineage, including its consumed artifact. It queues that artifact
when present; otherwise it creates the existing fresh pending row. A reported
causal failure remains terminal through `FailAttempt` and is never converted
to pending resume.

Repeated claims consuming the same artifact increment that artifact's
resume-attempt count. A newly accepted generation receives an independent
per-artifact count. Earlier accepted generations remain referenced by history;
retention cleanup is deferred.

Successful quantum yields do not exhaust the retry limit merely because the
lineage spans many attempts: each yield creates a new artifact whose
per-artifact count starts independently. The limit protects repeated
claim/launch loss against the same checkpoint, not the number of planned
execution quanta in a lineage.

## Concept Decision

This slice extends the existing `internal/persistence` concept. Accepted
checkpoint generations and suspended attempts need dedicated tables because
they are durable history with independent identities and foreign-key
relationships. Pending resume remains a nullable extension of `queued_work`,
preserving one FIFO queue and the existing scheduler.

The store persists the complete small manifest JSON plus indexed identity
columns. It does not normalize every strategy-specific field. OS-005 remains
the authority for manifest validation.

The nonterminal `continue` disposition and terminal `suspend` disposition
share validation but have intentionally different transactions. This prevents
a periodic confirmation from accidentally releasing assignment ownership.
Quantum and shutdown suspensions share the terminal transaction but retain
their reason in history for status, timing, and later policy analysis.

This Operational Slice spans two production files because schema
creation/migration and transaction behavior have separate owners:

1. `db_adapter_sqlite.go` owns schema version 7 and migration;
2. `store.go` owns confirmation, suspension, claim, and recovery.

Under `file(1)+test+doc+newfile`, implement them in two prompt-sized passes,
changing at most one production file per prompt.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/005-resume-artifact-contract-model.md`
- `internal/model/resume_artifact.go`
- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/db_adapter_sqlite_test.go`
- `internal/persistence/store.go`
- `internal/persistence/store_test.go`
- `internal/persistence/worker_session_test.go`

Do not read controller HTTP handlers, worker code, CareTaker implementation,
Slurm generation, container code, adapter implementations, workflow
compilation, or unrelated persistence tests unless a focused persistence test
failure exposes a direct dependency.

## Allowed Production Files

- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/store.go`

## Allowed Test Files

- `internal/persistence/db_adapter_sqlite_test.go`
- `internal/persistence/store_test.go`
- `internal/persistence/worker_session_test.go`
- `internal/persistence/testdata/schema-v6.sql` (new)

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/concepts/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/006-checkpoint-generation-and-pending-resume-store-lifecycle.md`

## Out of Scope

- Modifying the OS-005 manifest schema, `model.WorkItem`, `WorkCompletion`,
  `WorkFailure`, worker lifecycle, or controller-status transport.
- Adding controller HTTP confirmation, suspension, or resume-assignment
  behavior.
- Adding worker interval timers, adapter interfaces, DMTCP/native/manual
  implementations, manifest writing, filesystem validation, or resume
  execution.
- Quiescing R/Python, copying registered outputs, defining the adapter-owned
  output-index document, reconciling a workspace, or proving a DMTCP barrier.
- Deciding which work-item adapters support periodic snapshots.
- Configuring checkpoint modes/quantum duration, measuring active execution
  time, validating all queued work for yield mode, or deciding worker
  anti-affinity.
- Adding Slurm signals, worker drain state, administrative drain,
  completion/pause arbitration, or controller acknowledgement transport.
- Changing CareTaker wake calls, demand calculation, worker launch, or resource
  constraint semantics.
- Automatically retrying reported causal failures or silently restarting a
  work item without its accepted artifact.
- Converting resume-limit exhaustion into a terminal controller response; this
  slice returns a typed store error and leaves pending state intact.
- Deleting resume artifacts, retention cleanup, encryption, permission checks,
  or copying checkpoint bytes into SQLite.
- Adding resume priority, a second queue, Slurm requeue, or nested scheduler
  commands.
- Supporting migration from a schema version other than the exact complete
  version 6 fixture.

## Acceptance Criteria

- Implementation follows the two-pass `file(1)+test+doc+newfile` sequence and
  changes no production file outside the allowed list.
- `SupportedSchemaVersion` is `7`.
- A new database has `resume_artifacts`, `suspended_work`, resume columns on
  `queued_work` and `work_item_attempts`, and declared indexes/constraints.
- Suspended history distinguishes `quantum` from `shutdown`; invalid capture,
  disposition, or suspend-reason combinations fail closed.
- `resume_artifacts` permits multiple generations with one
  `producing_attempt_id` and rejects duplicate lineage generations.
- Opening the complete version 6 fixture migrates it once without losing
  representative queued, running, abandoned, completed, or failed records.
- Reopening a migrated database is idempotent; unsupported or incomplete
  schemas fail without partial migration.
- A valid periodic continue confirmation stores the exact validated
  manifest/reference, assigns lineage, and leaves running ownership and queue
  state unchanged.
- The exact persisted manifest retains the complete state/output file
  inventory and adapter-owned output-index reference; checkpointed outputs do
  not create completed artifact or promotion records.
- Two or more sequential generations from one owned attempt are accepted;
  reuse, skips, wrong lineage, and conflicting retries are rejected.
- Exact confirmation replay is idempotent and returns the same accepted
  artifact/generation.
- Validation rejects invalid JSON, OS-005 validation failures, reference or
  digest mismatch, out-of-directory paths, wrong work item/attempt,
  stale/mismatched session ownership, and unsupported capture/disposition
  combinations.
- Failure during confirmation rolls back all writes and preserves the previous
  latest accepted generation.
- A valid final suspend confirmation atomically stores the generation and
  suspended history, removes running ownership, and queues the logical item at
  `suspended_at`.
- A valid quantum suspend performs the same atomic transition, records reason
  `quantum`, and places the logical item at ordinary FIFO tail.
- Suspending from latest selects only a controller-accepted generation,
  performs the same terminal transition, and makes no mutation when no
  accepted/consumed artifact exists.
- Completion and failure reports for a suspended attempt return
  `ErrAssignmentNoLongerOwned`.
- Fresh and pending-resume rows share FIFO order by `queued_at, work_item_id`;
  suspension uses `suspended_at`.
- Claiming pending resume creates a different caller-supplied attempt ID and
  records predecessor, artifact, lineage, and resume-attempt number with
  existing worker/session fencing.
- Claimed resume data contains the exact persisted validated manifest and
  reference, including the registered-output snapshot inventory; fresh claims
  have empty resume metadata.
- Stop/expiry abandonment requeues the newest accepted generation available
  to the attempt/lineage, or a fresh row when none exists.
- A causal `FailAttempt` remains terminal and does not requeue.
- Repeated lost-work resume claims increment the count for the same artifact;
  a newer generation starts its own artifact-specific count.
- Arbitrarily many successful quantum generations may extend one execution
  lineage without consuming one shared lineage retry limit.
- A claim above the positive limit returns
  `ErrResumeAttemptLimitExceeded` without creating an attempt/running row or
  mutating the queue.
- Existing fresh claim, resource constraint, completion, failure, abandonment,
  stop, and expiry tests continue to pass.
- Focused schema, migration, store lifecycle, rollback, ownership, FIFO,
  lineage, generation, and resume-limit tests pass with:

  ```text
  go test ./internal/persistence -count=1
  ```

- Project/test documentation says persistence is implemented but no HTTP
  confirmation, worker timer/adapter, or runtime resume path exists.
- No file outside the allowed production, test, and documentation lists
  changes.

## Implementation Sequence

### Prompt 1: Schema version 7 and migration

Change only `internal/persistence/db_adapter_sqlite.go` as production code.
Add the version 6 fixture and focused schema/migration tests. Prove multiple
artifact generations can share a producing attempt and existing databases are
preserved.

### Prompt 2: Store lifecycle

Change only `internal/persistence/store.go` as production code. Add focused
tests for continue confirmation, final suspension, fallback suspension, resume
claim, quantum-yield suspension, latest-checkpoint lost-work recovery,
ownership fencing, idempotency, rollback, and attempt limits.

Update allowed project/test documentation after each prompt as required by the
implementation procedure.

## Implementation Evidence

Implemented on 2026-07-30 in the planned two production-file passes.
SQLite schema version 7 and its explicit version 6 migration are owned by
`db_adapter_sqlite.go`; checkpoint confirmation, suspension, resume-aware
claiming, latest-checkpoint recovery, lineage, and per-artifact attempt limits
are owned by `store.go`.

The persistence suite verifies fresh and migrated databases, immutable
generations, exact idempotent replay, invalid-bundle and ownership rejection,
transaction rollback, periodic continuation, quantum/final suspension,
fallback suspension, stop/expiry recovery, resume lineage, and limit
enforcement:

```text
go test ./internal/persistence -count=1
ok  	goetl/internal/persistence
```

This evidence ends at the persistence boundary. Controller HTTP transport,
worker checkpoint timers and adapters, bundle creation/byte verification, and
runtime restoration remain future Operational Slices.

## Notes

- Preserve caller-generated attempt and artifact IDs; the store validates and
  records them but does not generate them.
- Hash the exact manifest JSON byte sequence supplied to
  `ConfirmCheckpoint`; do not re-marshal before hashing.
- Decode persisted manifest JSON and call OS-005 validation again before
  returning a resume claim.
- Use nullable SQL values for fresh queue/attempt rows, not empty foreign-key
  sentinels.
- Use `suspended_at` for pending FIFO order and recovery time for lost-work
  requeue order.
- Count resume attempts by `resume_artifact_id`, not generation.
- Every generation has a new artifact ID and immutable directory, even when
  the producing attempt is unchanged.
- Do not delete or mutate an older artifact when accepting a newer one.
- Do not pass checkpoint image bytes through SQLite or controller transport.
- Store errors must not include manifest contents or sensitive state.
- Keep schema migration transactional; never rebuild or discard a complete
  version 6 database.

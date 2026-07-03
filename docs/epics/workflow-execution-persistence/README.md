# Workflow Execution Persistence Epic

Status: Proposed

## Purpose

Make the database authoritative for workflow runs, immutable resolver inputs,
step state, work items, attempts, logical outputs, source-control identities,
and reusable fingerprints so execution can continue correctly across controller
restart.

The current controller keeps pending, assigned, and failed work primarily in
process memory. Dependency-aware JIT compilation requires durable state that
can trace a completed `work_item_id` back to its step and workflow run and
reconstruct the exact variable context used at submission.

This epic also establishes the first controller-owned persistence boundary for
bootstrapping the main database, reading and writing execution tables, computing
canonical JSON SHA-256 values, and loading pinned project/workflow files from a
source-control implementation.

## Goals

- Bootstrap the main database when no database exists.
- Validate and migrate an existing database through explicit schema versions.
- Provide controller-owned repository methods for inserting, querying, and
  deleting rows in the workflow-execution tables.
- Keep SQL access behind a persistence package boundary rather than spreading
  ad hoc SQL through the controller.
- Persist the workflow definition, project identity, workflow identity, and a
  controller-generated `run_id` at submission.
- Snapshot workflow configuration, project configuration, submission
  overrides, and execution-relevant controller configuration immutably for the
  run.
- Persist ordered step/stage definitions and lifecycle state.
- Persist compiled work items before they become assignable.
- Represent current work placement with separate queued, running, completed,
  and failed state tables using transactional insert/delete transitions rather
  than an in-place mutable status column.
- Preserve the distinction between one logical work item and its multiple
  execution attempts.
- Persist typed logical outputs plus SHA-256 pre-state and post-state hashes for
  reusable terminal attempts.
- Provide canonical JSON-to-SHA-256 helpers for project config, workflow config,
  resolved inputs, work-item payloads, output JSON, state observations, and
  future fingerprint records.
- Provide a source-control abstraction for resolving repository revisions,
  reading pinned files, obtaining source commit/object identities, and checking
  out or materializing required files.
- Provide a GitHub-backed source-control implementation as the first concrete
  source-control adapter.
- Support transactional completion, stage-state updates, and creation of newly
  ready work.
- Reconstruct runnable, blocked, running, failed, and completed workflow state
  after controller restart.
- Make persisted state the sole work queue authority used by assignment,
  scaling, status, dependency scheduling, heartbeat recovery, and restart.
- Provide query and mutation boundaries usable by dependency scheduling,
  heartbeat recovery, status, and later cross-workflow execution.
- Evolve the existing SQLite ledger through explicit schema versions and
  migrations.

## Non-Goals

- Defining sequential or `parallel_with` dependency semantics.
- Defining heartbeat intervals, leases, fencing, or abandonment policy.
- Implementing a general artifact storage service.
- Implementing source-control support for providers other than GitHub.
- Implementing full Git cache retention, eviction, repacking, or garbage
  collection policy.
- Implementing secret storage or plaintext credential persistence.
- Moving workflow resolution or scheduling decisions into SQL triggers.
- Coordinating multiple active controllers before a concrete high-availability
  requirement exists.
- Replacing typed variables with database-specific configuration fields.

## Architectural Context

The persistent identity hierarchy is:

```text
project_id
  workflow_id
    run_id
      stage_index / step_instance_id
        work_item_id
          attempt_id
```

The controller remains the only process that talks directly to SQLite. Clients
and workers interact through HTTP APIs. The database is the execution source of
truth; caches may accelerate reads of immutable definitions, but must not become
a second queue or lifecycle authority.

The client submits or identifies a project and workflow source. The controller
loads the project and workflow from a pinned source-control revision, computes
canonical project/workflow SHA-256 values, creates `run_id`, and stores immutable
execution-relevant snapshots. Later JIT compilation rebuilds the resolver from
those snapshots, generated runtime variables, and completed prior-step outputs;
it does not read a newer project configuration.

At submission, the controller normalizes the ordered workflow into stage
definitions and assigns every stage its zero-based `stage_index`. The `run_id`
and complete stage plan exist before any work is assigned, but only initially
ready stages are compiled into `queued_work`. Work created for a stage carries
its immutable `run_id`, `stage_index`, and step position through every placement
table.

A work item is created and persisted when compilation produces logical work. An
attempt is created only when a worker claims that work item. Retries create new
attempts under the same `work_item_id`.

## Database Bootstrap and Migration

Controller startup owns database bootstrap.

Startup must:

1. Resolve and validate the configured database driver and connection string.
2. Open the database.
3. Enable required SQLite pragmas, including foreign-key enforcement.
4. Create the schema metadata table if the database is empty or missing schema
   metadata.
5. Apply forward-only migrations until the database reaches the controller's
   supported schema version.
6. Fail closed without serving normal admission if the database is newer than
   the controller understands.
7. Store the opened database handle on the controller only after successful
   bootstrap.

The bootstrapper should expose one small boundary such as:

```go
type StoreOpener interface {
    Open(ctx context.Context, cfg DatabaseConfig) (*Store, error)
}
```

`Open` is responsible for creating the database if it does not exist and for
migrating it if it does. The controller should not call individual migration
functions directly.

Migrations are ordered, forward-only, and transactional. Each migration either
commits completely or leaves the prior schema unchanged. Startup fails without
mutation when the stored schema version is greater than the binary's supported
version.

GOET has no production release yet, so the first workflow-execution schema may
replace the experimental ledger schema and update repository-owned fixtures
without preserving backward compatibility. Once a production schema is declared,
all subsequent changes follow the forward migration policy. Destructive
production migrations require a verified backup first; rollback restores that
backup rather than executing automatic down-migrations.

## Persistence Repository Boundary

The persistence layer should expose task-oriented methods instead of leaking SQL
queries into controller orchestration code.

Initial method families should include:

```text
Schema / bootstrap
- OpenStore
- CurrentSchemaVersion
- ApplyMigrations

Projects and workflows
- UpsertProjectRevision
- UpsertWorkflowRevision
- GetProjectRevision
- GetWorkflowRevision
- DeleteProjectRevisionIfUnused
- DeleteWorkflowRevisionIfUnused

Workflow runs and stages
- CreateWorkflowRun
- GetWorkflowRun
- ListActiveWorkflowRuns
- InsertStagePlan
- MarkStageReady
- MarkStageCompleted
- MarkStageFailed
- GetStageState
- ListReadyStages

Work items
- InsertWorkItems
- GetWorkItem
- ListWorkItemsForStage
- DeleteUnqueuedWorkItemIfUnused

Placement and attempts
- EnqueueWork
- ClaimNextWork
- CompleteAttempt
- FailAttempt
- RequeueAfterFailedAttempt
- GetRunningAttempt
- ListQueuedWork
- ListRunningWork
- ListTerminalAttempts

Workers
- RegisterWorker
- GetWorker
- DeleteWorkerIfNoActiveAttempts

Restart and status
- ReconstructExecutionState
- CountQueueState
- CountRunState
```

Names are planning names, not final API commitments. The important design rule is
that each method enforces one controller lifecycle transaction or one bounded
query. Callers should not assemble multi-table lifecycle transitions manually.

Deletes are allowed only where deletion is semantically safe:

- Immutable project/workflow references may be deleted only when no runs or
  dependent definitions reference them.
- Work-item definitions may be deleted only when they were never queued,
  attempted, completed, failed, or used by downstream state.
- Operational queue rows are deleted only as part of lifecycle transitions.
- Terminal records are deleted only through later retention policy after compact
  lineage, fingerprints, output references, and audit requirements are preserved.

The first implementation may prefer `Delete...IfUnused` methods over general
`Delete...` methods to make accidental destructive operations difficult.

## Core Tables

The current conceptual schema remains the starting point:

```text
projects
workflows
workflow_instances
workflow_stages
work_items
workers
work_item_attempts
queued_work
running_work
completed_work
failed_work
```

`projects` stores one immutable project configuration identity. A materially
different project configuration receives a new `project_id`, even when its
repository and path are unchanged.

`workflows` stores one immutable workflow configuration identity under its
project. A materially different workflow receives a new `workflow_id`.

`workflow_instances` stores one submitted run and its immutable submission
context, including source-control locator, source commit, submission overrides,
runtime variables, controller/plugin versions when relevant, and resolver input
snapshots.

`workflow_stages` stores the ordered stage plan for a run. It should carry at
least `run_id`, `stage_index`, stage definition JSON or normalized stage JSON,
state, timestamps, and optional output JSON/hash for stage-level outputs.

`work_items` stores immutable compiled worker payloads and resolved input
snapshots. Repeated stage compilation must be idempotent through uniqueness such
as `(run_id, stage_index, work_item_index)`.

`queued_work` and `running_work` represent current placement. `completed_work`
and `failed_work` represent terminal attempt outcomes. Only `queued_work` and
`running_work` should be treated as current work location.

## Transactional Lifecycle

Claiming work is one atomic database transaction. The controller selects the
oldest eligible row from `queued_work`, generates a globally unique
`attempt_id`, inserts the immutable attempt identity and corresponding
`running_work` row, deletes the selected `queued_work` row, and commits. Any
failure rolls back the entire claim, so no worker receives work that the
database still considers queued or that lacks an active attempt.

Every `running_work` row references exactly one unique active `attempt_id` and
one `work_item_id`. A retried `work_item_id` creates a new attempt identity; an
old attempt ID is never reused. Uniqueness and foreign-key constraints enforce
that an attempt cannot claim multiple running rows and a work item cannot have
multiple active attempts.

Completion processing should atomically:

1. Validate and terminate the active attempt.
2. Record SHA-256 pre/post-state hashes and the typed logical output.
3. Delete the active `running_work` row.
4. Insert the terminal `completed_work` or `failed_work` row.
5. Update the logical stage state if the stage is now complete or failed.
6. Record any newly compiled work items exactly once.
7. Enqueue newly ready work exactly once.

The database is the sole queue authority. Assignment queries `queued_work`
directly and transitions the selected row to `running_work` transactionally.
Worker scaling derives demand from persisted queued/running state, and status
reports persisted counts. The controller does not maintain a second in-memory
pending or assigned queue, including a nominal cache that could become a
competing source of truth.

## Canonical JSON and SHA-256 Helpers

GOET should provide a small shared helper boundary for canonical JSON and hashes.
This prevents every package from inventing slightly different hashing rules.

Initial helper responsibilities:

```text
CanonicalJSON(value any) ([]byte, error)
SHA256Hex(bytes []byte) string
CanonicalJSONSHA256(value any) (canonical []byte, sha256Hex string, err error)
ValidateSHA256Hex(value string) error
```

The canonical JSON contract must include:

- stable object-key ordering;
- stable string encoding;
- no insignificant whitespace;
- deterministic number formatting;
- explicit treatment of null versus missing values;
- schema-owned omission of fields declared non-semantic.

Unless a stronger requirement is introduced later, GOET fingerprints use SHA-256
over the canonical byte representation. If canonicalization changes, the schema
or fingerprint algorithm version must change so older fingerprints remain
explainable.

The first persistence slice should use these helpers for:

- `projects.config_sha256`;
- `workflows.workflow_sha256`;
- `work_items.resolved_inputs_sha256`;
- `completed_work.output_json_sha256`;
- `completed_work.pre_state_sha256`;
- `completed_work.post_state_sha256`.

Project and workflow source locators are recorded separately from their semantic
fingerprints. Repository, commit SHA, and path answer where GOET obtained one
known-valid copy. Canonical SHA-256 answers what semantic content GOET used.

## Source-Control Abstraction

The controller needs a source-control boundary because project/workflow
configuration and execution components should be loaded from pinned source
revisions, not from whichever branch happens to be current at restart.

The abstraction should support:

```text
ResolveRef(repository, ref) -> full commit/object ID
ReadFile(repository, commit, path) -> bytes plus object identity when available
CanonicalFileSHA256(repository, commit, path) -> canonical JSON plus SHA-256 for JSON documents
Checkout(repository, commit, manifest, destination) -> materialized files
GetCommitIdentity(repository, commit) -> validated immutable commit identity
```

The durable run record should store:

- stable repository identity when available;
- human-readable owner/name for diagnostics;
- full resolved commit/object ID;
- repository-relative project config path;
- project config SHA-256;
- repository-relative workflow path;
- workflow config SHA-256;
- optional originally requested branch/tag/ref for diagnostics only.

Branches and tags are discovery inputs, not durable revision identities. If a
client supplies one, the controller resolves it to a full commit/object ID before
creating the project/run reference. Every reload uses that exact commit. It must
never repeat branch or tag resolution for an existing run.

The source-control abstraction should reject unsafe paths before reading or
materializing files. Paths must be repository-relative, normalized, non-absolute,
and unable to escape the repository root.

## GitHub Source-Control Implementation

GitHub is the first concrete source-control implementation.

The GitHub implementation should:

- resolve owner/name plus branch, tag, or SHA into a full commit/object ID;
- read project and workflow files at an exact commit and path;
- return GitHub repository identity when available, plus owner/name for
  diagnostics;
- support obtaining blob/object SHA values where GitHub exposes them;
- compute GOET canonical JSON SHA-256 values separately from Git blob identity;
- materialize a selected dependency manifest into a controller staging directory
  when execution packaging requires a filesystem tree;
- avoid embedding credentials in cache paths, remote URLs, errors, or logs.

A local bare Git cache is the target long-term implementation shape, but full
cache retention and eviction are not part of this epic. The first GitHub
implementation may be thin if it preserves the same interface and durable
identity rules.

The important distinction is:

```text
GitHub repository / commit / path = source locator
GOET canonical SHA-256           = semantic fingerprint
Git blob SHA                     = Git object identity
```

All three may be useful. They are not interchangeable.

## Immutable Snapshots and Resolver Recipes

Immutable workflow definitions and configuration snapshots are stored as
canonical text JSON documents. Each document includes an explicit document schema
version, and its column uses `CHECK (json_valid(...))`. Query-critical identity,
ordering, lifecycle, timestamps, and foreign keys remain normalized relational
columns. Go normally decodes each immutable document as a whole; SQLite JSON
extraction is available for diagnostics and bounded migrations, not as the
primary scheduling interface.

SQLite JSONB is deferred. Text JSON is easier to inspect, export, test, and
migrate and does not couple stored definitions to SQLite's private binary
representation.

The intended resolver rule remains:

```text
Persist the inputs and outputs of resolution, not the resolver.
```

A resolver is created for one specific lifecycle decision, resolves the required
values, and is discarded. Durable records must contain enough information to
reconstruct the resolver recipe used for submission, stage compilation,
work-item compilation, assignment-time resolution, and output propagation.

## Stage Progression

When every required work item for a stage has completed successfully, the
controller records stage completion and compiles/persists newly ready work in the
same durable transition. A unique completed-stage identity `(run_id,
stage_index)` makes a repeated completion request harmless and prevents a stage
from compiling its successor twice.

A stage compilation that yields zero work items records immediate successful
stage completion with typed output `[]` and continues to the next ready stage in
the same idempotent progression. It creates no placeholder work item or attempt.

Completion is addressed by `attempt_id` plus its fencing token once fencing is
defined. Repeating an identical terminal report for the already recorded attempt
succeeds idempotently; a conflicting report or a report from a superseded token
is rejected.

Lost execution is primarily an attempt outcome rather than a terminal logical
work-item state. A lost attempt is recorded durably, while the same
`work_item_id` may return to `queued_work` and later create a new attempt. If a
`lost_work` operational table is retained for caretaker processing, its rows
must identify the lost `attempt_id`; moving the logical work item back to the
queue must not erase attempt history.

## Retention and Reuse

Completed and failed operational tables require configurable retention and
regular cleanup because they can grow without bound. Cleanup cannot blindly
delete the evidence required for reuse, audit, downstream output restoration, or
run lineage. Before purging hot terminal rows, GOET must retain or archive a
compact durable attempt record containing the required identities, fingerprints,
state hashes, timestamps, and terminal result.

Retention may remove large payloads earlier than compact provenance.

Reuse follows a convergence model. Each plugin defines one versioned canonical
state observation containing every external input and output relevant to the
operation. This may include input files, output files, remote metadata, or other
typed observations; functions with no input files remain valid.

Before mutation, the worker computes SHA-256 `pre_state_hash`. After successful
execution, it observes the same state domain again and computes
`post_state_hash`. On a later run, if the current pre-state hash equals the prior
successful post-state hash for the same composite execution fingerprint, the
requested operation is already satisfied and may be skipped.

State observations use deterministic ordering and unambiguous canonical encoding
including roles, names/paths, types, lengths, and selected metadata. They are not
formed by naively concatenating file hashes. A plugin that cannot observe enough
state to make a safe reuse decision must execute normally.

The initial model persists the typed logical output directly. External manifest
artifacts for very large collections are useful future work but are not required
by this epic.

## Relationship To Other Epics

- `dependency-aware-workflows` consumes this epic to retain run context and
  transition stages durably.
- `attempt-liveness-recovery` depends on persisted attempts, leases, and active
  claim state.
- `controller-resilience` owns broader controller-instance lifecycle and
  eventual high-availability behavior.
- `controller-retention-cleanup` should own detailed cleanup policy after this
  epic establishes which records and fingerprints are required for correctness.
- `workflow-compilation-resolution` owns the detailed resolver construction
  semantics consumed by this epic.
- `docs/fingerprints.md` defines the semantic fingerprint rules used by this
  epic.
- The existing attempt ledger is the starting persistence mechanism, not a
  separate source of execution truth.

## Proposed Slices

These are candidate slices for planning. They are not final implementation
instructions until explicitly agreed.

```text
001 Database Bootstrap and Schema Versioning

Create the store opener, schema metadata, empty-database bootstrap path,
foreign-key enforcement, and forward-only migration runner. Startup must fail
closed on unsupported newer schemas.

002 Canonical JSON and SHA-256 Helpers

Add shared helpers for canonical JSON bytes, SHA-256 hex strings, validation,
and tests for stable object ordering and hash reproducibility.

003 Core Execution Schema

Create the first workflow-execution schema for projects, workflows,
workflow_instances, workflow_stages, work_items, workers, attempts, and
placement/terminal tables.

004 Project and Workflow Persistence Methods

Add repository methods for immutable project/workflow revision upsert, lookup,
canonical hash storage, source locator storage, and safe unused-row deletion.

005 Workflow Run and Stage Persistence Methods

Add methods to create workflow runs, persist immutable submission snapshots,
insert ordered stage plans, query stage state, and list active runs after
restart.

006 Work Item and Queue Persistence Methods

Add methods to insert compiled work items, enqueue work, list queued/running
state, and reconstruct queue/status counts from the database.

007 Attempt Claim Transaction

Replace in-memory assignment authority with a transaction that moves one
queued_work row to work_item_attempts plus running_work and returns the claimed
payload to the worker only after commit.

008 Attempt Terminal Transition Transaction

Add idempotent completion/failure methods that validate the active attempt,
record output/error/state hashes, remove running_work, and optionally requeue the
logical work item.

009 Stage Completion and Ready-Work Publication

Add the durable transition that marks a stage complete exactly once and inserts
newly ready work items/queue rows in the same transaction.

010 Source-Control Abstraction

Define the controller-facing source-control interface for ref resolution,
pinned file reads, commit identity, safe path handling, and materialization of a
manifest into a staging directory.

011 GitHub Source-Control Implementation

Implement the first source-control adapter for GitHub, preserving the locator vs
semantic fingerprint distinction and returning repository/commit/path/blob
metadata where available.

012 Restart Reconstruction

On controller startup, rebuild active run, queue, running-attempt, and cache-pin
views from persisted rows without reconstructing an in-memory queue authority.

013 Controller Integration Cutover

Move `/workflow`, `/work/next`, `/work/complete`, `/work/fail`, `/status`, and
worker-scaling demand reads onto the store boundary and remove the old pending /
assigned / failed in-memory collections.
```

## Open Questions

1. Which execution-relevant controller variables are snapshotted into a run, and
   which remain deployment-only settings?
2. What retention periods apply to completed, failed, and lost operational
   records; where is compact provenance archived; and which project/controller
   configuration owns that policy?
3. Should `project_id` and `workflow_id` be user-provided stable names,
   content-derived IDs, or database/controller-generated IDs that carry
   canonical SHA-256 beside them?
4. Should the first GitHub implementation use a local bare Git cache immediately,
   or start with a thinner GitHub file-read implementation behind the same
   source-control interface?
5. What is the minimum stage table shape needed before dependency-aware
   workflows, and which stage fields should wait for that epic?
6. Should fingerprint algorithm versions be separate columns, embedded in each
   JSON document, or carried by schema version for the first implementation?
7. What exact canonical JSON rules should GOET adopt for numbers and
   schema-declared non-semantic fields?

## Completion Criteria

- Controller startup creates a valid empty database when none exists.
- Controller startup validates existing schema version and applies forward-only
  migrations transactionally.
- Controller startup fails closed when the database schema is newer than the
  binary understands.
- A shared canonical JSON and SHA-256 helper is used by persistence code.
- Project and workflow configuration hashes are computed from canonical content,
  not from source locator strings.
- GitHub repository, commit/object ID, path, Git blob identity when available,
  and GOET canonical SHA-256 are recorded as distinct concepts.
- A source-control abstraction exists and has a GitHub implementation.
- A submitted workflow run and its resolver inputs survive controller restart.
- A completed work item can be traced through stage instance, workflow run,
  workflow definition, and project identity.
- One logical work item may have multiple attempts without identity collision.
- Work-item claim and terminal transitions are transactional and idempotent.
- Each running row has one globally unique active attempt ID; retries preserve
  the work-item ID and create new attempt IDs.
- Run and zero-based stage identities are assigned at submission and carried
  unchanged from queued through running and terminal placement.
- A unique completed-stage record ensures successful stage completion compiles
  the next stage exactly once.
- A completed stage can cause newly ready work to be persisted exactly once.
- Later-step resolution uses immutable run snapshots rather than current project
  configuration.
- Typed outputs and SHA-256 pre/post-state hashes can be restored for downstream
  compilation and reuse decisions.
- Runnable, active, blocked, failed, and completed state can be reconstructed
  after restart.
- Queued, running, completed, and failed placement transitions use atomic
  insert/delete operations across separate current-state tables.
- Assignment, scaling, status, and recovery use the database as the only queue
  authority; no in-memory pending/assigned queue is maintained.
- Lost attempts remain attributable after their logical work item is requeued.
- Delete methods refuse unsafe deletion when referenced rows or active lifecycle
  state still exist.
- Terminal-state cleanup follows configured retention without deleting
  fingerprints, lineage, or output references still required for correctness.
- Schema creation and migration are versioned and tested.
- Migrations are forward-only and transactional, reject newer schemas, and use
  verified backup restoration rather than automatic down-migrations after a
  production schema is declared.
- Immutable definitions and configuration snapshots use schema-versioned,
  validated text JSON; scheduling identities and state remain relational.
- Existing ledger-based attempt history remains queryable or has an explicit
  migration path.

# Workflow Execution Persistence Epic

Status: Proposed

## Purpose

Make the database authoritative for workflow runs, resolver recipes, step state,
work items, attempts, logical outputs, source-control identities, and reusable
fingerprints so execution can continue correctly across controller restart.

The current controller keeps pending, assigned, and failed work primarily in
process memory. Dependency-aware JIT compilation requires durable state that
can trace a completed `work_item_id` back to its step and workflow run and
reconstruct the variable context used at submission from pinned sources,
submission context, and resolver recipes.

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
- Persist immutable source references, hashes, and resolver recipes for the
  project, workflow, submission overrides, and execution-relevant controller
  sources used by the run.
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
  resolved-input hashes, work-item payloads, output JSON, state observations, and
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

The controller remains the only process that talks directly to the main
database. The first database adapter is SQLite. Clients and workers interact
through HTTP APIs. The database is the execution source of truth; caches may
accelerate reads of immutable definitions, but must not become a second queue or
lifecycle authority.

The client submits or identifies a project and workflow source. The controller
loads the project and workflow from a pinned source-control revision, computes
canonical project/workflow SHA-256 values, creates `run_id`, and stores immutable
source references and resolver recipes. Later JIT compilation rebuilds the
resolver from those pinned sources, generated runtime variables, and completed
prior-step outputs; it does not read a newer project configuration.

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
3. Enable required database safety settings, including SQLite foreign-key
   enforcement for the first adapter.
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
context in `submission_context_json`, including source-control locator, source
commit, submission overrides or their pinned source reference, runtime variables,
controller/plugin versions when relevant, and the resolver recipe needed to
reconstruct run-time configuration from authoritative sources.
`submission_context_json` is a bounded list of key/variable pairs, not an
unstructured dump of source documents.

`workflow_stages` stores the ordered stage plan for a run. It should carry at
least `run_id`, `stage_index`, stage definition source/reference information,
state, timestamps, and optional output JSON/hash for stage-level outputs.
Initial state names may include `ready`, `running`, `completed`, `failed`,
`skipped`, and `blocked`.

`work_items` stores immutable compiled worker payloads and resolved-input
hashes. Resolved inputs are recreated from pinned configuration sources and the
resolver recipe; the stored hash verifies that reconstruction produced the same
semantic input before skip/reuse decisions rely on it. If a work item requires a
concrete worker payload after restart, that payload is persisted as compact
controller-generated JSON, initially shaped around the worker operation plugin
and resolved parameters:

```json
{
  "plugin": "plugin-name",
  "parameters": {
    "param1": "param1value"
  }
}
```

Repeated stage compilation must be idempotent through uniqueness such as
`(run_id, stage_index, work_item_index)`. A stage compilation that has no
external work still produces a deterministic skipped/no-op work item with typed
logical output `[]`, preserving the invariant that committed stage compilation
has durable work-item evidence.

`queued_work` and `running_work` represent current placement. `completed_work`
and `failed_work` represent terminal attempt outcomes. A `completed_work` row
may include `skipped_parent_id` when the row records reuse of an identical prior
completed result. Only `queued_work` and `running_work` should be treated as
current work location.

`workers` stores the minimum execution-environment-specific cancellation handle
needed after restart, such as a scheduler job ID, container ID, or process
identity. Liveness, heartbeat timing, and worker capability policy are separate
concerns owned by later recovery and scheduling slices.

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

## Source References and Resolver Recipes

Immutable workflow and project definitions remain in source control. The
database stores the source repository reference, repository-relative path,
resolved commit SHA, source object identity when available, canonical GOET
SHA-256, and schema/version metadata needed to reload the same document through
the controller's source-control cache.

The database should not become a duplicate document store for source-controlled
project, workflow, defaults, or controller JSON. Keeping those documents in
GitHub or a local source-control cache preserves provenance and avoids database
bloat. The database records which immutable document was used, not a second copy
of every source-controlled JSON file.

Query-critical identity, ordering, lifecycle, timestamps, and foreign keys
remain normalized relational columns. Compact controller-generated records that
do not already have a source-controlled home may be stored as canonical text JSON
when they are required for restart, audit, or downstream compilation.

A pinned source document must be reloadable from the local source-control cache
after submission. Remote source control is needed to create or refresh pins, not
to resume an already admitted run. Recovery should verify cached file content
against the stored canonical SHA-256 before trusting it.

SQLite JSONB is deferred. Text JSON is easier to inspect, export, test, and
migrate and does not couple stored definitions to SQLite's private binary
representation.

The intended resolver rule remains:

```text
Persist the resolver recipe and durable outputs of resolution, not the resolver.
```

A resolver is created for one specific lifecycle decision, resolves the required
values, and is discarded. Durable records must contain enough information to
reload canonical defaults/controller/project/workflow sources and reconstruct
the resolver recipe used for submission, stage compilation, work-item
compilation, assignment-time resolution, and output propagation.

## Stage Progression

When every required work item for a stage has completed successfully, the
controller records stage completion and compiles/persists newly ready work in the
same durable transition. A unique completed-stage identity `(run_id,
stage_index)` makes a repeated completion request harmless and prevents a stage
from compiling its successor twice.

Stage completion is derived from persisted work rows. For a given
`run_id/stage_index`, the stage is complete when every `work_items` row for that
stage has one matching successful `completed_work` terminal row, and there are
no remaining `queued_work`, `running_work`, or `failed_work` rows for that stage.
This allows the controller to determine stage completion by querying
`work_items` and terminal/current placement tables instead of maintaining a
separate in-memory stage counter.

A stage compilation that yields no external work creates one deterministic
skipped/no-op work item and records successful skipped completion with typed
output `[]`. It does not assign anything to a worker, but it still leaves
durable work-item and terminal-output evidence so downstream stage-completion
checks use the same persisted model as normal work.

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

Add methods to create workflow runs, persist immutable submission provenance,
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

## Planning Decisions

These decisions resolve the implementation-shaping questions for this epic. They
may still be revised by later implementation evidence, but they are strong
enough to guide the first slice decomposition.

### Controller Configuration Provenance

Workflow runs do not store a second materialized controller configuration.
Controller startup resolution already defines the authoritative source model:
canonical `defaults.json`, selected `controller.json`, allowed overrides,
captured environment access, generated runtime variables, and short-lived
resolvers.

The run record should persist the provenance needed to reconstruct the approved
execution-relevant recipe:

- defaults document source reference and canonical SHA-256;
- controller document source reference and canonical SHA-256;
- allowed submission or command overrides that affect the run;
- execution-environment identity and component types selected for the run;
- worker runtime image/artifact identity;
- plugin versions when they affect work-item behavior or state observation;
- generated runtime values that are part of the run lifecycle.

Deployment-only settings are not part of run provenance unless they affect
workflow semantics. Examples include controller listen host/port, HTTP timeout
values, local cache sizes, caretaker cadence, log level, and cleanup retention
policy. These may be recorded in operational diagnostics, but they are not used
to decide whether a run is reproducible or reusable.

### Identifier Ownership

Human-provided names and generated identities are distinct.

```text
project_name      optional human or customer-facing label
project_id        controller-generated immutable identity for one project config
project_sha256    canonical semantic fingerprint of project config

workflow_name     workflow-authored stable label
workflow_id       controller-generated immutable identity for one workflow config
workflow_sha256   canonical semantic fingerprint of workflow config

run_id            controller-generated identity for one submission
stage_index       controller-assigned zero-based position within the run
work_item_id      compiler-generated deterministic identity within run/stage
attempt_id        controller-generated identity at claim time
```

`project_id` and `workflow_id` should not be pure content hashes because they
also participate in database relationships and may need short, opaque,
controller-owned identities. The canonical SHA-256 values are stored beside
them and remain the semantic comparison mechanism.

Controller-generated database IDs should use UUIDv7. Table `created_at` fields
remain the authoritative timestamp columns for lifecycle queries, but UUIDv7
keeps generated IDs roughly time-ordered for index locality, logs, and operator
inspection. This does not change the distinction between human names, database
identities, and semantic SHA-256 values.

### Stage Minimum

Before dependency-aware workflows, `workflow_stages` needs only enough structure
to persist the ordered run plan and support later stage completion:

- `run_id`;
- `stage_index`;
- stable step or stage definition ID;
- stage source/reference information;
- lifecycle state;
- created, ready, started, completed, and failed timestamps as applicable;
- typed output JSON and output SHA-256 when the stage produces logical output.

Fields for dependency expressions, `parallel_with`, cross-workflow dependency
manifests, leases, and retry policy should wait for the dependency-aware and
liveness epics.

### Fingerprint and Canonical JSON Versioning

Fingerprint algorithm versioning should be stored explicitly beside each
fingerprint-bearing value. Database schema versioning says how rows are shaped;
fingerprint algorithm versioning says how semantic bytes were produced. The two
may advance independently.

The first implementation should use a single version label such as:

```text
canonical_json_v1_sha256
```

Canonical JSON version 1 should define stable object-key ordering, no
insignificant whitespace, stable string encoding, explicit null-versus-missing
treatment, and deterministic formatting for accepted integer JSON numbers.
Decimals should be represented as schema-defined strings rather than JSON
numbers. When a schema declares a field non-semantic, omission happens before
canonicalization and is recorded by the schema or document version.

### Store Boundary and Transactions

Persistence should expose one concrete `Store` type with task-oriented methods,
not one large public interface and not many repositories exposed directly to the
controller. Internally, `Store` may organize code into project, workflow, run,
work-item, attempt, and worker files as the package grows.

The persistence package owns database transactions. Controller code calls one
method for one lifecycle transition, such as claiming work or completing an
attempt. It should not assemble multi-table transactions across repository
objects.

### Source-Controlled Document Storage

Source-controlled documents should be referenced by immutable source identity,
not copied wholesale into SQLite:

- repository identity;
- resolved commit SHA;
- repository-relative path;
- source object identity when available;
- canonical GOET SHA-256;
- document schema/version metadata.

The controller's source-control cache is the local filesystem mechanism that
makes those pinned documents available during restart and later compilation.
GitHub or another source-control backend remains the provenance authority.
This source-control-first model is intentional: customer repositories are the
natural ownership and review boundary for project, workflow, controller, and
defaults JSON. The database records the immutable source identity and semantic
hashes needed to prove which customer-owned documents were admitted.

The cache should store pinned revisions under deterministic directories derived
from repository identity and commit SHA, such as:

```text
<cache-root>/<repo-name>-<commit-sha>/
```

The exact sanitization and collision rule belongs to the source-control
implementation slice. Cleanup may remove unpinned or no-longer-recoverable cache
entries to maintain size bounds, but it must not remove files needed by active
or recoverable admitted runs.

Compact controller-generated JSON may still be stored in the database when it
has no source-controlled home and is required for restart or downstream
decisions. Examples include `workflow_instances.submission_context_json`,
compiled work-item payloads, and typed logical outputs. Resolved inputs should
normally be reconstructed from pinned configs plus the resolver recipe and then
checked against stored hashes rather than stored wholesale. The implementation
should avoid storing large source documents in SQLite merely for convenience.

### Source-Control Scope

The source-control abstraction should provide pinned object retrieval and a
minimal materialization method. It should not own execution packaging policy.

In practice, source control owns:

- resolving refs to immutable commits;
- reading repository-relative files at a commit;
- returning repository, commit, path, and object identities;
- rejecting unsafe paths;
- materializing an explicit manifest into a destination directory.

A later packaging service owns deciding which files belong in the manifest and
how those files become worker artifacts.

The first GitHub implementation may be thin behind this interface. A local bare
Git cache remains the target shape, but cache retention and full cache
management are outside this epic.

### SQLite First

The first persistence implementation may be SQLite-specific, but SQLite details
should be contained behind a database-adapter boundary. Controller and
orchestration code should call persistence-level operations, not SQLite-specific
SQL or driver behavior.

The first implementation may use a file such as `db_adapter_sqlite.go` for
SQLite-specific connection, pragma, SQL, transaction, and migration details. A
future PostgreSQL implementation should be able to add a sibling adapter, such
as `db_adapter_postgres.go`, without rewriting controller lifecycle code.

This epic does not need to fully design PostgreSQL before a second backend
exists. It only requires keeping the SQLite dependency from leaking through the
controller-facing store boundary.

### Fingerprint Package

Canonical JSON and SHA-256 helpers should live in a small shared internal
package, such as `internal/fingerprint`, rather than in persistence,
source-control, workflow compilation, or plugin-specific code. Persistence uses
the helper to store hashes; source-control uses it to compute semantic document
hashes; workflow and plugin code use it when they define semantic identities.

### Migration Ownership

Schema migration infrastructure belongs inside the persistence package for now.
The controller should call `OpenStore` or equivalent; it should not know which
migrations exist or invoke individual migration steps.

In practical terms, migration ownership means the database adapter opens the
database, checks the recorded schema version, applies allowed forward migrations
transactionally, and rejects unsupported newer schemas before returning a usable
store. Controller startup treats this as one store-opening operation.

### Retention Ownership

This epic defines which records must exist and which lineage/fingerprint facts
must survive cleanup. `controller-retention-cleanup` owns retention periods,
cleanup scheduling, physical deletion policy, archive/compaction strategy, and
operator-facing cleanup configuration.

Persistence may provide safe primitives such as `Delete...IfUnused`; it should
not decide when old terminal records are old enough to purge.

### Database Cleanup and Retention Placeholder

Detailed cleanup policy is intentionally deferred to the
`controller-retention-cleanup` epic. That epic should define:

- retention periods for completed, failed, skipped, and lost attempts;
- whether retention is controller-wide, project-narrowable, or both;
- compact provenance rows that must survive hot-row cleanup;
- archive location and format for compact provenance;
- cleanup scheduling and backpressure behavior;
- operator-facing configuration variables;
- safeguards for source references, fingerprints, lineage, typed outputs, and
  state hashes required for audit, reuse, or downstream reconstruction.

Until that epic is implemented, persistence should avoid irreversible deletion
of terminal execution evidence except through narrow `Delete...IfUnused`
operations that prove the row is not referenced by active or terminal lifecycle
state.

### Controller-Created No-Op Work

Controller-created skipped/no-op work should use the same terminal model as
normal work by creating a controller-owned skipped attempt row. The attempt has
no worker claim, but it still records `attempt_id`, `work_item_id`, terminal
state, typed output `[]`, timestamps, and lifecycle variables.

This avoids a second terminal outcome shape and preserves the invariant that a
completed or skipped logical work item has durable attempt evidence.

## Deferred Design Edges

These ambiguities do not block the epic, but they should be settled in the
specific implementation slices that first need them:

- `submission_context_json` needs a narrow schema before implementation so it
  remains a bounded list of key/variable pairs.
- The local source-control cache needs a pinning rule: admitted-run files must
  not be evicted while any active or recoverable run depends on them.
- Compiled worker payload storage needs a size boundary so compact
  plugin/parameter payloads can restart correctly without turning SQLite into an
  artifact store.
- Controller-owned skipped attempts should carry an executor/source marker, and
  skipped/reused completed rows should point to the identical prior completion
  through `completed_work.skipped_parent_id`.
- Stage lifecycle transitions still need precise SQL constraints and transaction
  tests for the derived completion rule.

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
- A submitted workflow run and its resolver recipe survive controller restart.
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
- Later-step resolution uses immutable run provenance and pinned source
  references rather than current project configuration.
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
- Source-controlled definitions and configuration are reloaded through pinned
  source references; scheduling identities and state remain relational.
- Existing ledger-based attempt history remains queryable or has an explicit
  migration path.

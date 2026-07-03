# Workflow Execution Persistence Epic

Status: Proposed

## Purpose

Make the database authoritative for workflow runs, immutable resolver inputs,
step state, work items, attempts, and logical outputs so execution can continue
correctly across controller restart.

The current controller keeps pending, assigned, and failed work primarily in
process memory. Dependency-aware JIT compilation requires durable state that
can trace a completed `workitem_id` back to its step and workflow run and
reconstruct the exact variable context used at submission.

## Goals

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
- Support transactional completion, stage-state updates, and creation of newly
  ready work.
- Reconstruct runnable and blocked workflow state after controller restart.
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
      step position / step_instance_id
        workitem_id
          attempt_id
```

The client submits a workflow definition and `project_id`. The controller
creates `run_id`, loads project configuration by project ID, and stores
immutable execution-relevant snapshots. Later JIT compilation rebuilds the
resolver from those snapshots, generated runtime variables, and completed
prior-step outputs; it does not read a newer project configuration.

At submission, the controller normalizes the ordered workflow into stage
definitions and assigns every stage its zero-based `stage_index`. The `run_id`
and complete stage plan exist before any work is assigned, but only stage 0 is
compiled into `queued_work` initially. Work created for a stage carries its
immutable `run_id`, `stage_index`, and step position through every placement
table.

A work item is created and persisted when compilation produces logical work.
An attempt is created only when a worker claims that work item. Retries create
new attempts under the same `workitem_id`.

Claiming work is one atomic database transaction. The controller selects the
oldest eligible row from `queued_work`, generates a globally unique
`attempt_id`, inserts the immutable attempt identity and corresponding
`running_work` row, deletes the selected `queued_work` row, and commits. Any
failure rolls back the entire claim, so no worker receives work that the
database still considers queued or that lacks an active attempt.

Every `running_work` row references exactly one unique active `attempt_id` and
one `workitem_id`. A retried `workitem_id` creates a new attempt identity; an
old attempt ID is never reused. Uniqueness and foreign-key constraints enforce
that an attempt cannot claim multiple running rows and a work item cannot have
multiple active attempts.

Completion processing should atomically:

1. Validate and terminate the active attempt.
2. Record SHA-256 pre/post-state hashes and the typed logical output.
3. Update the logical work-item state.
4. Update its step/stage state.
5. Record any newly compiled work items exactly once.

The database is the sole queue authority. Assignment queries `queued_work`
directly and transitions the selected row to `running_work` transactionally.
Worker scaling derives demand from persisted queued/running state, and status
reports persisted counts. The controller does not maintain a second in-memory
pending or assigned queue, including a nominal cache that could become a
competing source of truth.

Immutable workflow definitions and configuration snapshots are stored as
canonical text JSON documents. Each document includes an explicit document
schema version, and its column uses `CHECK (json_valid(...))`. Query-critical
identity, ordering, lifecycle, timestamps, and foreign keys remain normalized
relational columns. Go normally decodes each immutable document as a whole;
SQLite JSON extraction is available for diagnostics and bounded migrations,
not as the primary scheduling interface.

SQLite JSONB is deferred. Text JSON is easier to inspect, export, test, and
migrate and does not couple stored definitions to SQLite's private binary
representation.

Database schema migrations are ordered and forward-only. At startup, the
controller reads `schema_version` and applies each pending migration in its own
transaction. Startup fails without mutation when the database schema is newer
than the controller understands. Destructive production migrations require a
verified backup first; rollback restores that backup rather than executing
automatic down-migrations.

GOET has no production release yet, so the first workflow-execution schema may
replace the experimental ledger schema and update repository-owned fixtures
without preserving backward compatibility. Once a production schema is
declared, all subsequent changes follow the forward migration policy.

Current logical work placement uses separate tables conceptually named:

```text
queued_work
running_work
completed_work
failed_work
```

Each row references an immutable `work_items` identity/definition record. A
state transition inserts the destination row and deletes the source row in one
transaction. Uniqueness constraints prevent one logical work item from
occupying multiple current-state tables after commit. This follows the desired
insert/delete model without copying the full work-item definition into every
state table.

The placement rows carry `run_id` and `stage_index` unchanged:

```text
queued_work -> running_work -> completed_work
```

When every required work item for stage 0 has completed successfully, the
controller records stage 0 completion and compiles/persists stage 1 work in the
same durable transition. The pattern repeats for later stage indexes. A unique
completed-stage identity `(run_id, stage_index)` makes a repeated completion
request harmless and prevents a stage from compiling its successor twice.

A stage compilation that yields zero work items records immediate successful
stage completion with typed output `[]` and continues to the next stage in the
same idempotent progression. It creates no placeholder work item or attempt.

Completion is addressed by `attempt_id` plus its fencing token. Repeating an
identical terminal report for the already recorded attempt succeeds
idempotently; a conflicting report or a report from a superseded token is
rejected.

Lost execution is primarily an attempt outcome rather than a terminal logical
work-item state. A lost attempt is recorded durably, while the same
`workitem_id` may return to `queued_work` and later create a new attempt. If a
`lost_work` operational table is retained for caretaker processing, its rows
must identify the lost `attempt_id`; moving the logical work item back to the
queue must not erase attempt history.

Completed and failed operational tables require configurable retention and
regular cleanup because they can grow without bound. Cleanup cannot blindly
delete the evidence required for reuse, audit, downstream output restoration,
or run lineage. Before purging hot terminal rows, GOET must retain or archive a
compact durable attempt record containing the required identities,
fingerprints, state hashes, timestamps, and terminal result.
Retention may remove large payloads earlier than compact provenance.

Reuse follows a convergence model. Each plugin defines one versioned canonical
state observation containing every external input and output relevant to the
operation. This may include input files, output files, remote metadata, or
other typed observations; functions with no input files remain valid.

Before mutation, the worker computes SHA-256 `pre_state_hash`. After successful
execution, it observes the same state domain again and computes
`post_state_hash`. On a later run, if the current pre-state hash equals the
prior successful post-state hash for the same composite execution fingerprint,
the requested operation is already satisfied and may be skipped.

State observations use deterministic ordering and unambiguous canonical
encoding including roles, names/paths, types, lengths, and selected metadata.
They are not formed by naively concatenating file hashes. A plugin that cannot
observe enough state to make a safe reuse decision must execute normally.

The initial model persists the typed logical output directly. External
manifest artifacts for very large collections are useful future work but are
not required by this epic.

## Relationship To Other Epics

- `dependency-aware-workflows` consumes this epic to retain run context and
  transition stages durably.
- `attempt-liveness-recovery` depends on persisted attempts, leases, and active
  claim state.
- `controller-resilience` owns broader controller-instance lifecycle and
  eventual high-availability behavior.
- The existing attempt ledger is the starting persistence mechanism, not a
  separate source of execution truth.

## Proposed Slices

No implementation slices are agreed yet. They will be drafted after the
remaining configuration-snapshot and retention policies are agreed.

## Open Questions

1. Which execution-relevant controller variables are snapshotted into a run,
   and which remain deployment-only settings?
2. What retention periods apply to completed, failed, and lost operational
   records; where is compact provenance archived; and which project/controller
   configuration owns that policy?

## Completion Criteria

- A submitted workflow run and its resolver inputs survive controller restart.
- A completed work item can be traced through step instance, workflow run,
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
- Terminal-state cleanup follows configured retention without deleting
  fingerprints, lineage, or output references still required for correctness.
- Schema creation and migration are versioned and tested.
- Migrations are forward-only and transactional, reject newer schemas, and use
  verified backup restoration rather than automatic down-migrations.
- Immutable definitions and configuration snapshots use schema-versioned,
  validated text JSON; scheduling identities and state remain relational.
- Existing ledger-based attempt history remains queryable or has an explicit
  migration path.

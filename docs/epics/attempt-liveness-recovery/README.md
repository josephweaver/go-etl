# Attempt Liveness and Caretaker Recovery Epic

Status: Proposed

## Purpose

Detect workers that stop reporting through an outbound heartbeat API and an
in-memory controller heartbeat tracker, then use a scheduled caretaker to move
their current attempts from `running_work` to `failed_work`, requeue the logical
work, and request best-effort termination of the abandoned worker.

Heartbeat observations are intentionally ephemeral. The database remains
authoritative for current attempt ownership and terminal attempt outcomes, but
it is not updated for every heartbeat.

## Goals

- Add an outbound worker heartbeat API for active attempts.
- Track the latest heartbeat/report time for current attempts in controller
  memory.
- Configure the worker reporting timeout through the serialized controller
  variable document.
- Start the caretaker after a configured recovery window so workers have time
  to report to a restarted controller.
- Accept heartbeats and terminal reports while normal submission and work-claim
  APIs remain gated during restart recovery.
- Treat absence from the in-memory tracker after the recovery deadline as an
  abandoned attempt.
- Atomically append the abandoned attempt to `failed_work`, delete its
  `running_work` ownership row, and return the same `work_item_id` to
  `queued_work`.
- Use absence of the matching `running_work` row as the database fence for
  heartbeats and terminal reports from abandoned attempts.
- Return a stale-attempt response that tells a reachable abandoned worker to
  exit.
- Use persisted `workers.worker_state_json` to request best-effort cancellation
  through the project execution environment, such as cancelling a Slurm job by
  job ID.
- Make heartbeat, caretaker, abandonment, requeue, cancellation, and recovery
  state observable.
- Preserve the assumption that work-item operations are idempotent even when an
  old worker continues running after its attempt is abandoned.

## Non-Goals

- Persisting every heartbeat or implementing renewable database leases.
- Guaranteeing that a timed-out worker process has stopped before its work item
  is requeued.
- Guaranteeing exactly-once external side effects.
- Requiring workers to accept inbound controller connections.
- Making scheduler cancellation a prerequisite for retry.
- Defining workflow step dependencies or Case 3 compilation behavior.
- Coordinating multiple active controllers or implementing leader election.
- Building a separate caretaker service in the first implementation.

## Architectural Context

Workers may run behind NAT, inside containers, or on HPC compute nodes that the
controller cannot contact directly. Workers therefore report outbound to the
controller. Each heartbeat identifies at least:

```text
worker_id
work_item_id
attempt_id
```

The heartbeat handler first verifies that the matching `(attempt_id,
work_item_id)` still exists in `running_work`. If it does, the handler records
the current report time in the in-memory tracker. If it does not, the handler
makes no state change and tells the worker that its attempt is stale.

Completion and failure reports follow the same ownership rule. The transaction
that deletes `running_work` wins a race between completion and caretaker
abandonment:

- if completion removes the row first, the caretaker does nothing;
- if abandonment removes the row first, the late completion does nothing.

The initial timing variables are conceptually:

```text
runtime.controller_started_at
runtime.controller_recovery_started_at
controller_config.worker_timeout_seconds
worker_config.heartbeat_interval_seconds
```

`controller_recovery_started_at` is captured when the heartbeat/report endpoint
becomes available. The restart recovery deadline is:

```text
controller_recovery_started_at + worker_timeout_seconds
```

This gives workers a full reporting interval even when database migration or
Git-cache recovery delays HTTP startup.

### Restart recovery

After normal controller bootstrap, the controller enters recovery API mode:

```text
load persisted running attempts
        |
        v
start heartbeat/report API and empty in-memory tracker
        |
        v
wait worker_timeout_seconds while accepting reports
        |
        v
caretaker compares running_work with observed attempt IDs
        |
        +-- observed --> keep running
        |
        +-- absent ----> abandon attempt and requeue work item
        |
        v
enter normal API mode
```

A valid heartbeat, completion, or failure report counts as contact. The
controller does not require a heartbeat immediately before a terminal report.

### Normal caretaker operation

After recovery, the tracker retains the most recent accepted report time for
each current attempt. On its configured schedule, the caretaker finds entries
whose last report is older than `worker_timeout_seconds`. It rechecks
`running_work` inside the abandonment transaction before recording failure and
requeueing.

The heartbeat interval must be materially shorter than the worker timeout and
allow for expected scheduler and network delay. The tracker removes entries
when their attempts leave `running_work`.

### Abandonment and cancellation

Abandonment is one database transaction:

```text
verify current running_work ownership
→ insert failed_work with timeout/abandoned error_json
→ delete running_work
→ insert queued_work for the same work_item_id
```

A later claim creates a new `attempt_id`. The old failed attempt remains
history. After commit, the caretaker reads the attempt's `worker_id`, loads the
minimal cancellation handle from `workers.worker_state_json`, and asks the
project execution environment to terminate the worker. Cancellation is
idempotent and best-effort; failure is reported through logs, metrics, and
status but does not roll back requeueing.

Because heartbeat state is ephemeral, a controller crash intentionally loses
all observations. The recovery window reconstructs current liveness from fresh
worker contact. The persisted `running_work`, `failed_work`, `queued_work`,
attempt, worker, and cancellation-state records are sufficient to reconstruct
authoritative placement and cleanup decisions.

## Relationship to Other Epics

- `workflow-execution-persistence` owns durable attempts, placement tables,
  failed outcomes, and atomic requeue transitions.
- `dependency-aware-workflows` consumes terminal attempt/work-item state but
  does not own liveness detection.
- `controller-resilience` owns controller-instance lifecycle and the
  single-controller restart boundary.
- Project execution-environment adapters interpret `worker_state_json` and
  implement best-effort worker cancellation.
- Worker plugins/runtime retain responsibility for idempotent or atomically
  published external effects.

## Proposed Slices

No implementation slices are agreed yet. They will be drafted only after the
remaining API, scheduling, and concurrency questions are resolved and the epic
is explicitly moved from `Proposed` to `Ready`.

## Open Questions

1. Is `worker_timeout_seconds` also the caretaker interval, or should the
   caretaker have a separate shorter scan interval?
2. What HTTP status and response body instruct a worker with a stale attempt to
   terminate?
3. Does worker registration itself count as an initial heartbeat, or does the
   timer begin when the attempt is claimed?
4. How does the in-memory tracker bound and clean entries when reports race with
   completion or abandonment?
5. Should a terminal report received during recovery immediately remove the
   attempt from the recovery candidate set, or is querying `running_work`
   sufficient?
6. Which logs, metrics, and status fields expose observed, timed-out,
   abandoned, requeued, cancellation-requested, and cancellation-failed state?
7. What minimum authentication prevents one worker from refreshing another
   worker's attempt?

## Completion Criteria

- Workers report active attempt identity through an outbound heartbeat API.
- Heartbeats update an in-memory tracker without writing heartbeat timestamps
  to the database.
- Restart exposes heartbeat/report endpoints for a full configured recovery
  window before abandoning non-reporting running attempts.
- Normal API admission remains gated until initial caretaker recovery finishes.
- The caretaker rechecks `running_work` and performs abandonment/requeue in one
  transaction.
- Late reports cannot mutate an attempt after its `running_work` row is gone
  and tell the stale worker to exit.
- A retry preserves `work_item_id` and receives a new `attempt_id`.
- Persisted worker cancellation state is used for best-effort zombie cleanup.
- Controller restart reconstructs authoritative work placement without durable
  heartbeat history.
- Heartbeat tracking remains bounded and race-safe under concurrent reports,
  completion, abandonment, and shutdown.
- Relevant API, caretaker, persistence, cancellation, restart, and integration
  tests pass.

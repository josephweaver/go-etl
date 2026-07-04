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
- Track whether each current attempt reported at least once during each
  caretaker interval.
- Give workers several consecutive caretaker intervals in which to report
  before declaring an attempt abandoned.
- Configure the caretaker schedule and allowed missed-interval count through
  the serialized controller variable document.
- Accept heartbeats and terminal reports while normal submission and work-claim
  APIs remain gated during restart recovery.
- Treat absence from the in-memory tracker after the recovery deadline as an
  abandoned attempt.
- Atomically append the abandoned attempt to `failed_work`, delete its
  `running_work` ownership row, and return the same `work_item_id` to
  `queued_work`.
- Use absence of the matching `running_work` row as the database fence for
  heartbeats and terminal reports from abandoned attempts.
- Ignore stale heartbeats and terminal reports for state mutation without
  requiring the worker to stop its current idempotent operation.
- Refuse another work claim from a worker whose latest attempt was abandoned so
  the worker shuts down after finishing or failing its old operation.
- Retain persisted `workers.worker_state_json` so a cleanup policy may request
  best-effort cancellation through the project execution environment, such as
  cancelling a Slurm job by job ID.
- Make heartbeat, caretaker, abandonment, requeue, cancellation, and recovery
  state observable.
- Preserve the assumption that work-item operations are idempotent even when an
  old worker continues running after its attempt is abandoned.

## Non-Goals

- Persisting every heartbeat or implementing renewable database leases.
- Guaranteeing that an abandoned worker process has stopped before its work
  item is requeued.
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
work_item_id)` still exists in `running_work`. If it does, the handler marks the
attempt observed in the current in-memory caretaker interval. If it does not,
the handler makes no state change. The worker does not need to interpret a
stale heartbeat response and may continue trying to finish its idempotent
operation.

Completion and failure reports follow the same ownership rule. The transaction
that deletes `running_work` wins a race between completion and caretaker
abandonment:

- if completion removes the row first, the caretaker does nothing;
- if abandonment removes the row first, the late completion does nothing.

The initial timing variables are conceptually:

```text
runtime.controller_started_at
runtime.controller_recovery_started_at
controller_config.caretaker_interval_schedule
controller_config.caretaker_missed_interval_limit
worker_config.heartbeat_interval_schedule
```

`controller_recovery_started_at` is captured when the heartbeat/report endpoint
becomes available. The earliest abandonment time is approximately:

```text
controller_recovery_started_at
+ (caretaker interval × missed interval limit)
```

This gives workers multiple reporting opportunities even when one or more
heartbeats are lost. The worker heartbeat interval should be shorter than the
caretaker interval so a healthy worker normally has multiple chances to report
within each interval.

### Restart recovery

After normal controller bootstrap, the controller enters recovery API mode:

```text
load persisted running attempts
        |
        v
start heartbeat/report API and empty in-memory tracker
        |
        v
run caretaker on its configured interval schedule
        |
        v
atomically consume reports observed during that interval
        |
        +-- observed --> reset consecutive misses to zero
        |
        +-- absent ----> increment consecutive misses
                              |
                              +-- below limit --> keep waiting
                              +-- at limit ----> abandon and requeue
        |
        v
enter normal API mode
```

A valid heartbeat, completion, or failure report counts as contact. The
controller does not require a heartbeat immediately before a terminal report.

### Normal caretaker operation

The in-memory tracker contains:

- a concurrency-safe set of attempt IDs observed in the current interval;
- a consecutive-missed-interval count for current attempts.

At each scheduled caretaker run, it atomically swaps the observed set for a new
empty set, then compares the consumed set with current `running_work` rows. One
or many heartbeats in an interval have the same meaning: the worker reported at
least once. An observed attempt resets its missed count to zero. An unobserved
attempt increments its count and is abandoned only when the configured limit
is reached.

An attempt newly claimed during an interval is initialized as observed so it
receives a complete future interval before its first possible miss. Tracker
entries and counters are removed when attempts leave `running_work`.

The caretaker schedule is independent from the worker heartbeat schedule. The
worker heartbeat interval must be shorter and allow for expected scheduler and
network delay.

### Abandonment and cancellation

Abandonment is one database transaction:

```text
verify current running_work ownership
→ insert failed_work with timeout/abandoned error_json
→ delete running_work
→ insert queued_work for the same work_item_id
```

A later claim creates a new `attempt_id`. The old failed attempt remains
history. A heartbeat or terminal report from the old attempt is ignored because
its `running_work` row no longer exists.

The abandoned worker may continue its idempotent operation. When it eventually
requests more work, the controller recognizes that the worker's latest attempt
was abandoned, refuses another assignment, and the worker shuts down. A
separate cleanup policy may instead read `workers.worker_state_json` and ask the
project execution environment to terminate it. Cancellation remains
idempotent, best-effort, and independent from the correctness of requeueing.

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

1. What schedule syntax represents `caretaker_interval_schedule` and
   `heartbeat_interval_schedule` before GOET has a duration type?
2. What default missed-interval limit gives workers enough opportunities
   without delaying recovery excessively?
3. What response should stale heartbeat and terminal-report APIs return while
   making clear that no state mutation occurred?
4. How does the in-memory tracker bound and clean entries when reports race with
   completion or abandonment?
5. Should a terminal report received during recovery immediately remove the
   attempt from the recovery candidate set, or is querying `running_work`
   sufficient?
6. Which logs, metrics, and status fields expose observed, missed-interval,
   abandoned, requeued, cancellation-requested, and cancellation-failed state?
7. What minimum authentication prevents one worker from refreshing another
   worker's attempt?
8. When should optional scheduler cancellation be attempted instead of allowing
   an abandoned idempotent worker to finish and exit after its next claim is
   refused?

## Completion Criteria

- Workers report active attempt identity through an outbound heartbeat API.
- Heartbeats mark an attempt observed in an in-memory interval tracker without
  writing heartbeat timestamps to the database.
- Caretaker and worker heartbeat schedules are independently configured.
- Attempts are abandoned only after the configured number of consecutive
  caretaker intervals without a report.
- Restart exposes heartbeat/report endpoints for the full configured number of
  caretaker intervals before abandoning non-reporting running attempts.
- Normal API admission remains gated until initial caretaker recovery finishes.
- The caretaker rechecks `running_work` and performs abandonment/requeue in one
  transaction.
- Late reports cannot mutate an attempt after its `running_work` row is gone.
- A worker whose latest attempt was abandoned is refused another assignment so
  it exits after finishing its old idempotent operation.
- A retry preserves `work_item_id` and receives a new `attempt_id`.
- Persisted worker cancellation state is used for best-effort zombie cleanup.
- Controller restart reconstructs authoritative work placement without durable
  heartbeat history.
- Heartbeat tracking remains bounded and race-safe under concurrent reports,
  completion, abandonment, and shutdown.
- Relevant API, caretaker, persistence, cancellation, restart, and integration
  tests pass.

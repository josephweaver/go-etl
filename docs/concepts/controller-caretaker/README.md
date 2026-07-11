# Controller CareTaker SC/OS Package

This package defines the strategic concept and operational slices for moving GORC worker lifecycle recovery and worker launch decisions into a single event-driven controller `CareTaker`.

## Repository basis

Prepared against `josephweaver/go-etl` `main` as inspected on 2026-07-11.

The current implementation already contains:

- `cmd/controller/worker_execution.go`, including `WorkerCapacityManager`, inflight start reservations, and `one_by_one_until_saturated`.
- direct `EvaluateWorkerCapacity` calls from controller startup, raw work submission, workflow admission, work completion, and post-claim asynchronous evaluation.
- `queued_work`, `running_work`, `workers`, and `work_item_attempts` persistence tables.
- a worker loop that exits when `/work/next` returns no work.
- no worker registration, worker session, heartbeat, graceful-stop, assignment fencing, or abandoned-attempt history.

This package replaces the current approximation

```text
active capacity = running attempts + inflight starts
```

with durable live worker sessions:

```text
live capacity = worker sessions whose most recent controller-received
heartbeat is within worker_dead_after
```

## Documents

Implementation order:

1. `SC-controller-caretaker.md`
   - Full architecture, invariants, state machines, wake semantics, and rollout.

2. `OS-001-worker-lifecycle-persistence-foundation.md`
   - Schema version, worker sessions, abandoned-attempt history, and persistence queries.

3. `OS-002-worker-registration-heartbeat-and-stop-protocol.md`
   - Controller endpoints and worker-side background heartbeat lifecycle.

4. `OS-003-assignment-fencing-abandonment-and-requeue.md`
   - Session-bound claims, late-outcome rejection, and atomic dead-worker recovery.

5. `OS-004-event-driven-caretaker-reconciliation-loop.md`
   - Blocking CareTaker loop, deadline timers, capacity calculation, launch retry, and lifecycle.

6. `OS-005-exclusive-scheduler-cutover-and-state-change-wakes.md`
   - Remove API-triggered scheduling, signal the CareTaker after queue/running transitions, and prove it is the sole launch authority.

## Target branch

```text
concept/controller-caretaker
```

Implement the OS documents in order. Each slice should leave the repository compiling and its focused tests passing.

## Final behavior

```text
controller state mutation commits
        |
        +-- signal CareTaker through capacity-one wake channel
                |
                +-- coalesce duplicate signals
                +-- expire dead worker sessions
                +-- abandon and requeue their running assignments
                +-- load claimable queued work
                +-- count live worker sessions and inflight starts
                +-- request at most the configured start increment
                +-- sleep until:
                        - queued_work or running_work changes,
                        - worker registration or stop changes capacity,
                        - a heartbeat deadline arrives,
                        - an inflight launch deadline arrives,
                        - a retry/fallback deadline arrives,
                        - controller shutdown
```

A successful heartbeat updates liveness but does not normally wake the CareTaker. The previously scheduled heartbeat-expiry timer may fire early, observe the renewed heartbeat, and calculate a later deadline. This avoids waking the scheduler for every heartbeat while retaining correct expiry behavior.

# SC: Controller CareTaker

## Status

Proposed.

## Strategic decision

Introduce one controller-owned `CareTaker` goroutine that is the exclusive authority for:

1. reconciling durable work demand with live worker capacity;
2. requesting additional worker launches through the existing worker launch backend;
3. detecting expired worker sessions;
4. fencing dead worker sessions;
5. atomically abandoning and requeueing work owned by dead sessions; and
6. sleeping until durable work state, worker capacity, or a relevant deadline changes.

HTTP handlers and workflow transition code may signal the CareTaker, but they must not invoke worker scheduling directly.

## Problem

The current controller can launch workers, but active capacity is inferred from:

```text
running attempts + unexpired inflight starts
```

This approximation cannot distinguish:

- a healthy idle worker from no worker;
- a running assignment whose process has died from a healthy running worker;
- a worker that started but never claimed work from an available worker;
- a late completion from a dead worker from a valid completion;
- a worker that exited after receiving `204 No Content` from a worker that remains available.

Scheduling is also coupled to request paths. Current code calls `EvaluateWorkerCapacity` from startup and several HTTP-driven transitions. This creates two undesirable properties:

1. a scheduling/launch error can turn an otherwise successful state mutation into an HTTP 500 response; and
2. every new state transition must remember to call the scheduler synchronously or spawn another scheduling goroutine.

The controller needs a durable worker-liveness model and one reconciliation owner.

## Current implementation audit

### Existing scheduling components

`cmd/controller/worker_execution.go` contains:

- `WorkerDemand`;
- `WorkerExecutionConfig`;
- `WorkerCapacityManager`;
- inflight start reservations;
- `OneByOneUntilSaturatedPattern`;
- `EvaluateWorkerCapacity`;
- `ConfirmWorkerStartClaimedAndEvaluateAsync`.

The older `cmd/controller/worker_scaler.go` start-planning state was removed during the exclusive scheduler cutover after references were eliminated.

The existing launch backend remains useful:

- `Controller.startWorkers`;
- configured `ExecutionEnvironment`;
- `Scheduler.Submit`;
- `DirectProcessScheduler`;
- `SlurmScheduler`;
- `LocalWorkerStarter`.

The CareTaker does not replace these launch backends. It becomes their only controller-side caller.

### Existing direct scheduler call sites

The implementation currently invokes worker-capacity evaluation from:

1. controller startup after recovery;
2. raw `/work` submission;
3. workflow-run admission;
4. cache-data completion after dependent work is queued;
5. ordinary completion after stage advancement; and
6. `/work/next` through `ConfirmWorkerStartClaimedAndEvaluateAsync`.

These calls must be replaced by CareTaker wake signals.

The work-failure path also changes `running_work` and therefore must signal the CareTaker even though it does not currently evaluate capacity.

### Existing persistence model

The persistence schema already includes:

```text
workers
work_item_attempts
queued_work
running_work
completed_work
failed_work
```

However:

- `workers` has no status, session, or heartbeat;
- worker claims currently do not provide a worker identity;
- `running_work.worker_id` may be empty;
- completion and failure are authorized by attempt ID rather than active assignment ownership;
- there is no `abandoned_work` attempt-history table.

### Existing worker behavior

The normal worker:

1. requests `/work/next`;
2. executes the returned work;
3. reports completion or failure;
4. requests the next item; and
5. exits immediately when no work is available.

A five-minute controller heartbeat timeout alone is therefore insufficient. A healthy worker that exits normally would otherwise remain counted as live for five minutes. The worker must explicitly register, heartbeat during long-running work, and report graceful stop before exit.

## Goals

The finished design must provide:

- durable worker sessions;
- controller-received heartbeats;
- a configurable five-minute default death threshold;
- session-bound assignment ownership;
- rejection of claims and outcomes from dead, stopped, expired, or mismatched sessions;
- atomic dead-session transition, attempt abandonment, and work requeue;
- event-driven scheduling after `queued_work` or `running_work` changes;
- dynamic deadline wakeups for heartbeat and launch-reservation expiry;
- bounded retry after reconciliation or launch failure;
- one CareTaker goroutine per controller process;
- no direct scheduling from web API handlers;
- no scheduler failure propagated through an already-committed submission, claim, completion, or failure response.

## Non-goals

This SC does not add:

- worker scale-down commands;
- preemption of healthy workers;
- more than one concurrent assignment per worker session;
- durable distributed leader election for multiple controller processes;
- exactly-once external side effects;
- checkpointing of arbitrary worker processes;
- Slurm job cancellation after a session is declared dead;
- cross-controller worker pools;
- workflow JSON changes.

The existing single-controller database ownership rule remains the CareTaker leadership mechanism.

## Terminology

### Queued or pending work

The current durable table is `queued_work`.

In this SC, "pending work" means current work-item state represented by `queued_work`, not a new `pending_work` table.

### Claimable queued work

Queued work that is eligible for immediate assignment after resource constraints and other admission rules are evaluated.

### Running work

A current assignment represented by `running_work`.

### Worker

A logical worker record in `workers`.

### Worker session

One process lifetime or scheduler-job lifetime registered with the controller.

A process restart creates a new session. A dead or stopped session never becomes active again.

### Live worker session

A session is live at controller time `now` when:

```text
status = active
and last_heartbeat_at >= now - worker_dead_after
```

The controller's receipt time is authoritative. Client clocks do not determine liveness.

### Inflight worker start

A launch request that succeeded at the launch-backend boundary but has not yet produced a registered worker session.

### Assignment ownership

The tuple:

```text
attempt_id
worker_id
worker_session_id
```

Only the live session owning that tuple may complete or fail the attempt.

### Abandoned attempt

An attempt whose worker session ended or expired before a valid terminal outcome was accepted.

The attempt remains historical. Its work item is placed back in `queued_work`.

## Core invariants

### Invariant 1: One launch authority

Only the CareTaker reconciliation path may call a worker launch backend.

No HTTP handler, workflow compiler, stage-activation helper, claim handler, completion handler, or failure handler may call:

```text
EvaluateWorkerCapacity
startWorkers
startConfiguredWorkers
WorkerStarter.StartWorker
Scheduler.Submit
```

for automatic worker scaling.

Explicit administrative/test-only launch commands, if added later, must be clearly separate from automatic scheduling.

### Invariant 2: Durable state is authoritative

Wake signals are hints that durable state may have changed. They are not state.

Every reconciliation reloads:

- claimable queued demand;
- running assignments;
- live worker sessions;
- inflight launch reservations.

A coalesced or duplicated signal must not change correctness.

### Invariant 3: Active capacity comes from sessions

Running attempts are demand occupancy, not proof that a worker process is alive.

```text
observed_capacity = live_worker_sessions + unexpired_inflight_starts
```

### Invariant 4: One concurrent assignment per session

A live worker session may own at most one row in `running_work`.

A database uniqueness constraint or transaction predicate must enforce this.

### Invariant 5: Session-bound outcomes

A completion or failure is accepted only when:

```text
running_work.attempt_id = request.attempt_id
running_work.worker_id = request.worker_id
running_work.worker_session_id = request.worker_session_id
worker_session.status = active
worker_session.last_heartbeat_at >= controller_now - worker_dead_after
```

A stale attempt ID is insufficient authorization.

### Invariant 6: Dead sessions never resurrect

After a session enters `dead` or `stopped`:

- heartbeat returns a conflict/session-not-active response;
- claim returns a conflict/session-not-active response;
- completion and failure return assignment-not-owned;
- the same session ID is never returned to `active`.

A restarted worker registers a new session.

### Invariant 7: Recovery is atomic

For each expired or explicitly stopped session with running work, one transaction must:

1. mark the session terminal;
2. insert an `abandoned_work` record for each owned running attempt;
3. remove those attempts from `running_work`;
4. reinsert their work items into `queued_work`; and
5. commit all state together.

There must be no committed state in which the session is dead while its work remains assigned, or in which work is requeued without preserving the abandoned attempt.

### Invariant 8: State commits precede wake signals

A state-changing operation signals the CareTaker only after its transaction commits.

The signal may be sent even when the operation only might have changed `queued_work` or `running_work`; duplicate signals are deliberately coalesced.

### Invariant 9: Request success is independent of launch success

Once an API mutation has committed, a later CareTaker launch failure cannot change that API operation into a failure.

The CareTaker records the error and retries according to its backoff/deadline rules.

### Invariant 10: Requeue is at-least-once execution

A worker may continue computing after the controller expires its session. The controller fences its result and requeues the work.

Therefore work execution is at least once. Worker operations and artifact promotion must remain idempotent or protected by existing commit/fingerprint semantics.

## Worker state machine

```text
                         register
                            |
                            v
                         active
                     /      |       \
           graceful stop    |        heartbeat expires
                  /         |                 \
                 v          |                  v
              stopped       |                 dead
                             |
                     controller rejects
                  heartbeat/claim/outcome
```

Allowed transitions:

```text
new -> active
active -> stopped
active -> dead
```

Disallowed transitions:

```text
dead -> active
stopped -> active
dead -> stopped
stopped -> dead
```

A new process uses a new session.

## Assignment state machine

```text
queued_work
    |
    | live session claims
    v
running_work
   /       |          \
complete   fail     session dead/stopped
  |         |              |
  v         v              v
completed  failed       abandoned_work
                              |
                              | same transaction
                              v
                          queued_work
```

An abandoned attempt is historical and the work item is current again in `queued_work`.

## Proposed persistence shape

Preserve `workers` as the logical worker record and add a session table.

```sql
CREATE TABLE worker_sessions (
    worker_session_id TEXT PRIMARY KEY,
    worker_id TEXT NOT NULL,
    status TEXT NOT NULL
        CHECK (status IN ('active', 'stopped', 'dead')),
    registered_at TEXT NOT NULL,
    last_heartbeat_at TEXT NOT NULL,
    ended_at TEXT,
    end_reason TEXT,
    execution_handle TEXT,

    FOREIGN KEY (worker_id) REFERENCES workers(worker_id)
);
```

Add session ownership to attempts and current assignments:

```text
work_item_attempts.worker_session_id
running_work.worker_session_id
```

For worker-executed attempts:

```text
worker_id is required
worker_session_id is required
```

For controller-executed attempts both may remain null.

Add historical abandonment:

```sql
CREATE TABLE abandoned_work (
    attempt_id TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL,
    worker_id TEXT NOT NULL,
    worker_session_id TEXT NOT NULL,
    queued_at TEXT NOT NULL,
    started_at TEXT NOT NULL,
    abandoned_at TEXT NOT NULL,
    reason TEXT NOT NULL,

    FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
    FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
    FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
    FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id)
);
```

Recommended indexes:

```text
worker_sessions(status, last_heartbeat_at)
worker_sessions(worker_id, registered_at)
running_work(worker_session_id)
abandoned_work(work_item_id, abandoned_at)
```

The schema version must advance once for this concept. Do not silently treat a version-5 core schema as version 6. Use the repository's adopted development-schema policy: either add an explicit migration or fail closed with a clear rebuild instruction.

## Worker protocol

### Register

```http
POST /workers/register
```

Request metadata may include:

```json
{
  "execution_handle": "optional scheduler/process handle",
  "execution_environment": "hpcc"
}
```

The controller generates identifiers and records the registration at controller time:

```json
{
  "worker_id": "worker-...",
  "worker_session_id": "worker-session-...",
  "heartbeat_interval_seconds": 60,
  "dead_after_seconds": 300
}
```

Because the initial execution pattern allows at most one unconfirmed launch at a time, a successful registration may confirm the oldest inflight start reservation. Manually started workers may register when no reservation exists.

### Heartbeat

```http
POST /workers/heartbeat
```

```json
{
  "worker_id": "worker-...",
  "worker_session_id": "worker-session-..."
}
```

Success:

```text
204 No Content
```

Unknown, stopped, dead, or mismatched session:

```text
409 Conflict
worker_session_not_active
```

The controller stores its own current time as `last_heartbeat_at`.

### Claim

Retain the current route initially:

```http
GET /work/next
```

Require worker identity through headers:

```text
X-GORC-Worker-ID
X-GORC-Worker-Session-ID
```

This avoids a GET request body while minimizing route churn. A future API version may replace it with `POST /work/claim`.

### Complete and fail

Add worker and session identity to completion and failure payloads.

Invalid ownership returns:

```text
409 Conflict
assignment_no_longer_owned
```

### Graceful stop

```http
POST /workers/stop
```

```json
{
  "worker_id": "worker-...",
  "worker_session_id": "worker-session-...",
  "reason": "no_work"
}
```

The worker cancels its heartbeat goroutine before calling stop.

A stop with an owned running attempt uses the same atomic abandonment/requeue path as expiry. Under normal operation, the worker calls stop only after it has no assigned work.

## Heartbeat timing

Recommended defaults:

```text
worker_heartbeat_interval = 1m
worker_dead_after = 5m
```

Validation:

```text
heartbeat interval > 0
dead-after > heartbeat interval
dead-after >= 2 * heartbeat interval
```

The stronger recommended ratio is five missed one-minute heartbeats.

A worker must heartbeat in a background goroutine while executing long work.

A worker that cannot obtain a successful heartbeat for the full `dead_after` duration must self-fence:

- stop requesting new work;
- cancel work when the execution contract supports cancellation;
- do not report a completion/failure as if ownership remains valid;
- exit and require a new process/session registration.

The controller remains authoritative and rejects any late outcome regardless of worker behavior.

## Capacity calculation

The first pattern remains one-by-one, but its active-capacity calculation changes.

```text
running = count(running_work)
pending = count(claimable queued_work)
live = count(live worker sessions)
inflight = count(unexpired inflight starts)

desired = running + pending

if worker_max_active > 0:
    desired = min(desired, worker_max_active)

shortfall = desired - (live + inflight)

if shortfall > 0 and no unconfirmed one-by-one start is blocking:
    start 1
else:
    start 0
```

Equivalent idle-capacity reasoning:

```text
idle_live = max(0, live - running)
unserved_pending = max(0, pending - idle_live - inflight)
```

Start a worker when `unserved_pending > 0`, subject to the maximum and one-by-one launch policy.

This avoids both prior errors:

- a dead running attempt no longer counts as active capacity;
- a live idle worker can satisfy queued demand without launching another worker.

## CareTaker wake model

Use a capacity-one buffered wake channel.

```go
type CareTaker struct {
    wakeCh chan struct{}
    // state source, reconciler, clock, timer factory, logger, lifecycle...
}

func (c *CareTaker) Signal() {
    select {
    case c.wakeCh <- struct{}{}:
    default:
    }
}
```

Properties:

- signaling never blocks a request;
- many rapid mutations become one reconciliation;
- no goroutine is created per signal;
- the database remains the source of truth.

### Reconciliation trigger sources

Signal after a successful operation that may change `queued_work` or `running_work`:

- raw work admission;
- workflow run admission;
- stage advancement;
- cache-data dependent activation;
- successful work claim;
- work completion;
- work failure;
- worker stop;
- worker registration;
- dead-worker abandonment/requeue;
- startup recovery completion.

Do not normally signal for every successful heartbeat.

### Timer sources

The CareTaker also wakes at the earliest relevant deadline:

```text
earliest active session heartbeat expiry
earliest inflight launch reservation expiry
launch/reconciliation retry deadline
configured fallback safety sweep
```

The existing:

```text
caretaker_interval_schedule_milliseconds
caretaker_missed_interval_limit
```

may serve as the safety sweep/watchdog policy. The normal path is event/deadline driven, not a busy periodic loop.

### Sleep behavior

When capacity is sufficient, the CareTaker blocks in `select`.

If there are no live sessions, no inflight starts, no pending work, and the fallback sweep is disabled or not due, it may wait only for a state-change signal or shutdown.

If live sessions exist, it sleeps until the earliest possible heartbeat expiry unless another state-change signal arrives first.

A successful heartbeat does not need to reset the active timer immediately. If the old timer fires, reconciliation observes the renewed timestamp and computes a later deadline.

## Reconciliation algorithm

```text
reconcile(now):

  1. expire sessions whose:
       status = active
       last_heartbeat_at < now - worker_dead_after

  2. in the same recovery transaction:
       mark sessions dead
       preserve each current attempt in abandoned_work
       delete each matching running_work row
       insert each work item into queued_work

  3. prune expired inflight launch reservations

  4. load:
       queued total
       queued claimable
       running total
       live worker sessions

  5. calculate desired capacity

  6. ask the configured WorkerExecutionPattern for a start plan

  7. reserve inflight capacity before launch

  8. call the existing launch backend

  9. on launch failure:
       remove the reservation
       record error
       schedule bounded retry
       do not fail an HTTP mutation

 10. compute next timer deadline

 11. return to blocking select
```

The loop must not immediately resignal itself merely because enough workers exist.

## Race handling

### Heartbeat versus expiry

Expiry uses a transactional predicate:

```text
status = active
and last_heartbeat_at < cutoff
```

Heartbeat updates only an active matching session.

Whichever transaction wins determines the state:

- heartbeat first: expiry predicate no longer matches;
- expiry first: heartbeat cannot resurrect the session.

### Completion versus expiry

Completion validates the session and ownership inside the same transaction that moves `running_work` to `completed_work`.

- completion first: no running row remains to abandon;
- expiry first: no owned running row remains to complete.

Exactly one transition succeeds.

### Duplicate stop or heartbeat

- duplicate stop is idempotent when the same session is already stopped;
- heartbeat after stop/death is rejected;
- stop for a dead session does not rewrite its terminal reason.

### Duplicate wake signals

Signals are coalesced. Reconciliation is idempotent over current durable state.

## Error behavior

### Persistence reconciliation error

- log/observe the error;
- keep the CareTaker alive;
- retry with bounded backoff;
- do not launch based on a partial state snapshot.

### Worker launch error

- remove the just-created inflight reservation;
- record launch failure;
- schedule retry;
- do not change the already-committed API response that caused the wake.

### Worker heartbeat transport error

The worker retries according to its client policy while tracking time since the last accepted heartbeat. It self-fences at `dead_after`.

### Late completion/failure

Return `409 Conflict` with a stable machine-readable reason. Do not insert `completed_work` or `failed_work`.

## Observability

At minimum record:

```text
caretaker_started
caretaker_signaled
caretaker_reconcile_started
caretaker_reconcile_completed
caretaker_sleeping
caretaker_reconcile_failed
worker_registered
worker_heartbeat_rejected
worker_stopped
worker_session_expired
work_attempt_abandoned
work_item_requeued
worker_capacity_evaluation
worker_start_requested
worker_start_failed
worker_start_registered
worker_start_reservation_expired
caretaker_stopped
```

Useful fields:

```text
reason
pending_queued
pending_claimable
running_attempts
live_worker_sessions
idle_worker_sessions
inflight_starts
desired_workers
start_count
expired_sessions
abandoned_attempts
requeued_work_items
next_wake_at
sleep_reason
worker_id
worker_session_id
attempt_id
work_item_id
```

Do not log controller bearer tokens or sensitive variables.

## Configuration

Retain:

```text
worker_execution_pattern
worker_max_active
worker_inflight_start_timeout
caretaker_interval_schedule_milliseconds
caretaker_missed_interval_limit
```

Add:

```text
worker_heartbeat_interval = "1m"
worker_dead_after = "5m"
```

The heartbeat interval is returned to workers at registration.

The dead-after value is used by:

- CareTaker expiry;
- claim validation;
- completion/failure ownership validation;
- worker self-fencing policy communicated during registration.

## Controller lifecycle

Startup:

```text
open/validate store
complete existing startup recovery
construct CareTaker
start one CareTaker goroutine
signal startup reconciliation
begin serving HTTP
```

The CareTaker initial run, not `buildControllerServer`, performs capacity evaluation.

Shutdown:

```text
stop accepting new HTTP work
cancel CareTaker context
wait for CareTaker goroutine
close execution environment
release database ownership
close store
```

No second CareTaker may be started for the same controller.

## Rollout sequence

1. Add worker-session and abandoned-attempt persistence.
2. Add worker registration, heartbeat, and graceful stop.
3. Bind claims and outcomes to live sessions; add atomic expiry recovery.
4. Add the event/deadline-driven CareTaker loop.
5. Replace every direct capacity-evaluation call with a wake signal.
6. Delete obsolete claim-confirmation scheduling and old scaler code when unreferenced.
7. Run controller, worker, persistence, direct-process, and Slurm-script tests.
8. Run a smoke where a worker is killed during a long assignment and the work is requeued.
9. Run a late-result smoke and prove the stale completion is rejected.

## Acceptance criteria

The concept is complete when all of the following are true:

1. A worker is live only when its active session heartbeat is within the configured death threshold.
2. A worker heartbeat is sent throughout long-running work.
3. A worker that exits after no work sends a graceful stop and stops counting as live immediately.
4. An expired session is marked dead.
5. Every assignment owned by that session is atomically recorded as abandoned and returned to `queued_work`.
6. A dead/stopped/expired session cannot claim, complete, or fail work.
7. A late outcome for an abandoned attempt returns 409 and creates no terminal success/failure record.
8. Worker launch decisions use live sessions plus inflight starts, not running attempts as a liveness proxy.
9. The CareTaker starts workers one by one according to configured policy.
10. The CareTaker blocks when capacity is sufficient and wakes on queue/running state signals or relevant deadlines.
11. API handlers never invoke automatic worker scheduling directly.
12. A worker-launch error cannot convert an already-committed work submission or completion into HTTP 500.
13. Startup queued work triggers CareTaker reconciliation without a direct startup scheduler call.
14. Shutdown cancels and joins the CareTaker cleanly.
15. Tests prove there is one automatic launch authority.

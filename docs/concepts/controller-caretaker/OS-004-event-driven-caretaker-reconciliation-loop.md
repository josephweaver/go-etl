# OS-004: Event-Driven CareTaker Reconciliation Loop

## Status

Implementation in progress.

Current implementation:

- `CareTaker` has fakeable clock/timer interfaces and a capacity-one non-blocking signal channel.
- Pure next-wake calculation selects the earliest worker-expiry, inflight-start-expiry, retry, or fallback deadline and clamps already-past deadlines to an immediate wake.
- Worker registration confirms the oldest inflight worker-start reservation; the legacy claim-confirm path remains temporarily in place until the OS-005 cutover.
- Worker capacity planning counts live worker sessions plus unexpired inflight starts as observed capacity, so idle live workers satisfy pending work and dead running attempts do not.
- Controller demand snapshots count live sessions using the heartbeat cutoff before applying the one-by-one capacity policy.
- `CareTaker.reconcile` now performs the OS-004 order through testable interfaces: recover expired sessions, prune expired inflight starts, load a fresh snapshot, plan/reserve/launch, roll back reservations on launch failure, and schedule retry deadlines.
- `CareTaker.Run` now performs an initial startup reconciliation, blocks on coalesced state signals or the calculated timer deadline, stops/drains timers during reset and shutdown, and exits cleanly on context cancellation without closing the wake channel.
- Controller startup now builds exactly one CareTaker from the resolved heartbeat, worker execution, and caretaker interval policy, routes worker-state signals to it, and stops/joins it before closing external execution, ownership, and persistence resources.
- The loop logs caretaker start/stop, reconcile start/completion/failure, sleeping deadlines, recovery counts, snapshot demand, inflight count, start count, plan reason, and next-wake details.
- Deterministic tests now cover signal wake, startup queued-work launch, heartbeat-expiry wake, inflight-reservation-expiry wake, fallback sweep wake, launch rollback retry, persistence failure no-launch retry, successful backoff reset, and controller lifecycle start/stop.
- The controller implements the recovery, capacity snapshot, and worker launcher adapter methods used by the CareTaker.

## Minimum capable model

Use **GPT-5.5** with **Extra High** reasoning.

This slice contains concurrent signaling, dynamic timers, controller lifecycle, backoff, capacity policy, and launch reservations. A small mistake can create duplicate launches, missed recovery, or shutdown leaks.

Use **GPT-5.4-mini** only for isolated logging or documentation cleanup.

## Goal

Add one long-lived controller `CareTaker` that:

- blocks instead of busy-polling;
- wakes when signaled that work/capacity state may have changed;
- wakes at heartbeat, inflight-start, retry, or fallback deadlines;
- expires dead sessions and recovers their work;
- computes demand and live capacity;
- launches workers through the existing backend;
- retries failures without affecting HTTP responses; and
- shuts down cleanly.

This slice may leave legacy direct `EvaluateWorkerCapacity` calls temporarily in place. OS-005 performs the exclusive-authority cutover.

## Scope

### In scope

- `CareTaker` type and run loop.
- Capacity-one coalesced signal channel.
- Clock/timer abstraction for deterministic tests.
- Dynamic next-deadline calculation.
- Dead-session recovery invocation.
- Live-capacity demand snapshot.
- Updated one-by-one policy.
- Inflight launch reservation management.
- Launch error backoff.
- Startup/shutdown lifecycle.
- Observability.
- Unit and controller lifecycle tests.

### Out of scope

- Removing every legacy direct scheduler call.
- Auditing every queue/running mutation signal.
- Scale down.
- Multi-controller leadership.
- Durable inflight start reservations.
- Scheduler job cancellation.

## Preferred file budget

```text
cmd/controller/caretaker.go
cmd/controller/caretaker_test.go
cmd/controller/worker_execution.go
cmd/controller/worker_execution_test.go
cmd/controller/main.go
cmd/controller/config.go
cmd/controller/defaults.json
```

Persistence changes should already be complete. Add only a narrow query helper if a test proves it is missing.

## CareTaker dependencies

Use interfaces so the loop is testable without SQLite, HTTP, Slurm, or real time.

Suggested shape:

```go
type CareTakerStateSource interface {
    RecoverExpiredWorkerSessions(
        ctx context.Context,
        now time.Time,
        deadAfter time.Duration,
    ) (WorkerRecoverySummary, error)

    WorkerCapacitySnapshot(
        ctx context.Context,
        now time.Time,
        deadAfter time.Duration,
    ) (WorkerCapacitySnapshot, error)
}

type WorkerLauncher interface {
    StartWorkers(ctx context.Context, count int) error
}

type CareTakerClock interface {
    Now() time.Time
    NewTimer(d time.Duration) CareTakerTimer
}
```

Adapt existing controller/store/launch methods to these interfaces.

Do not make tests start real worker processes.

## Data objects

Suggested snapshot:

```go
type WorkerCapacitySnapshot struct {
    PendingQueued      int
    PendingClaimable   int
    RunningAttempts    int
    LiveWorkerSessions int
    EarliestHeartbeat  time.Time
}

type WorkerRecoverySummary struct {
    ExpiredSessions   int
    AbandonedAttempts int
    RequeuedWorkItems int
}

type CareTakerConfig struct {
    DeadAfter             time.Duration
    InflightStartTimeout  time.Duration
    FallbackInterval      time.Duration
    RetryInitial          time.Duration
    RetryMaximum          time.Duration
    WorkerExecution       WorkerExecutionConfig
}
```

Reuse existing operational policy fields where sensible.

## Signal channel

Create once:

```go
wakeCh := make(chan struct{}, 1)
```

Signal:

```go
func (c *CareTaker) Signal(reason string) {
    c.recordSignal(reason)

    select {
    case c.wakeCh <- struct{}{}:
    default:
    }
}
```

Requirements:

- never block the caller;
- never spawn a goroutine;
- duplicate signals may coalesce;
- reason is observational only;
- durable state determines work.

The signal method must be safe before/after loop startup and during shutdown.

## Run loop

Suggested structure:

```go
func (c *CareTaker) Run(ctx context.Context) error {
    c.Signal("startup")

    var timer CareTakerTimer
    var timerC <-chan time.Time

    for {
        select {
        case <-ctx.Done():
            stopAndDrainTimer(timer)
            return nil

        case <-c.wakeCh:
            drainCoalescedSignals(c.wakeCh)
            next, err := c.reconcile(ctx, c.clock.Now())
            timer, timerC = c.resetTimer(timer, next, err)

        case <-timerC:
            next, err := c.reconcile(ctx, c.clock.Now())
            timer, timerC = c.resetTimer(timer, next, err)
        }
    }
}
```

Exact implementation may differ. Preserve:

- one reconciliation at a time;
- no busy loop;
- cancelable wait;
- safe timer stop/reset/drain;
- deterministic tests.

The loop should perform an initial reconciliation without requiring HTTP traffic.

## Reconciliation order

The order is mandatory:

```text
1. Recover expired worker sessions and their assignments.
2. Prune expired inflight start reservations.
3. Load a fresh capacity snapshot.
4. Calculate a start plan.
5. Reserve inflight start capacity.
6. Invoke worker launch.
7. Remove the reservation on launch failure.
8. Calculate the next wake deadline.
```

Recovery comes first so a dead running attempt does not distort demand/capacity.

If recovery requeues work, the same reconciliation continues using a fresh snapshot; it does not need to signal itself.

## Updated capacity policy

Extend `WorkerDemand` or replace it with an explicit snapshot.

Do not use:

```text
running attempts = live workers
```

Calculation:

```text
pending = PendingClaimable
running = RunningAttempts
live = LiveWorkerSessions
inflight = UnexpiredInflightStarts

desired = running + pending

if MaxActiveWorkers > 0:
    desired = min(desired, MaxActiveWorkers)

observed = live + inflight
shortfall = desired - observed
```

For `one_by_one_until_saturated`:

```text
if pending <= 0:
    start 0
elif shortfall <= 0:
    start 0
elif an unconfirmed one-by-one launch is still inflight:
    start 0
else:
    start 1
```

A live idle worker satisfies pending demand:

```text
idle_live = max(0, live - running)
pending_after_idle = max(0, pending - idle_live)
```

Tests should cover both formulations.

## Inflight start confirmation

Move confirmation from "a worker claimed work" to "a worker registered."

Provide:

```go
ConfirmOldestInflightStartRegistered(now)
```

Registration signals the CareTaker after confirmation.

If no reservation exists, registration represents a manually/external started worker and is still counted through live sessions.

Do not remove an inflight reservation merely because any existing worker claims another item.

## Next wake deadline

Calculate the earliest non-zero deadline among:

### Worker expiry

```text
earliest active last_heartbeat_at + worker_dead_after
```

### Inflight start expiry

```text
earliest inflight created_at + worker_inflight_start_timeout
```

### Error retry

A bounded exponential or stepped backoff:

```text
retry_initial <= retry <= retry_maximum
```

Reset retry state after a successful reconciliation.

### Fallback safety sweep

Use:

```text
now + caretaker_interval_schedule_milliseconds
```

as an upper-bound safety check, not the sole normal trigger.

### No deadline

If no deadline applies and fallback is intentionally disabled, use a nil timer channel so the loop waits only for signal or cancellation.

Never pass a negative duration to a timer. An already-due deadline should produce one immediate reconciliation, then state/backoff must change so it cannot spin forever.

## Heartbeat behavior and timers

A successful heartbeat does not normally signal.

Example:

```text
t0: CareTaker sees last heartbeat=t0 and sets expiry timer for t0+5m
t0+1m ... t0+4m: worker heartbeats; no CareTaker signal
t0+5m: old timer fires
CareTaker sees last heartbeat=t0+4m
sets next expiry timer for t0+9m
```

This performs one safe early check, not one scheduler wake per heartbeat.

Registration and stop do signal because they change capacity immediately.

## Launch behavior

Reuse existing:

```text
Controller.startWorkers
configured ExecutionEnvironment
Scheduler.Submit
LocalWorkerStarter fallback
```

The CareTaker owns the call.

On plan `StartCount > 0`:

1. create reservation before launch;
2. log `worker_start_requested`;
3. call launcher;
4. on failure remove reservation;
5. set retry deadline;
6. remain alive.

A launch error is not returned to the HTTP request that caused the wake.

If worker launch target is intentionally not configured, preserve the existing no-launch behavior with a stable reason. Do not retry in a tight loop.

## Startup integration

Add controller lifecycle fields similar to:

```go
caretaker       *CareTaker
caretakerCancel context.CancelFunc
caretakerDone   chan struct{}
```

Build sequence:

```text
resolve config
open store
complete startup recovery
construct CareTaker
start exactly one goroutine
signal startup
register routes / begin serving
```

Remove the direct startup capacity evaluation only in OS-005.

For this slice, if both old startup evaluation and CareTaker are temporarily present, the shared capacity-manager lock and inflight reservation must prevent duplicate starts. Add a temporary test or keep the CareTaker disabled in production construction until OS-005. Prefer enabling it with duplicate-start protection so OS-005 is only a wiring cutover.

## Shutdown integration

Shutdown/release must:

1. cancel CareTaker context;
2. stop/drain timer;
3. wait for the CareTaker goroutine;
4. close execution environment;
5. release database ownership;
6. close store.

Do not close `wakeCh`; senders may race with shutdown. Context cancellation owns loop termination.

Apply a bounded shutdown context only around external cleanup; do not leak the goroutine.

## Error policy

### Reconciliation store error

- do not launch;
- log structured error;
- schedule retry;
- keep serving API where safe.

### Launch error

- remove reservation;
- log;
- schedule retry;
- keep CareTaker alive.

### Context cancellation

Return cleanly without retry.

### Repeated errors

Apply bounded backoff and fallback/watchdog observations. Do not spin.

## Observability

Log/observe:

```text
caretaker_started
caretaker_signaled
caretaker_reconcile_started
caretaker_reconcile_completed
caretaker_reconcile_failed
caretaker_sleeping
caretaker_stopped
```

Reconciliation fields:

```text
trigger
pending_queued
pending_claimable
running_attempts
live_worker_sessions
inflight_starts
desired_workers
start_count
plan_reason
expired_sessions
abandoned_attempts
requeued_work_items
next_wake_at
next_wake_reason
duration
```

Avoid logging every coalesced signal at info level; debug or aggregate.

## Tests

Use fake clock/timer, state source, and launcher.

### Signal tests

```text
TestCareTakerSignalDoesNotBlock
TestCareTakerSignalsCoalesce
TestCareTakerSignalDoesNotSpawnReconciliationsConcurrently
TestCareTakerShutdownDoesNotRequireClosingWakeChannel
```

### Sleep/deadline tests

```text
TestCareTakerSleepsWhenCapacitySufficient
TestCareTakerWakesOnStateSignal
TestCareTakerWakesAtEarliestHeartbeatExpiry
TestCareTakerWakesAtInflightReservationExpiry
TestCareTakerUsesFallbackSafetySweep
TestHeartbeatRenewalMovesDeadlineOnNextReconcile
TestCareTakerDoesNotWakeForOrdinaryHeartbeat
```

### Reconciliation order tests

```text
TestCareTakerRecoversExpiredSessionsBeforeCapacityPlan
TestRecoveredWorkParticipatesInSameCapacityPlan
TestCareTakerPrunesExpiredInflightBeforePlan
```

### Capacity policy tests

```text
TestLiveIdleWorkerSatisfiesPendingWork
TestDeadRunningAttemptDoesNotCountAsCapacity
TestDesiredWorkersEqualsRunningPlusPendingSubjectToMax
TestOneByOneStartsExactlyOneForShortfall
TestOneByOneWaitsForInflightRegistration
TestRegistrationConfirmsInflightStart
TestClaimDoesNotConfirmInflightStart
```

### Error/backoff tests

```text
TestCareTakerLaunchFailureRemovesReservation
TestCareTakerLaunchFailureSchedulesRetry
TestCareTakerPersistenceFailureDoesNotLaunch
TestSuccessfulReconcileResetsBackoff
TestPastDeadlineDoesNotBusyLoop
```

### Lifecycle tests

```text
TestControllerStartsOneCareTaker
TestControllerShutdownCancelsAndJoinsCareTaker
TestCareTakerInitialReconcileFindsStartupQueuedWork
```

## Implementation sequence

1. Add fakeable clock/timer interfaces.
2. Add capacity-one signal method and tests.
3. Add pure next-deadline calculation and tests.
4. Update capacity snapshot/policy to use live sessions.
5. Move inflight confirmation to registration.
6. Implement reconciliation order.
7. Add launch error rollback/backoff.
8. Implement blocking run loop.
9. Integrate controller startup/shutdown.
10. Add observations.
11. Run race-enabled controller tests when practical.

## Acceptance criteria

1. Exactly one CareTaker loop runs per controller.
2. Signals are non-blocking and coalesced.
3. The loop blocks when no immediate action is needed.
4. Queue/capacity signals wake it immediately.
5. Heartbeat and inflight deadlines wake it without polling tightly.
6. Expired sessions are recovered before capacity is planned.
7. Capacity uses live sessions plus inflight starts.
8. A live idle worker prevents an unnecessary launch.
9. One-by-one starts at most one unconfirmed worker.
10. Registration, not claim, confirms an inflight start.
11. Launch/store errors schedule bounded retry and do not kill the loop.
12. Startup queued work is found by the initial reconciliation.
13. Shutdown cancels and joins the CareTaker.
14. Legacy direct scheduler calls may still exist until OS-005, but duplicate launch is prevented.

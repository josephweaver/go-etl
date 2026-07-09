# OS-001: One-by-One Worker Capacity Manager

## Status

Ready for implementation after review.

## Minimum capable model

Use **GPT-5.5 Thinking** for the first pass because this slice crosses controller admission, worker claim, completion, persistence demand, and configured execution environment launch.

A narrower follow-up implementation that only changes tests or renames types can likely use **GPT-5.4-mini**.

Avoid very small/smoke-test-focused models for this slice because the highest risk is missing a demand transition, not writing the interface syntax.

## Goal

Implement the first controller worker execution pattern:

```text
if active worker capacity < claimable pending work:
    start exactly one worker
else:
    start zero workers
```

The implementation must work with the existing configured execution environment path and preserve the existing fallback local worker starter path.

## User story

Given a configured worker execution environment, when the controller has claimable pending work, the controller starts worker processes one by one until active capacity is no longer below claimable pending work.

If starting workers is slow, each inflight start delays the next start. This creates a natural power-curve ramp instead of a burst of worker starts.

## Scope

### In scope

- Add a controller-side worker capacity manager.
- Add a worker execution pattern abstraction.
- Implement `one_by_one_until_saturated`.
- Track inflight worker starts in memory with timeout.
- Trigger capacity evaluation after durable work demand changes.
- Ensure configured execution environment launch still works.
- Ensure fallback `LocalWorkerStarter` path still works.
- Add focused unit tests.

### Out of scope

- Durable worker heartbeat table.
- Worker process lifecycle tracking.
- Worker plugin changes.
- Workflow JSON schema changes.
- Artifact publication changes.
- LandCore workflow fixes.
- The separate `POST /workflow` generic 500 admission/compile bug.

## Allowed files

Prefer to stay within this file budget:

```text
cmd/controller/main.go
cmd/controller/worker_execution.go
cmd/controller/worker_execution_test.go
cmd/controller/worker_scaler.go
cmd/controller/worker_scaler_test.go
cmd/controller/config.go
cmd/controller/defaults.json
```

Touch fewer files if possible.

Do not modify worker plugins, Python scripts, or workflow compiler internals unless a test proves the controller cannot compute claimable demand without a small persistence helper.

## Current implementation notes

The code already contains `WorkerScaleState`, `WorkerScaleConfig`, and `PlanStarts`. It has the right instinct, especially the "waiting for claim" guard, but it is embedded too directly in workflow admission.

The controller currently starts workers after initial workflow admission using queued and running counts. This is not enough because work can become queued later when a worker completes and the next stage becomes ready.

The new implementation should move this behavior into a named capacity manager that can be invoked from several controller events.

## Proposed types

Create `cmd/controller/worker_execution.go`.

```go
type WorkerDemand struct {
    PendingQueued    int
    PendingClaimable int
    RunningAttempts  int
}

type WorkerStartReservation struct {
    ID        string
    CreatedAt time.Time
}

type WorkerExecutionState struct {
    InflightStarts []WorkerStartReservation
}

type WorkerExecutionConfig struct {
    Pattern             string
    MaxActiveWorkers    int
    InflightStartTimeout time.Duration
}

type WorkerStartPlan struct {
    StartCount int
    Reason     string
}

type WorkerExecutionPattern interface {
    Plan(now time.Time, demand WorkerDemand, state WorkerExecutionState, cfg WorkerExecutionConfig) WorkerStartPlan
}
```

If you can adapt the existing `WorkerScaleState` cleanly, do that. Do not preserve `StartedWorkers` as a total-ever-started cap unless it is renamed and semantically corrected. For this first pattern, active capacity should be based on running attempts plus unexpired inflight start reservations.

## Demand calculation

Add a controller method similar to:

```go
func (c *Controller) workerDemand(ctx context.Context) (WorkerDemand, error)
```

Rules:

1. `PendingQueued` is total queued work.
2. `RunningAttempts` is total running work.
3. `PendingClaimable` is queued work that is eligible to claim now.

If resource constraint eligibility is already available through existing persistence/status logic, reuse it. Do not start workers for queued work that cannot be claimed because of resource constraints.

If claimability cannot be computed cheaply in this slice, create a narrow helper with a conservative rule:

```text
claimable = queued - resource_blocked
```

Do not implement a broad scheduler rewrite.

## Launch configuration boundary

Add a controller field for worker launch/config resolution if needed:

```go
type Controller struct {
    ...
    launchResolver variable.Resolver
    workerExecutor *WorkerCapacityManager
}
```

Set it in `buildControllerServer` from the controller startup resolver.

Worker launch should not depend on workflow-run resolver contents. Workflow variables should not be required to define controller worker executable paths or scheduler settings.

## Capacity manager method

Add a controller method similar to:

```go
func (c *Controller) EvaluateWorkerCapacity(ctx context.Context, now time.Time) error
```

It should:

1. Load worker execution config from controller config/defaults.
2. Compute `WorkerDemand`.
3. Prune expired inflight start reservations.
4. Ask the selected execution pattern for a plan.
5. If `StartCount == 0`, return.
6. Record an inflight start reservation before launching.
7. Call `c.startWorkers(ctx, c.launchResolver, plan.StartCount)`.
8. If launch fails, remove the reservation and return the error.
9. Log or observe start-plan reason when useful.

Use a mutex around demand evaluation and reservation updates to avoid duplicate starts from simultaneous HTTP requests.

## Event hooks

Call `EvaluateWorkerCapacity` after these transitions:

### 1. Raw work submission

After `submitRawWorkToStore` succeeds.

### 2. Workflow run admission

Replace the embedded worker start logic near initial queueing with `EvaluateWorkerCapacity`.

### 3. Successful work claim

After `/work/next` successfully claims and encodes a work item, mark one inflight start as confirmed and trigger another capacity evaluation.

Preferred behavior:

- Do not make the claiming worker wait on slow Slurm/SSH launch.
- Trigger the next evaluation asynchronously after the claim is recorded.
- Use a controller-level mutex so concurrent evaluations do not double-start.

### 4. Work completion

After completion can create new ready work:

- after `enqueueReadyCacheDataDependents`
- after `activateNextReadyWorkflowStage`

Call capacity evaluation once after all completion-side enqueue/activation logic has finished.

### 5. Startup recovery

After `completeStartupRecovery` and before the HTTP server begins accepting traffic, evaluate capacity once if normal admission is open.

If launch config is not valid during startup, return a startup error rather than silently leaving queued work stranded.

### 6. Optional caretaker

If there is already a caretaker loop or missed-interval policy, wire a periodic capacity evaluation there. If not, leave this for a future OS.

## Configuration

Add defaults only if needed:

```json
{
  "name": {"namespace": "controller_config", "key": "worker_execution_pattern"},
  "type": "string",
  "expression": "one_by_one_until_saturated"
}
```

Recommended optional defaults:

```json
{
  "name": {"namespace": "controller_config", "key": "worker_max_active"},
  "type": "int",
  "expression": 2
}
```

```json
{
  "name": {"namespace": "controller_config", "key": "worker_inflight_start_timeout"},
  "type": "string",
  "expression": "5m"
}
```

If the existing config system makes adding duration strings awkward, use milliseconds instead:

```json
{
  "name": {"namespace": "controller_config", "key": "worker_inflight_start_timeout_milliseconds"},
  "type": "int",
  "expression": 300000
}
```

## Tests

Add or revise tests to cover these cases.

### Pattern unit tests

```text
TestOneByOneStartsWhenActiveBelowClaimable
TestOneByOneDoesNotStartWhenNoClaimableWork
TestOneByOneDoesNotStartWhenActiveEqualsClaimable
TestOneByOneDoesNotStartWhenMaxActiveReached
TestOneByOneCountsInflightStartAsActive
TestOneByOneIgnoresExpiredInflightStart
```

### Controller manager tests

Use fake launcher/backend objects. Do not start real OS processes.

```text
TestEvaluateWorkerCapacityStartsOneWorker
TestEvaluateWorkerCapacityRecordsInflightBeforeLaunch
TestEvaluateWorkerCapacityRemovesInflightOnLaunchFailure
TestEvaluateWorkerCapacityDoesNotStartForResourceBlockedQueuedWork
TestWorkerClaimConfirmsInflightStart
TestWorkerClaimCanTriggerNextOneByOneStart
```

### Integration-ish controller tests

Use in-memory or temp SQLite persistence if available.

```text
TestWorkflowAdmissionEvaluatesWorkerCapacity
TestRawWorkSubmissionEvaluatesWorkerCapacity
TestCompletionStageActivationEvaluatesWorkerCapacity
TestStartupRecoveryEvaluatesExistingQueuedWork
```

## Expected state transitions

### First queued work

Before:

```text
queued=3
running=0
inflight=0
```

Plan:

```text
start 1
```

After:

```text
queued=3
running=0
inflight=1
```

### First worker claims work

Before:

```text
queued=2
running=1
inflight=1
```

Confirm claim:

```text
inflight=0
```

Re-evaluate:

```text
active=1
claimable=2
start 1
```

After:

```text
queued=2
running=1
inflight=1
```

### Saturated

Before:

```text
queued=1
running=1
inflight=0
```

Plan:

```text
start 0
reason=active_capacity_satisfies_claimable_work
```

## Failure behavior

If worker launch fails:

1. Remove the inflight reservation created for that launch.
2. Return/log the error at the triggering controller path.
3. Do not leave the controller believing capacity exists.

If an inflight worker start never claims work:

1. Keep it as active until `worker_inflight_start_timeout`.
2. After timeout, ignore/remove it.
3. Allow the next evaluation to start another worker.

## Observability

At minimum, log these events at info/debug level:

```text
worker_capacity_evaluation
worker_start_requested
worker_start_failed
worker_start_confirmed_by_claim
worker_start_reservation_expired
```

Suggested fields:

```text
pending_queued
pending_claimable
running_attempts
inflight_starts
start_count
reason
pattern
```

## Implementation sequence

1. Add pure policy types and tests.
2. Add in-memory inflight reservation state.
3. Add controller demand method.
4. Add fake launch tests around capacity evaluation.
5. Replace embedded workflow-admission start logic with capacity evaluation.
6. Add raw submission hook.
7. Add successful claim hook.
8. Add completion/stage-activation hook.
9. Add startup recovery hook.
10. Run controller tests.
11. Run LandCore local smoke again after the separate `POST /workflow` 500 is resolved.

## Acceptance criteria

The OS is complete when:

1. With 5 claimable queued work items and no active capacity, the controller requests exactly one worker start.
2. A second worker is not started until a launched worker claims work or its inflight reservation expires.
3. After a worker claims work, the controller can start one additional worker if claimable work still exceeds active capacity.
4. Work created by stage activation triggers the same capacity evaluation.
5. Resource-blocked queued work does not trigger worker starts.
6. Worker launch uses controller configuration, not workflow-run variables.
7. Existing direct local and configured execution environment launches still pass their tests.

# SC: Controller Worker Execution Framework

## Status

Proposed.

## Problem

The controller can persist workflow work items and workers can claim work through `/work/next`, but the controller does not yet have a consistent framework for deciding when and how to start worker processes as work becomes available.

This leaves a gap between these two states:

1. The controller has pending work in durable persistence.
2. A worker process exists and asks the controller for that work.

The controller should own the transition from "pending work exists" to "worker capacity has been requested."

## Decision

Introduce a controller-side worker execution framework with two separate concerns:

1. **Worker execution pattern**: decides whether the controller should request more worker capacity.
2. **Worker launch backend**: actually starts the worker using the configured execution environment.

The first execution pattern is:

```text
one_by_one_until_saturated
```

Its rule is:

```text
if active_worker_capacity < claimable_pending_work:
    start exactly one worker
else:
    start zero workers
```

This pattern intentionally starts workers one at a time. Worker startup may be slow, especially for SSH, Slurm, Singularity, or containerized environments. A one-at-a-time launch rule creates a natural ramp curve: capacity only grows quickly when workers start and claim work quickly.

## Non-goals

This SC does not redesign workflow JSON, worker plugin contracts, artifact publication, source admission, or the worker process loop.

It also does not require a durable worker heartbeat table in the first implementation. A future slice may add one if accurate worker lifecycle tracking becomes necessary.

## Current useful pieces

The codebase already has most of the launch backend pieces:

- `ExecutionEnvironment` combines transports, shell dialect, scheduler, and runtime.
- `Scheduler.Submit` is already a backend boundary for launching worker jobs.
- `DirectProcessScheduler` can start a local worker process.
- `SlurmScheduler` can submit a worker script.
- `WorkerRuntime` can prepare runtime directories, worker config, data directories, artifact cache, and data location roots.
- `SingularityWorkerRuntime` can rewrite worker script execution into a Singularity invocation.
- The controller already has a `startWorkers` / `startConfiguredWorkers` path.

The missing layer is a reusable controller capacity manager that is invoked whenever durable work demand changes.

## Vocabulary

### Pending work

Work item records that are queued in persistence.

### Claimable pending work

Queued work that a worker could actually claim now.

This should exclude work that is resource-blocked, dependency-blocked, or otherwise not eligible for immediate claim.

### Running work

Work items currently claimed by workers and represented by active attempts.

### Inflight worker start

A worker launch that has been requested but has not yet been observed claiming work.

### Active worker capacity

For the first implementation:

```text
active_worker_capacity = running_attempts + unexpired_inflight_worker_starts
```

This is not a perfect count of OS processes. It is a conservative controller-side capacity approximation that does not require worker heartbeats.

### Worker execution pattern

A small policy object that converts demand and state into a start plan.

### Worker launch backend

A mechanism that starts a worker through local process, configured execution environment, scheduler, or future backends.

## Proposed data objects

```go
type WorkerDemand struct {
    PendingQueued    int
    PendingClaimable int
    RunningAttempts  int
}

type WorkerExecutionState struct {
    InflightStarts []WorkerStartReservation
}

type WorkerStartReservation struct {
    ID        string
    CreatedAt time.Time
}

type WorkerStartPlan struct {
    StartCount int
    Reason     string
}

type WorkerExecutionPattern interface {
    Plan(now time.Time, demand WorkerDemand, state WorkerExecutionState, cfg WorkerExecutionConfig) WorkerStartPlan
}

type WorkerExecutionConfig struct {
    Pattern                    string
    MaxActiveWorkers            int
    InflightStartTimeout        time.Duration
    StartEvaluationOnClaim      bool
    StartEvaluationOnCompletion bool
}
```

The exact names are flexible. The important boundary is not flexible: demand policy should not know how Slurm, Docker, SSH, or local process launch works.

## First pattern: one_by_one_until_saturated

```go
type OneByOneUntilSaturatedPattern struct{}

func (p OneByOneUntilSaturatedPattern) Plan(
    now time.Time,
    demand WorkerDemand,
    state WorkerExecutionState,
    cfg WorkerExecutionConfig,
) WorkerStartPlan {
    active := demand.RunningAttempts + len(state.UnexpiredInflightStarts(now, cfg.InflightStartTimeout))

    if demand.PendingClaimable <= 0 {
        return WorkerStartPlan{Reason: "no_claimable_work"}
    }

    if cfg.MaxActiveWorkers > 0 && active >= cfg.MaxActiveWorkers {
        return WorkerStartPlan{Reason: "max_active_workers_reached"}
    }

    if active >= demand.PendingClaimable {
        return WorkerStartPlan{Reason: "active_capacity_satisfies_claimable_work"}
    }

    return WorkerStartPlan{
        StartCount: 1,
        Reason:     "active_capacity_below_claimable_work",
    }
}
```

## Controller event hooks

The capacity manager should be invoked after any controller action that can create claimable work or prove a launched worker is alive.

Required hooks for the first implementation:

1. After raw work submission queues a work item.
2. After workflow submission queues the first ready stage.
3. After `/work/next` successfully claims work.
4. After work completion enqueues cache-data dependents.
5. After work completion activates the next workflow stage.
6. After startup recovery, if durable queued work exists from a previous controller run.

Optional but useful:

7. Periodic caretaker tick, to recover from missed hooks or expired inflight starts.

## Important resolver boundary

Worker launch configuration should come from the controller's startup/config resolver or the `ExecutionEnvironmentConfig`, not from a workflow-run resolver.

A workflow run resolver is for workflow variables and source submission variables. It should not be required to contain controller worker launch details such as worker executable path, worker config path, Slurm script path, or max active worker settings.

The controller should store a launch/config resolver or resolve launch configuration during controller startup.

## State transitions

### Work queued

```text
queued_work_count increases
controller recomputes demand
if active capacity < claimable pending work:
    request exactly one worker start
    record an inflight start reservation
```

### Worker claims work

```text
running_attempt_count increases
one inflight start reservation is confirmed/removed
controller recomputes demand
if active capacity < claimable pending work:
    request exactly one more worker start
```

This is what creates the gradual ramp.

### Worker completes work

```text
running_attempt_count decreases
terminal attempt is recorded
dependent work may be queued
next workflow stage may become ready
controller recomputes demand
if active capacity < claimable pending work:
    request exactly one worker start
```

### Inflight start expires

```text
reservation age > inflight_start_timeout
reservation is ignored or removed
controller may request another worker start on next demand evaluation
```

This prevents a failed Slurm submission, slow SSH launch, or dead local process from blocking capacity forever.

## Future execution patterns

This SC intentionally leaves room for later patterns:

- `fixed_min_pool`: keep N workers warm while controller is running.
- `batch_until_saturated`: start up to K workers per evaluation.
- `adaptive_rate`: increase start rate when claims are fast and decrease when starts are slow.
- `external_pool`: do not launch workers; report desired capacity to an external service.
- `manual_only`: never start workers automatically.

## Acceptance criteria for the SC

The architecture is successful when:

1. Worker spin-up policy is testable without invoking Slurm, Docker, SSH, or OS processes.
2. Worker launch backends are testable without knowing the demand policy.
3. The first policy starts one worker when active capacity is below claimable work.
4. Resource-blocked work does not create worker starts.
5. New work created by stage advancement causes capacity evaluation.
6. Startup recovery can resume previously queued work.

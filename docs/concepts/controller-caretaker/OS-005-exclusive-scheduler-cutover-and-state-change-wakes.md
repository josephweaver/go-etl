# OS-005: Exclusive Scheduler Cutover and State-Change Wakes

## Status

Implementation in progress.

Current implementation:

- Raw `/work` submission now signals `raw_work_queued` after persistence instead of calling `EvaluateWorkerCapacity`, so launch failures cannot change a successful raw submission response.
- Workflow run admission now signals `workflow_work_queued` after successful admission instead of calling `EvaluateWorkerCapacity`, so submission acknowledgement is no longer coupled to worker launch.
- Controller startup no longer calls `EvaluateWorkerCapacity`; the started CareTaker owns the startup reconciliation.
- `Controller.signalCareTaker` provides the no-fail notification seam for subsequent state-transition cutovers.

## Minimum capable model

Use **GPT-5.4** with **High** reasoning for the primary cutover.

The architecture is established; the risk is missing one state-transition call site. Use **GPT-5.3-codex-spark** with **Medium** reasoning for static-search verification and extra tests.

Escalate to **GPT-5.5 High** if implementation reveals queue/running mutations outside the audited controller paths.

## Goal

Make the CareTaker the only automatic worker-launch authority.

Replace every synchronous/asynchronous scheduler call from startup and request paths with a non-blocking signal emitted after durable state commits.

Prove that all relevant `queued_work` and `running_work` transitions wake the CareTaker.

## Scope

### In scope

- Remove direct `EvaluateWorkerCapacity` calls from startup and HTTP/workflow paths.
- Remove claim-based inflight confirmation and delayed evaluation goroutine.
- Signal after state transitions that may alter queued/running demand.
- Ensure scheduler errors cannot alter committed API responses.
- Confirm registration-based inflight start completion.
- Remove obsolete scaler code if unreferenced.
- Add transition-matrix integration tests.
- Add static/structural tests or checks for one launch authority.
- Update concept state documentation.

### Out of scope

- New worker policies.
- Worker scale down.
- New heartbeat semantics.
- New persistence state.
- Multi-controller leadership.
- Broad handler refactoring unrelated to state signaling.

## Preferred file budget

```text
cmd/controller/main.go
cmd/controller/workflow_stage_activation.go
cmd/controller/worker_execution.go
cmd/controller/caretaker.go
cmd/controller/*_test.go
cmd/controller/worker_scaler.go
cmd/controller/worker_scaler_test.go
docs/concepts/controller-worker-execution-framework/*
docs/concepts/controller-caretaker/*
PROJECT_STATE.md
```

Touch persistence only if a missing commit-result signal cannot be expressed at the controller boundary.

## Exclusive launch rule

After this slice, the only production automatic launch call chain is:

```text
CareTaker.Run
  -> CareTaker.reconcile
     -> worker capacity reconciler
        -> Controller.startWorkers
           -> configured launch backend
```

No API handler may call or indirectly trigger the launch backend.

Rename/restrict methods if useful:

```text
EvaluateWorkerCapacity -> reconcileWorkerCapacity
```

Make it unexported and owned by the CareTaker component.

If tests need direct access, test the reconciler through an interface or package-local method rather than preserving a public controller scheduler method.

## Direct call sites to remove

Audit and replace the current paths.

### Startup

Current:

```text
completeStartupRecovery
EvaluateWorkerCapacity
build server
```

Target:

```text
completeStartupRecovery
start CareTaker
CareTaker.Signal("startup")
build/serve server
```

A launch failure no longer fails controller construction. It is observed and retried by the CareTaker.

### Raw `/work` submission

Current:

```text
persist work
EvaluateWorkerCapacity
return 204
```

Target:

```text
persist work
Signal("raw_work_queued")
return 204
```

### Workflow run admission

Current:

```text
persist/compile/queue initial work
EvaluateWorkerCapacity
return acknowledgement
```

Target:

```text
persist/compile/queue initial work
Signal("workflow_work_queued")
return acknowledgement
```

Signal once after the whole admission transaction/high-level operation succeeds.

### Work claim

Current:

```text
claim work
encode response
ConfirmWorkerStartClaimedAndEvaluateAsync
```

Target:

```text
claim work
Signal("work_claimed")
encode/return response
```

The order of signal versus response encoding may follow existing error semantics, but signal only after the claim transaction commits.

Delete:

```text
ConfirmWorkerStartClaimed
ConfirmWorkerStartClaimedAndEvaluateAsync
50ms sleep goroutine
```

Inflight launch confirmation already occurs on worker registration.

### Work completion

Current:

```text
complete attempt
possibly enqueue cache dependents
possibly activate next stage
EvaluateWorkerCapacity
return 204
```

Target:

```text
complete attempt
enqueue/activate all resulting work
Signal("work_completed")
return 204
```

A CareTaker launch error cannot produce completion HTTP 500.

### Work failure

Target:

```text
fail attempt
perform existing workflow failure transition
Signal("work_failed")
return existing response
```

Failure removes a running assignment and may alter claimability/resource usage, so it must wake the CareTaker.

### Worker registration

After registration and inflight reservation confirmation:

```text
Signal("worker_registered")
```

### Worker stop

After stop/recovery transaction:

```text
Signal("worker_stopped")
```

### Dead-session recovery

The CareTaker performs this inside its own reconciliation. It does not need to enqueue another channel signal for work it just requeued; it must reload state in the same reconciliation.

External/manual recovery helpers should signal after commit.

## State-change trigger matrix

Create a test/audit table equivalent to:

| Transition | Durable effect | Signal reason | Direct scheduling allowed |
|---|---|---|---|
| raw work admitted | `queued_work +` | `raw_work_queued` | no |
| workflow initial stage admitted | `queued_work +` | `workflow_work_queued` | no |
| next stage activated | `queued_work +` | `workflow_stage_activated` or completion aggregate | no |
| cache dependent activated | `queued_work +` | `cache_dependents_queued` or completion aggregate | no |
| worker claims | `queued_work -`, `running_work +` | `work_claimed` | no |
| work completes | `running_work -`, maybe `queued_work +` | `work_completed` | no |
| work fails | `running_work -` | `work_failed` | no |
| worker registers | live capacity `+` | `worker_registered` | no |
| worker stops | live capacity `-`, maybe requeue | `worker_stopped` | no |
| session expires | live capacity `-`, requeue | same reconciliation reload | CareTaker only |
| startup recovery | existing queue may remain | `startup` | no |

The exact reason strings are observational. Tests should assert a signal occurred, not couple all behavior to log wording.

## Signal placement

Signal after commit.

Good:

```go
if err := c.workflowStore.QueueWorkItems(ctx, request); err != nil {
    return err
}
c.caretaker.Signal("raw_work_queued")
return nil
```

Bad:

```go
c.caretaker.Signal("raw_work_queued")
return c.workflowStore.QueueWorkItems(ctx, request)
```

If a high-level operation performs several store transactions and only its final success makes work valid, signal once after the high-level operation.

If a transaction commits and a later non-transactional response-encoding step fails, the signal must still occur because durable demand changed.

## API response decoupling

Remove patterns such as:

```go
if err := c.EvaluateWorkerCapacity(...); err != nil {
    http.Error(w, "evaluate worker capacity", http.StatusInternalServerError)
    return
}
```

After a successful state mutation:

- signal cannot fail;
- API returns according to the state mutation/encoding result;
- CareTaker records launch failure separately.

This is a required semantic change, not merely refactoring.

## Notification abstraction

Prefer a concrete no-fail method:

```go
func (c *Controller) signalCareTaker(reason string) {
    if c == nil || c.caretaker == nil {
        return
    }
    c.caretaker.Signal(reason)
}
```

For tests, inject a fake CareTaker/signaler or expose a package-local interface.

Do not return an error from signaling. The channel is an optimization hint backed by fallback/deadline reconciliation.

## Queue/running mutation audit

Search for all production calls that can mutate:

```text
QueueWorkItems
ClaimNextWork
CompleteAttempt
FailAttempt
CompleteStage
queued_work
running_work
```

Also inspect helpers that wrap those operations:

```text
submitRawWorkToStore
submitWorkflowRunToStore
enqueueReadyCacheDataDependents
activateNextReadyWorkflowStage
RecordWorkItemTerminalState
worker stop/recovery
```

Document every found mutation in a test or code comment near the signal seam.

Do not rely only on the web API routes; controller-internal stage transitions also matter.

## Obsolete code removal

After cutover:

- remove `ConfirmWorkerStartClaimed`;
- remove `ConfirmWorkerStartClaimedAndEvaluateAsync`;
- remove `asyncWorkerCapacity`;
- remove direct startup evaluation;
- remove old `WorkerScaleState` / `worker_scaler.go` if no production/test references remain;
- remove stale tests asserting handler-triggered launch;
- retain pure policy and inflight reservation logic used by the CareTaker.

Do not remove `startWorkers`, scheduler backends, or execution-environment launch code.

## Structural enforcement

Add one or more lightweight safeguards.

Possible package test:

```text
TestAutomaticWorkerLaunchOwnedByCareTaker
```

It can build the controller with a fake launcher and exercise:

- raw submit;
- workflow submit;
- claim;
- complete;
- fail.

Before running the CareTaker, none directly calls the launcher.

After one CareTaker reconciliation, the expected launch occurs.

A source-level test/CI grep may also reject new calls to the reconciler from `main.go` handlers, but prefer behavior tests over fragile source parsing.

## Integration scenarios

### Submission launch decoupling

1. fake launcher always fails;
2. submit valid raw work;
3. persistence succeeds;
4. API returns 204;
5. CareTaker observes launch failure;
6. queued work remains;
7. retry is scheduled.

### Completion launch decoupling

1. worker completes work that activates another stage;
2. fake launcher fails;
3. completion remains committed;
4. API returns 204;
5. next-stage work remains queued;
6. CareTaker retries separately.

### Claim transition wake

1. two queued work items;
2. one registered worker claims one;
3. claim signals CareTaker;
4. CareTaker sees one running + one pending and one live worker;
5. it requests one additional worker subject to max/inflight policy.

### Failure transition wake

1. one running work item and one resource-blocked queued item;
2. running work fails and releases resource usage;
3. failure signals CareTaker;
4. queued item becomes claimable;
5. CareTaker requests capacity if needed.

### Idle sufficient capacity

1. one live idle worker;
2. one queued claimable item is admitted;
3. signal wakes CareTaker;
4. it starts zero workers because idle live capacity is sufficient.

### Startup queue

1. store contains queued work before controller start;
2. controller starts CareTaker;
3. startup signal reconciles;
4. worker launch is requested;
5. no direct startup evaluation exists.

## Tests

### Handler decoupling

```text
TestRawSubmissionSignalsButDoesNotLaunchDirectly
TestWorkflowAdmissionSignalsButDoesNotLaunchDirectly
TestWorkClaimSignalsButDoesNotLaunchDirectly
TestCompletionSignalsButDoesNotLaunchDirectly
TestFailureSignalsButDoesNotLaunchDirectly
TestLaunchFailureDoesNotChangeSubmissionResponse
TestLaunchFailureDoesNotChangeCompletionResponse
```

### Transition coverage

```text
TestStageActivationSignalsCareTaker
TestCacheDependentQueueSignalsCareTaker
TestWorkerRegistrationSignalsCareTaker
TestWorkerStopSignalsCareTaker
TestClaimNoLongerConfirmsInflightStart
```

### Exclusive authority

```text
TestCareTakerIsOnlyAutomaticLauncher
TestStartupUsesCareTakerInitialReconcile
TestNoClaimEvaluationGoroutineIsStarted
```

### End-to-end controller/worker test

Use temp SQLite, fake launcher, fake clock:

```text
1. admit two work items;
2. CareTaker launches one worker;
3. register session;
4. worker claims first item;
5. CareTaker launches one more worker if allowed;
6. expire first worker during assignment;
7. CareTaker abandons/requeues;
8. late completion returns 409;
9. second live worker claims requeued item;
10. completion succeeds.
```

No real five-minute wait.

### Race test command

Run when supported:

```text
go test -race ./cmd/controller ./cmd/worker ./internal/persistence
```

Also run normal repository tests required by current CI.

## Documentation updates

Update the prior controller worker execution concept to state:

```text
active capacity is now live worker sessions + inflight starts
CareTaker is the exclusive automatic scheduling owner
claim no longer confirms worker start
```

Mark the old "future heartbeat" non-goal as superseded.

Update `PROJECT_STATE.md` with implemented OS status only after implementation actually lands.

## Implementation sequence

1. Add fake signaler to controller tests.
2. Replace raw submission evaluation with signal.
3. Replace workflow admission evaluation with signal.
4. Replace completion evaluation with signal.
5. Add failure signal.
6. Replace claim confirmation/evaluation with signal.
7. Remove startup direct evaluation.
8. Ensure registration confirms inflight and signals.
9. Remove obsolete async fields/methods.
10. Delete old scaler code if unreferenced.
11. Add exclusive-authority tests.
12. Add launch-failure/API-decoupling tests.
13. Run transition matrix and end-to-end test.
14. Update concept/project documentation.

## Acceptance criteria

1. CareTaker is the only automatic caller of worker launch.
2. Startup does not call worker capacity evaluation directly.
3. Raw and workflow submissions signal after commit.
4. Claim signals after its queue/running transaction commits.
5. Completion and failure signal after their terminal transactions.
6. Stage/cache activation cannot strand newly queued work.
7. Worker registration/stop signal capacity changes.
8. Claim no longer confirms an inflight start or spawns a delayed evaluation goroutine.
9. Scheduling/launch errors cannot change successful submission or completion API responses.
10. Signals are non-blocking and coalesced.
11. One live idle worker prevents unnecessary launch.
12. Static/behavior tests prove handlers do not launch directly.
13. Obsolete scaler/async scheduling code is removed when unreferenced.
14. The full dead-worker/requeue/late-outcome integration scenario passes.

# Controller Worker Execution Framework Slice

This package contains an implementation-ready strategic context and one operational slice for the GORC controller worker spin-up gap.

## Files

- `docs/SC-controller-worker-execution-framework.md`  
  Strategic context for the controller-side worker execution abstraction.

- `docs/OS-001-one-by-one-worker-capacity-manager.md`  
  First operational slice: start configured workers one at a time while active worker capacity is below claimable pending work.

- `docs/MODEL_IMPLEMENTATION_PLAN.md`  
  Suggested model choices and decomposition notes.

## Current behavior

This concept introduced the original worker capacity manager. It is now superseded by the controller CareTaker for automatic launch ownership.

The current worker execution pattern remains:

```text
if observed_worker_capacity < desired_worker_capacity:
    start exactly one worker
else:
    start zero workers
```

Observed worker capacity is now:

```text
live worker sessions + unexpired inflight worker-start reservations
```

Worker registration confirms an inflight start. Claiming work no longer confirms a start. API handlers and startup paths signal the CareTaker after durable state changes; the CareTaker is the exclusive automatic scheduling owner and calls the launch backend during reconciliation.

Set `controller_config.worker_execution_pattern` to `null` when the controller should admit and serve work without scheduling worker processes. The null pattern still evaluates durable demand for observability, but always returns zero worker starts and records no inflight launch reservations.

## Related note

The attached LandCore `issues.md` reports a separate smoke blocker: `POST /workflow` returns a generic 500 before creating a workflow run. This worker-capacity slice should not try to fix that admission/compile bug directly, but it should make the controller ready to consume queued work once admission succeeds.

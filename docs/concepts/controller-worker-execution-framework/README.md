# Controller Worker Execution Framework Slice

This package contains an implementation-ready strategic context and one operational slice for the GORC controller worker spin-up gap.

## Files

- `docs/SC-controller-worker-execution-framework.md`  
  Strategic context for the controller-side worker execution abstraction.

- `docs/OS-001-one-by-one-worker-capacity-manager.md`  
  First operational slice: start configured workers one at a time while active worker capacity is below claimable pending work.

- `docs/MODEL_IMPLEMENTATION_PLAN.md`  
  Suggested model choices and decomposition notes.

## Intended first behavior

The first worker execution pattern is:

```text
if active_workers < claimable_pending_work:
    start exactly one worker
else:
    start zero workers
```

A newly started worker counts as active capacity until it claims work or its launch reservation expires. This makes slow worker startup naturally dampen launch rate, producing the desired power-curve ramp without requiring a complex scheduler.

## Related note

The attached LandCore `issues.md` reports a separate smoke blocker: `POST /workflow` returns a generic 500 before creating a workflow run. This worker-capacity slice should not try to fix that admission/compile bug directly, but it should make the controller ready to consume queued work once admission succeeds.

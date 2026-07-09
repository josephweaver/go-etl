# Model Implementation Plan

## Recommended first model

Use **GPT-5.5 Thinking** for OS-001.

Reason: this slice is not algorithmically hard, but it crosses multiple controller lifecycle points. The risk is missing a state transition.

## Suggested prompt boundary

Give the model only:

```text
docs/SC-controller-worker-execution-framework.md
docs/OS-001-one-by-one-worker-capacity-manager.md
cmd/controller/main.go
cmd/controller/worker_scaler.go
cmd/controller/worker_scaler_test.go
cmd/controller/execution_environment.go
cmd/controller/worker_launch_config.go
cmd/controller/scheduler.go
cmd/controller/direct_process_scheduler.go
cmd/controller/slurm_scheduler.go
cmd/controller/runtime.go
cmd/controller/defaults.json
```

Then instruct it to avoid worker plugin internals.

## Decomposition warning

Do not let the model turn this into a broad scheduler rewrite.

The first implementation should be narrow:

```text
demand snapshot -> policy plan -> start one worker -> record inflight reservation
```

## Follow-up slices that may become necessary

### OS-002: Durable worker registry

Only needed if attempt-backed active capacity is too approximate.

Would add:

```text
worker_id
launch_id
started_at
last_seen_at
state
backend_handle
```

### OS-003: Caretaker capacity reconciliation

Only needed if capacity evaluation hooks are not enough or if expired inflight starts need automatic cleanup without new events.

### OS-004: Worker execution pattern registry

Only needed once a second pattern is introduced.

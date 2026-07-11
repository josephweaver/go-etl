# OS-002: Worker Registration, Heartbeat, and Stop Protocol

## Status

Implementation in progress.

Current implementation:

- shared lifecycle wire models exist in `internal/model`;
- controller defaults and validation resolve `worker_heartbeat_interval` and `worker_dead_after`;
- controller routes `/workers/register`, `/workers/heartbeat`, and `/workers/stop` persist session state;
- lifecycle routes require worker or admin authorization;
- registration confirms one inflight worker-start reservation and signals worker state changed after persistence commits;
- heartbeat updates only active matching sessions and does not signal worker capacity;
- stop is idempotent for stopped sessions, rejects dead/unknown sessions, and signals after a successful active-to-stopped transition.
- worker-side lifecycle client methods can register, heartbeat, and stop with typed session state and `ErrWorkerSessionNotActive` conflict handling.
- worker-side `RunHeartbeat` uses a cancellable ticker loop, refreshes liveness after accepted heartbeats, maps rejected sessions distinctly, and self-fences after `dead_after` of transient heartbeat failures.

Remaining OS-002 work:

- normal worker startup sequence changes;
- no-work graceful stop;
- self-fencing behavior.

## Minimum capable model

Use **GPT-5.5** with **High** reasoning.

The slice spans controller HTTP, authorization, worker client lifecycle, background cancellation, and configuration. Use **GPT-5.3-codex-spark** for isolated test expansion after the primary implementation.

## Goal

Make every normal worker process:

1. register before requesting work;
2. hold a controller-issued worker/session identity;
3. send heartbeats in the background throughout long-running work;
4. stop heartbeating before shutdown;
5. report a graceful stop when it exits because no work is available; and
6. self-fence after it has been unable to maintain liveness for the configured death interval.

This slice establishes the protocol. OS-003 enforces assignment ownership and performs dead-session abandonment/requeue.

## Scope

### In scope

- Worker lifecycle request/response models.
- `/workers/register`.
- `/workers/heartbeat`.
- `/workers/stop`.
- Worker-side identity state.
- Background heartbeat loop.
- Graceful stop on no work.
- Heartbeat timing configuration and validation.
- Stable HTTP status/error behavior.
- Worker/controller tests.
- Route authorization updates.

### Out of scope

- CareTaker loop.
- Worker launch decisions.
- Expiring sessions.
- Abandoning/requeueing running work.
- Enforcing session ownership in claim/completion/failure.
- Changing workflow semantics.
- Worker scale down.

## Preferred file budget

```text
cmd/controller/main.go
cmd/controller/worker_lifecycle.go
cmd/controller/worker_lifecycle_test.go
cmd/controller/config.go
cmd/controller/defaults.json
cmd/worker/main.go
cmd/worker/state.go
cmd/worker/config.go
cmd/worker/*_test.go
internal/model/worker_lifecycle.go
internal/controllerauth/*
```

Create `worker_lifecycle.go` rather than expanding `main.go` with all lifecycle logic.

Do not touch worker plugins.

## Configuration

Add controller defaults:

```json
{
  "name": {
    "namespace": "controller_config",
    "key": "worker_heartbeat_interval"
  },
  "type": "string",
  "expression": "1m"
}
```

```json
{
  "name": {
    "namespace": "controller_config",
    "key": "worker_dead_after"
  },
  "type": "string",
  "expression": "5m"
}
```

Validation:

```text
worker_heartbeat_interval > 0
worker_dead_after > worker_heartbeat_interval
worker_dead_after >= 2 * worker_heartbeat_interval
```

Recommended default ratio is 5:1.

The controller returns the resolved timing values in the registration response. The worker does not need duplicate static timing configuration.

## Protocol models

Suggested shared models:

```go
type WorkerRegistrationRequest struct {
    ExecutionHandle      string `json:"execution_handle,omitempty"`
    ExecutionEnvironment string `json:"execution_environment,omitempty"`
}

type WorkerRegistration struct {
    WorkerID                string `json:"worker_id"`
    WorkerSessionID         string `json:"worker_session_id"`
    HeartbeatIntervalSeconds int   `json:"heartbeat_interval_seconds"`
    DeadAfterSeconds         int   `json:"dead_after_seconds"`
}

type WorkerHeartbeatRequest struct {
    WorkerID        string `json:"worker_id"`
    WorkerSessionID string `json:"worker_session_id"`
}

type WorkerStopRequest struct {
    WorkerID        string `json:"worker_id"`
    WorkerSessionID string `json:"worker_session_id"`
    Reason          string `json:"reason"`
}
```

Use integer seconds on the wire and `time.Duration` internally.

Do not accept a client-provided `last_heartbeat_at`. Controller receipt time is authoritative.

## Controller routes

Register:

```text
POST /workers/register
```

Heartbeat:

```text
POST /workers/heartbeat
```

Stop:

```text
POST /workers/stop
```

All three are worker-role routes under the existing controller authentication policy.

### Register handler

Required behavior:

1. require POST;
2. require normal controller operation;
3. decode bounded JSON;
4. resolve/validate heartbeat timing;
5. generate cryptographically strong worker and session IDs;
6. call persistence registration with controller current time;
7. return `201 Created` and registration JSON;
8. signal worker-capacity state changed through an injectable callback, initially a no-op until the CareTaker slice;
9. never expose controller credentials.

Suggested identifiers:

```text
worker-<random>
worker-session-<random>
```

A registration confirms the oldest inflight start reservation only when one exists. Because `one_by_one_until_saturated` permits one unconfirmed start, FIFO confirmation is unambiguous. Move the current confirmation concept from successful claim to registration, but do not yet remove the old claim hook until OS-005 cutover.

Manual workers may register with no reservation.

### Heartbeat handler

Required behavior:

1. require POST;
2. decode worker/session IDs;
3. use controller current time;
4. update only the matching active session;
5. return 204 on success;
6. return 409 with stable error code when no active matching session exists;
7. do not create or reactivate a session;
8. do not normally trigger a worker-capacity evaluation or wake.

Suggested response body for conflict:

```json
{
  "error": "worker_session_not_active"
}
```

Use the repository's existing error-response convention if one exists.

### Stop handler

Required behavior:

1. require POST;
2. validate a non-empty reason;
3. mark an active matching session stopped when it has no running assignment;
4. make duplicate same-session stop idempotent;
5. return 204 when stopped/already stopped;
6. reject a worker/session mismatch;
7. initially return 409 if running work prevents a safe stop;
8. invoke an injectable state-change callback after commit.

OS-003 will extend stop to atomically abandon/requeue owned work when necessary.

## Worker-side session object

Add a lifecycle object similar to:

```go
type WorkerSession struct {
    WorkerID         string
    WorkerSessionID  string
    HeartbeatInterval time.Duration
    DeadAfter         time.Duration
}
```

The normal worker controller client should hold the active registration after register succeeds.

Direct one-shot `worker execute` mode must remain controller-free and must not register or heartbeat.

## Worker startup sequence

Revise the normal path:

```text
load config
build controller client
validate worker
register worker
start heartbeat supervisor
run worker loop
cancel heartbeat supervisor
wait for heartbeat goroutine
send graceful stop when appropriate
exit
```

Registration must complete before the first `/work/next`.

If registration fails, do not request work.

## Background heartbeat supervisor

Use one goroutine and a cancellable context.

Pseudo-code:

```go
func RunHeartbeat(
    ctx context.Context,
    session WorkerSession,
    heartbeat func(context.Context, WorkerSession) error,
    clock Clock,
) error {
    ticker := clock.NewTicker(session.HeartbeatInterval)
    defer ticker.Stop()

    lastAccepted := clock.Now()

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            err := heartbeat(ctx, session)
            if err == nil {
                lastAccepted = clock.Now()
                continue
            }

            if errors.Is(err, ErrWorkerSessionNotActive) {
                return ErrWorkerSessionNotActive
            }

            if clock.Now().Sub(lastAccepted) >= session.DeadAfter {
                return ErrWorkerSelfFenced
            }
        }
    }
}
```

Do not use `time.Sleep` loops that cannot be canceled.

The implementation may retry transient errors more frequently than the normal heartbeat interval, but it must avoid a tight loop.

## Self-fencing

If the controller explicitly rejects the session, or no accepted heartbeat has occurred for `dead_after`:

- stop future work claims;
- cancel the current run context when supported;
- prevent normal completion/failure reporting under the stale session;
- return a distinct worker process error;
- require a new process to register a new session.

Do not silently register a replacement session in the same process after fencing. That could allow a stale execution to regain authority.

Because current `Worker.Run` may not accept context everywhere, the first implementation may guarantee:

```text
no new claim
no outcome report after self-fence
process exits after current call returns
```

and create a follow-up issue for cooperative plugin cancellation. Do not falsely claim arbitrary work has been interrupted.

## Graceful no-work exit

Current behavior exits when `/work/next` returns 204. Preserve that behavior initially, but report stop:

```text
FetchWorkItem -> no work
cancel heartbeat context
join heartbeat goroutine
POST /workers/stop reason=no_work
exit success
```

Cancel heartbeat before stop so an in-flight heartbeat cannot race after terminal state.

If stop transport fails, log the error and exit. The controller will eventually expire the session.

## Other exits

Use reason values from a small stable set:

```text
no_work
worker_error
controller_rejected_session
shutdown
```

Do not put arbitrary stack traces in `end_reason`.

For an execution error already reported through `/work/fail`, cancel heartbeat and stop with `worker_error`.

For OS/process termination where a stop call cannot run, heartbeat expiry is the recovery path.

## Client methods

Add methods similar to:

```go
RegisterWorker(ctx context.Context, request WorkerRegistrationRequest) (WorkerSession, error)
HeartbeatWorker(ctx context.Context, session WorkerSession) error
StopWorker(ctx context.Context, session WorkerSession, reason string) error
```

Use the authenticated worker controller client.

Keep existing unauthenticated test helpers only where still required by tests; do not create public lifecycle routes to preserve old helpers.

## Error typing

Worker client code must distinguish:

```text
ErrWorkerSessionNotActive
ErrWorkerSelfFenced
transient transport/server error
```

Do not parse human-readable log text to determine error type.

## CareTaker signal seam

This slice may add a controller callback/interface without implementing the loop:

```go
type WorkerStateChangeSignaler interface {
    SignalWorkerStateChanged(reason string)
}
```

or a function field:

```go
signalWorkerStateChanged func(reason string)
```

Default it to a no-op for tests and until OS-004.

Registration and stop invoke the seam only after persistence commits.

Heartbeat does not invoke it on ordinary success.

## Tests

### Controller handler tests

```text
TestRegisterWorkerCreatesActiveSessionAtControllerTime
TestRegisterWorkerReturnsHeartbeatPolicy
TestRegisterWorkerRequiresWorkerAuthorization
TestHeartbeatUpdatesMatchingActiveSession
TestHeartbeatRejectsUnknownSession
TestHeartbeatRejectsStoppedSession
TestHeartbeatRejectsDeadSession
TestHeartbeatDoesNotUseClientTimestamp
TestStopWorkerMarksSessionStopped
TestStopWorkerIsIdempotent
TestStopWorkerRejectsWorkerSessionMismatch
TestStopWorkerSignalsAfterCommit
TestHeartbeatDoesNotSignalCapacityChange
```

### Worker client tests

```text
TestWorkerRegistersBeforeFirstClaim
TestHeartbeatRunsWhileWorkItemExecutes
TestHeartbeatStopsOnContextCancellation
TestHeartbeatRejectedSessionSelfFences
TestHeartbeatTransportFailuresSelfFenceAfterDeadAfter
TestTransientHeartbeatFailureBeforeDeadAfterDoesNotFence
TestNoWorkCancelsHeartbeatBeforeStop
TestNoWorkSendsGracefulStop
TestDirectExecuteDoesNotRegisterOrHeartbeat
```

Use fake clocks/tickers. Do not make minute-long tests.

### Race-oriented test

Create a fake heartbeat function blocked on a channel:

1. let heartbeat begin;
2. trigger no-work exit;
3. release heartbeat;
4. prove stop occurs only after heartbeat goroutine has joined.

## Implementation sequence

1. Add shared lifecycle wire models.
2. Add config defaults and validation.
3. Add controller lifecycle handlers.
4. Add authorization profiles for lifecycle routes.
5. Add client register/heartbeat/stop methods.
6. Add worker session state.
7. Add cancellable heartbeat supervisor with fake-clock tests.
8. Register before the first claim.
9. Cancel/join heartbeat before graceful stop.
10. Preserve direct execute behavior.
11. Run worker, controller, and auth tests.

## Acceptance criteria

1. A normal worker registers before claiming work.
2. Registration creates an active durable session using controller time.
3. The worker heartbeats while a work item runs.
4. A successful heartbeat does not wake worker scheduling.
5. Dead/stopped/unknown sessions are not reactivated.
6. No-work exit cancels and joins heartbeat before sending stop.
7. A session rejected by the controller self-fences.
8. A worker without a successful heartbeat for `dead_after` self-fences.
9. Direct one-shot execution remains unchanged.
10. This slice does not yet abandon/requeue assignments or introduce the CareTaker loop.

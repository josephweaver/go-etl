# 007 Controller Checkpoint Confirmation and Resume Assignment Transport

Status: implemented

## Objective

Expose the OS-006 checkpoint persistence lifecycle through authenticated,
owner-fenced controller HTTP endpoints and the existing `/work/next` claim
response. A worker can confirm a periodic, quantum, or final checkpoint,
suspend from its latest accepted checkpoint, receive an exact acknowledgement,
and receive validated resume metadata when a later claim consumes that
checkpoint.

Convert a pending resume whose configured per-artifact attempt limit is
exhausted into the durable terminal failure
`resume_attempt_limit_exhausted` instead of returning a generic claim error or
leaving permanently unclaimable work in the queue.

## Current State

OS-006 implements SQLite schema version 7 and the persistence transactions in
`internal/persistence/store.go`:

- `Store.ConfirmCheckpoint` accepts exact manifest JSON and its
  `ResumeArtifactReference`;
- periodic/continue leaves the current assignment running;
- quantum/suspend and final/suspend atomically create suspended history,
  remove running ownership, and queue a resume artifact;
- `Store.SuspendFromLatestCheckpoint` performs the same suspension transition
  from the newest accepted generation;
- `ClaimNextWork` returns predecessor, lineage, resume-attempt number, and the
  validated `ResumeArtifactRecord`; and
- a positive `ResumeAttemptLimit` returns
  `ErrResumeAttemptLimitExceeded` before mutating the pending row.

These store operations have no controller transport.

`cmd/controller/main.go` currently registers:

```text
GET  /work/next
POST /work/complete
POST /work/fail
```

The claim handler:

- authenticates assignment ownership through `X-Goetl-Worker-Id` and
  `X-Goetl-Worker-Session-Id`;
- obtains the heartbeat cutoff from controller policy;
- calls `ClaimNextWork` without a positive resume-attempt limit;
- converts the persisted worker payload to `model.WorkItem`; and
- returns only that work-item JSON.

It discards the resume metadata already returned by the store.

`internal/model.WorkItem` has `attempt_id` but no runtime resume assignment.
There is no shared checkpoint HTTP request or acknowledgement type. The worker
HTTP client has no checkpoint confirmation methods.

`internal/controllerauth.ControllerPolicy` does not recognize checkpoint
routes. `Controller.signalCareTaker` already provides a nonblocking
state-change wake, but no checkpoint handler calls it.

Exact suspend replays currently return the same persisted artifact and
suspended row as a newly committed suspend. The store result does not tell an
HTTP handler whether that call performed the transition, so the handler cannot
distinguish a new demand-producing commit from an idempotent replay.

At resume-attempt-limit exhaustion, `ClaimNextWork` returns the sentinel error
without identifying the selected pending item to the caller and without
creating terminal history. Returning HTTP 500 would leave the same queue row
to fail every later claim.

## Target State

### Shared transport model

Add `internal/model/checkpoint_transport.go` with versioned, validated transport
types:

```text
WorkCheckpointConfirmation
WorkCheckpointSuspendLatest
WorkCheckpointAcknowledgement
WorkItemResumeAssignment
```

`WorkCheckpointConfirmation` contains:

```text
attempt_id
manifest_json
reference
capture_kind
disposition
suspended_at (required only for suspend)
```

`manifest_json` is a JSON string, not an embedded JSON object. The controller
passes that exact byte sequence to `Store.ConfirmCheckpoint`; it does not
decode and re-marshal the manifest before OS-006 hashes it. This preserves
exact idempotent replay after an ambiguous HTTP response.

Only these confirmation pairs are valid:

```text
periodic / continue
quantum  / suspend
final    / suspend
```

`WorkCheckpointSuspendLatest` contains a stable caller-supplied:

```text
attempt_id
suspended_at
suspend_reason (quantum or shutdown)
```

The caller repeats the same timestamps and request body when resolving an
ambiguous response. The controller chooses `accepted_at` when a new checkpoint
generation is first accepted.

`WorkCheckpointAcknowledgement` names the exact controller-accepted state:

```text
operation (checkpoint_confirmation or suspend_latest)
resume_artifact_id
execution_lineage_id
resume_generation
reference
capture_kind
accepted_at
disposition
suspended
suspended_at (when suspended)
```

An exact confirmation or suspension replay returns the same acknowledgement.
For `suspend_latest`, `capture_kind` describes the selected artifact's original
capture (often `periodic`) while `disposition` describes the new `suspend`
transition. Acknowledgement validation permits that combination only when
`operation=suspend_latest` and `suspended=true`.

`WorkItemResumeAssignment` uses schema
`goet/work-item-resume-assignment/v1` and contains:

```text
schema
resumed_from_attempt_id
execution_lineage_id
resume_attempt_number
manifest_json
reference
```

Its validation:

- hashes the exact `manifest_json` string and matches the reference;
- decodes and validates `goet/resume-artifact/v1`;
- matches the artifact work item to the containing `WorkItem.ID`;
- matches the producing attempt to `resumed_from_attempt_id`;
- matches the execution lineage and reference identity; and
- matches the manifest work-item type to the containing `WorkItem.Type`; and
- requires a positive resume-attempt number.

Add:

```go
Resume *WorkItemResumeAssignment `json:"resume,omitempty"`
```

to `model.WorkItem`. `WorkItem.Validate` validates the nested assignment when
present. Fresh assignments omit the field, preserving their current JSON
shape and behavior.

### Controller checkpoint endpoints

Register two authenticated worker routes:

```text
POST /work/checkpoint/confirm
POST /work/checkpoint/suspend-latest
```

Both routes permit only `worker` and `admin` credentials. Both require the
existing worker and worker-session identity headers. Body fields cannot
override header ownership.

`/work/checkpoint/confirm`:

1. bounds the request body with `controller_max_request_bytes`;
2. strictly decodes and validates `WorkCheckpointConfirmation`;
3. supplies header ownership, the current live-session cutoff, and a
   controller-selected `accepted_at` to `Store.ConfirmCheckpoint`;
4. returns the exact `WorkCheckpointAcknowledgement`; and
5. signals the CareTaker with `checkpoint_suspended` only when this call
   commits a new suspend transition.

A periodic confirmation does not change worker demand and does not wake the
CareTaker. An exact suspend replay returns `200` with the original
acknowledgement but does not emit another wake.

`/work/checkpoint/suspend-latest` performs the same request bounding,
ownership, cutoff, acknowledgement, and one-time CareTaker behavior through
`Store.SuspendFromLatestCheckpoint`. When the owned running attempt has no
accepted checkpoint, it returns:

```text
409 no_accepted_checkpoint
```

without changing ownership.

The endpoint status contract is:

```text
200  accepted or exact idempotent replay
400  malformed or invalid transport contract
409  assignment_no_longer_owned, checkpoint_conflict,
     or no_accepted_checkpoint
413  request body exceeds controller_max_request_bytes
503  workflow store is unavailable
500  unexpected persistence failure
```

Responses and logs never include checkpoint file contents. Validation errors
are bounded and must not echo `manifest_json`.

### One-time suspend transition evidence

Extend `ConfirmCheckpointResult` and
`SuspendFromLatestCheckpointResult` with a boolean that is true only when the
current transaction changed a running assignment into suspended history.

New quantum/final/fallback suspension returns true after commit. Exact replay,
periodic continuation, and no-checkpoint fallback return false. This value is
controller orchestration evidence only; it is not persisted or returned as a
separate public state.

### Resume assignment claim

Add controller policy:

```text
controller_config.resume_attempt_limit = 3
```

The value must be a positive integer. It is the controller enforcement ceiling
for this slice and is passed to every `ClaimNextWork` call. A future
worker-job-specific override may choose a lower value, but a worker request
must never be able to raise the controller ceiling.

For a successful resumed claim, `nextPersistedWorkHandler` maps the store
record into `WorkItem.Resume` without re-marshalling the persisted manifest.
The mapping retains:

- the new claim's ordinary `attempt_id`;
- the producing predecessor attempt;
- the execution lineage;
- the per-artifact resume-attempt number;
- the exact manifest JSON; and
- its validated reference.

Fresh claims continue to return the existing `WorkItem` shape without a
`resume` property.

### Resume-attempt-limit terminalization

Replace the context-free limit error with an error that still matches
`ErrResumeAttemptLimitExceeded` through `errors.Is` and also identifies:

```text
work_item_id
resume_artifact_id
execution_lineage_id
next_resume_attempt_number
configured_limit
queued_at
```

Add an idempotent store transaction owned by `internal/persistence`, named
equivalently to `FailPendingResumeAttemptLimit`, that:

1. verifies the pending row still names the same artifact and remains over the
   supplied positive limit;
2. creates a caller-supplied controller-executor attempt carrying the artifact,
   predecessor, lineage, and next resume-attempt number as terminal-decision
   evidence;
3. writes `failed_work` with exact error
   `resume_attempt_limit_exhausted`;
4. removes the pending row; and
5. returns the failed record and work item.

The controller-executor attempt records the refused claim decision; it does not
represent a worker payload launch and does not acquire running ownership.

When `/work/next` receives the typed limit error, it invokes this transaction
with the same generated attempt ID, applies the existing failed-work dependency
propagation, signals the CareTaker with
`resume_attempt_limit_exhausted`, and returns `204`. It does not expose a
generic worker error, silently restart fresh work, or leave the exhausted row
pending.

If the pending artifact changed before terminalization, the transaction fails
closed and does not fail different work.

### Worker HTTP client

Add a focused worker checkpoint client file with methods on
`WorkerControllerClient` that:

- POST `WorkCheckpointConfirmation` to `/work/checkpoint/confirm`;
- POST `WorkCheckpointSuspendLatest` to
  `/work/checkpoint/suspend-latest`;
- attach the existing worker/session identity headers;
- decode and validate `WorkCheckpointAcknowledgement`; and
- return controller HTTP errors without logging request bodies or manifest
  contents.

This slice proves the callable transport but does not invoke these methods from
the worker run loop. OS-008 owns checkpoint timers, drain state, serialization,
and retry decisions.

## Concept Decision

This slice updates the existing controller HTTP, authorization, worker-client,
work-item assignment, and persistence concepts.

The versioned transport types form a new shared model concept with independent
validation and therefore belong in new
`internal/model/checkpoint_transport.go` and its focused test.

The worker checkpoint HTTP methods form a separate client responsibility and
belong in new `cmd/worker/checkpoint_client.go` rather than enlarging the
existing completion/failure code in `state.go`.

Controller route wiring and work claim already belong to
`cmd/controller/main.go`; keep the two small handlers there for this slice.
Do not create a second controller service or bypass the existing
`Controller`, authentication middleware, heartbeat cutoff, or CareTaker signal
function.

No schema migration is required. Terminal limit exhaustion uses the existing
version 7 attempt and failure tables.

## Required Context

Read these files first:

- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/006-checkpoint-generation-and-pending-resume-store-lifecycle.md`
- `internal/model/resume_artifact.go`
- `internal/model/work_item.go`
- `internal/persistence/store.go`
- `cmd/controller/main.go`
- `cmd/controller/config.go`
- `internal/controllerauth/policy.go`
- `cmd/worker/state.go`

Read the corresponding allowed test files only when implementing their
production owner. Do not read worker execution, Slurm, container, adapter,
workflow compiler, or unrelated persistence code unless a focused test failure
exposes a direct dependency.

## Allowed Production Files

- `internal/model/checkpoint_transport.go` (new)
- `internal/model/work_item.go`
- `internal/persistence/store.go`
- `cmd/controller/config.go`
- `cmd/controller/defaults.json`
- `internal/controllerauth/policy.go`
- `cmd/controller/main.go`
- `cmd/worker/checkpoint_client.go` (new)

## Allowed Test Files

- `internal/model/checkpoint_transport_test.go` (new)
- `internal/model/work_item_test.go`
- `internal/persistence/store_test.go`
- `cmd/controller/config_test.go`
- `internal/controllerauth/policy_test.go`
- `cmd/controller/main_test.go`
- `cmd/worker/checkpoint_client_test.go` (new)

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/README.md`
- `docs/concepts/dmtcp-work-item-checkpoint-resume/007-controller-checkpoint-confirmation-and-resume-assignment-transport.md`

## Out Of Scope

- Worker checkpoint timers, drain state, `SIGUSR1`, execution quantum, final
  pause escalation, completion/pause mutual exclusion, or shutdown exit logic.
- Creating checkpoint directories, quiescing a payload, copying registered
  outputs, writing manifests, hashing checkpoint files, or reconciling a
  resumed workspace.
- Invoking DMTCP, CRIU, rclone continuation, or manual-Go pause/resume
  adapters.
- Changing Slurm scripts, warning-signal configuration, Singularity execution,
  container images, or scheduler requeue behavior.
- Adding administrative drain endpoints, operator retry/reset endpoints,
  retention cleanup, artifact deletion, or compatibility rejection reporting.
- Allowing a worker-supplied resume-attempt limit to raise the controller
  policy.
- Changing schema version 7 or adding a second pending queue.
- Promoting checkpointed output snapshots to completed workflow outputs.
- Retrying causal `WorkFailure`; it remains terminal.

## Acceptance Criteria

- The shared transport types validate supported capture/disposition/reason
  combinations, acknowledgement operation, timestamps, exact manifest JSON
  hash, artifact identity, predecessor, lineage, work-item identity/type, and
  positive resume-attempt number.
- Invalid or mismatched resume assignments make `WorkItem.Validate` fail;
  ordinary fresh work-item validation remains unchanged.
- Fresh `/work/next` responses have no `resume` field.
- Resumed `/work/next` responses contain the new attempt ID and exact validated
  predecessor, lineage, per-artifact attempt number, manifest JSON, and
  reference persisted by OS-006.
- The default controller document declares `resume_attempt_limit` as `3`;
  configuration rejects zero, negative, and non-integer values.
- Every claim passes the resolved positive limit to `ClaimNextWork`.
- `POST /work/checkpoint/confirm` and
  `POST /work/checkpoint/suspend-latest` are registered and authorized only for
  worker/admin roles.
- Both endpoints require the existing worker/session headers, enforce the live
  heartbeat cutoff, bound request bodies, and reject body data that conflicts
  with persisted assignment or artifact identity.
- Periodic confirmation returns `200` with the accepted artifact ID,
  generation, lineage, reference, and acceptance time while leaving the
  assignment running and emitting no CareTaker wake.
- Quantum/final confirmation returns the exact acknowledgement only after the
  atomic suspension commit and emits one `checkpoint_suspended` wake.
- Fallback suspension returns the selected latest accepted generation and
  emits one wake after the transition; no accepted generation returns
  `409 no_accepted_checkpoint` without mutation.
- Exact periodic, suspend, and fallback request replay is idempotent, returns
  the original acknowledgement, and emits no duplicate CareTaker wake.
- Wrong, expired, stopped, or stale ownership returns
  `409 assignment_no_longer_owned`.
- Invalid bodies return `400`, oversized bodies return `413`, conflicting
  artifact reuse/state returns `409`, missing store returns `503`, and
  unexpected persistence failures return `500`.
- Error responses and logs do not contain `manifest_json`, manifest file
  inventory, or checkpoint file content.
- A claim above the configured limit atomically creates one controller
  terminal-decision attempt and `failed_work` record with
  `resume_attempt_limit_exhausted`, removes the pending row, propagates the
  existing workflow failure semantics, signals the CareTaker, and returns
  `204`.
- Limit terminalization is idempotent and cannot fail a different or newly
  updated pending artifact.
- A limit-exhausted item is not silently restarted fresh and does not remain in
  worker demand.
- The worker checkpoint client attaches session headers, sends the shared
  request unchanged, and validates the acknowledgement.
- Existing work claim, completion, failure, authentication, worker-session,
  CareTaker, and persistence tests remain green.
- Focused package tests pass:

  ```text
  go test ./internal/model ./internal/persistence ./internal/controllerauth ./cmd/controller ./cmd/worker -count=1
  ```

- Project/test documentation says controller transport is implemented but
  worker checkpoint scheduling and adapter execution are not.
- No production or test file outside the allowed lists changes.

## Implementation Sequence

Implement in bounded production-owner passes:

1. shared checkpoint/resume transport model and optional work-item assignment;
2. one-time suspend evidence and limit terminalization in the store;
3. resume-limit controller configuration and checked-in default;
4. checkpoint route authorization;
5. controller handlers, claim mapping, terminal failure propagation, and
   CareTaker signaling; and
6. callable worker checkpoint HTTP client plus final documentation.

Run the narrowest package test after each pass. Stop for review between passes
when the selected implementation change budget requires it.

## Implementation Progress

2026-08-01 pass 1 implemented
`internal/model/checkpoint_transport.go` and its focused test. The shared model
now validates the three confirmation modes, suspend-latest requests,
confirmation versus fallback acknowledgements, exact manifest JSON/reference
digests, and work-item resume assignments. `go test ./internal/model -count=1`
passes.

2026-08-01 pass 2 attached the optional resume assignment to `model.WorkItem`.
Runtime validation now requires a distinct nonempty new attempt ID and validates
the nested artifact against the containing work-item identity and type. Fresh
JSON omits `resume`; resumed JSON round-trips the exact manifest string.

2026-08-01 pass 3 extended `internal/persistence/store.go`. Checkpoint
confirmation and suspend-latest results now identify only a newly committed
running-to-suspended transition, leaving periodic confirmations and idempotent
replays false. Resume-attempt-limit claims return a typed error containing the
exact pending artifact, lineage, next attempt number, configured limit, and
queue timestamp. `FailPendingResumeAttemptLimit` rechecks those pending facts
and the live count in one fail-closed transaction, records a controller-owned
terminal attempt with `resume_attempt_limit_exhausted`, removes only the exact
pending row, and supports exact replay. Focused and complete
`go test ./internal/persistence -count=1` verification passes. Configuration,
authorization, controller, and worker-client passes remain pending.

2026-08-01 pass 4 added `resumeAttemptLimitConfig` to
`cmd/controller/config.go`. The focused resolver returns the initial ceiling of
`3` when the value is omitted, accepts a configured positive integer, and
rejects zero, negative, and non-integer values. Focused
`go test ./cmd/controller -run 'TestResumeAttemptLimitConfig' -count=1`
verification passes. The checked-in `defaults.json` declaration is separated
into the next one-production-file prompt; startup/claim wiring,
authorization, handlers, and the worker client remain pending.

2026-08-01 pass 5 added the integer `controller_config.resume_attempt_limit`
declaration with value `3` to `cmd/controller/defaults.json`. The checked-in
defaults test now requires the key and resolves it through the OS-007 policy
function. Focused
`go test ./cmd/controller -run 'TestLoadDefaultsDocument' -count=1`
verification passes. Controller claim wiring, authorization, handlers, and the
worker client remain pending.

2026-08-01 pass 6 updated `internal/controllerauth/policy.go` so only `worker`
and `admin` roles may POST to `/work/checkpoint/confirm` and
`/work/checkpoint/suspend-latest`. Tests cover the role matrix, protected-route
classification, and wrong-method behavior. Focused package tests pass. HTTP
handler registration, claim wiring, and the worker client remain pending.

2026-08-01 pass 7 registered and implemented both checkpoint handlers in
`cmd/controller/main.go`. They require existing worker/session headers,
strictly decode one bounded JSON document, validate the shared model, supply
the live-session cutoff and controller acceptance time, map persistence results
to validated acknowledgements, sanitize request and persistence errors, and
signal `checkpoint_suspended` only for a newly committed suspension. Focused
tests prove periodic continuation, quantum/final suspension, idempotent replay,
fallback suspension, no-checkpoint conflict, request limits, unknown-field
rejection, manifest-error redaction, owner fencing, artifact conflicts, and mux
registration. The focused command passes. The complete `cmd/controller`
package run reaches the unrelated
pre-existing `TestNewStartupRuntimeScope` precision mismatch: the unchanged
implementation emits second precision while that test expects nanoseconds.
Resume claim mapping, limit terminalization propagation, and the worker client
remain pending.

2026-08-01 pass 8 completed controller claim transport in
`cmd/controller/main.go`. Every store claim now receives the resolved positive
resume ceiling. Successful resumed claims map the exact persisted manifest
string and reference plus the new attempt ID, predecessor, lineage, and
per-artifact attempt number into `WorkItem.Resume`; fresh JSON still omits the
field. A typed over-limit decision is terminalized under the existing claim
mutex with the same generated attempt ID, then receives the existing dependency
failure propagation, emits one `resume_attempt_limit_exhausted` CareTaker wake,
and returns `204`. Focused controller tests pass for exact resume assignment,
fresh omission, and persisted controller-owned limit failure with no remaining
queue demand. The worker checkpoint client and final slice-wide verification
remain pending.

2026-08-01 pass 9 added `cmd/worker/checkpoint_client.go` with callable
`ConfirmCheckpoint` and `SuspendLatestCheckpoint` methods. Both validate the
shared request before posting, attach the required worker/session headers via
the existing client boundary, require `200`, decode exactly one acknowledgement,
validate its shared contract, and fence the response operation and request
identity without including manifest content in HTTP error wrapping. Focused
client tests and `go test ./cmd/worker -count=1` pass. The exact slice-wide
package command passes model, persistence, authorization, and worker packages;
`cmd/controller` reaches only the already-recorded unrelated
`TestNewStartupRuntimeScope` precision mismatch. OS-007 is implemented with
that explicit verification gap retained in the test-status record.

## Notes

- The worker owns request idempotency values: artifact ID, exact manifest JSON,
  reference digest, and `suspended_at`.
- The controller owns `accepted_at`, session cutoff, terminal-decision attempt
  ID, and enforcement of the resume-attempt ceiling.
- Do not trust timestamps, worker IDs, or session IDs embedded in a manifest as
  proof of live ownership; the existing request headers and SQLite assignment
  fence remain authoritative.
- Do not expose `persistence.ResumeArtifactRecord` directly as public JSON.
  Map it into the versioned model transport contract.
- Do not use a JSON object or `json.RawMessage` for `manifest_json` if encoding
  the outer request could change its exact bytes.
- A successful HTTP acknowledgement means the SQLite transaction committed. It
  does not prove checkpoint file bytes; the later adapter is responsible for
  validating them before confirmation and before restore.

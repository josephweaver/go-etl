# 012e Persistence-backed Completion and Failure Reports

Status: implemented

## Objective

Move `/work/complete` and `/work/fail` terminal reporting onto
`internal/persistence.Store` when the workflow-execution store is configured.

This slice should terminate the active persisted attempt created by 012d. It
also updates the controller-worker report contract so workers receive and report
the controller-owned `attempt_id`.

## Required Context

Read these files first:

- `docs/concepts/workflow-execution-persistence/012-controller-integration-cutover.md`
- `docs/concepts/workflow-execution-persistence/012d-persistence-backed-work-claim.md`
- `docs/concepts/workflow-execution-persistence/008-attempt-terminal-transition-transaction.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `cmd/worker/state.go`
- `cmd/worker/state_test.go`
- `cmd/worker/work_demo.go`
- `cmd/worker/work_demo_test.go`
- `internal/model/work_item.go`
- `internal/model/work_item_test.go`
- `internal/persistence/store.go`

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/worker/state.go`
- `cmd/worker/work_demo.go`
- `internal/model/work_item.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/worker/state_test.go`
- `cmd/worker/work_demo_test.go`
- `internal/model/work_item_test.go`

## Documentation Files

- `docs/concepts/workflow-execution-persistence/012e-persistence-backed-terminal-reports.md`
- `PROJECT_STATE.md`
- `epi_ctl/20260703.md`

## Attempt Report Contract

The workflow-execution store creates the real `attempt_id` during
`ClaimNextWork`. Workers must report that controller-owned attempt ID when they
complete or fail work. A worker-generated attempt ID is no longer valid for the
workflow-execution persistence path.

Update the transport model so assignment carries attempt metadata:

```go
type WorkItem struct {
    ID string
    AttemptID string `json:"attempt_id,omitempty"`
    ...
}
```

When `workflowStore` is configured, `/work/next` should set
`WorkItem.AttemptID` from `ClaimedWorkRecord.AttemptID` before returning the
assignment. The worker should echo `item.AttemptID` in completion and failure
reports. If an assigned item lacks `AttemptID`, the worker may keep its existing
fallback only for legacy in-memory controller mode.

## Completion Evidence Contract

Update the completion transport so the worker reports discernible terminal
evidence, not only fingerprints:

```go
type WorkCompletion struct {
    ID string
    AttemptID string
    OutputJSON string `json:"output_json,omitempty"`
    PreStateJSON string `json:"pre_state_json,omitempty"`
    PostStateJSON string `json:"post_state_json,omitempty"`
    ...
}
```

For the demo worker plugin, the operation should produce simple JSON evidence:

```json
{
  "output_json": {
    "work_item_id": "test-001",
    "output_filename": "result.txt",
    "output_path": ".../data/result.txt",
    "bytes_written": 19
  },
  "pre_state_json": {
    "output_exists": false
  },
  "post_state_json": {
    "output_exists": true,
    "output_path": ".../data/result.txt",
    "bytes_written": 19
  }
}
```

Exact field names can be adjusted during implementation, but the values should
be deterministic enough for tests to distinguish pre-state from post-state and
logical output. The worker reports JSON text; the controller computes hashes
before writing the database.

## Completion Mapping

When `Controller.workflowStore` is configured, `/work/complete` should:

1. Decode and validate the existing `model.WorkCompletion` enough to require
   `id` and `attempt_id`.
2. Call `Store.CompleteAttempt` using the reported `attempt_id`.
3. Validate `output_json`, `pre_state_json`, and `post_state_json` as JSON.
4. Compute SHA-256 hashes for those JSON documents.
5. Store `output_json`, `output_json_sha256`, `pre_state_sha256`, and
   `post_state_sha256` through `Store.CompleteAttempt`.
6. Return `204 No Content` on successful or idempotent completion.

Initial hash mapping:

```text
output_json          = completion.output_json
output_json_sha256   = sha256(canonical or raw normalized output_json)
pre_state_sha256     = sha256(canonical or raw normalized pre_state_json)
post_state_sha256    = sha256(canonical or raw normalized post_state_json)
completed_at         = completion.completed_at when present, otherwise controller time
```

If the existing canonical JSON helper is available and appropriate, use it.
Otherwise use a deterministic raw JSON normalization step in this slice and
document the limitation. The exact plugin state-observation schema can evolve
later; the important 012e requirement is that the worker reports actual
pre-state, post-state, and logical output evidence, and the controller computes
the stored hashes.

## Failure Mapping

When `Controller.workflowStore` is configured, `/work/fail` should:

1. Decode and validate the existing `model.WorkFailure`.
2. Require `attempt_id` when `workflowStore` is configured.
3. Call `Store.FailAttempt` using the reported `attempt_id`.
4. Return `204 No Content` on successful or idempotent failure.

## Acceptance Criteria

- `/work/complete` keeps existing behavior when no workflow-execution store is
  configured.
- `/work/fail` keeps existing behavior when no workflow-execution store is
  configured.
- `model.WorkItem` can carry optional `attempt_id` from controller assignment
  to worker execution.
- `model.WorkFailure` can carry optional `attempt_id` from worker failure report
  to controller terminal handling.
- When `workflowStore` is configured, `/work/next` returns the store-created
  `attempt_id` in the assigned work payload.
- Worker completion reports echo the assigned `attempt_id` instead of
  generating a new attempt ID when one is present.
- Worker failure reports include the assigned `attempt_id` when one is present.
- `model.WorkCompletion` can carry `output_json`, `pre_state_json`, and
  `post_state_json`.
- The demo worker operation creates discernible output, pre-state, and
  post-state JSON evidence.
- The controller computes `output_json_sha256`, `pre_state_sha256`, and
  `post_state_sha256` from worker-reported JSON evidence before calling
  `CompleteAttempt`.
- When `workflowStore` is configured, `/work/complete` completes the active
  persisted attempt for the reported attempt ID.
- When `workflowStore` is configured, `/work/fail` fails the active persisted
  attempt for the reported attempt ID.
- Completion removes the matching `running_work` row and creates one
  `completed_work` row.
- Failure removes the matching `running_work` row and creates one `failed_work`
  row.
- Completion/failure handlers do not mutate `Controller.assigned` when
  `workflowStore` is configured.
- Missing active persisted attempts return `404 Not Found`.
- Persisted terminal reports missing `attempt_id` return `400 Bad Request`.
- Duplicate identical terminal reports are accepted if the store reports an
  idempotent terminal result.
- Conflicting duplicate terminal reports fail.
- This slice does not change `/work/next`, `/work`, `/workflow`, or worker
  scaling behavior.
- Existing tests continue to pass.

## Out Of Scope

- UUIDv7 attempt IDs.
- Retry or requeue policy.
- Stage completion publication after terminal reports.
- Plugin-defined state observation.
- Final semantic output JSON schema beyond the demo plugin evidence.
- Removing old ledger helper code.
- Removing in-memory queue fallback behavior.

## Ambiguity To Review

The current contract still lacks a fencing token. `attempt_id` is enough to
identify the active attempt in the current schema, but liveness recovery should
eventually add a fencing/lease token so a stale worker cannot report completion
after the controller has abandoned and requeued the same logical work item.

The demo plugin evidence is intentionally small and local-filesystem-specific.
It proves the controller-worker-database evidence path without defining the
final plugin state-observation schema for every future operation.

## Notes

- Prefer helper functions for adding attempt metadata to assignments and
  building `persistence.CompleteAttemptRequest`.
- Do not use controller-side SQL.
- Do not hold `Controller.mu` for persistence-backed terminal reports.

## Implementation Notes

- Persisted `/work/next` now returns the store-created `attempt_id` on the
  assigned work item.
- Persisted `/work/complete` validates worker-reported JSON evidence,
  canonicalizes `output_json`, computes SHA-256 hashes for output, pre-state,
  and post-state, then calls `Store.CompleteAttempt`.
- Persisted `/work/fail` requires the worker-reported `attempt_id` and calls
  `Store.FailAttempt`; failure reports now include optional `failed_at` so an
  identical duplicate report can remain idempotent.
- Worker completion reports echo an assigned `attempt_id`; the old generated
  attempt ID remains only as a legacy fallback when the assignment has no
  attempt ID.
- The worker demo and summary operations now return `WorkEvidence` so report
  generation can include output, pre-state, and post-state JSON.

The original allowed production file list was too narrow for real worker
evidence. Passing evidence from execution to reporting required touching
`cmd/worker/main.go`, `cmd/worker/worker.go`, and
`cmd/worker/work_summary.go` in addition to the planned files.

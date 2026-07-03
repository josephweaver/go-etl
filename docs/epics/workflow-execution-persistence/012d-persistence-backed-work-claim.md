# 012d Persistence-backed Work Claim Endpoint

Status: proposed

## Objective

Move `/work/next` assignment onto `internal/persistence.Store` when the
workflow-execution store is configured.

The endpoint should claim one persisted queued work item transactionally through
`Store.ClaimNextWork`, decode the stored worker payload back into the existing
`model.WorkItem` transport shape, and return that payload only after the claim
transaction succeeds.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/012-controller-integration-cutover.md`
- `docs/epics/workflow-execution-persistence/012c-persistence-backed-raw-work-submission.md`
- `docs/epics/workflow-execution-persistence/007-attempt-claim-transaction.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/model/work_item.go`
- `internal/persistence/store.go`

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/main_test.go`

## Documentation Files

- `docs/epics/workflow-execution-persistence/012d-persistence-backed-work-claim.md`
- `PROJECT_STATE.md`
- `epi_ctl/20260703.md`

## Claim Behavior

When `Controller.workflowStore` is configured:

1. `/work/next` calls `Store.ClaimNextWork`.
2. The controller supplies an attempt ID, executor type, and start timestamp.
3. Empty persisted queue returns `204 No Content`.
4. A successful claim decodes `ClaimedWorkRecord.WorkItem.WorkerPayloadJSON`
   into `model.WorkItem`.
5. The decoded payload is returned as JSON to the worker.

When no workflow-execution store is configured, the existing in-memory
`pending`/`assigned` behavior remains as fallback.

## Attempt ID

This slice may use an existing random helper to create a transitional attempt
ID, such as:

```text
attempt-<random hex>
```

UUIDv7 remains the target generated identity strategy, but UUIDv7 generation is
not part of this endpoint cutover slice.

## Payload Assumption

012c stores the full existing `model.WorkItem` transport object as
`worker_payload_json` for raw work. 012d may decode that JSON directly.

If future workflow compilation stores a compact plugin payload instead, the
assignment path will need an explicit conversion layer. This slice should not
invent that final conversion.

## Acceptance Criteria

- `/work/next` keeps existing behavior when no workflow-execution store is
  configured.
- When `workflowStore` is configured, `/work/next` claims from persisted
  `queued_work` through `Store.ClaimNextWork`.
- Empty persisted queue returns `204 No Content`.
- Claimed work is not stored in `Controller.assigned`.
- A successful claim removes the queued row and creates a running row through
  the persistence transaction.
- The worker response remains the existing `model.WorkItem` JSON shape.
- Invalid persisted worker payload JSON returns a server error rather than
  assigning an untracked item.
- This slice does not change `/work/complete`, `/work/fail`, `/workflow`, or
  worker scaling behavior.
- Existing tests continue to pass.

## Out Of Scope

- Persistence-backed completion.
- Persistence-backed failure.
- Retry/requeue behavior.
- Skip/reuse behavior.
- Worker liveness or fencing.
- UUIDv7 generation.
- Full workflow submission persistence.
- Removing in-memory queue maps.
- Changing worker response schema.

## Ambiguity To Review

The existing worker completion payload includes `attempt_id` only when the
worker generates it. With persistence-backed claim, the controller now creates
the attempt ID. This slice can return the old work-item payload unchanged, but
012e must decide how workers learn and report the controller-created attempt ID.

Options for 012e include:

- add `attempt_id` to the worker assignment payload;
- include attempt metadata in a wrapper response;
- temporarily derive terminal reports by work item ID while the worker contract
  catches up.

This slice should surface that issue but not solve terminal reporting.

## Notes

- Do not use controller-side SQL.
- Do not hold `Controller.mu` for persistence-backed claim.
- Returning payload before commit is forbidden; use `Store.ClaimNextWork` as the
  transaction boundary.

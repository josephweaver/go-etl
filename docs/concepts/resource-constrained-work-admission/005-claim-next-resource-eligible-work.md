# 005 Claim Next Resource Eligible Work

Status: Ready

## Objective

Replace oldest-only work claiming with resource-aware claim admission.

A worker should receive the first queued work item, in deterministic queue order, whose resolved resource constraints all pass in Go code.

## Current State

The current claim path selects the oldest queued work item, inserts a work-item attempt, inserts `running_work`, deletes the queued row, commits, and returns the claimed payload.

That lifecycle is correct, but it does not check resource constraints and would cause invalid concurrent work for shared resources such as Python environment creation or memory capacity.

## Target State

The claim path behaves as follows:

```text
serialized claim section
  begin transaction
  read queued candidates in order
  for each candidate:
    read candidate constraint-check rows from queued_resource_constraint_checks
    evaluate all rows in Go
    if every row passes:
      insert work_item_attempts
      insert running_work
      delete queued_work
      commit
      return claimed work
  commit/rollback empty result
  return no work available
end serialized claim section
```

Work items with no resource constraints pass resource admission.

Resource-blocked work items remain queued.

If the oldest item is blocked but a later item is eligible, the later item may be claimed. Do not introduce head-of-line blocking.

## Concept Decision

Use controller-local serialization around claim evaluation in this slice. The repository currently has a single controller owner for the database; claim serialization inside that process is sufficient for the first implementation.

Do not try to encode operator evaluation into the SQL `WHERE` clause. Use the view as a read model and the Go comparator as the authority.

## Required Context

Read these files first:

- `internal/persistence/store.go`
- `cmd/controller/main.go`
- controller handler file containing `nextWorkHandler` if split from `main.go`
- `cmd/worker` files that call `/work/next`
- `internal/model/resource_constraint.go`
- `docs/concepts/resource-constrained-work-admission/README.md`

## Allowed Production Files

- `internal/persistence/store.go`
- focused helper files under `internal/persistence/`
- `cmd/controller/main.go`
- controller handler files if route handlers are split

## Allowed Test Files

- `internal/persistence/*_test.go`
- `cmd/controller/*_test.go`

## Out Of Scope

- New worker behavior.
- New worker request fields.
- Status/log changes beyond minimal error logging needed for debugging.
- Distributed multi-controller locking.
- Scheduling priorities.

## Acceptance Criteria

- If no queued work exists, `/work/next` behavior remains unchanged.
- If the oldest queued constrained item is eligible, it is claimed.
- If the oldest queued constrained item is blocked and the next queued item is eligible, the next item is claimed.
- If all queued items are blocked by resources, no work is returned and no queue/running mutation occurs.
- A work item with no resource constraints is claimable under existing dependency/queue rules.
- Claiming a constrained item atomically inserts `work_item_attempts`, inserts `running_work`, and deletes from `queued_work`.
- A second claim sees the first claim's running resource usage before evaluating the next candidate.
- Completing or failing an attempt releases the resource naturally by deleting from `running_work` through existing terminal paths.
- All six operators can affect claim eligibility through the Go evaluator.
- Existing dependency-aware workflow tests still pass.

## Notes

- Keep the worker polling contract unchanged. A worker that receives no work does not need to know whether the reason was an empty queue or resource blocking.
- If tests call the store directly, add store-level tests for resource-aware claim behavior before handler-level tests.
- Do not add resource reservations separate from `running_work` in this slice.

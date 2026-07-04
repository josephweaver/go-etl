# 012b Persistence-backed Status Read Model

Status: implemented

## Objective

Teach the controller status path to read queue/running/failed-equivalent counts
from `internal/persistence.Store` when the workflow-execution store is
configured.

This slice changes status reads only. It does not move assignment, completion,
failure, workflow submission, worker scaling, or skip/reuse behavior onto the
new store.

## Required Context

Read these files first:

- `docs/concepts/workflow-execution-persistence/012-controller-integration-cutover.md`
- `docs/concepts/workflow-execution-persistence/011-restart-reconstruction-queries.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/persistence/store.go`
- `internal/model/work_item.go`

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/main_test.go`

## Documentation Files

- `docs/concepts/workflow-execution-persistence/012b-persistence-backed-status.md`
- `PROJECT_STATE.md`
- `epi_ctl/20260703.md`

## Status Mapping

The existing HTTP status shape is:

```go
type ControllerStatus struct {
    Pending  int
    Assigned int
    Failed   int
    PendingReuseCandidates int
    Attempts int
    AttemptVariables int
}
```

For this slice, when `Controller.workflowStore` is configured:

```text
Pending  = persisted queued_work count
Assigned = persisted running_work count
Failed   = persisted failed_work count
```

The status response keeps the existing JSON field names so clients and tests do
not change yet.

## Acceptance Criteria

- `/status` continues to work when no workflow-execution store is configured.
- When `workflowStore` is configured, `/status` derives pending/assigned/failed
  counts from persistence, not from `Controller.pending`, `Controller.assigned`,
  or `Controller.failed`.
- Persisted counts are derived from placement and terminal tables through store
  query methods.
- The old in-memory behavior remains as fallback for tests and legacy paths
  that construct controllers without a store.
- This slice does not change `/work/next`, `/work/complete`, `/work/fail`, or
  `/workflow` behavior.
- This slice does not change worker-scaling behavior.
- Existing tests continue to pass.

## Out Of Scope

- Adding new status JSON fields.
- Changing client/demo status interpretation.
- Counting completed attempts in status.
- Replacing old ledger attempt/attempt-variable count fields.
- Reuse-candidate calculation from persistence.
- Assignment, completion, failure, or workflow submission cutover.
- Removing in-memory queues.
- Removing the old ledger helper code.

## Ambiguity To Review

`ControllerStatus.Failed` currently means in-memory failed work. In the
workflow-execution store, failed terminal attempts are attempt outcomes, not
necessarily final logical work-item failures once retry policy exists. For this
temporary read model, `Failed` maps to `failed_work` because retry/requeue
policy is not implemented yet. Later status design should separate failed
attempts from permanently failed logical work.

`Attempts` and `AttemptVariables` still refer to the old ledger counts. This
slice should leave them alone or return zero when the old ledger is absent; a
later status-contract slice should replace them with workflow-execution counts
or remove them.

## Notes

- Prefer a small internal helper such as `controllerStatus(ctx)` so the handler
  has one place to choose persistence-backed versus legacy in-memory reads.
- Do not read SQLite directly from controller code. Use `persistence.Store`
  methods.

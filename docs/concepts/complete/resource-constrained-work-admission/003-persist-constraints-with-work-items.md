# 003 Persist Constraints With Work Items

Status: Complete

## Objective

Persist resolved resource constraints atomically with work-item insertion and queueing for raw submissions, initial workflow stages, and just-in-time dependency stage activation.

## Current State

Work items are inserted into `work_items` and then queued through `queued_work`. Dependency-aware workflows compile and queue only the currently ready stage, then activate downstream stages just in time after predecessor completion.

Resource constraints may now be resolved in memory, but they are not yet stored beside work items.

## Target State

Every path that creates queueable work items can also persist zero or more resolved resource constraints for those items.

Required insertion behavior:

```text
begin transaction
insert work_items
insert work_item_resource_constraints
insert queued_work
commit
```

For stage activation:

```text
begin transaction
complete predecessor stage
insert newly activated work_items
insert newly activated resource constraints
insert newly activated queued_work
commit
```

A failure to persist constraints must roll back the associated work insertion/queueing operation.

## Concept Decision

Resource constraints are part of the controller admission state for a work item. They should be stored in the same persistence layer that owns work item lifecycle facts.

Do not persist constraint expressions. Persist only resolved facts.

Do not make resource constraints a dependency-state membership concern. Dependency membership answers “which step/stage owns this work item?” Resource constraints answer “may this queued work item be claimed now?”

## Required Context

Read these files first:

- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`
- `cmd/controller/main.go`
- `cmd/controller/workflow_dependency_store.go`
- files that define `persistenceRecordsFromCompiledStageResults`
- files that define `activateNextReadyWorkflowStage`
- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/complete/resource-constrained-work-admission/README.md`

## Allowed Production Files

- `internal/persistence/store.go`
- `internal/persistence/*resource*.go` if new helper files are useful
- `cmd/controller/main.go`
- `cmd/controller/workflow_dependency_store.go`
- focused controller helper files involved in compiled work item admission

## Allowed Test Files

- `internal/persistence/*_test.go`
- `cmd/controller/*_test.go`

## Out Of Scope

- Changing the claim algorithm.
- Evaluating operator predicates.
- Status/log changes.
- Smoke scripts.

## Acceptance Criteria

- Raw work submission without constraints still persists and queues as before.
- Raw work submission with constraints persists the constraints if raw wrapper support exists from slice 002.
- Initial workflow stage submission persists resource constraints for all compiled constrained work items.
- Just-in-time downstream stage activation persists resource constraints for newly activated work items.
- Constraint insert failures roll back associated work-item/queue mutations.
- Work items with zero constraints are still accepted.
- Constraints are not stored in `completed_work.output_json`, dependency output JSON, or worker output evidence.
- Existing dependency-aware workflow tests still pass.

## Notes

- If `InsertWorkItems` currently accepts only `[]WorkItemRecord`, consider adding a new request type rather than widening every call site ambiguously.
- Prefer names such as `InsertWorkItemsWithConstraints` or `QueueWorkItemsWithConstraints` only if they make transaction ownership clearer.
- Keep idempotency behavior consistent with existing work-item insertion behavior.

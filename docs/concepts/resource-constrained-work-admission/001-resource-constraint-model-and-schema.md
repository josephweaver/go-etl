# 001 Resource Constraint Model And Schema

Status: Ready

## Objective

Add the resolved resource-constraint model, operator enum, SQLite table, indexes, and candidate-check view needed for controller-owned resource admission.

This slice introduces persistence and validation only. It must not change work claiming behavior yet.

## Current State

The persistence schema has `work_items`, `queued_work`, `running_work`, `completed_work`, and `failed_work`, but no table for resource constraints.

The current claim path selects the oldest queued work item and moves it to running inside one transaction. There is no resource-admission check.

## Target State

The repository has a resolved resource-constraint model equivalent to:

```text
work_item_id
constraint_index
resource_key
requested_units
operator
target_units
created_at
```

The SQLite schema has a new table:

```sql
CREATE TABLE work_item_resource_constraints (
    work_item_id TEXT NOT NULL,
    constraint_index INTEGER NOT NULL,
    resource_key TEXT NOT NULL,
    requested_units INTEGER NOT NULL,
    operator TEXT NOT NULL CHECK (operator IN ('=', '!=', '<', '>', '<=', '>=')),
    target_units INTEGER NOT NULL,
    created_at TEXT NOT NULL,

    PRIMARY KEY (work_item_id, constraint_index),
    UNIQUE (work_item_id, resource_key),
    FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),

    CHECK (constraint_index >= 0),
    CHECK (resource_key <> ''),
    CHECK (requested_units > 0),
    CHECK (target_units >= 0)
);
```

Add indexes needed by resource-key aggregation and work-item lookup:

```sql
CREATE INDEX idx_work_item_resource_constraints_resource_key
    ON work_item_resource_constraints(resource_key);

CREATE INDEX idx_work_item_resource_constraints_work_item_id
    ON work_item_resource_constraints(work_item_id);
```

Add a view equivalent to:

```sql
CREATE VIEW queued_resource_constraint_checks AS
SELECT
    q.work_item_id,
    q.queued_at,
    c.constraint_index,
    c.resource_key,
    COALESCE((
        SELECT SUM(r.requested_units)
        FROM running_work rw
        JOIN work_item_resource_constraints r
            ON r.work_item_id = rw.work_item_id
        WHERE r.resource_key = c.resource_key
    ), 0) AS total_units,
    c.requested_units,
    c.operator,
    c.target_units
FROM queued_work q
JOIN work_item_resource_constraints c
    ON c.work_item_id = q.work_item_id;
```

The view must expose facts only. It must not decide whether a candidate passes.

## Concept Decision

Resource constraints are persisted as already-resolved facts. This slice does not store expressions such as `${step.memory_allocated_mib}` in the scheduling table.

The table uses integer units only. Memory should use `memory-mib` or another exact integer unit rather than floating-point GB.

The `UNIQUE (work_item_id, resource_key)` constraint intentionally rejects duplicate constraints for the same resource key on the same work item. That avoids double-counting ambiguity in the running total.

## Required Context

Read these files first:

- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/store.go`
- `internal/persistence/validation.go` if present
- `internal/persistence/*_test.go`
- `internal/model/work_item.go`
- `docs/concepts/resource-constrained-work-admission/README.md`

Do not read unrelated worker execution files unless tests require them.

## Allowed Production Files

- `internal/model/work_item.go`
- `internal/model/resource_constraint.go` if a new model file is cleaner
- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/store.go`
- `internal/persistence/validation.go` if present
- focused persistence helper files under `internal/persistence/`

## Allowed Test Files

- `internal/model/*_test.go`
- `internal/persistence/*_test.go`

## Out Of Scope

- Workflow JSON declaration support.
- Resolver integration.
- Persisting actual constraint rows during work insertion.
- Changing `/work/next` behavior.
- Status/log changes.
- Smoke scripts.

## Acceptance Criteria

- `work_item_resource_constraints` exists in the SQLite schema.
- `queued_resource_constraint_checks` exists and returns candidate-check rows for queued constrained work.
- Schema validation rejects unsupported operators.
- Schema validation rejects empty resource keys.
- Schema validation rejects non-positive `requested_units`.
- Schema validation rejects negative `target_units`.
- Duplicate `resource_key` rows for the same `work_item_id` are rejected.
- Work items without resource constraints remain representable.
- Existing tests for work-item persistence and queueing still pass.

## Notes

- If the store schema version must be bumped, do so explicitly and update tests that assert schema version.
- If the repository currently recreates development schemas automatically, keep that behavior consistent with existing conventions.
- Do not create mutable resource-counter tables in this slice.

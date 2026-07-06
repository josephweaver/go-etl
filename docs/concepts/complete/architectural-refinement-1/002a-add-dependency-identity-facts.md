# Architectural Refinement 1 â€” Revised 002

Shared architecture:

```text
Persist minimal dependency recovery facts.
Derive current workflow state from SQL views or focused queries.
Avoid duplicated mutable state that must be synchronized.
```

Implementation cadence:

```text
1. gpt5.4-mini refinement: implementation plan only
2. gpt5.3-codex-spark: implement approved plan
```

Use:

```text
EC-3 / operational slice / files(4)+test+doc
CSx(IR)x
```

# 002a Add Dependency Identity Facts

Status: Complete

## Implementation Notes

- Added persisted identity/membership facts in `workflow_dependency_steps` and `workflow_dependency_work_items`.
- Added store and controller persistence paths to write these facts while keeping JSON as authoritative source for execution state.
- No derived mutable status was added in this slice.

## Objective

Persist the minimal dependency identity facts needed to reconstruct logical workflow structure:

```text
stage -> step -> concrete work items
```

This slice should avoid persisting mutable derived state such as `queued`, `active`, `completed`, or `failed`.

## Target Problem

Current dependency-aware state is embedded in:

```text
workflow_instances.submission_context_json.dependency_state
```

That JSON stores stage/step/work-item membership. Some of that is not derived from the execution ledger.

The important non-derived facts are:

```text
which logical step exists
which stage owns the step
which concrete work item belongs to the step
which order work-item outputs should use
```

## Target Tables

### workflow_dependency_steps

Suggested columns:

| Column | Purpose |
|---|---|
| run_id | Owning workflow run. |
| stage_index | Parent stage index. |
| step_index | Logical step index. |
| step_id | Logical workflow step ID. |
| parallel_with | Parallel/stage relationship metadata if needed. |
| created_at | Admission/creation timestamp. |

Suggested constraints:

```sql
PRIMARY KEY (run_id, step_index)
UNIQUE (run_id, stage_index, step_id)
FOREIGN KEY (run_id) REFERENCES workflow_instances(run_id)
```

### workflow_dependency_work_items

Suggested columns:

| Column | Purpose |
|---|---|
| run_id | Owning workflow run. |
| stage_index | Parent stage index. |
| step_index | Parent step index. |
| work_item_id | Concrete persisted work item ID. |
| work_item_index | Deterministic output order within step/stage. |
| created_at | Creation timestamp. |

Suggested constraints:

```sql
PRIMARY KEY (run_id, work_item_id)
UNIQUE (run_id, step_index, work_item_index)
FOREIGN KEY (run_id, step_index) REFERENCES workflow_dependency_steps(run_id, step_index)
FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id)
```

## Design Note

A separate `workflow_dependency_stages` table may not be necessary if:

```text
workflow_stages already stores stage_index
workflow_dependency_steps stores stage_index
```

5.4-mini should decide whether a stage identity table is actually needed.

## Scope

Do:
- add minimal identity/membership schema;
- add persistence structs/methods if needed;
- add tests proving insert/list/order/uniqueness;
- preserve existing behavior.

Do not:
- persist derived status/state;
- move controller reads yet;
- stop JSON writes yet;
- change workflow semantics.

## Required Context

Read:

```text
internal/persistence/db_adapter_sqlite.go
internal/persistence/store.go
internal/persistence/store_test.go
internal/model/workflow_dependency.go
cmd/controller/workflow_dependency_store.go
PROJECT_STATE.md
```

## Acceptance Criteria

- Logical steps can be persisted and listed deterministically.
- Work-item memberships can be persisted and listed deterministically.
- No duplicated mutable status/state columns are introduced unless 5.4-mini explicitly justifies them.
- Existing tests pass.
- PROJECT_STATE.md is updated.
- This OS is marked implemented.

## 5.4-mini Refinement Prompt

```text
Please read operational slice (OS):

docs\concepts\archtectural-refinement-1\002a-add-dependency-identity-facts.md

Analyze the operational slice and produce a concise implementation plan only.

Use "EC-3 / operational slice / files(4)+test+doc" with CSx(IR)x Cadence.

Focus on minimal persisted facts, not synchronized derived state.

Do not modify files yet.

End with READY FOR SPARK IMPLEMENTATION if safe.
```

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

# 002f Add Indexes And Cleanup Compatibility

Status: Complete

## Objective

Add query-support indexes for the derived recovery model, then clean up leftover compatibility code and documentation.

## Index Direction

Use indexes that support bottom-up reconstruction from base facts.

Examples:

```sql
CREATE INDEX idx_work_items_run_stage_order
ON work_items(run_id, stage_index, work_item_index);

CREATE INDEX idx_dependency_work_items_run_step_order
ON workflow_dependency_work_items(run_id, step_index, work_item_index);

CREATE INDEX idx_completed_work_item_completed_at_desc
ON completed_work(work_item_id, completed_at DESC);

CREATE INDEX idx_failed_work_item_failed_at_desc
ON failed_work(work_item_id, failed_at DESC);

CREATE INDEX idx_running_work_item
ON running_work(work_item_id);

CREATE INDEX idx_queued_work_queued_at
ON queued_work(queued_at, work_item_id);
```

5.4-mini should refine exact indexes based on real query/view plans.

## Scope

Do:
- add indexes that directly support derived views/query helpers;
- clean up dead JSON dependency-state compatibility paths if safe;
- update docs;
- keep behavior unchanged.

Do not:
- add speculative indexes not tied to actual queries;
- redesign scheduler;
- remove useful in-memory DTOs.

## Required Context

Read:

```text
internal/persistence/db_adapter_sqlite.go
internal/persistence/store.go
cmd/controller/workflow_dependency_store.go
PROJECT_STATE.md
docs/concepts/archtectural-refinement-1/002a-add-dependency-identity-facts.md
docs/concepts/archtectural-refinement-1/002b-add-derived-status-views.md
docs/concepts/archtectural-refinement-1/002c-persist-irreducible-logical-outputs.md
docs/concepts/archtectural-refinement-1/002d-switch-controller-reads-to-derived-recovery-model.md
docs/concepts/archtectural-refinement-1/002e-stop-mutating-submission-context-for-dependency-state.md
```

## Acceptance Criteria

- Derived status queries have supporting indexes.
- No speculative/unjustified index bloat.
- Dead compatibility paths are removed or explicitly documented.
- Documentation reflects the revised architecture.
- Existing tests pass.
- PROJECT_STATE.md is updated.
- This OS is marked implemented.

## Implementation Notes

Implemented indexes are limited to current derived-recovery and status query
shapes:

- `work_items(run_id, stage_index, work_item_id)` supports run/stage status
  joins from work items to queue/running/terminal tables.
- `workflow_dependency_steps(run_id, stage_index, step_index)` supports
  deterministic dependency-step reconstruction for one run.
- `workflow_dependency_work_items(run_id, stage_index, step_index,
  work_item_index, work_item_id)` supports deterministic membership
  reconstruction for one dependency step.
- `queued_work(queued_at, work_item_id)` supports global claim/list queue
  ordering.
- `running_work(started_at, attempt_id)` supports running-work list ordering.
- `completed_work(work_item_id, completed_at, attempt_id)` and
  `failed_work(work_item_id, failed_at, attempt_id)` support terminal-state
  joins from run-scoped work items.

Indexes are applied with `CREATE INDEX IF NOT EXISTS` during SQLite store
initialization so existing supported databases receive them without a table
rewrite or schema-version bump.

Compatibility cleanup result: legacy dependency-state reads and the narrow
failed-state compatibility write remain intentionally. Removing them is not safe
yet because failed dependency state still carries failure evidence that does not
have a dedicated durable failure fact.

## 5.4-mini Refinement Prompt

```text
Please read operational slice (OS):

docs\concepts\archtectural-refinement-1\002f-add-indexes-and-cleanup-compatibility.md

Analyze the operational slice and produce a concise implementation plan only.

Use "EC-3 / operational slice / files(4)+test+doc" with CSx(IR)x Cadence.

Focus on indexes and cleanup after the derived recovery model is working.

Do not modify files yet.

End with READY FOR SPARK IMPLEMENTATION if safe.
```

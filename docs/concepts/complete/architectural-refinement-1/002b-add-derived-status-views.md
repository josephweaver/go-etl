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

# 002b Add Derived Status Views

Status: Complete

## Objective

Create SQL views or focused query helpers that derive current dependency-aware status from persisted base facts.

The goal is to answer status questions without storing duplicated mutable state.

## Derived Concepts

Derive:

```text
step total work items
step queued/running/completed/failed counts
step status
stage total steps
stage completed/failed/active counts
stage status
run status/progress
ordered step outputs
```

## Candidate Views

Names may be refined:

```text
workflow_step_status_v
workflow_stage_status_v
workflow_run_status_v
workflow_step_ordered_outputs_v
```

If Go query helpers are safer than SQLite views for now, 5.4-mini may choose query helpers, but the concept should stay the same: derived, not duplicated.

## Step Status Derivation

Suggested rule:

```text
failed:
  any member work item is in failed_work

completed:
  all member work items have completed_work
  OR an irreducible step-output fact says an empty/logical step completed

active:
  any member work item is running
  OR some member work item is completed while others remain non-terminal

ready:
  all member work items are queued and none are running/completed/failed

blocked:
  no executable work exists and no irreducible completion fact exists
```

5.4-mini should refine this against actual dependency-aware workflow behavior.

## Stage Status Derivation

Suggested rule:

```text
failed:
  any step failed

completed:
  all steps completed

active:
  any step active or completed, but not all completed

ready:
  at least one runnable step has queued work and no active/completed work yet

blocked:
  no runnable/active/completed step exists
```

## Scope

Do:
- add views or query helpers;
- add tests for derived status;
- add indexes only if required by view performance;
- preserve existing controller behavior.

Do not:
- switch controller reads yet;
- persist status/state columns;
- remove JSON dependency state yet.

## Required Context

Read:

```text
internal/persistence/db_adapter_sqlite.go
internal/persistence/store.go
internal/persistence/store_test.go
docs/concepts/archtectural-refinement-1/002a-add-dependency-identity-facts.md
```

## Acceptance Criteria

- Step status can be derived from base facts.
- Stage status can be derived from step status.
- Run status/progress can be derived from stage/work counts.
- Views/query helpers do not require duplicated mutable dependency state.
- Tests cover queued, running, completed, failed, and mixed states.
- Existing tests pass.
- PROJECT_STATE.md is updated.
- This OS is marked implemented.

## 5.4-mini Refinement Prompt

```text
Please read operational slice (OS):

docs\concepts\archtectural-refinement-1\002b-add-derived-status-views.md

Analyze the operational slice and produce a concise implementation plan only.

Use "EC-3 / operational slice / files(4)+test+doc" with CSx(IR)x Cadence.

Focus on deriving state from base facts instead of persisting synchronized state.

Do not modify files yet.

End with READY FOR SPARK IMPLEMENTATION if safe.
```

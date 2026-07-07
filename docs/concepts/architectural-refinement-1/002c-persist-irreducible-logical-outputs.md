# Architectural Refinement 1 Ã¢â‚¬â€ Revised 002

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

# 002c Persist Irreducible Logical Outputs

Status: Complete

## Objective

Persist logical step output facts that cannot be safely reconstructed from `completed_work` alone.

This includes:

```text
empty fan-out output []
aggregate logical step output hash/bytes/pruned metadata
possibly skipped/reused logical-output decisions
```

## Why This Table Is Needed

Most current state should be derived.

But some facts are controller decisions or logical values:

```text
A fan-out step produced zero work items and therefore output []
A step aggregate was pruned, but hash/byte metadata must remain
A skipped logical output was accepted through reuse
```

These are not always reconstructable from `completed_work`.

## Target Table

Suggested name:

```text
workflow_step_output_facts
```

Suggested columns:

| Column | Purpose |
|---|---|
| run_id | Owning run. |
| step_index | Logical step. |
| output_json | Optional bounded logical output JSON. |
| output_json_sha256 | Hash of canonical logical output. |
| output_json_bytes | Size before pruning. |
| output_json_pruned | Whether JSON was pruned. |
| output_kind | `aggregate`, `empty_fanout`, `skipped`, etc. |
| created_at | Creation timestamp. |
| updated_at | Last update timestamp. |

Suggested key:

```sql
PRIMARY KEY (run_id, step_index)
FOREIGN KEY (run_id, step_index) REFERENCES workflow_dependency_steps(run_id, step_index)
```

## Scope

Do:
- add table or store support for irreducible step output facts;
- write empty fan-out output `[]` here;
- preserve OS 007 bounded output/pruning behavior;
- add tests.

Do not:
- duplicate every completed work output here;
- persist derived step status here;
- change worker behavior.

## Required Context

Read:

```text
cmd/controller/workflow_outputs.go
cmd/controller/workflow_dependency_store.go
cmd/controller/workflow_stage_queue.go
internal/persistence/store.go
internal/model/workflow_dependency.go
docs/concepts/dependency-aware-workflows/007-capture-typed-step-outputs.md
docs/concepts/dependency-aware-workflows/009-handle-empty-fanout-and-auto-advance.md
```

## Acceptance Criteria

- Empty fan-out logical output `[]` can be persisted without synthetic work item or attempt.
- Aggregated step output metadata can be persisted.
- Pruning retains hash, byte count, and pruned flag.
- Completed work outputs remain in `completed_work`; this table stores logical step output facts only.
- Existing output-resolution behavior is preserved.
- Tests cover empty output and pruned aggregate output.
- PROJECT_STATE.md is updated.
- This OS is marked implemented.

## 5.4-mini Refinement Prompt

```text
Please read operational slice (OS):

docs\concepts\archtectural-refinement-1\002c-persist-irreducible-logical-outputs.md

Analyze the operational slice and produce a concise implementation plan only.

Use "EC-3 / operational slice / files(4)+test+doc" with CSx(IR)x Cadence.

Focus on outputs that cannot be reconstructed from completed_work alone.

Do not modify files yet.

End with READY FOR SPARK IMPLEMENTATION if safe.
```

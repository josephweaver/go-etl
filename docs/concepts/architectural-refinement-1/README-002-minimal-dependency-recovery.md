# 002 Persist Minimal Dependency Recovery Facts And Derive Current State With SQL Views

Status: Revised architecture direction

## Purpose

This replaces the earlier "persist dependency-aware recovery state as first-class synchronized tables" direction.

The revised goal is:

```text
Persist minimal dependency recovery facts.
Derive current stage/step/run state from base facts using SQL views or focused queries.
Avoid duplicating mutable state that can be reconstructed.
```

## Core Rule

```text
Base facts are persisted.
Derived state is queried.
Controller decisions are persisted only when they are not safely derivable.
```

## Why This Revision Exists

The previous design proposed persisted state tables such as:

```text
workflow_dependency_stages.state
workflow_dependency_steps.state
workflow_dependency_work_items.state
```

Those columns duplicate facts already represented by:

```text
work_items
queued_work
running_work
work_item_attempts
completed_work
failed_work
```

Duplicated mutable state creates synchronization risk:

```text
completed_work says work item completed
but dependency membership state says queued
```

This revision reduces that risk by making durable execution facts authoritative and deriving current state from them.

## Design Direction

### Persisted Base Facts

These remain authoritative:

```text
projects
workflows
workflow_instances
workflow_stages
work_items
work_item_attempts
queued_work
running_work
completed_work
failed_work
workers
```

### Persisted Minimal Dependency Facts

Persist only facts needed to reconstruct dependency-aware logical structure and non-derivable decisions:

```text
workflow_dependency_steps
workflow_dependency_work_items
workflow_step_output_facts
workflow_stage_activation_facts   optional, only if needed
```

Exact names may be refined by 5.4-mini.

### Derived Views / Queries

Use SQL views or focused query helpers for:

```text
workflow_step_status
workflow_stage_status
workflow_run_status
ordered step outputs
completion checks
```

## Revised 002 Slice Plan

| OS | Title | Purpose |
|---|---|---|
| 002a | Add Dependency Identity Facts | Persist logical step identity and work-item membership/order facts. |
| 002b | Add Derived Status Views | Derive step/stage/run status from base fact tables. |
| 002c | Persist Irreducible Logical Outputs | Persist outputs that cannot be derived from completed work alone, including empty fan-out `[]` and pruned aggregate metadata. |
| 002d | Switch Controller Reads To Derived Recovery Model | Reconstruct dependency state from facts/views instead of JSON blob state. |
| 002e | Stop Mutating Submission Context For Dependency State | Keep `submission_context_json` for admission/source context only. |
| 002f | Add Indexes And Cleanup Compatibility | Add query-support indexes, remove compatibility paths, and update documentation. |

## Mental Model

```text
Provenance / base facts:
  What happened?

Minimal dependency facts:
  What logical workflow structure or controller decision cannot be derived?

Views / derived queries:
  What is true right now?

Submission context:
  What was submitted/admitted?
```

## Implementation Technique

Each OS is intended to go through:

```text
1. gpt5.4-mini: refine implementation plan only
2. gpt5.3-codex-spark: implement approved plan
```

Use:

```text
EC-3 / operational slice / files(4)+test+doc
CSx(IR)x Cadence
```

If a slice appears too large, split it again before Spark implementation.

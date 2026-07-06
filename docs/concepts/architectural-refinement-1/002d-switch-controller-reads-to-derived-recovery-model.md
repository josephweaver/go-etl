# Architectural Refinement 1 — Revised 002

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

# 002d Switch Controller Reads To Derived Recovery Model

Status: Implemented

## Objective

Switch controller dependency-state reads from JSON blob state to a reconstructed model built from:

```text
minimal dependency identity facts
base execution tables
irreducible logical output facts
derived SQL views/query helpers
```

The controller may still use `model.WorkflowDependencyPlan` as an in-memory DTO, but the source of truth should no longer be:

```text
workflow_instances.submission_context_json.dependency_state
```

## Target State

Controller helpers such as:

```text
getWorkflowDependencyState
ListWorkflowStages
ListWorkflowSteps
ReadStageState
ReadStepState
workflow.step resolver scope
```

should reconstruct current state from facts/views.

## Scope

Do:
- reconstruct stage/step/membership DTOs from facts/views;
- preserve deterministic ordering;
- keep existing public behavior;
- add recovery tests using a fresh controller over the same store.

Do not:
- stop JSON writes yet;
- delete compatibility code yet;
- change dependency workflow semantics.

## Required Context

Read:

```text
cmd/controller/workflow_dependency_store.go
cmd/controller/workflow_outputs.go
cmd/controller/workflow_stage_queue.go
internal/persistence/store.go
internal/model/workflow_dependency.go
docs/concepts/archtectural-refinement-1/002a-add-dependency-identity-facts.md
docs/concepts/archtectural-refinement-1/002b-add-derived-status-views.md
docs/concepts/archtectural-refinement-1/002c-persist-irreducible-logical-outputs.md
```

## Acceptance Criteria

- Controller can reconstruct dependency state without relying on JSON blob dependency state.
- `workflow.step` resolution still works.
- Stage/step listing remains deterministic.
- Recovery test proves fresh controller/store readback.
- Existing tests pass.
- PROJECT_STATE.md is updated.
- This OS is marked implemented.

## 5.4-mini Refinement Prompt

```text
Please read operational slice (OS):

docs\concepts\archtectural-refinement-1\002d-switch-controller-reads-to-derived-recovery-model.md

Analyze the operational slice and produce a concise implementation plan only.

Use "EC-3 / operational slice / files(4)+test+doc" with CSx(IR)x Cadence.

Focus on read-source migration to derived facts/views.

Do not modify files yet.

End with READY FOR SPARK IMPLEMENTATION if safe.
```

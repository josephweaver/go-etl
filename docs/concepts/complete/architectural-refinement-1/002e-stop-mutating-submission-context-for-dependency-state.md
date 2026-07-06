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

# 002e Stop Mutating Submission Context For Dependency State

Status: Complete

## Objective

Stop writing routine dependency runtime state into:

```text
workflow_instances.submission_context_json
```

After this slice, submission context should hold admission/source/submission metadata, not mutable scheduler state.

## Target State

```text
submission_context_json:
  source/admission context
  submitted variables
  cache/source references

minimal dependency tables:
  logical step identity
  work item membership/order
  irreducible logical outputs

views/query helpers:
  current state
```

## Scope

Do:
- remove or narrow `setWorkflowDependencyState`;
- stop mutating dependency state inside submission context;
- update tests that expected dependency state in JSON;
- preserve admission/source context.

Do not:
- delete historical compatibility read paths unless safe;
- change workflow behavior;
- change submission API.

## Required Context

Read:

```text
cmd/controller/workflow_dependency_store.go
cmd/controller/main.go
internal/persistence/store.go
docs/concepts/archtectural-refinement-1/002d-switch-controller-reads-to-derived-recovery-model.md
```

Search for:

```text
DependencyState
SubmissionContextJSON
UpdateWorkflowRunSubmissionContext
setWorkflowDependencyState
workflowRunSubmissionContext
```

## Acceptance Criteria

- Routine dependency mutations no longer update submission context.
- Submission context remains valid JSON and keeps admission/source metadata.
- Dependency reads still work from facts/views.
- Tests verify dependency mutations do not rewrite submission context.
- Existing tests pass.
- PROJECT_STATE.md is updated.
- This OS is marked implemented.

## 5.4-mini Refinement Prompt

```text
Please read operational slice (OS):

docs\concepts\archtectural-refinement-1\002e-stop-mutating-submission-context-for-dependency-state.md

Analyze the operational slice and produce a concise implementation plan only.

Use "EC-3 / operational slice / files(4)+test+doc" with CSx(IR)x Cadence.

Do not modify files yet.

End with READY FOR SPARK IMPLEMENTATION if safe.
```

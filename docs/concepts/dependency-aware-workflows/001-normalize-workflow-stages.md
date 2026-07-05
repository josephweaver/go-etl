# 001 Normalize Workflow Stages

Status: Ready

## Objective

Add stage normalization to `internal/workflow` so a workflow definition can be converted into ordered dependency stages before any work items are queued.

## Current State

`internal/workflow.Workflow` contains a flat `Steps []Step` list. `CompileWorkflowResult` currently iterates over every step and compiles all generated work items in workflow-definition order.

`workflow.Step` has an `ID` and a `FanOut` compiler. It does not yet have a `parallel_with` declaration or any concept of a dependency stage.

The existing workflow compiler validates duplicate step IDs and duplicate generated work-item IDs only while compiling the full workflow.

## Target State

`internal/workflow` exposes a small stage-normalization concept, such as `NormalizeStages(workflow Workflow) (WorkflowPlan, error)`.

The normalized plan should contain enough information for later slices to compile and track one stage at a time:

```text
WorkflowPlan
  WorkflowID
  StepCount
  Stages []WorkflowStage

WorkflowStage
  Index
  ParallelWith label, if any
  Steps []WorkflowStageStep

WorkflowStageStep
  StageIndex
  StepIndex in workflow-definition order
  StepID
  Step definition or reference to it
```

`workflow.Step` accepts an optional `ParallelWith string` field serialized as `parallel_with`.

Normalization rules:

- An untagged step forms its own stage.
- A contiguous run of adjacent steps with the same non-empty `parallel_with` label forms one stage.
- A `parallel_with` label is workflow-local and controls concurrency only.
- A closed label cannot be reopened later in the workflow.
- Empty or whitespace-only `parallel_with` values are treated as untagged or rejected consistently; prefer trimming and treating empty as untagged.
- Duplicate step IDs remain invalid.
- A workflow with no steps is invalid unless the existing compiler already explicitly permits it; prefer rejecting it for this concept.

## Concept Decision

This slice adds a new workflow-planning concept. A new file such as `internal/workflow/stage.go` is justified because stage normalization has its own types, validation rules, and tests independent of fan-out compilation.

Keep normalization in `internal/workflow`. Do not put dependency-stage parsing in `cmd/controller`.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `internal/workflow/workflow.go`
- `internal/workflow/step.go`
- `internal/workflow/workflow_test.go`
- `internal/workflow/step_test.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `internal/workflow/stage.go`
- `internal/workflow/workflow.go`
- `internal/workflow/step.go`

## Allowed Test Files

- `internal/workflow/stage_test.go`
- `internal/workflow/workflow_test.go`
- `internal/workflow/step_test.go`

## Out Of Scope

- Controller submission behavior.
- Queue mutation.
- Work-item assignment.
- Stage persistence.
- JIT compilation.
- Output capture.
- Status APIs.
- Observability.
- Resource constraints.
- Arbitrary `depends_on` edges.

## Acceptance Criteria

- `workflow.Step` has an optional `parallel_with` JSON field.
- A workflow with only untagged steps normalizes into one stage per step.
- Adjacent steps with the same non-empty `parallel_with` normalize into one stage.
- A step after a parallel group normalizes into a later dependent stage.
- A second parallel group with a different label normalizes into its own later stage.
- Reusing a closed `parallel_with` label returns a validation error.
- Duplicate step IDs still return a validation error.
- Stage and step indexes are zero-based and preserve workflow-definition order.
- Normalization does not compile work items.
- Existing workflow compile tests still pass or are adjusted only for the new field.

## Notes

- `parallel_with` is not a reference to another step.
- Do not infer dependencies from work-item IDs or output filenames.
- Keep the public semantics stage-based even if the internal type names differ.

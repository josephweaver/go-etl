# 001 Normalize Workflow Stages

Status: Implemented on visible branch — preserve as regression checklist

## Objective

Preserve and verify the completed stage-normalization work in `internal/workflow` so later slices can consume one normalized workflow plan without reimplementing grouping rules.

## Current State

This slice is visible as implemented on `concept/dependency-aware-workflows`.

Expected completed artifacts include:

- `workflow.Step` supports optional `parallel_with` / `ParallelWith` metadata.
- `internal/workflow` exposes a normalized plan concept such as `WorkflowPlan`, `WorkflowStage`, and `WorkflowStageStep`.
- `NormalizeStages` or equivalent validates duplicate step IDs, empty workflows, contiguous `parallel_with` groups, and rejected reopened labels.
- Tests cover sequential workflows, valid parallel groups, and invalid non-contiguous label reuse.

If the working copy does not contain these artifacts, finish 001 before continuing 004 or later slices.

## Target State

No new target behavior is expected beyond preserving the completed 001 contract.

Later slices should treat the normalized plan as the only source of dependency-stage structure. They should not infer stage membership from queue order, work-item IDs, or `parallel_with` labels at controller time.

## Concept Decision

Keep stage normalization in `internal/workflow`.

Do not move stage parsing into `cmd/controller`, and do not create a second normalizer for later slices. If helper names differ from this document, update references in the implementation report rather than duplicating behavior.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `internal/workflow/workflow.go`
- `internal/workflow/step.go`
- `internal/workflow/workflow_test.go`
- `internal/workflow/step_test.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

No production changes are expected when verifying this completed slice.

Only modify these files if review finds a narrow 001 regression:

- `internal/workflow/stage.go`
- `internal/workflow/workflow.go`
- `internal/workflow/step.go`

## Allowed Test Files

No test changes are expected unless verification reveals a missing regression case.

Allowed verification-test files:

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

## Verification Criteria

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

# Codex Amendment: OS 007 Must Not Use `workflow_stages.output_json` as Canonical Output

Apply this amendment to the in-flight OS 007 implementation.

## Goal

Keep OS 007 functional behavior, but prevent accidental use of stage-level `output_json` as the source of truth for `workflow.step[index]`.

## Required Behavior

1. `workflow.step[index]` is backed by logical **step** output, not stage output.
2. Store completed logical output on dependency step state.
3. Do not write logical step output into `workflow_stages.output_json`.
4. Do not read `workflow_stages.output_json` when constructing generated `workflow.step` resolver scope.
5. Leave `workflow_stages.output_json` null/unused for dependency-aware workflow output semantics.
6. Do not create a synthetic stage output object or array to hold multiple step outputs.

## Rationale

A dependency stage may contain multiple logical workflow steps when contiguous steps share the same `parallel_with` label.

Example:

```text
stage 0:
  step 0: parallel_with = "A"
  step 1: parallel_with = "A"

stage 1:
  step 2: references workflow.step[0] and workflow.step[1]
```

One stage-level `output_json` cannot safely represent both `workflow.step[0]` and `workflow.step[1]` without an undocumented wrapper shape. The public contract is explicitly step-indexed.

## Implementation Instructions

### 1. Dependency Model

In `internal/model/workflow_dependency.go`, keep output fields on logical step state, not stage state:

```go
type WorkflowDependencyStep struct {
    // existing fields...
    OutputJSON       string `json:"output_json,omitempty"`
    OutputJSONSHA256 string `json:"output_json_sha256,omitempty"`
}
```

Do not add `OutputJSON` to `WorkflowDependencyStage` for OS 007 unless it already exists. If it already exists, do not use it for resolver scope construction.

### 2. Step Aggregation

When all work items for a step complete:

```text
membership outputs -> aggregateStepOutputJSON -> step.OutputJSON
```

Do not additionally write the aggregate into `workflow_stages.output_json`.

### 3. Resolver Scope

The generated `workflow.step` scope must be built from dependency step outputs:

```text
plan.Stages[*].Steps[*].OutputJSON
```

It must not query or consult:

```text
workflow_stages.output_json
```

### 4. Stage Completion

When a stage completes, update stage state only:

```text
stage.State = completed
stage.CompletedAt = now, if timestamp exists
```

Do not attempt to compute a stage-level output.

### 5. Existing SQLite Column

Leave this column alone:

```sql
workflow_stages.output_json
```

It is nullable and can remain null. Do not add a migration to remove it in this slice.

## Tests to Add

Add or update tests in the controller package.

### Test: stage output does not drive `workflow.step`

Create dependency state with:

```text
step 0 OutputJSON = {"value":"step-output"}
workflow_stages.output_json = {"value":"wrong-stage-output"}
```

Build generated scope for `beforeStepIndex = 1`.

Assert:

```text
workflow.step[0].value == "step-output"
```

The bogus stage output must not affect resolution.

### Test: parallel stage preserves distinct step outputs

Create a plan with one stage containing two completed steps:

```text
stage 0
  step 0 OutputJSON = {"left":1}
  step 1 OutputJSON = {"right":2}
```

Build generated scope for `beforeStepIndex = 2`.

Assert:

```text
workflow.step[0].left == 1
workflow.step[1].right == 2
```

Do not aggregate these into one stage value.

### Test: stage output remains null/unused after aggregation

If there is a public or test-accessible store method that can inspect `workflow_stages.output_json`, assert it remains null after step aggregation.

If no such method exists, do not add broad DB plumbing only for this test. Instead, add a narrower unit test that the workflow scope helper accepts only dependency step outputs.

## Acceptance Criteria Amendment

OS 007 is not complete if:

- `workflow.step` is built from `workflow_stages.output_json`;
- logical step outputs are duplicated into `workflow_stages.output_json`;
- a parallel stage collapses multiple step outputs into one stage output;
- status/log output includes full `output_json` values by default.

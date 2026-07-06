# Implement OS 007: Persist and Retrieve Typed Logical Step Outputs

## Repository / Branch

Repository:

```text
https://github.com/josephweaver/go-etl
```

Branch:

```text
concept/dependency-aware-workflows
```

## Goal

Close the persistence/retrieval gap in operational slice:

```text
docs/concepts/dependency-aware-workflows/007-capture-typed-step-outputs.md
```

OS 007 requires the controller to:

1. Convert successful terminal work outputs from canonical JSON into `variable.ResolvedValue`.
2. Aggregate completed work-item outputs into logical workflow step outputs.
3. Persist those logical step outputs by submission/run context.
4. Build generated `workflow.step[index]` resolver scope for downstream step/stage compilation.
5. Preserve deterministic fanout ordering by original `work_item_index`, not completion order.

The current DB schema has `completed_work.output_json` and `workflow_stages.output_json`, but the controller does not currently persist or retrieve **logical dependency step outputs** for `workflow.step[index]`.

Implement this in the controller/dependency-state layer first. Do not introduce a new schema migration unless existing tests or repository design clearly require it.

## Required Reading Before Editing

Read these files first:

```text
docs/concepts/dependency-aware-workflows/README.md
docs/concepts/dependency-aware-workflows/006-record-terminal-work-item-state.md
docs/concepts/dependency-aware-workflows/007-capture-typed-step-outputs.md

cmd/controller/workflow_dependency_store.go
cmd/controller/main.go

internal/model/workflow_dependency.go
internal/model/work_item.go

internal/variable/variable.go
internal/variable/type.go
internal/variable/scope.go
internal/variable/accessor.go
internal/variable/resolver.go

internal/persistence/db_adapter_sqlite.go
```

Only read additional files when compilation or tests require it.

## Design Decision

Use the dependency state stored in the workflow run submission context as the first persistence layer for logical step outputs.

Rationale:

- `workflow_dependency_store.go` already stores `model.WorkflowDependencyPlan` in `workflow_instances.submission_context_json`.
- OS 007 says outputs must be keyed by submission/run context.
- Logical step output belongs with dependency step state, not only with raw completed work attempts.
- `completed_work.output_json` should remain the raw terminal work output record.
- `workflow_stages.output_json` is not sufficient as the canonical source for `workflow.step[index]` because a stage can contain multiple logical steps.

This implementation may require editing:

```text
internal/model/workflow_dependency.go
```

even though OS 007's allowed production file list did not mention it. Keep that as the only additional production file unless tests reveal a stronger need.

## Production Files to Change

Primary files:

```text
cmd/controller/workflow_outputs.go
cmd/controller/workflow_dependency_store.go
internal/model/workflow_dependency.go
internal/variable/variable.go
```

Potential file if the completion path exists or needs to be introduced:

```text
cmd/controller/workflow_completion.go
```

Avoid broad edits to:

```text
cmd/controller/main.go
internal/persistence/db_adapter_sqlite.go
```

unless necessary to hook output capture into the existing work-completion path.

## Step 1: Extend Dependency Model With Output Fields

In:

```text
internal/model/workflow_dependency.go
```

Add output fields to `WorkflowDependencyWorkItemMembership`:

```go
OutputJSON       string `json:"output_json,omitempty"`
OutputJSONSHA256 string `json:"output_json_sha256,omitempty"`
```

Add output fields to `WorkflowDependencyStep`:

```go
OutputJSON       string `json:"output_json,omitempty"`
OutputJSONSHA256 string `json:"output_json_sha256,omitempty"`
```

Validation rules:

- Do not require output fields for queued/running/blocked/failed states.
- For completed work-item memberships, if `OutputJSON` is present, validate that it is valid JSON.
- For completed logical steps, if `OutputJSON` is present, validate that it is valid JSON.
- Do not reject older persisted dependency state that lacks these fields.
- Keep validation backward compatible.

Update clone/copy helpers in:

```text
cmd/controller/workflow_dependency_store.go
```

so output fields are preserved when cloning stages, steps, and work-item memberships.

Specifically update:

```go
cloneDependencyStep
```

and any work-item clone logic inside it.

## Step 2: Add `cmd/controller/workflow_outputs.go`

Create:

```text
cmd/controller/workflow_outputs.go
```

This file should own controller-side output conversion and aggregation. Keep helper names unexported unless tests or existing style require exported names.

Implement these core helpers.

### JSON to ResolvedValue

Add a helper like:

```go
func resolvedOutputFromJSON(raw string) (variable.ResolvedValue, error)
```

Rules from OS 007:

- JSON object -> `variable.ResolvedObject`
- JSON array -> `variable.ResolvedList`
- JSON string -> `variable.TypeString`
- JSON boolean -> `variable.TypeBool`
- JSON integer -> `variable.TypeInt`
- non-integer JSON number -> error
- JSON `null` -> error

Implementation notes:

- Decode with `json.Decoder`.
- Use `decoder.UseNumber()` so integer-vs-float can be detected safely.
- Reject trailing tokens.
- For `json.Number`, accept only exact integers representable as Go `int`.
- Return clear path-aware errors when practical, for example:
  - `output /count has non-integer number 1.25`
  - `output /value is null, null outputs are not supported`

### ResolvedValue to TypedExpression

If needed, add a very small helper in:

```text
internal/variable/variable.go
```

Suggested function:

```go
func TypedExpressionFromResolved(value ResolvedValue) (TypedExpression, error)
```

Expected behavior:

- `TypeString`, `TypeBool`, `TypeInt`, `TypePath`, `TypeDatetime` become scalar typed expressions.
- `TypeObject` recursively converts each field into `map[string]TypedExpression`.
- `TypeList` recursively converts each item into `[]TypedExpression`.
- Reject unknown/invalid types.

Use this only to construct generated read-only `workflow.step` variables for resolver scope.

### Canonical JSON

Add a helper like:

```go
func canonicalOutputJSONFromResolved(value variable.ResolvedValue) (string, string, error)
```

or equivalent.

Requirements:

- Produce stable JSON.
- Compute SHA-256 over the canonical JSON bytes.
- Return lowercase hex SHA-256.
- Use existing hash helper if one already exists in the repo; otherwise use `crypto/sha256` and `encoding/hex`.

Important:

- Go's `encoding/json` marshals map keys in stable order.
- Preserve list order exactly.

### Step Aggregation

Add a helper like:

```go
func aggregateStepOutputJSON(step model.WorkflowDependencyStep) (outputJSON string, outputJSONSHA256 string, err error)
```

Rules:

- A non-fanout step with exactly one completed work item stores that item's logical output object as the step output.
- A fanout step stores a list of item outputs ordered by `WorkItemIndex`.
- Completion order must not affect output order.
- Every completed/skipped item used for aggregation must have `OutputJSON`.
- Missing required output is an error.
- Failed/incomplete work items mean the step cannot aggregate yet.
- Reject duplicate `WorkItemIndex`.
- Validate each item output using `resolvedOutputFromJSON`.

Fanout detection:

- If the step has more than one work item, aggregate as a list.
- If the step has one work item, aggregate as the single output object/value.
- Do not flatten fanout outputs.

Be conservative: OS 007 specifically says a non-fanout step stores one logical output object. If the output is not an object for a non-fanout completed step, reject it with a clear error unless existing tests indicate scalar step outputs are accepted. Fanout items should also preferably be objects unless the existing concept docs clearly allow scalar item outputs.

## Step 3: Add Dependency Store Output Capture

In:

```text
cmd/controller/workflow_dependency_store.go
```

Add a method to persist completed work-item output into dependency state.

Suggested request type:

```go
type RecordCompletedWorkItemOutputRequest struct {
	SubmissionID      string
	WorkItemID        string
	OutputJSON        string
	OutputJSONSHA256  string
}
```

Suggested method:

```go
func (c *Controller) RecordCompletedWorkItemOutput(
	ctx context.Context,
	req RecordCompletedWorkItemOutputRequest,
) error
```

Behavior:

1. Validate:
   - `workflowStore` exists.
   - `SubmissionID` is non-empty.
   - `WorkItemID` is non-empty.
   - `OutputJSON` is non-empty.
   - `OutputJSON` parses under OS 007 rules.
   - If `OutputJSONSHA256` is empty, compute it.
   - If `OutputJSONSHA256` is present, verify it matches canonical output JSON or the exact stored JSON, depending on existing hash semantics. Prefer canonical output hash if no existing convention is obvious.

2. Load dependency plan for `SubmissionID` using existing dependency-state accessors.

3. Find the membership by `WorkItemID`.

4. Update that membership:
   - `State = model.WorkItemMembershipStateCompleted`
   - `OutputJSON = canonical output JSON`
   - `OutputJSONSHA256 = computed hash`

5. If all work items for the logical step are completed or skipped and each has usable output:
   - aggregate the logical step output with `aggregateStepOutputJSON`
   - set:
     - `step.OutputJSON`
     - `step.OutputJSONSHA256`
     - `step.State = model.WorkflowStepStateCompleted`

6. Persist the updated dependency plan back to the workflow run submission context.

7. Preserve existing state-transition behavior. Do not accidentally mark stages/workflows complete unless existing code already does that nearby.

Add helper functions as needed:

```go
func findDependencyMembershipByWorkItemID(
	plan *model.WorkflowDependencyPlan,
	workItemID string,
) (*model.WorkflowDependencyStep, *model.WorkflowDependencyWorkItemMembership, bool)
```

```go
func dependencyStepOutputsReady(step model.WorkflowDependencyStep) bool
```

```go
func dependencyStepHasIncompleteWork(step model.WorkflowDependencyStep) bool
```

## Step 4: Hook Output Capture Into Work Completion Path

Find the existing worker completion endpoint / handler that receives `model.WorkCompletion`.

`model.WorkCompletion` already carries output JSON and evidence hashes. Use the existing completion path after raw completed work is recorded successfully.

Add a call similar to:

```go
if completion.OutputJSON != "" {
	if err := c.RecordCompletedWorkItemOutput(ctx, RecordCompletedWorkItemOutputRequest{
		SubmissionID:      completion.SubmissionID,
		WorkItemID:        completion.WorkItemID,
		OutputJSON:        completion.OutputJSON,
		OutputJSONSHA256:  completion.OutputJSONSHA256,
	}); err != nil {
		// fail the workflow or completion path with a clear error, according to existing controller error policy
	}
}
```

Adapt field names to the actual `model.WorkCompletion` struct.

Important:

- Do not swallow output-capture errors.
- A completed step with missing or invalid required output should transition the workflow to failure if the existing failure mechanism supports that. If no workflow failure helper exists yet, return a clear error from the completion path and add a TODO only where unavoidable.
- Ensure outputs from one submission cannot be read by another submission.

## Step 5: Build Generated `workflow.step` Scope

In:

```text
cmd/controller/workflow_outputs.go
```

Add a helper like:

```go
func workflowStepScope(
	plan model.WorkflowDependencyPlan,
	beforeStepIndex int,
) (variable.Scope, error)
```

Expected behavior:

- Build a read-only generated variable named:
  - namespace: `workflow`
  - key: `step`
- Its value should be a list in workflow-definition order.
- Include only completed outputs available before the step/stage being compiled.
- Do not include future steps.
- If a prior required step output is unavailable, return an error.
- Future unavailable outputs should fail through normal resolver behavior when referenced.

Implementation outline:

1. Flatten all dependency steps.
2. Sort by `StepIndex`.
3. Include steps with `StepIndex < beforeStepIndex`.
4. For each included step:
   - require `State == completed`
   - require `OutputJSON != ""`
   - parse using `resolvedOutputFromJSON`
   - convert to `variable.TypedExpression`
5. Create:

```go
variable.Variable{
	Name: variable.Name{
		Namespace: variable.NamespaceWorkflow,
		Key:       "step",
	},
	TypedExpression: variable.TypedExpression{
		Type:       variable.TypeList,
		Expression: typedStepOutputs,
	},
}
```

6. Return `variable.NewScope(...)`.

If the existing namespace constant name differs, use the existing one from `internal/variable`.

## Step 6: Do Not Use `workflow_stages.output_json` as Canonical Step Output

Do not implement `workflow.step[index]` by reading only:

```text
workflow_stages.output_json
```

Reason:

- The dependency model can represent multiple logical steps inside a stage.
- A single stage-level output column cannot safely represent each logical step output.

It is acceptable to leave `workflow_stages.output_json` unused for OS 007 unless existing repository tests require it.

## Step 7: Tests

Add or update tests in:

```text
cmd/controller/workflow_outputs_test.go
cmd/controller/workflow_dependency_store_test.go
internal/variable/variable_test.go
```

Potentially update:

```text
cmd/controller/workflow_completion_test.go
```

only if the completion hook is implemented there.

### Required Unit Tests: JSON Conversion

Add tests for:

```go
func TestResolvedOutputFromJSONConvertsNestedObject(t *testing.T)
func TestResolvedOutputFromJSONConvertsNestedList(t *testing.T)
func TestResolvedOutputFromJSONConvertsScalars(t *testing.T)
func TestResolvedOutputFromJSONRejectsNull(t *testing.T)
func TestResolvedOutputFromJSONRejectsNonIntegerNumber(t *testing.T)
func TestResolvedOutputFromJSONRejectsTrailingTokens(t *testing.T)
```

Cover at least this object:

```json
{
  "path": "s3://bucket/value",
  "count": 3,
  "ok": true,
  "items": [
    {"name": "a"},
    {"name": "b"}
  ]
}
```

Expected:

- `path` -> string
- `count` -> int
- `ok` -> bool
- `items` -> list of objects

### Required Unit Tests: Aggregation

Add tests for:

```go
func TestAggregateStepOutputNonFanoutStoresSingleObject(t *testing.T)
func TestAggregateStepOutputFanoutStoresOutputsByWorkItemIndex(t *testing.T)
func TestAggregateStepOutputFanoutIgnoresCompletionOrder(t *testing.T)
func TestAggregateStepOutputRejectsMissingItemOutput(t *testing.T)
func TestAggregateStepOutputRejectsDuplicateWorkItemIndex(t *testing.T)
```

Fanout test shape:

- Work item index 2 completes first with `{"value":"c"}`
- Work item index 0 completes second with `{"value":"a"}`
- Work item index 1 completes third with `{"value":"b"}`

Aggregated step output must be:

```json
[
  {"value":"a"},
  {"value":"b"},
  {"value":"c"}
]
```

### Required Store Tests

Add tests for:

```go
func TestRecordCompletedWorkItemOutputPersistsMembershipOutput(t *testing.T)
func TestRecordCompletedWorkItemOutputPersistsAggregatedStepOutput(t *testing.T)
func TestRecordCompletedWorkItemOutputKeepsFanoutOrderStable(t *testing.T)
func TestRecordCompletedWorkItemOutputDoesNotLeakAcrossSubmissions(t *testing.T)
func TestRecordCompletedWorkItemOutputRejectsUnknownWorkItem(t *testing.T)
func TestRecordCompletedWorkItemOutputRejectsInvalidOutputJSON(t *testing.T)
```

The persistence test must:

1. Create a workflow run/submission.
2. Create a dependency plan.
3. Record work-item memberships.
4. Record completed output.
5. Reload dependency state from the store.
6. Assert membership output and step output survived reload.

### Required Scope Tests

Add tests for:

```go
func TestWorkflowStepScopeResolvesCompletedPriorStep(t *testing.T)
func TestWorkflowStepScopeExcludesFutureStep(t *testing.T)
func TestWorkflowStepScopeErrorsOnMissingPriorOutput(t *testing.T)
func TestWorkflowStepScopeUsesSubmissionScopedPlanOnly(t *testing.T)
```

Example assertion:

- Step 0 output:

```json
{"answer": 42, "label": "done"}
```

- Build scope with `beforeStepIndex = 1`.
- Create resolver with that scope.
- Resolve:

```text
workflow.step[0].answer
workflow.step[0].label
```

Expected:

- `answer` resolves as int `42`.
- `label` resolves as string `"done"`.

Future-step behavior:

- Build scope with `beforeStepIndex = 1`.
- Attempt to resolve `workflow.step[1]`.
- It should fail through the normal resolver path/accessor behavior.

## Step 8: Backward Compatibility

Existing dependency state JSON without output fields must still unmarshal and validate.

Add a test that constructs old-style dependency state JSON like:

```json
{
  "run_id": "submission-1",
  "workflow_id": "workflow-1",
  "state": "running",
  "stages": [
    {
      "stage_index": 0,
      "state": "ready",
      "parallel_with": "",
      "steps": [
        {
          "stage_index": 0,
          "step_index": 0,
          "step_id": "step-0",
          "state": "ready",
          "work_items": [
            {
              "work_item_id": "work-0",
              "work_item_index": 0,
              "state": "queued"
            }
          ]
        }
      ]
    }
  ]
}
```

Then assert:

- It unmarshals into `model.WorkflowDependencyPlan`.
- It validates successfully.
- Missing output fields do not cause errors.

## Step 9: Commands to Run

Run focused tests first:

```bash
go test ./internal/variable ./cmd/controller
```

Then run the whole suite:

```bash
go test ./...
```

Run formatting:

```bash
gofmt -w \
  internal/model/workflow_dependency.go \
  internal/variable/variable.go \
  cmd/controller/workflow_outputs.go \
  cmd/controller/workflow_dependency_store.go
```

Include any additional edited files in the `gofmt` command.

## Acceptance Criteria

The implementation is complete when:

- JSON objects convert to nested `variable.ResolvedValue` objects.
- JSON arrays convert to nested `variable.ResolvedValue` lists.
- Strings, booleans, and integers convert to typed scalar values.
- Non-integer numbers are rejected with a clear error.
- `null` is rejected with a clear error.
- A non-fanout step stores one logical output object.
- A fanout step stores a list ordered by original `work_item_index`.
- Completion order does not affect fanout output order.
- Completed work-item output is persisted into dependency state.
- Aggregated logical step output is persisted into dependency state.
- Reloading dependency state by `submission_id` returns the persisted outputs.
- `workflow.step[0]` can resolve from a completed prior step.
- Referencing unavailable future step output fails through normal resolver behavior.
- Outputs from one submission are not visible in another submission's generated scope.
- Existing old dependency state without output fields remains valid.
- `go test ./...` passes.

## Non-Goals

Do not implement:

- JIT compiling downstream stages.
- Float support.
- Null support.
- Datetime inference.
- Path inference.
- Aliases like `workflow.previous`.
- Fanout flattening.
- Worker-side output parsing.
- CLI output display.
- A new relational `workflow_step_outputs` table unless existing code architecture clearly requires it.

## PR Summary Template

Use this summary when done:

```text
Implemented OS 007 typed logical step output capture.

Changes:
- Added persisted output fields to dependency work-item membership and logical step state.
- Added controller helpers for JSON output conversion, canonical hashing, fanout aggregation, and generated workflow.step scope construction.
- Added dependency-store output capture keyed by submission_id/work_item_id.
- Hooked successful work completion into dependency output capture.
- Added tests for JSON conversion, deterministic fanout aggregation, dependency-state persistence, generated workflow.step scope, and backward compatibility.

Notes:
- completed_work.output_json remains the raw terminal work-attempt output.
- workflow_stages.output_json is not used as the canonical logical step output source because a stage may contain multiple logical steps.
```

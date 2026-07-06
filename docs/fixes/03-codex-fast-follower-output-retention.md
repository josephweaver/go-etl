# Fast Follower: Bounded Output Retention Without Stage-Level Output Persistence

## Context

This fast follower follows OS 007 typed step output capture.

Important principle:

```text
output_json is not provenance.
```

It is a control-plane handoff mechanism used to resolve downstream workflow expressions. Large results should live outside the controller DB and be referenced by small artifact metadata.

## Goal

Prevent database growth caused by large or duplicated `output_json` values while preserving `workflow.step[index]` semantics.

## Non-Negotiable Stage-Level Rule

Do **not** use `workflow_stages.output_json` as canonical output storage.

For dependency-aware workflows:

```text
workflow_stages.output_json should remain NULL / unused.
```

The canonical running-workflow output is dependency **step** output:

```text
WorkflowDependencyStep.OutputJSON
```

## Output Retention Policy

Use this lifecycle:

```text
worker completion output
  -> validate byte limit
  -> persist raw completed_work.output_json, because current schema requires it
  -> copy temporarily to dependency work-item membership
  -> aggregate into dependency step output
  -> prune membership output JSON after aggregation
  -> retain step output JSON while workflow is running
  -> prune step output JSON when workflow reaches terminal completed/failed state
```

Do not prune step output immediately after the owning stage completes. Later stages may reference any prior `workflow.step[index]`.

## Recommended Byte Limits

Add constants near controller output helpers:

```go
const (
    maxCompletedWorkOutputJSONBytes = 16 * 1024
    maxLogicalStepOutputJSONBytes   = 256 * 1024
)
```

Use byte length:

```go
len([]byte(outputJSON))
```

Do not truncate oversized output. Reject it.

Suggested error:

```text
output_json is 92144 bytes, limit is 16384 bytes; store bulk data externally and return a small artifact reference
```

## Preferred Output Shape

Encourage workers to return small artifact references:

```json
{
  "artifact": {
    "uri": "s3://bucket/key.json",
    "sha256": "0123456789abcdef...",
    "bytes": 1234567,
    "content_type": "application/json"
  },
  "summary": {
    "row_count": 12500,
    "partition_count": 8
  }
}
```

Do not try to semantically detect every bad shape. Enforce size limits.

## Model Additions

In `internal/model/workflow_dependency.go`, add metadata fields to membership and step output state if they do not already exist.

For work-item membership:

```go
OutputJSON        string `json:"output_json,omitempty"`
OutputJSONSHA256  string `json:"output_json_sha256,omitempty"`
OutputJSONBytes   int    `json:"output_json_bytes,omitempty"`
OutputJSONPruned  bool   `json:"output_json_pruned,omitempty"`
```

For logical step:

```go
OutputJSON        string `json:"output_json,omitempty"`
OutputJSONSHA256  string `json:"output_json_sha256,omitempty"`
OutputJSONBytes   int    `json:"output_json_bytes,omitempty"`
OutputJSONPruned  bool   `json:"output_json_pruned,omitempty"`
```

Do not add these fields to `WorkflowDependencyStage` for this fast follower.

## Membership Pruning

After step aggregation succeeds:

1. Keep each membership's:
   - `WorkItemID`
   - `WorkItemIndex`
   - `State`
   - `OutputJSONSHA256`
   - `OutputJSONBytes`
   - `OutputJSONPruned = true`
2. Clear:
   - `OutputJSON = ""`
3. Persist the dependency plan.

Do not prune membership output before step aggregation succeeds.

## Step Output Retention

Keep `WorkflowDependencyStep.OutputJSON` while the workflow is running.

Reason:

A later stage may reference any prior step:

```text
workflow.step[0]
workflow.step[1]
workflow.step[2]
```

The first fast follower should not attempt static expression-use analysis.

## Terminal-State Pruning

When the workflow reaches terminal `completed` or terminal `failed` state:

1. Iterate all dependency steps.
2. For each step with `OutputJSON != ""`:
   - preserve `OutputJSONSHA256`
   - preserve `OutputJSONBytes`
   - set `OutputJSON = ""`
   - set `OutputJSONPruned = true`
3. Persist the dependency plan.

A pruned output must remain distinguishable from a real empty object/list.

Do not represent pruned output as:

```json
{}
```

or:

```json
null
```

Use metadata flags instead.

## Scope Construction Behavior

`workflowStepScope(...)` must behave as follows:

- If a needed prior step has `OutputJSON != ""`, parse and expose it normally.
- If a needed prior step has `OutputJSON == ""` and `OutputJSONPruned == true`, return a clear error such as:

```text
workflow.step[0] output was pruned and is no longer available for resolver scope construction
```

This should not occur in normal running workflow execution if terminal pruning happens only after completion/failure.

## Completed Work Table Note

The current SQLite schema defines:

```sql
completed_work.output_json TEXT NOT NULL CHECK (json_valid(output_json))
```

Because it is `NOT NULL`, this fast follower should not try to null it out without a schema migration.

The first protection is the hard byte limit.

A later schema migration could move raw completion output to optional retained metadata, but that is out of scope here.

## Status and Logs

Do not expose full output JSON in status or logs.

Status/logs may include:

```text
step index
state
output_json_sha256
output_json_bytes
output_json_pruned
failure reason
```

They should not include:

```text
OutputJSON
completed_work.output_json
workflow_stages.output_json
```

## Tests

Add tests for these behaviors.

### Limit enforcement

```go
func TestRecordCompletedWorkItemOutputRejectsOversizedCompletedOutput(t *testing.T)
func TestAggregateStepOutputRejectsOversizedLogicalOutput(t *testing.T)
```

### Stage-level non-use

```go
func TestWorkflowStepScopeDoesNotUseWorkflowStagesOutputJSON(t *testing.T)
func TestParallelStageOutputsAreNotCollapsedIntoStageOutputJSON(t *testing.T)
```

### Membership pruning

```go
func TestRecordCompletedWorkItemOutputPrunesMembershipOutputAfterAggregation(t *testing.T)
```

Assert:

```text
membership.OutputJSON == ""
membership.OutputJSONSHA256 != ""
membership.OutputJSONBytes > 0
membership.OutputJSONPruned == true
step.OutputJSON still available while workflow is running
```

### Terminal pruning

```go
func TestTerminalWorkflowPrunesStepOutputJSON(t *testing.T)
```

Assert after terminal completion/failure:

```text
step.OutputJSON == ""
step.OutputJSONSHA256 != ""
step.OutputJSONBytes > 0
step.OutputJSONPruned == true
```

### Scope behavior after prune

```go
func TestWorkflowStepScopeErrorsOnPrunedOutput(t *testing.T)
```

Assert the error clearly says the output was pruned and no longer available.

### Status/log safety

```go
func TestDependencyStatusDoesNotExposeFullOutputJSON(t *testing.T)
func TestDependencyLogsDoNotExposeFullOutputJSON(t *testing.T)
```

Only add these tests if status/log code from OS 011 is already present.

## Acceptance Criteria

The fast follower is complete when:

- Oversized completed work output is rejected before persistence.
- Oversized logical step output is rejected before persistence.
- Logical step output is not written to `workflow_stages.output_json`.
- `workflow.step` is never built from `workflow_stages.output_json`.
- Membership output JSON is pruned after step aggregation.
- Step output JSON remains available while workflow is running.
- Step output JSON is pruned at terminal workflow state.
- Hashes and byte counts remain after pruning.
- Pruned output causes a clear resolver error if accessed after pruning.
- Status and logs do not dump full output JSON.
- Existing OS 007 through OS 012 behavior remains intact.

## Out of Scope

Do not implement:

- Schema migration to remove `workflow_stages.output_json`.
- Schema migration to make `completed_work.output_json` nullable.
- Static analysis of future workflow expressions to prune step outputs before terminal state.
- External artifact storage.
- New provenance model.
- New public CLI commands.

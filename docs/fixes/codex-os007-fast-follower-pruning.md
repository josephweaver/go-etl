# Fast Follower: Bound and Prune OS 007 `output_json`

## Context

This is a follow-up to the OS 007 implementation prompt:

```text
codex-os007-step-outputs.md
```

The original OS 007 implementation closes the functional gap: completed work output is captured, converted into typed logical step output, persisted, and exposed through generated `workflow.step[index]` scope.

This fast follower tightens storage behavior.

## Core Principle

`output_json` is **not provenance**.

It is a transient handoff mechanism used by the controller to get a small typed value from one logical workflow step into the next step's resolver scope.

Do not treat `output_json` as:

- audit storage
- provenance storage
- bulk result storage
- debug-log storage
- row/file/blob storage
- permanent workflow history

Provenance should be represented by state, timestamps, hashes, evidence hashes, artifact references, and external artifact/log systems.

## Goal

Add bounded-retention behavior without changing the OS 007 user-facing semantics.

After this fast follower:

1. Oversized `output_json` is rejected early.
2. Workers are forced to return small metadata/artifact references instead of large payloads.
3. Full per-work-item output JSON is pruned after logical step output aggregation.
4. Logical step output JSON is pruned when it is no longer needed to materialize downstream work.
5. Hash/byte metadata remains after pruning.
6. `workflow.step[index]` still works while the value is legitimately needed.
7. A pruned output is never silently treated as `{}` or `null`.

## Recommended Limits

Add package-level constants near the controller output helpers:

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

Do not truncate oversized output. Reject it with a clear error.

Error text should be close to:

```text
output_json is 92144 bytes, limit is 16384 bytes; store bulk data externally and return a small artifact reference
```

## Intended Output Shape

Workers should return small control-plane metadata and artifact references.

Preferred:

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

Rejected by size, not necessarily semantic inspection:

```json
{
  "rows": [
    {"id": 1, "name": "..."},
    {"id": 2, "name": "..."}
  ]
}
```

```json
{
  "file_contents_base64": "..."
}
```

```json
{
  "log": "very large log text..."
}
```

The controller does not need to detect every bad key. The hard byte limits are the enforcement mechanism.

## Model Additions

In `internal/model/workflow_dependency.go`, add or confirm these fields exist on logical steps:

```go
OutputJSON        string `json:"output_json,omitempty"`
OutputJSONSHA256  string `json:"output_json_sha256,omitempty"`
OutputJSONBytes   int    `json:"output_json_bytes,omitempty"`
OutputJSONPruned  bool   `json:"output_json_pruned,omitempty"`
```

If the first OS 007 implementation added full output JSON to work-item memberships, keep it backward compatible but add pruning metadata:

```go
OutputJSON        string `json:"output_json,omitempty"`
OutputJSONSHA256  string `json:"output_json_sha256,omitempty"`
OutputJSONBytes   int    `json:"output_json_bytes,omitempty"`
OutputJSONPruned  bool   `json:"output_json_pruned,omitempty"`
```

If the first OS 007 implementation did **not** add full output JSON to work-item memberships, do not add it now. Prefer metadata only:

```go
OutputJSONSHA256  string `json:"output_json_sha256,omitempty"`
OutputJSONBytes   int    `json:"output_json_bytes,omitempty"`
OutputJSONPruned  bool   `json:"output_json_pruned,omitempty"`
```

Backward compatibility requirement:

- Existing dependency state JSON without these fields must still unmarshal and validate.
- Do not require pruning fields for old state.

## Size Validation

In `cmd/controller/workflow_outputs.go`, add helpers like:

```go
func validateCompletedWorkOutputJSONSize(outputJSON string) error
func validateLogicalStepOutputJSONSize(outputJSON string) error
```

Apply them in these places:

1. Before accepting `model.WorkCompletion.OutputJSON` into output capture.
2. After canonicalizing a completed work output.
3. After aggregating a logical step output.
4. Before persisting a logical step output into dependency state.
5. Before building `workflow.step` scope from persisted step output.

Rules:

- Empty output is invalid when output is required.
- Oversized output is invalid.
- Canonical output must also fit the limit.
- Error should explain that large data must be externalized as artifacts.

## Pruning Behavior

Add helpers in `cmd/controller/workflow_outputs.go` or `cmd/controller/workflow_dependency_store.go`:

```go
func pruneWorkItemOutputJSON(step *model.WorkflowDependencyStep)
```

```go
func pruneStepOutputJSON(step *model.WorkflowDependencyStep)
```

Behavior:

- Clear the full `OutputJSON` string.
- Keep `OutputJSONSHA256`.
- Keep `OutputJSONBytes`.
- Set `OutputJSONPruned = true`.
- Do not change completed/failed state.
- Do not replace pruned output with `{}` or `null`.

### Safe Minimum Pruning

Implement this first because it is safe and quick.

After a logical step output has been successfully aggregated and persisted:

1. Clear full output JSON from all work-item memberships for that step, if membership-level full output exists.
2. Keep membership hashes and byte counts.
3. Persist the dependency plan.

This avoids storing both:

- each item output JSON, and
- the aggregate logical step output JSON.

### Terminal Workflow Pruning

When the workflow reaches a terminal state, prune all remaining full `OutputJSON` values from dependency state:

- work-item membership outputs
- logical step outputs

Keep hashes, bytes, IDs, states, and timestamps.

If there is no single terminal-state hook yet, add the helper and call it from whichever completion/failure path currently persists terminal dependency state.

### Downstream-Materialization Pruning

Add this only if the repository already has a clear place to tell that downstream work has been materialized.

A logical step output can be pruned when every downstream step that can reference it has already had its work items created with resolved inputs.

Preferred implementation:

- Use explicit dependency edges/reference analysis if already available.
- Prune step `i` after all known consumers of `workflow.step[i]` are materialized.

Conservative fallback:

- If exact consumers are not known, do **not** prune step output immediately.
- Keep it until terminal workflow pruning.

Avoid unsafe pruning that would break resume/restart before downstream work exists.

## Retrieval Behavior After Pruning

If `workflowStepScope` or another resolver helper needs an output and finds:

```go
OutputJSON == "" && OutputJSONPruned == true
```

return a clear internal error like:

```text
workflow.step[2] output was pruned before downstream work was materialized
```

Do not return an empty object.
Do not return `null`.
Do not silently skip the step.

## Optional DB Pruning

If `completed_work.output_json` is nullable and existing persistence APIs make this easy, add a small method to clear raw completed-work output after step aggregation:

```go
func (s *Store) PruneCompletedWorkOutputJSON(ctx context.Context, workItemID string) error
```

or batch form:

```go
func (s *Store) PruneCompletedWorkOutputJSONByWorkItemIDs(ctx context.Context, workItemIDs []string) error
```

Rules:

- Set `output_json = NULL` only if the schema permits it.
- Keep `output_json_sha256`.
- Keep work completion state.
- Keep timestamps and evidence metadata.
- Scope by submission/run if the table supports it.

If the column is not nullable or this requires a broad migration, skip DB-level pruning in this fast follower. Dependency-state pruning and size limits are the required minimum.

Do **not** store fake `{}` just to satisfy a non-null JSON column. That hides the fact that the original value was pruned.

## Tests to Add

Add or update tests in:

```text
cmd/controller/workflow_outputs_test.go
cmd/controller/workflow_dependency_store_test.go
```

Add DB pruning tests only if DB-level pruning is implemented:

```text
internal/persistence/db_adapter_sqlite_test.go
```

### Size Tests

```go
func TestValidateCompletedWorkOutputJSONSizeAcceptsSmallArtifactReference(t *testing.T)
func TestValidateCompletedWorkOutputJSONSizeRejectsOversizedOutput(t *testing.T)
func TestValidateLogicalStepOutputJSONSizeRejectsOversizedAggregate(t *testing.T)
func TestRecordCompletedWorkItemOutputRejectsOversizedOutputJSON(t *testing.T)
```

Assert the oversized error includes:

- actual byte count
- configured byte limit
- instruction to use artifact/external storage

### Pruning Tests

```go
func TestRecordCompletedWorkItemOutputPrunesMembershipOutputAfterStepAggregation(t *testing.T)
func TestPruneWorkItemOutputJSONKeepsHashAndByteCount(t *testing.T)
func TestPruneStepOutputJSONKeepsHashAndByteCount(t *testing.T)
func TestWorkflowStepScopeErrorsIfRequiredOutputWasPruned(t *testing.T)
func TestTerminalWorkflowPruningClearsAllDependencyOutputJSON(t *testing.T)
```

### Scope Safety Tests

```go
func TestWorkflowStepScopeStillResolvesBeforePrune(t *testing.T)
func TestWorkflowStepScopeDoesNotTreatPrunedOutputAsEmptyObject(t *testing.T)
```

### Optional DB Tests

Only add these if DB-level pruning is implemented:

```go
func TestPruneCompletedWorkOutputJSONClearsOutputButKeepsHash(t *testing.T)
func TestPruneCompletedWorkOutputJSONIsSubmissionScoped(t *testing.T)
```

## Acceptance Criteria

This fast follower is complete when:

- Oversized completed-work output JSON is rejected.
- Oversized logical-step aggregate output JSON is rejected.
- Error messages direct workers to external artifact storage.
- Small artifact-reference outputs are accepted.
- Full membership-level output JSON is cleared after logical step aggregation, if membership-level full output exists.
- Logical step output JSON is kept while needed for downstream `workflow.step[index]` resolution.
- Logical step output JSON is pruned at terminal workflow state.
- Hash and byte metadata survive pruning.
- Pruned outputs are not treated as `{}` or `null`.
- Existing old dependency state remains valid.
- `go test ./...` passes.

## Non-Goals

Do not implement in this fast follower:

- artifact storage itself
- worker upload protocols
- row/result pagination
- semantic detection of all large-output key names
- long-term debug output retention
- a new step-output table unless the current implementation already requires it
- JIT compilation changes unrelated to pruning

## PR Summary Template

```text
Added bounded retention for OS 007 output_json.

Changes:
- Added hard byte limits for completed-work output_json and logical step aggregate output_json.
- Added pruning metadata for dependency output state.
- Pruned membership-level output_json after logical step aggregation where applicable.
- Pruned remaining dependency output_json at terminal workflow state.
- Preserved output hashes and byte counts after pruning.
- Added tests for oversized output rejection, artifact-reference output acceptance, pruning, and resolver safety.

Notes:
- output_json is treated as transient controller handoff data, not provenance.
- Bulk data must be stored externally and referenced by small output_json metadata.
```

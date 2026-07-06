# Review: OS 007+ and Stage-Level `output_json`

## Scope

Reviewed dependency-aware workflow slices:

```text
docs/concepts/dependency-aware-workflows/007-capture-typed-step-outputs.md
docs/concepts/dependency-aware-workflows/008-compile-next-ready-stage.md
docs/concepts/dependency-aware-workflows/009-handle-empty-fanout-and-auto-advance.md
docs/concepts/dependency-aware-workflows/010-propagate-step-and-workflow-failure.md
docs/concepts/dependency-aware-workflows/011-surface-dependency-state-in-status-and-logs.md
docs/concepts/dependency-aware-workflows/012-update-dependency-workflow-docs-and-smoke.md
```

Also checked the README output contract and the SQLite schema shape that contains:

```sql
workflow_stages.output_json TEXT CHECK (output_json IS NULL OR json_valid(output_json))
workflow_stages.output_json_sha256 TEXT
```

## Finding

`workflow_stages.output_json` should **not** be the canonical persistence point for dependency-aware workflow output.

The output contract is step-oriented, not stage-oriented. The public generated namespace is:

```text
workflow.step[index]
```

A stage may contain multiple logical steps when adjacent workflow steps share a contiguous `parallel_with` label. In that case, one stage-level `output_json` cannot safely represent each `workflow.step[index]` entry without inventing a second aggregation shape that the docs do not define.

## Why Stage-Level Output Is Wrong for OS 007+

### README Output Contract

The dependency-aware workflow README defines `workflow.step` as a controller-generated read-only list in workflow-definition order.

It says each list entry is the logical output of one workflow step:

```text
non-fanout step -> one object
fanout step     -> list of item-output objects in deterministic fanout order
future/failed/unavailable step -> resolution error
```

This means the durable resolver input must be keyed by logical `step_index`, not by `stage_index`.

### OS 007: Capture Typed Step Outputs

OS 007 repeatedly describes **step outputs**:

```text
Convert successful terminal work outputs into typed step outputs.
A non-fan-out step with one completed work item stores that item's output object as the step output.
A fan-out step stores a list of completed item outputs ordered by work_item_index.
```

It also requires outputs to be keyed by `submission_id` and exposed through generated `workflow.step` scope.

No part of OS 007 requires a stage-level output value.

### OS 008: Compile Next Ready Stage

OS 008 activates stage `N+1` by assembling resolver scopes from retained submission/workflow context plus generated `workflow.step` outputs.

That means next-stage compilation depends on prior **step** outputs, not a prior **stage** output.

For a parallel stage containing multiple steps, downstream stage compilation may need:

```text
workflow.step[0]
workflow.step[1]
```

A single `workflow_stages.output_json` for stage 0 cannot represent both unless it stores an undocumented synthetic structure. That synthetic structure would then need to be reverse-mapped back to individual step indexes, which is unnecessary and risky.

### OS 009: Empty Fanout

OS 009 defines the empty fanout step output as:

```json
[]
```

That is again a step output. A parallel stage could contain one empty fanout step and one non-empty step. Stage-level output would be ambiguous:

```text
stage 0 contains step 0 -> []
stage 0 contains step 1 -> [{...}, {...}]
```

There is no documented stage output shape for that case.

### OS 010: Failure Handling

OS 010 says output-capture failure is workflow-fatal:

```text
output JSON missing or invalid for a step that must expose output
```

Failure reasons should identify the failed step/stage, but the invalid output itself should not become a stage output artifact.

### OS 011: Status and Logs

OS 011 says human-readable status should remain compact and not dump large internal JSON. It also says logs should not include full workflow documents or user data.

This supports a retention policy where full output JSON is not exposed through status/log surfaces and is not duplicated into stage records.

### OS 012: Docs and Smoke

OS 012 requires docs to describe `workflow.step[index]` output access and its limitations. It does not introduce `workflow.stage[index]` or stage-level output semantics.

## Recommended Persistence Policy

Use these persistence roles:

| Location | Role | Retention |
|---|---|---|
| `completed_work.output_json` | Raw terminal work completion output, already part of existing completion schema. | Must be byte-limited because schema is `NOT NULL`. Later pruning requires schema change. |
| dependency work-item membership `OutputJSON` | Temporary copy used to aggregate a logical step output. | Prune immediately after owning step output is aggregated. |
| dependency step `OutputJSON` | Canonical running-workflow value for `workflow.step[index]`. | Keep while workflow is running; prune at terminal state unless explicit retention is configured. |
| `workflow_stages.output_json` | Existing nullable schema column. | Leave `NULL` / unused for dependency-aware step output semantics. |
| status/log payloads | Diagnostics only. | Show state, indexes, IDs, byte counts, hashes, pruned flags, and failure reasons; do not include full output JSON. |

## Stage-Level Column Decision

Recommended decision:

```text
Do not remove workflow_stages.output_json right now.
Do not populate it with step outputs.
Do not read it to build workflow.step scope.
Treat it as legacy/unused for dependency-aware workflows.
```

Reasons:

1. Removing it is a schema migration and not required to close OS 007.
2. Populating it duplicates data and grows the DB faster.
3. Reading from it creates incorrect behavior for multi-step stages.
4. Keeping it null is backward-compatible because the column allows null.

## If a Relational Output Table Is Needed Later

If dependency output needs to move out of `submission_context_json`, use a step-level table, not stage-level output:

```sql
CREATE TABLE workflow_step_outputs (
  run_id TEXT NOT NULL,
  step_index INTEGER NOT NULL,
  stage_index INTEGER NOT NULL,
  step_id TEXT NOT NULL,
  output_json TEXT NOT NULL CHECK (json_valid(output_json)),
  output_json_sha256 TEXT NOT NULL,
  output_json_bytes INTEGER NOT NULL,
  pruned_at TEXT,
  PRIMARY KEY (run_id, step_index)
);
```

Do not use `workflow_stages.output_json` as a substitute for this table.

## Required Adjustments to the Current OS 007 Prompt

Add the following constraints to the implementation:

```text
- Do not persist logical step output into workflow_stages.output_json.
- Do not build workflow.step scope from workflow_stages.output_json.
- Persist logical step outputs only on dependency step state for this slice.
- In tests, include a parallel stage with two steps and assert both step outputs remain distinct.
- Add a test that a bogus value in workflow_stages.output_json cannot affect workflow.step resolution.
```

## Fast-Follower Adjustments

The earlier pruning idea should be refined:

1. Membership-level output JSON can be aggressively pruned after step aggregation.
2. Step-level output JSON cannot be pruned immediately after a stage completes because OS 008/012 allow later stages to reference any prior `workflow.step[index]`.
3. The conservative safe prune point for step-level output JSON is workflow terminal state.
4. More aggressive pre-terminal pruning is only safe if the controller analyzes all future workflow expressions and proves a given step output cannot be referenced again. That analysis is out of scope for the fast follower.

## Final Recommendation

For OS 007 through OS 012:

```text
Step output: canonical, required for workflow.step[index]
Stage output: not canonical, should stay null/unused
Completed work output: raw terminal input to aggregation, byte-limited
Membership output: temporary aggregation buffer, prune after aggregation
Status/logs: metadata only, no full output_json
```

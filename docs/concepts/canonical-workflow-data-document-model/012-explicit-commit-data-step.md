# 012 Explicit Commit Data Step

Status: Proposed

## Objective

Add visible canonical `commit_data` workflow steps using `step.data.outputs`, preserving current publication runtime while temporarily retaining legacy implicit publish planning until OS-013.

## Minimum Model

Primary: `GPT-5.5`, `High` reasoning. First escalation or review: `GPT-5.6-Terra`, `Medium` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

## Required Context

Read:

- OS-006 and OS-011
- `internal/workflow/cache_data_plan.go` commit planner
- `cmd/controller/workflow_outputs.go`
- `cmd/worker/published_asset.go`
- `internal/model/published_data_asset.go`
- current publish archive smoke workflow

## Allowed Production Files

- `internal/workflow/explicit_commit_data.go` new
- `internal/workflow/compile_stage.go`
- `internal/document/workflow.go`
- `internal/model/published_data_asset.go` only if target model requires extension

## Allowed Test Files

- `internal/workflow/explicit_commit_data_test.go`
- worker published asset tests
- controller output tests

## Data State Transition

```text
explicit commit_data step
    + source step artifact reference
    + effective named output target
    -> visible commit_data work item
    -> current publication worker operation
    -> published asset evidence
```


## Implementation Requirements

- Require `work.type: commit_data`.
- Resolve target from project/workflow/output overlay.
- Require an explicit `from.step` and `from.artifact` or equivalent durable lineage.
- Attach current target write resource constraints to this visible work item.
- Preserve overwrite-policy validation and safe destination behavior.
- Do not append publication work to Python steps implicitly.
- Keep legacy planner only until OS-013 removes it.

## Out of Scope

- Global catalog registration.
- Destructive overwrite defaults.
- Implicit publication.
- Cross-workflow artifact references.

## Acceptance Criteria

- Explicit publish archive step compiles and runs.
- Publication evidence identifies source artifact and target.
- Missing artifact or target fails clearly.
- No hidden commit item is generated from canonical compute parameters.
- Existing worker publication tests remain green.

## Test Commands

```bash
go test ./internal/workflow ./cmd/controller ./cmd/worker
```

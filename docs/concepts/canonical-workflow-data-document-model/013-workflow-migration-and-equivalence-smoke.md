# 013 Workflow Migration and Equivalence Smoke

Status: Implemented

## Objective

Migrate repository examples and smoke workflows to the canonical JSON/YAML model, remove legacy implicit data-operator planning, and prove end-to-end equivalence.

## Minimum Model

Primary: `GPT-5.4`, `High` reasoning. First escalation or review: `GPT-5.5`, `Medium` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

## Required Context

Read:

- OS-001 through OS-012
- current smoke scripts and workflows
- current workflow authoring documentation
- `internal/workflow/cache_data_plan.go`
- controller/client submission tests

## Allowed Production Files

- current repository-owned workflow fixtures
- `docs/workflow-authoring-template.md`
- `docs/CUSTOMER_API.md`
- `internal/workflow/cache_data_plan.go`
- controller/client decode compatibility code
- smoke scripts affected by file extension or document shape

## Allowed Test Files

- repository workflow/controller/client tests
- smoke tests for data assets and publication
- new JSON/YAML semantic-equivalence integration tests

## Data State Transition

```text
legacy workflow fixtures
    -> canonical JSON/YAML fixtures
    -> explicit cache_data/compute/commit_data stages
    -> remove legacy parameter scanners
    -> one supported public model
```


## Implementation Requirements

- Migrate the supplied field-boundaries cache, release-data cache, and publish-archive workflows.
- Move provider/binding definitions to project or workflow `data` sections.
- Replace `data_assets` and `publish` compute parameters.
- Add at least one complete JSON/YAML equivalent workflow pair.
- Remove or disable automatic `PlanCacheDataWorkItems` and `PlanCommitDataWorkItems` discovery from compute parameters.
- Reject legacy public workflow shape with a focused migration error.
- Update authoring docs and concept state.
- Run full tests and fixture-sized smoke paths.

## Out of Scope

- Backward compatibility with experimental public shapes.
- Real Google Drive credentials in automated tests.
- Worker-scope materialization.
- Large real Yan-Roy archive smoke.

## Acceptance Criteria

- All repository workflows use canonical shape.
- Explicit data operators are visible in normalized plans and status.
- No legacy compute-parameter scanner generates work.
- JSON and YAML workflows produce the same semantic workflow/data fingerprints.
- Existing fixture outcomes remain equivalent.
- `go test ./...` passes.

## Test Commands

```bash
go test ./...
# Run repository fixture smoke scripts appropriate to the current platform.
```

## Implementation Notes

- 2026-07-11: Repository JSON/YAML fixture search found no checked-in workflow fixture files still using the legacy public wrapper shape.
- 2026-07-11: `go test ./...` passes after disabling normal stage-compilation discovery of compute-side `data_assets` and `publish` planner parameters.

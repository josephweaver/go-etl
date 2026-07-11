# 005 Named Data Tree Overlay and Precedence

Status: Proposed

## Objective

Implement deterministic recursive composition of `project.data`, `workflow.data`, and submission data overrides with workflow-over-project precedence.

## Minimum Model

Codex 5.5, high reasoning. Incorrect merge behavior can silently change data acquisition and scientific inputs.

## Required Context

Read:

- OS-001 through OS-004
- `internal/variable/scope.go`
- `internal/variable/resolver.go`
- controller resolver assembly paths
- data-assets concept and current bound-data models

## Allowed Production Files

- `internal/document/overlay.go` new
- `internal/document/data.go` new
- controller workflow-admission assembly file or focused new helper

## Allowed Test Files

- `internal/document/overlay_test.go`
- controller tests for project/workflow/submission precedence

## Data State Transition

```text
project.data
    -> recursively overlay workflow.data
    -> recursively overlay submission data overrides
    -> validate effective named data tree
```

The resulting effective tree is then used for step binding. The current generic variable resolver is not modified to deep-merge root objects.

## Implementation Requirements

- Merge maps recursively by key.
- Replace scalars and lists from later sources.
- Reject object/scalar structural type mismatches.
- Reject null as a deletion mechanism.
- Preserve deterministic map ordering for fingerprints.
- Ensure workflow values override project values.
- Ensure submission data overrides workflow values.
- Keep step usage fields separate from the global effective data definition.
- Add diagnostics identifying both the effective path and source layer.

## Out of Scope

- Asset schema details.
- Fan-out parameter resolution.
- Materialization execution.
- Generic deep merge inside `internal/variable.Resolver`.

## Acceptance Criteria

- Project default selection `[raster, header]` becomes `[header]` when workflow overrides it.
- Workflow provider-location leaf overrides project leaf while sibling project fields remain inherited.
- Lists replace rather than concatenate.
- Type mismatch and null tests fail clearly.
- Effective data tree is stable across JSON/YAML source encoding.

## Test Commands

```bash
go test ./internal/document ./cmd/controller
```

# 007 Parameterized Asset Instantiation

Status: Proposed

## Objective

Instantiate canonical asset definitions after fan-out binding, resolve `asset` parameter templates, and compute deterministic asset-instance identity.

## Minimum Model

Primary: `GPT-5.5`, `Extra High` reasoning. First escalation or review: `GPT-5.6-Terra`, `High` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

## Required Context

Read:

- OS-006
- current fan-out compiler and parameter-accessor code
- `internal/variable/reference.go`, `resolver.go`, and `accessor.go`
- `internal/fingerprint`
- `internal/workflow/cache_data_plan.go` asset-key logic

## Allowed Production Files

- `internal/workflow/data_instance.go` new
- `internal/workflow/fanout.go`
- `internal/variable/namespace.go`
- `internal/model/data_definition.go`

## Allowed Test Files

- `internal/workflow/data_instance_test.go`
- focused fan-out tests

## Data State Transition

```text
fan-out item
    -> `fanout` scope
step `with` bindings
    -> validated asset parameter map
    -> `asset` scope
file/cache/location templates
    -> resolved asset instance
    -> canonical asset key
```

The instance still has no concrete materialized path.

## Implementation Requirements

- Add internal `fanout` and `asset` namespaces or equivalent explicit lifecycle scopes.
- Require exact parameter names; reject missing and unknown parameters.
- Resolve parameter references through existing typed resolver/accessor rules.
- Resolve file member/as templates through the asset scope.
- Include effective selection in asset identity.
- Exclude step alias from physical identity.
- Include definition/binding fingerprint and resolved provider facts.
- Produce deterministic diagnostic notation without making it executable syntax.
- Keep Yan-Roy year fixed in file templates; only tile is a parameter.

## Out of Scope

- Materialization execution.
- Worker-local paths.
- Nested function calls in asset parameters.
- Cross-run data catalog.

## Acceptance Criteria

- Tiles `h18v07` and `h18v08` create distinct asset keys.
- Two aliases with identical instance facts create the same key.
- Header-only and raster+header selections create different keys.
- Missing/unknown parameter tests fail before work is queued.
- Resolved Yan-Roy file names contain fixed `2010`.

## Test Commands

```bash
go test ./internal/workflow ./internal/variable ./internal/model
```

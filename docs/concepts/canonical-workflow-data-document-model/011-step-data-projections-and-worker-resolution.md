# 011 Step Data Projections and Worker Resolution

Status: Proposed

## Objective

Build an assignment-local `data` namespace containing ordered `path` lists and named file-role projections, then resolve worker parameters just before execution.

## Minimum Model

Primary: `GPT-5.5`, `High` reasoning. First escalation or review: `GPT-5.6-Terra`, `High` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

## Required Context

Read:

- OS-010
- `internal/variable/namespace.go`, `resolver.go`, and structured accessors
- `cmd/worker/python_arg_binding.go`
- `cmd/worker/work_python.go`
- materialized data manifest model
- assignment-time resolution concept

## Allowed Production Files

- `internal/variable/namespace.go`
- `internal/model/materialized_projection.go` new
- `cmd/worker/data_scope.go` new
- `cmd/worker/python_arg_binding.go`

## Allowed Test Files

- `internal/model/materialized_projection_test.go`
- `cmd/worker/data_scope_test.go`
- `cmd/worker/python_arg_binding_test.go`

## Data State Transition

```text
materialized manifest + consumer alias + effective select order
    -> typed `data.<alias>` object
       path: list[path]
       files: object by role
       metadata/evidence
    -> worker resolver
    -> concrete work parameters
```


## Implementation Requirements

- Add explicit internal `data` namespace.
- `data.<alias>.path` must always be a list.
- Each list element must retain `path` type where the typed model supports it.
- `files.<role>.path` is a scalar path.
- Preserve selection order from the asset instance.
- Reject bare unqualified alias access as a data-binding feature.
- Resolve only assignment/runtime-dependent values at the worker boundary.
- Workers receive normalized data requirements and manifests, not project/workflow source documents.
- Maintain sensitivity/redaction behavior if paths or provider metadata derive from sensitive values.

## Out of Scope

- Worker-scope materialization.
- Plugin-specific implicit argument ordering.
- Direct network access from compute plugins.

## Acceptance Criteria

- Header-only asset exposes one path at index zero.
- Raster/header selection exposes two paths in declared order.
- Named header path matches positional projection.
- Python args resolve `${data.field_segments.path[0]}`.
- `${field_segments}` does not resolve through data-binding magic.
- Resolver depth/cycle/sensitivity tests remain green.

## Test Commands

```bash
go test ./internal/model ./internal/variable ./cmd/worker
```

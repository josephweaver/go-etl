# 008 Materialization Scope and Shared Domain

Status: Proposed

## Objective

Add stable materialization-scope vocabulary, implement `shared` capability validation, and recognize `worker` as valid but not implemented.

## Minimum Model

Codex 5.4-mini or stronger, medium-high reasoning. The code is local but error taxonomy and execution-environment assumptions matter.

## Required Context

Read:

- OS-007
- `internal/model/data_asset.go`
- controller execution-environment configuration
- worker cache-root configuration
- current `target_environment_id` use in cache asset keys

## Allowed Production Files

- `internal/model/data_asset.go`
- `internal/model/materialization_scope.go` new
- focused controller execution-environment validation file
- worker config only if domain ID must be transported

## Allowed Test Files

- `internal/model/materialization_scope_test.go`
- controller execution-environment tests
- worker config tests if touched

## Data State Transition

```text
materialization definition
    -> definition validation (`shared` and `worker` known)
    -> supported-capability validation
       shared -> domain facts
       worker -> sentinel not-implemented error
```


## Implementation Requirements

- Add `scope` to template and bound materialization models.
- Require explicit or inherited scope; no silent default.
- Define `shared` and `worker` constants.
- Add `ErrMaterializationScopeNotImplemented` or equivalent sentinel.
- Unknown scope is invalid; `worker` is known but unsupported.
- Define a stable shared materialization domain ID, preferably from target execution environment configuration rather than path text.
- Include scope and domain in materialization lookup identity.
- Ensure errors retain JSON/YAML document path context.

## Out of Scope

- Worker-local materialization.
- Worker cache locks.
- Worker affinity.
- Shared filesystem discovery.

## Acceptance Criteria

- `shared` passes when a shared domain is configured.
- `shared` fails clearly without required domain facts.
- `worker` returns `errors.Is(err, ErrMaterializationScopeNotImplemented) == true`.
- unknown scope fails as invalid, not not-implemented.
- scope/domain affect identity tests.

## Test Commands

```bash
go test ./internal/model ./cmd/controller ./cmd/worker
```

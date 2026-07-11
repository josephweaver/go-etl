# 003 Canonical Typed Variable Loader

Status: Proposed

## Objective

Extract and generalize the current inline-project literal conversion so ordinary maps under `variables` load into typed variables with an implicit source namespace.

## Minimum Model

Primary: `GPT-5.3-codex-spark`, `High` reasoning. First escalation or review: `GPT-5.4-mini`, `Medium` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

## Required Context

Read:

- OS-001 and OS-002
- `cmd/controller/main.go` functions `projectVariablesFromJSON` and `typedExpressionFromJSON`
- `internal/variable/expression.go`
- `internal/variable/variable.go`
- `internal/variable/namespace.go`
- structured-variable-resolution concept

## Allowed Production Files

- `internal/document/variables.go` new
- `internal/variable/namespace.go` only if aliases/constants are required
- `cmd/controller/main.go` only to call the shared loader

## Allowed Test Files

- `internal/document/variables_test.go`
- focused controller tests proving inline and source-referenced project parity

## Data State Transition

```text
canonical variables map + source namespace
        -> sorted named variable entries
        -> recursive typed literal expressions
        -> validated variable scope
```

Structural document fields remain outside the variable scope.

## Implementation Requirements

- Map JSON/YAML string, integer, boolean, list, and object values to existing typed expressions.
- Reject null and unsupported numeric forms.
- Sort map keys before constructing variables for deterministic output.
- Apply `controller_config`, `project_config`, `workflow`, or `override` according to the caller.
- Do not require authors to repeat namespaces.
- Use the same loader for inline and repository-backed project documents.
- Preserve current recursive list/object type information.
- Reserve directive-object recognition for OS-014; ordinary objects must remain ordinary objects in this slice.

## Out of Scope

- Workflow shape cleanup.
- YAML parsing details.
- Function calls.
- Data-tree overlay.

## Acceptance Criteria

- Ordinary project values load as `project_config.*`.
- Ordinary workflow values load as `workflow.*`.
- Inline and repository-backed project paths produce equal scopes.
- Deterministic output ordering is tested.
- Existing internal variable serialization remains valid for persistence/internal transport.

## Test Commands

```bash
go test ./internal/document ./cmd/controller ./internal/variable
```

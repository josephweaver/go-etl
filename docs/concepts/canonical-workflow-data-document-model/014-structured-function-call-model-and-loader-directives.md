# 014 Structured Function Call Model and Loader Directives

Status: Proposed

## Objective

Replace the proposed `$expr` JSON containers and textual call parser with a structured semantic function-call model produced by the canonical variable loader.

## Minimum Model

Codex 5.5, high reasoning. This defines a durable internal semantic boundary and must avoid coupling public directives to runtime JSON.

## Required Context

Read:

- OS-003
- original expression-function-framework README and OS-001/OS-002
- `internal/variable/expression.go`
- `internal/variable/variable.go`
- `internal/variable/reference.go`
- `internal/document/variables.go`

## Allowed Production Files

- `internal/variable/function_call.go` new
- `internal/variable/function_registry.go` new
- `internal/document/expression_directive.go` new
- `internal/document/variables.go`

## Allowed Test Files

- corresponding focused tests

## Data State Transition

```text
public directive object
    -> canonical loader validation
    -> semantic FunctionCallExpression
    -> typed variable expression node
```

`$call`, `$ref`, and `$type` are not preserved as runtime JSON containers.

## Implementation Requirements

- Define `FunctionName`, `FunctionCallExpression`, argument reference nodes, and immutable registry.
- Require namespaced function names.
- Phase-one call arguments are references/accessors only.
- Public call directives require an expected result `$type`.
- Validate exact directive key sets and reject ambiguous ordinary/directive objects.
- Do not add resolved value type `expression`.
- Do not add `$expr` handling to `TypedExpression.UnmarshalJSON`.
- Keep function namespaces separate from variable namespaces.
- Add an empty/default registry without concrete functions in this slice.

## Out of Scope

- Function evaluation.
- Concrete built-ins.
- Textual call parsing.
- Nested calls, literals, operators, or conditions.

## Acceptance Criteria

- Structured call directive normalizes successfully.
- `$expr` is not accepted as a new internal typed-expression payload.
- Unknown function names may parse but remain unresolved until registry lookup.
- Duplicate registration is impossible or rejected.
- Ordinary objects containing non-directive `$`-like data are handled according to the exact reserved-key rule.

## Test Commands

```bash
go test ./internal/document ./internal/variable
```

# 015 Resolver JIT Function Evaluation

Status: Proposed

## Objective

Evaluate semantic function calls inside the existing typed resolver while preserving normal argument resolution, accessors, depth, cycles, result types, sensitivity, and provenance.

## Minimum Model

Codex 5.5, high reasoning. This is the most delicate expression-runtime slice.

## Required Context

Read:

- OS-014
- `internal/variable/resolver.go`
- `internal/variable/expression.go`
- sensitivity and protected-reference code/tests
- accessor and cycle/depth tests

## Allowed Production Files

- `internal/variable/resolver.go`
- `internal/variable/function_call.go`
- `internal/variable/function_registry.go`

## Allowed Test Files

- `internal/variable/function_evaluation_test.go` new
- existing resolver tests

## Data State Transition

```text
semantic call expression
    -> resolve each argument normally
    -> aggregate sensitivity/provenance
    -> registry function evaluation
    -> validate expected result type
    -> resolved value
```


## Implementation Requirements

- Add registry to resolver construction/config without global mutation.
- Resolve reference/accessor arguments through normal resolver logic.
- Count function arguments in normal depth/cycle behavior.
- Propagate sensitivity if any argument is sensitive.
- Require returned type to equal the enclosing expected type.
- Produce clear unknown-function, arity, argument-type, and result-type errors.
- Keep functions pure and deterministic.
- Do not allow functions to read resolver internals beyond supplied values.

## Out of Scope

- Concrete functions.
- Nested calls.
- Runtime plugin invocation.
- File/network/environment access.

## Acceptance Criteria

- Mock functions evaluate successfully.
- Unknown function and type mismatch tests fail clearly.
- Sensitive argument produces sensitive result.
- Cycle and maximum-depth safeguards still apply.
- Existing literal/reference/interpolation behavior is unchanged.

## Test Commands

```bash
go test ./internal/variable
```

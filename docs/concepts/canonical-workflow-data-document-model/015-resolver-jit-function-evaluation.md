# 015 Resolver JIT Function Evaluation

Status: Implemented

## Objective

Evaluate semantic function calls inside the existing typed resolver while preserving normal argument resolution, accessors, depth, cycles, result types, sensitivity, and provenance.

## Minimum Model

Primary: `GPT-5.5`, `Extra High` reasoning. First escalation or review: `GPT-5.6-Terra`, `Extra High` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

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

## Implementation Notes

- 2026-07-11: `ResolverConfig` now accepts an immutable `FunctionRegistry`; no global mutable registry was introduced.
- 2026-07-11: `Resolver` evaluates semantic `FunctionCallExpression` nodes just-in-time, resolves `$ref` arguments through normal reference/accessor logic, enforces expected result type, and propagates argument sensitivity/provenance to the function result.
- 2026-07-11: Focused tests cover successful mock-function evaluation, unknown function errors, function-returned arity and argument-type errors, result-type mismatch, sensitive arguments, reference cycles, and max-depth enforcement.
- 2026-07-11: `go test ./internal/variable` passes.

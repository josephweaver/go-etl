# 003 Resolver JIT Function Evaluation

Status: proposed

## Objective

Hook functional-expression evaluation into `internal/variable.Resolver` so a typed variable can compute its value from other variables during normal resolution.

Example target behavior:

```json
{
  "name": {"namespace": "workflow", "key": "pairs"},
  "type": "list",
  "expression": {"$expr": "list.crossproduct(A, B)"}
}
```

Resolving `pairs` should resolve `A` and `B`, call `list.crossproduct`, check that the result is a `list`, and return the computed `ResolvedValue`.

## Minimum Model

Codex 5.4, high reasoning.

This slice touches resolver recursion, depth limits, cycle detection, type checking, accessors, and sensitivity behavior.

## Required Context

Read these files first:

- `docs/concepts/expression-function-framework/README.md`
- `docs/concepts/expression-function-framework/001-expression-container-forms.md`
- `docs/concepts/expression-function-framework/002-namespaced-function-parser-and-registry.md`
- `internal/variable/resolver.go`
- `internal/variable/expression.go`
- `internal/variable/variable.go`
- `internal/variable/accessor.go`
- `internal/variable/resolver_test.go`

## Allowed Production Files

- `internal/variable/resolver.go`
- `internal/variable/expression_function.go`
- `internal/variable/function_call.go`
- `internal/variable/function_registry.go`

## Allowed Test Files

- `internal/variable/resolver_test.go`
- `internal/variable/expression_function_test.go`
- `internal/variable/function_registry_test.go`

## Resolver Configuration

Extend resolver configuration to accept a function registry, or use an internal default registry when none is supplied.

Recommended behavior:

- `NewResolver` should remain easy to call from existing code.
- Existing callers that pass no registry should retain current behavior plus default built-ins once they exist.
- Tests should be able to inject a tiny fake function registry.

## Evaluation Rules

When `resolveExpression` sees a `FunctionalExpression` payload:

1. parse the call source;
2. look up the function in the registry;
3. resolve each argument using normal resolver reference/accessor rules;
4. call the function with ordered resolved arguments;
5. verify returned type equals the enclosing `TypedExpression.Type`;
6. conservatively propagate sensitivity from sensitive arguments to scalar outputs unless the returned structure already preserves sensitivity in copied child values;
7. return the computed `ResolvedValue`.

Argument resolution must use the existing recursion depth and chain context. A function expression must not create a path around cycle detection.

## Error Rules

Errors should identify:

- malformed function call;
- unknown function name;
- failed argument reference parsing;
- missing argument variable;
- accessor failure;
- function arity/type error;
- result type mismatch.

Where possible, reuse existing JSON Pointer-style location wrapping from the resolver.

## Out Of Scope

- Adding concrete built-in functions, except for fake test functions inside tests.
- Supporting nested function calls.
- Supporting literal function arguments.
- Changing fan-out syntax.
- Changing worker runtime behavior.
- Calling plugins or any external system.

## Acceptance Criteria

- A fake registered function can be evaluated through a variable's expression payload.
- Function arguments are resolved through normal namespace precedence.
- Function argument accessors work.
- Missing function produces a clear error.
- Function arity/type errors propagate clearly.
- Function result type mismatch is rejected.
- A reference cycle through a functional expression is detected.
- Maximum depth still applies through functional-expression argument resolution.
- Sensitive argument values conservatively mark the result sensitive when the function output does not otherwise preserve child sensitivity.
- Existing resolver tests continue to pass.

## Suggested Tests

Add resolver tests with fake functions:

- `test.echo(A)` returns `A` and respects declared type.
- `test.firstPair(A, B)` returns a list containing both resolved values.
- Unknown `test.missing(A)` errors.
- `test.echo(A.prop)` resolves field accessor.
- `A -> function(B)` and `B -> function(A)` reports a cycle.
- Function returns `int` while enclosing type is `list` reports a type mismatch.

## Test Command

```bash
go test ./internal/variable
```

# 007 list.length Function

Status: proposed

## Objective

Add exactly one built-in expression function:

```text
list.length(items)
```

It returns the number of items in a resolved list.

## Minimum Model

Codex 5.3-codex-spark, low reasoning.

This is the smallest function slice once the registry and resolver integration exist.

## Required Context

Read these files first:

- `docs/concepts/expression-function-framework/README.md`
- `docs/concepts/expression-function-framework/003-resolver-jit-function-evaluation.md`
- `internal/variable/functions_list.go`
- `internal/variable/function_registry.go`
- `internal/variable/variable.go`

## Allowed Production Files

- `internal/variable/functions_list.go`
- `internal/variable/function_registry.go`

## Allowed Test Files

- `internal/variable/functions_list_test.go`
- `internal/variable/resolver_test.go` only if needed to prove registration through resolver

## Function Contract

Name:

```text
list.length
```

Arity:

```text
1
```

Argument type:

```text
items: list
```

Result type:

```text
int
```

The return value is `len(items.List)`.

## Sensitivity

The length of a sensitive list can leak information. If the input list is sensitive, the returned int should be marked sensitive with provenance tied to the sensitive argument.

## Out Of Scope

- Counting object fields.
- Counting string length.
- Counting recursively nested list items.
- Adding math functions.
- Adding comparison or conditional expressions.

## Acceptance Criteria

- `list.length([])` returns `0`.
- `list.length([1,2,3])` returns `3`.
- Non-list argument is rejected.
- Wrong arity is rejected.
- Function is registered in the default registry.
- Resolver can evaluate a variable with declared type `int` using `{"$expr":"list.length(A)"}`.
- If the input list is sensitive, the returned int is sensitive.
- If the enclosing variable declares type `list`, a `list.length(A)` result is rejected as a result type mismatch by resolver integration.

## Suggested Tests

Use direct function tests for:

- zero length;
- positive length;
- wrong argument type;
- wrong arity;
- sensitive input sensitivity propagation.

Use one resolver test for declared type `int` success and one resolver test for declared type mismatch.

## Test Command

```bash
go test ./internal/variable
```

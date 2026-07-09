# 006 list.flatten Function

Status: proposed

## Objective

Add exactly one built-in expression function:

```text
list.flatten(items)
```

It flattens one level of a list of lists.

## Minimum Model

Codex 5.3-codex-spark, medium reasoning.

This is a narrow pure-function slice after the expression framework exists.

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
list.flatten
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
list
```

Each top-level item of `items` must itself be a list. The output concatenates those child lists in order.

Example:

```text
list.flatten([[1,2], [3], []]) -> [1,2,3]
```

This is one-level flatten only.

## Sensitivity

Preserve sensitivity carried by copied child values. If the input list or any child list is sensitive for a reason not represented in child values, conservatively mark the output list sensitive.

## Out Of Scope

- Recursive deep flattening.
- Flattening object fields.
- Ignoring or passing through non-list children.
- Adding any other list function.
- Changing accessor behavior.

## Acceptance Criteria

- `list.flatten([[1,2],[3],[]])` returns `[1,2,3]` as resolved values.
- Empty outer list returns empty list.
- Empty child lists are skipped naturally.
- A non-list outer argument is rejected.
- A non-list child item is rejected with an index-specific error.
- Wrong arity is rejected.
- Item types are preserved.
- Sensitivity metadata is preserved or conservatively propagated.
- Function is registered in the default registry.
- Resolver can evaluate a variable using `{"$expr":"list.flatten(A)"}`.

## Suggested Tests

Use direct function tests for:

- normal one-level flatten;
- empty outer list;
- empty child lists;
- non-list child error;
- wrong arity;
- wrong argument type;
- heterogeneous child values.

Use one resolver test if needed.

## Test Command

```bash
go test ./internal/variable
```

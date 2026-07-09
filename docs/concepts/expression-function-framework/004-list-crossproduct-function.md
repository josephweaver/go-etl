# 004 list.crossproduct Function

Status: proposed

## Objective

Add exactly one built-in expression function:

```text
list.crossproduct(left, right)
```

It returns a list of two-item lists representing the Cartesian product of two resolved input lists.

## Minimum Model

Codex 5.3-codex-spark, medium reasoning.

This is a narrow pure-function slice after the expression framework exists. Escalate to Codex 5.4-mini if failures involve sensitivity propagation or resolver integration rather than local function behavior.

## Required Context

Read these files first:

- `docs/concepts/expression-function-framework/README.md`
- `docs/concepts/expression-function-framework/003-resolver-jit-function-evaluation.md`
- `internal/variable/function_registry.go`
- `internal/variable/variable.go`
- `internal/variable/resolver.go`

## Allowed Production Files

- `internal/variable/functions_list.go` new file allowed
- `internal/variable/function_registry.go`

## Allowed Test Files

- `internal/variable/functions_list_test.go` new file allowed
- `internal/variable/resolver_test.go` only if needed to prove registration through resolver

## Function Contract

Name:

```text
list.crossproduct
```

Arity:

```text
2
```

Argument types:

```text
left:  list
right: list
```

Result type:

```text
list
```

For every item `l` in `left` and every item `r` in `right`, emit one pair:

```text
[l, r]
```

The output order is stable and left-major:

```text
left[0], right[0]
left[0], right[1]
...
left[1], right[0]
```

Each pair is represented as a resolved `list` containing exactly two items.

## Sensitivity

Preserve sensitivity carried by copied child values. If either source item is sensitive, the pair should be sensitive through normal list aggregation. If either input list is sensitive for a reason not represented in its child values, conservatively mark the output list sensitive.

## Out Of Scope

- Adding any other list function.
- Supporting more than two input lists.
- Returning object pairs such as `{left: ..., right: ...}`.
- Deduplicating pairs.
- Sorting pairs.
- Evaluating nested expressions.
- Changing fan-out token behavior for pair lists.

## Acceptance Criteria

- `list.crossproduct([1,2], ["a","b"])` returns four pairs in left-major order.
- Empty left list returns an empty list.
- Empty right list returns an empty list.
- Non-list first argument is rejected with a clear error.
- Non-list second argument is rejected with a clear error.
- Wrong arity is rejected with a clear error.
- Pair values preserve original item types.
- Pair values preserve sensitivity metadata through list aggregation.
- Function is registered in the default registry.
- Resolver can evaluate a variable using `{"$expr":"list.crossproduct(A, B)"}`.

## Suggested Tests

Use direct function tests for:

- normal two-by-two product;
- heterogeneous item preservation;
- empty left;
- empty right;
- wrong arity;
- wrong argument type.

Use one resolver test for default-registry integration.

## Test Command

```bash
go test ./internal/variable
```

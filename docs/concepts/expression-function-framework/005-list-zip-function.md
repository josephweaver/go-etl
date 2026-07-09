# 005 list.zip Function

Status: proposed

## Objective

Add exactly one built-in expression function:

```text
list.zip(left, right)
```

It returns a list of two-item lists pairing equal-index items from two input lists.

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
list.zip
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

For each index `i`, emit:

```text
[left[i], right[i]]
```

The two input lists must have the same length. Length mismatch is an error.

## Sensitivity

Preserve sensitivity carried by copied child values. If either source item is sensitive, the pair should be sensitive through normal list aggregation. If either input list is sensitive for a reason not represented in its child values, conservatively mark the output list sensitive.

## Out Of Scope

- Adding any other list function.
- Zipping more than two lists.
- Truncating to the shorter list.
- Padding to the longer list.
- Returning object pairs.
- Changing fan-out behavior.

## Acceptance Criteria

- `list.zip([1,2], ["a","b"])` returns `[[1,"a"],[2,"b"]]` as resolved pair lists.
- Empty lists zip to an empty list.
- Length mismatch is rejected.
- Non-list first argument is rejected.
- Non-list second argument is rejected.
- Wrong arity is rejected.
- Pair values preserve original item types.
- Pair values preserve sensitivity metadata.
- Function is registered in the default registry.
- Resolver can evaluate a variable using `{"$expr":"list.zip(A, B)"}`.

## Suggested Tests

Use direct function tests for:

- normal zip;
- empty zip;
- length mismatch;
- wrong arity;
- wrong argument type;
- heterogeneous item preservation.

Use one resolver test for default-registry integration if OS-004 did not already establish a reusable pattern.

## Test Command

```bash
go test ./internal/variable
```

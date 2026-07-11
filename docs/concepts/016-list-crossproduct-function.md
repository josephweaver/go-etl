# 016 List Crossproduct Function

Status: Proposed

## Objective

Add exactly `list.crossproduct(left, right)` to the expression-function registry.

## Minimum Model

Codex 5.3-spark or stronger, medium reasoning, after OS-015 is green.

## Required Context

Read OS-014, OS-015, and list/resolved-value constructors.

## Allowed Production Files

- one focused new file under `internal/variable`, such as `function_list_crossproduct.go`
- default registry construction file

## Allowed Test Files

- focused crossproduct test file

## Data State Transition

```text
left list + right list
    -> left-major ordered list
    -> each pair represented as a two-item list
```


## Implementation Requirements

- Require exactly two list arguments.
- Preserve each item’s resolved type and sensitivity.
- Produce left-major deterministic ordering.
- Return empty list when either input is empty.
- Do not mutate input lists.

## Out of Scope

- Cartesian products of more than two lists.
- Named tuple objects.
- Filtering or limits.

## Acceptance Criteria

- `[1,2] x [a,b]` returns `[[1,a],[1,b],[2,a],[2,b]]`.
- Empty input behavior is tested.
- Arity and type errors are tested.
- Sensitive item propagation is tested.

## Test Commands

```bash
go test ./internal/variable
```

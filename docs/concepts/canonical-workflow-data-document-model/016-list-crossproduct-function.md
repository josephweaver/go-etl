# 016 List Crossproduct Function

Status: Implemented

## Objective

Add exactly `list.crossproduct(left, right)` to the expression-function registry.

## Minimum Model

Primary: `GPT-5.3-codex-spark`, `Medium` reasoning. First escalation or review: `GPT-5.4-mini`, `Light` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

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

## Implementation Notes

- 2026-07-11: `DefaultFunctionRegistry` now registers exactly `list.crossproduct` as the first concrete expression function.
- 2026-07-11: `list.crossproduct(left, right)` requires exactly two list arguments and returns left-major two-item list pairs while preserving each item value, type, and sensitivity.
- 2026-07-11: Empty left or right inputs return an empty list.
- 2026-07-11: `go test ./internal/variable` passes.

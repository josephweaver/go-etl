# 017 List Zip Function

Status: Implemented

## Objective

Add exactly `list.zip(left, right)` to the expression-function registry.

## Minimum Model

Codex 5.3-spark or stronger, medium reasoning.

## Required Context

Read OS-014 through OS-016 and existing list helpers.

## Allowed Production Files

- one focused new list-zip production file
- default registry construction file

## Allowed Test Files

- focused list-zip test file

## Data State Transition

```text
equal-length left/right lists
    -> ordered list of two-item lists by index
```


## Implementation Requirements

- Require exactly two lists.
- Require equal length.
- Preserve item types and order.
- Return empty list for two empty lists.
- Do not truncate silently.

## Out of Scope

- Variadic zip.
- Padding.
- Object output.

## Acceptance Criteria

- Equal-length success is tested.
- Length mismatch fails clearly.
- Empty lists and heterogeneous item types are tested.

## Test Commands

```bash
go test ./internal/variable
```

## Implementation Notes

- 2026-07-11: `DefaultFunctionRegistry` now registers `list.zip` alongside `list.crossproduct`.
- 2026-07-11: `list.zip(left, right)` requires exactly two list arguments of equal length and returns ordered same-index two-item list pairs.
- 2026-07-11: Empty left and right lists return an empty list; mismatched lengths fail clearly rather than truncating.
- 2026-07-11: `go test ./internal/variable` passes.

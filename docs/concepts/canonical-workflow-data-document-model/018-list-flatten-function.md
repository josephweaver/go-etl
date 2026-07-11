# 018 List Flatten Function

Status: Implemented

## Objective

Add exactly `list.flatten(items)` to flatten one list nesting level.

## Minimum Model

Primary: `GPT-5.3-codex-spark`, `Medium` reasoning. First escalation or review: `GPT-5.4-mini`, `Light` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

## Required Context

Read OS-014 through OS-017 and resolved list semantics.

## Allowed Production Files

- one focused flatten production file
- default registry construction file

## Allowed Test Files

- focused flatten test file

## Data State Transition

```text
list of lists
    -> concatenate child items in order
    -> one-level flattened list
```


## Implementation Requirements

- Require exactly one list argument.
- Require every top-level item to be a list.
- Flatten exactly one level.
- Preserve order, types, and sensitivity.
- Do not recursively flatten deeper levels.

## Out of Scope

- Recursive flatten.
- Scalar coercion.
- Depth parameter.

## Acceptance Criteria

- One-level success, empty children, nested-list preservation, and scalar-child errors are tested.

## Test Commands

```bash
go test ./internal/variable
```

## Implementation Notes

- 2026-07-11: `DefaultFunctionRegistry` now registers `list.flatten` alongside `list.crossproduct` and `list.zip`.
- 2026-07-11: `list.flatten(items)` requires exactly one list argument, requires every top-level item to be a list, and concatenates child items in order.
- 2026-07-11: Flattening is intentionally one level only; nested child lists are preserved rather than recursively flattened.
- 2026-07-11: Empty child lists contribute no items, scalar children fail clearly, and item sensitivity is preserved.
- 2026-07-11: `go test ./internal/variable` passes.

# 018 List Flatten Function

Status: Proposed

## Objective

Add exactly `list.flatten(items)` to flatten one list nesting level.

## Minimum Model

Codex 5.3-spark or stronger, medium reasoning.

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

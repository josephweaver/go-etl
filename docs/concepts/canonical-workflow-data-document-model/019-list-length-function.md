# 019 List Length Function

Status: Proposed

## Objective

Add exactly `list.length(items)` returning typed integer length.

## Minimum Model

Codex 5.3-spark or stronger, low-medium reasoning.

## Required Context

Read OS-014 through OS-018 and integer resolved-value helpers.

## Allowed Production Files

- one focused length production file
- default registry construction file

## Allowed Test Files

- focused length test file

## Data State Transition

```text
resolved list
    -> count items
    -> resolved int
```


## Implementation Requirements

- Require exactly one list argument.
- Return zero for an empty list.
- Preserve sensitivity/provenance according to framework rules.
- Use existing integer type conventions.

## Out of Scope

- String/object length.
- Generic collection reflection.

## Acceptance Criteria

- Empty, nonempty, wrong-type, and wrong-arity tests pass.

## Test Commands

```bash
go test ./internal/variable
```

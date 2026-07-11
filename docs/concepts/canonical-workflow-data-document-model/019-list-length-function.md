# 019 List Length Function

Status: Implemented

## Objective

Add exactly `list.length(items)` returning typed integer length.

## Minimum Model

Primary: `GPT-5.3-codex-spark`, `Light` reasoning. First escalation or review: `GPT-5.4-mini`, `Light` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

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

## Implementation Notes

- 2026-07-11: `DefaultFunctionRegistry` now registers `list.length` alongside the other default list functions.
- 2026-07-11: `list.length(items)` requires exactly one list argument and returns an existing typed integer value with the top-level item count.
- 2026-07-11: Empty lists return `0`; non-list arguments and wrong arity fail clearly.
- 2026-07-11: Sensitivity/provenance propagation remains resolver-owned: a sensitive list argument produces a sensitive integer result through the existing function-call framework.
- 2026-07-11: `go test ./internal/variable` passes.

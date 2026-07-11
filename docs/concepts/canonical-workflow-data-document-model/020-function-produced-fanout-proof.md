# 020 Function Produced Fanout Proof

Status: Implemented

## Objective

Prove that a canonical-loader structured function call can produce a list variable consumed by existing reference-based fan-out and, where relevant, parameterized data-asset instantiation.

## Minimum Model

Primary: `GPT-5.3-codex-spark`, `High` reasoning. First escalation or review: `GPT-5.4-mini`, `High` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

## Required Context

Read all expression slices, OS-004, OS-007, and workflow fan-out compiler tests.

## Allowed Production Files

- no broad production changes expected
- focused integration helper only if necessary

## Allowed Test Files

- `internal/workflow/function_fanout_test.go` new
- optional document/controller integration test

## Data State Transition

```text
JSON/YAML variables with structured list.crossproduct call
    -> canonical semantic call
    -> resolved list
    -> `${workflow.pairs[*]}` fan-out
    -> stable work items and optional asset parameters
```


## Implementation Requirements

- Use the normal reference-based fan-out surface; fan-out must not parse function calls directly.
- Prove JSON and YAML variants produce equivalent work-item identities.
- Prove pair accessors bind expected values.
- Include at least one fixture where function-produced fan-out supplies a data-asset parameter, without adding hidden cache work.

## Out of Scope

- Performance optimization for huge products.
- Nested function calls.
- General expression syntax inside fan-out.

## Acceptance Criteria

- Function-produced list fans out successfully.
- Stable IDs and ordering are deterministic.
- Data asset instance keys use the bound pair values.
- No regression in ordinary list fan-out.
- `go test ./...` passes.

## Test Commands

```bash
go test ./internal/document ./internal/variable ./internal/workflow ./cmd/controller
go test ./...
```

## Implementation Notes

- 2026-07-11: `internal/workflow/function_fanout_test.go` proves a canonical JSON/YAML structured function call can produce a workflow list variable consumed by normal `${workflow.pairs[*]}` fan-out.
- 2026-07-11: The proof uses `list.flatten` to produce object pair values because the existing canonical fan-out and asset-binding surfaces support `fanout.<field>` accessors without adding new function-call syntax to fan-out.
- 2026-07-11: JSON and YAML variants compile to equivalent ordered cache work identities, pair-field bindings, selected archive members, and deterministic asset keys.
- 2026-07-11: The proof includes visible `cache_data` work whose data-asset parameters come from function-produced fan-out values; no hidden cache work path is used.
- 2026-07-11: Ordinary literal-list fan-out remains covered in the same proof file.

# 008 Fan-out Integration Proof

Status: proposed

## Objective

Prove that an expression-produced list can drive the existing reference-based fan-out surface without making the workflow compiler parse function calls directly.

Target pattern:

```json
{
  "name": {"namespace": "workflow", "key": "pairs"},
  "type": "list",
  "expression": {"$expr": "list.crossproduct(years, regions)"}
}
```

Fan-out expression remains:

```text
${pairs[*]}
```

## Minimum Model

Codex 5.4-mini, medium reasoning.

The change should be mostly tests, but it crosses `internal/variable` and `internal/workflow` assumptions.

## Required Context

Read these files first:

- `docs/concepts/expression-function-framework/README.md`
- `docs/concepts/expression-function-framework/004-list-crossproduct-function.md`
- `internal/variable/resolver.go`
- `internal/workflow/fanout.go`
- `internal/workflow/fanout_test.go`
- `internal/workflow/README.md`

## Allowed Production Files

No production files are expected.

If a production edit appears necessary, stop and explain why the framework slices did not make expression-produced lists indistinguishable from other resolved lists.

## Allowed Test Files

- `internal/workflow/fanout_test.go`
- `internal/variable/resolver_test.go` only if an additional resolver proof is needed

## Out Of Scope

- Changing fan-out syntax.
- Allowing `list.crossproduct(A, B)[*]` directly in the fan-out field.
- Changing workflow compilation ownership.
- Changing work-item ID or output token rules for list pairs.
- Adding more list functions.

## Acceptance Criteria

- A workflow fan-out test defines a list variable using `list.crossproduct`.
- `ResolveFanOutExpression("${pairs[*]}")` returns the expression-produced pairs.
- `CompileFanOutWorkItemResults` can use accessors such as `[0]` and `[1]` against each pair.
- Generated work-item parameters can bind the first and second pair elements through existing `ParameterAccessors`.
- No workflow production code needs to parse or understand function-call syntax.
- Existing fan-out tests continue to pass.

## Suggested Test Shape

Use small variables:

```text
years   = [2010, 2011]
regions = ["h18v07", "h18v08"]
pairs   = list.crossproduct(years, regions)
```

Compile fan-out from:

```text
${pairs[*]}
```

Use parameter accessors:

```text
year   -> [0]
region -> [1]
```

Verify four compiled work items in left-major order.

## Test Command

```bash
go test ./internal/variable ./internal/workflow
```

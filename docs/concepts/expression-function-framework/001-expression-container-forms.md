# 001 Expression Container Forms

Status: proposed

## Objective

Add an explicit functional-expression payload shape to `internal/variable.TypedExpression` without changing the declared result type of variables.

Support both agreed forms:

```json
{"$expr": "list.crossproduct(A, B)"}
```

and

```json
{"type": "expression", "value": "list.crossproduct(A, B)"}
```

The enclosing typed variable still declares the expected resolved type:

```json
{
  "type": "list",
  "expression": {"$expr": "list.crossproduct(A, B)"}
}
```

## Minimum Model

Codex 5.4-mini, medium reasoning.

This is mostly JSON decoding and definition validation, but it is easy to accidentally confuse expression-container type with resolved value type.

## Required Context

Read these files first:

- `docs/concepts/expression-function-framework/README.md`
- `internal/variable/README.md`
- `internal/variable/type.go`
- `internal/variable/expression.go`
- `internal/variable/variable.go`
- `internal/variable/literal.go`
- `internal/variable/expression_test.go`
- `internal/variable/variable_test.go`

## Allowed Production Files

- `internal/variable/expression.go`
- `internal/variable/variable.go`
- `internal/variable/expression_function.go` new file allowed

## Allowed Test Files

- `internal/variable/expression_test.go`
- `internal/variable/variable_test.go`

## Implementation Notes

Add a small internal representation such as:

```go
type FunctionalExpression struct {
    Source string
}
```

The exact name may vary, but the representation should preserve only the expression source string at this slice.

Modify expression decoding so that before decoding an expression payload according to the declared result type, the decoder recognizes:

```json
{"$expr": "..."}
```

and

```json
{"type": "expression", "value": "..."}
```

as functional-expression containers.

Validation in this slice should only reject empty or non-string expression sources. Full call parsing belongs to OS-002.

The variable type vocabulary should not add `expression` as a resolved value type. `expression` is only a payload form.

## Out Of Scope

- Parsing function-call syntax.
- Registering functions.
- Evaluating functions.
- Allowing function calls inside `${...}`.
- Changing existing scalar, list, object, or interpolation behavior.
- Adding any specific built-in function.

## Acceptance Criteria

- A variable with declared type `list` and expression payload `{"$expr":"list.crossproduct(A, B)"}` decodes successfully.
- A variable with declared type `list` and expression payload `{"type":"expression","value":"list.crossproduct(A, B)"}` decodes successfully.
- Empty expression source is rejected.
- Non-string expression source is rejected.
- A variable whose top-level declared type is `expression` is rejected as an unsupported resolved value type.
- Existing literal scalar, object, list, reference, and interpolation tests still pass.
- Marshal behavior for functional expressions is deterministic. Prefer emitting the compact `$expr` form unless existing test style makes preserving form easier.

## Suggested Tests

Add tests covering:

- compact form decoding;
- verbose form decoding;
- missing `$expr` value;
- verbose form with `type: expression` but missing `value`;
- verbose form with unknown fields;
- top-level variable `type: expression` rejection;
- existing typed object/list decode unaffected.

## Test Command

```bash
go test ./internal/variable
```

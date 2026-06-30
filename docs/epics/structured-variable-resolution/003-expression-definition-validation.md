# 003 Expression Definition Validation

Status: proposed

## Objective

Add context-free validation for a `TypedExpression` definition so malformed
types, literals, references, interpolation tokens, and recursive child nodes
can be rejected without requiring project configuration or any other variable
scope.

## Required Context

Read these files first:

- `docs/epics/structured-variable-resolution/README.md`
- `docs/epics/structured-variable-resolution/002-typed-expression-json-model.md`
- `internal/variable/expression.go`
- `internal/variable/expression_test.go`
- `internal/variable/reference.go`
- `internal/variable/accessor.go`

Do not read unrelated files unless test or compile failures directly require
it.

## Allowed Production Files

- `internal/variable/expression.go`

## Allowed Test Files

- `internal/variable/expression_test.go`

## Out Of Scope

- Looking up a referenced variable in any scope.
- Requiring controller, project, workflow, or override context to be present.
- Applying namespace precedence.
- Checking that a referenced value matches the expression node's declared
  type.
- Resolving whole-value references or interpolated text.
- Detecting reference cycles or enforcing maximum resolution depth.
- Changing `Variable.Expression` to use `TypedExpression`.
- Establishing the final nested error-path format used during resolution.
- Migrating or rejecting legacy structured expressions.

## Acceptance Criteria

- Definition validation recursively visits every object field and list item.
- A `string` expression requires a JSON string and accepts literal text,
  syntactically valid whole-value references, and valid interpolation tokens.
- A `path` expression follows the same definition rules as `string`.
- An `int` expression accepts an integer JSON literal or one syntactically
  valid whole-value reference and rejects fractional, string-literal, boolean,
  object, list, and null values.
- A `bool` expression accepts a JSON boolean literal or one syntactically valid
  whole-value reference and rejects other expression shapes.
- A `datetime` expression accepts a valid datetime string literal or one
  syntactically valid whole-value reference.
- Whole-value references accept the existing qualified or unqualified
  reference grammar and supported scalar field or index accessors.
- Embedded interpolation is accepted only for `string` and `path` expressions.
- Multiple interpolation tokens and escaped literal `\${` sequences are
  accepted in string and path expressions.
- Malformed, unterminated, empty, or otherwise invalid reference and
  interpolation tokens return an error.
- Definition validation does not fail merely because a referenced variable or
  namespace scope is absent from the current process.
- Existing variable-package tests continue to pass.

## Notes

- Definition validation answers whether an expression is well formed in
  isolation. It does not answer whether the expression can resolve in a
  particular submission.
- Context-dependent validation occurs later, after controller, project,
  workflow, and override scopes have been assembled.
- Reusing the existing reference and accessor parsers avoids introducing a
  second reference grammar.

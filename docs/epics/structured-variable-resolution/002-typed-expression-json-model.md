# 002 Typed Expression JSON Model

Status: implemented

## Objective

Add the recursive `TypedExpression` data model and its language-neutral JSON
representation so scalar, object, and list nodes encode and decode using the
same `{ "type": ..., "expression": ... }` shape at every layer.

## Required Context

Read these files first:

- `docs/epics/structured-variable-resolution/README.md`
- `docs/epics/structured-variable-resolution/001-generic-list-type.md`
- `internal/variable/type.go`
- `internal/variable/type_test.go`

Do not read unrelated files unless test or compile failures directly require
it.

## Allowed Production Files

- `internal/variable/expression.go` (new)
- `internal/variable/type.go`

## Allowed Test Files

- `internal/variable/expression_test.go` (new)
- `internal/variable/type_test.go`

## Out Of Scope

- Changing `Variable.Expression` to use `TypedExpression`.
- Resolving whole-value references.
- Resolving interpolation in string or path expressions.
- Validating scalar literal values against their declared types.
- Applying namespace precedence or resolution-depth limits.
- Producing nested resolver error paths.
- Parsing, migrating, or rejecting legacy structured expressions.
- Updating controller, workflow, worker, fixture, or demo JSON files.

## Acceptance Criteria

- Every JSON node has exactly the public fields `type` and `expression`.
- Type names use compact strings such as `"string"`, `"object"`, and
  `"list"`; the JSON contract does not expose a Go struct layout.
- A scalar node round-trips without losing its declared type or JSON
  expression value.
- An object node decodes `expression` as an unordered map from field names to
  typed-expression nodes.
- A list node decodes `expression` as an ordered array of independently typed
  expression nodes.
- Empty objects and empty lists are valid representations.
- Nested objects and lists round-trip without losing declared node types,
  object field names, scalar expression values, or list order.
- Missing or unknown types, missing expressions, and object or list expressions
  with the wrong JSON container shape return errors.
- JSON decoding does not infer a child node's type from its expression value.
- Existing variable-package tests continue to pass.

## Notes

- This slice establishes recursive representation and structural JSON
  validation only. Later slices connect the model to `Variable` and perform
  type-specific semantic validation and resolution.
- Scalar expressions may be literals or textual reference/interpolation forms;
  distinguishing and validating those meanings is outside this slice.
- Object field order has no semantic meaning. List item order must be
  preserved.

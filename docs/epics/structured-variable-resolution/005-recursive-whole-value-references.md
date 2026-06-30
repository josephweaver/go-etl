# 005 Recursive Whole-Value References

Status: implemented

## Objective

Resolve whole-value `${...}` references at every typed-expression node so
object fields and list items can reference variables or supported structured
accessors while preserving the node's declared type.

## Required Context

Read these files first:

- `docs/epics/structured-variable-resolution/README.md`
- `docs/epics/structured-variable-resolution/004-variable-typed-expression-integration.md`
- `internal/variable/resolver.go`
- `internal/variable/resolver_test.go`
- `internal/variable/reference.go`
- `internal/variable/accessor.go`

Do not read unrelated files unless test or compile failures directly require
it.

## Allowed Production Files

- `internal/variable/resolver.go`
- `internal/variable/literal.go`

## Allowed Test Files

- `internal/variable/resolver_test.go`
- `internal/variable/literal_test.go`

## Out Of Scope

- Resolving `${...}` tokens mixed with literal string or path text.
- Translating or normalizing resolved paths for an execution environment.
- Changing namespace definitions or precedence.
- Adding arithmetic, conditionals, functions, or other expression forms.
- Establishing the final nested error-path format.
- Migrating additional consumers or demo configuration.
- Adding domain-specific validation for resolved objects or lists.

## Acceptance Criteria

- A whole-value reference may appear at the root variable, in any object
  field, or in any list item.
- Qualified references select their declared namespace, and unqualified
  references use the existing namespace precedence.
- Supported field and scalar index accessors may select a nested resolved value
  before type checking.
- The referenced or selected value must match the referencing node's declared
  type.
- A whole-value reference to an object or list preserves the complete typed
  `ResolvedValue` subtree.
- Heterogeneous and nested lists may contain whole-value references without
  losing the type of any item.
- Missing references and type mismatches return errors rather than falling back
  to literal text.
- Every recursive reference traversal counts toward the configured maximum
  resolution depth, including references originating in object fields and list
  items.
- Cyclic reference chains terminate through bounded resolution rather than
  recursing indefinitely.
- Existing top-level reference, accessor, namespace-precedence, and fan-out
  behavior continues to pass.
- Relevant variable-package tests pass.

## Notes

- A node whose entire textual expression is `${reference}` is a whole-value
  reference. Mixed literal text belongs to the later interpolation slice.
- Declared-type matching prevents a node declared as `int`, for example, from
  silently becoming a resolved `string` merely because its reference points to
  one.
- This slice may report bounded-cycle failures as maximum-depth errors. A later
  error-path slice owns richer diagnostic context and may distinguish cycles
  explicitly.

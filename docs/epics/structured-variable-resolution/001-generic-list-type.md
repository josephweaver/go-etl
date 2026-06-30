# 001 Generic List Type

Status: proposed

## Objective

Replace the parameterized `TypeList(element)` model with one generic `list`
type whose resolved items retain their individual declared types. Move
homogeneous item requirements to the accessors and consumers that need them.

## Required Context

Read these files first:

- `docs/epics/structured-variable-resolution/README.md`
- `internal/variable/type.go`
- `internal/variable/variable.go`
- `internal/variable/literal.go`
- `internal/variable/resolver.go`

Read a listed cleanup file only when updating its direct use of
`TypeList(element)`.

## Allowed Production Files

Primary production files:

- `internal/variable/type.go`
- `internal/variable/variable.go`
- `internal/variable/literal.go`
- `internal/variable/resolver.go`

Required cleanup files:

- `internal/client/local_controller.go`
- `cmd/controller/local_worker.go`
- `cmd/demo-client/main.go`

## Allowed Test Files

- `internal/variable/type_test.go`
- `internal/variable/variable_test.go`
- `internal/variable/literal_test.go`
- `internal/variable/resolver_test.go`
- `internal/variable/accessor_test.go`
- `internal/workflow/fanout_test.go`
- `internal/workflow/workflow_test.go`
- `internal/client/workflow_test.go`
- `internal/client/local_controller_test.go`
- `cmd/controller/local_worker_test.go`

## Out Of Scope

- Adding the recursive `TypedExpression` JSON model.
- Changing `Variable.Expression` from its current representation.
- Resolving references or interpolation inside object fields or list items.
- Migrating or rejecting legacy raw-JSON structured expressions.
- Changing namespace precedence, accessor syntax, or fan-out semantics.
- Refactoring affected consumers beyond replacing list-type assumptions with
  item validation where required.
- Updating demo JSON that does not use the removed homogeneous-list type.

## Acceptance Criteria

- `Type` represents `list` without storing or requiring one element type.
- The `TypeList(element)` constructor and parameterized strings such as
  `list[string]` are removed.
- A resolved list has type `list` and preserves the type of every item.
- A resolved list may contain heterogeneous scalar values, objects, and nested
  lists.
- Empty resolved lists are valid because their type no longer depends on an
  inferred first element.
- `Resolver.StringList` and `OptionalStringList` require a generic list and
  reject any non-string item with an index-specific error.
- Object-field string-list access performs the same per-item validation.
- Existing fan-out and accessor behavior continues to operate on generic
  lists.
- All direct production and test call sites compile without retaining a
  homogeneous-list type assumption.
- Relevant repository tests pass.

## Notes

- List item types remain on each `ResolvedValue`; they are not erased when the
  enclosing list becomes generic.
- Homogeneity is a consumer validation rule, not part of the list's declared
  type.
- Changes outside `internal/variable` are cleanup only. If the generic-list
  change were reverted, those edits would no longer be necessary.
- Legacy literal parsing remains temporary in this slice. The later recursive
  typed-expression slices replace inferred structured literals and define
  their rejection behavior.

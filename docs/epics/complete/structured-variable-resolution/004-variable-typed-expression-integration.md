# 004 Variable Typed-Expression Integration

Status: implemented

## Objective

Make `Variable` a named root `TypedExpression`, migrate direct repository-owned
constructions to that model, and convert fully literal typed-expression trees
into the existing `ResolvedValue` tree without inferring nested types.

## Required Context

Read these files first:

- `docs/epics/structured-variable-resolution/README.md`
- `docs/epics/structured-variable-resolution/002-typed-expression-json-model.md`
- `docs/epics/structured-variable-resolution/003-expression-definition-validation.md`
- `internal/variable/variable.go`
- `internal/variable/literal.go`
- `internal/variable/resolver.go`

Read a listed cleanup file only when migrating its direct construction or use
of `Variable`.

## Allowed Production Files

Primary production files:

- `internal/variable/variable.go`
- `internal/variable/literal.go`
- `internal/variable/resolver.go`

Required cleanup files:

- `cmd/controller/main.go`
- `cmd/demo-client/main.go`
- `cmd/controller/demo-config.json`
- `cmd/controller/controller-default-config.json`
- `cmd/controller/fake-hpcc-ssh-config.json`
- `cmd/controller/local-singularity-config.json`

## Allowed Test Files

- `internal/variable/variable_test.go`
- `internal/variable/literal_test.go`
- `internal/variable/resolver_test.go`
- `internal/variable/scope_test.go`
- `internal/workflow/fanout_test.go`
- `internal/workflow/workflow_test.go`
- `internal/client/workflow_test.go`
- `internal/client/local_controller_test.go`
- `internal/ledger/sqlite_test.go`
- `cmd/controller/config_test.go`
- `cmd/controller/main_test.go`
- `cmd/controller/local_worker_test.go`
- `cmd/controller/worker_launch_config_test.go`

## Out Of Scope

- Resolving references inside object fields or list items.
- Resolving embedded interpolation in string or path expressions.
- Changing namespace precedence.
- Adding contextual reference type validation.
- Changing cycle detection or maximum resolution depth.
- Establishing the final nested resolver error-path format.
- Retaining or accepting the legacy raw-JSON structured-expression model.
- Refactoring migrated callers beyond the representation change required by
  this slice.

## Acceptance Criteria

- `Variable` consists of `Name` plus an embedded root `TypedExpression`.
- Variable JSON uses the flat language-neutral `name`, `type`, and
  `expression` fields rather than nesting the root typed expression under an
  additional property.
- `Variable.Validate` applies name validation and context-free definition
  validation to the embedded typed expression.
- Literal scalar nodes convert to `ResolvedValue` without changing their
  declared types.
- Literal object nodes recursively convert every named field to a typed
  `ResolvedValue` without JSON value-type inference.
- Literal list nodes recursively convert every item, preserving heterogeneous
  item types, nested lists, and order.
- Empty literal objects and lists resolve successfully.
- Existing top-level scalar whole-value reference behavior continues to work;
  expanding reference resolution into structured children remains deferred.
- Repository-owned variable constructions and controller demo JSON use the new
  representation.
- Legacy raw-JSON object and list expression strings are not retained in
  migrated repository-owned usages.
- All direct production and test call sites compile, and relevant repository
  tests pass.

## Notes

- Go struct embedding models the agreed relationship: a variable is a name
  attached to one typed-expression node. It is not inheritance or a union.
- The expression-value representation remains the discriminated portion: its
  concrete shape is governed by the node's declared type.
- Changes outside `internal/variable` are cleanup only. If the variable model
  change were reverted, those edits would no longer be necessary.
- Recursive reference and interpolation behavior belongs to later resolver
  slices; this slice establishes the production data boundary they consume.

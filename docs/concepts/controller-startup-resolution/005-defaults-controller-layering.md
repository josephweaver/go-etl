# 005 Defaults and Controller Layering

Status: proposed

## Objective

Load and retain the selected controller document together with its adjacent
defaults document, then expose separate ordered `controller_config` scopes in
which an explicit controller declaration wins over a matching default without
discarding either source document.

## Required Context

Read these files first:

- `docs/concepts/controller-startup-resolution/README.md`
- `docs/concepts/controller-startup-resolution/004-defaults-document-loading.md`
- `cmd/controller/config.go`
- `cmd/controller/config_test.go`
- `internal/variable/scope.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/config.go`

## Allowed Test Files

- `cmd/controller/config_test.go`

## Out Of Scope

- Wiring the retained sources into `main` or existing service constructors
- Controller environment, command-line override, or generated runtime scopes
- Resolver-depth bootstrap or general startup resolver assembly
- Adding provenance fields to `variable.Variable` or `variable.ResolvedValue`
- Layering non-controller namespaces for workflow submission
- Changing namespace precedence
- Structural execution-environment defaults
- Service construction or readiness behavior

## Acceptance Criteria

- A controller-startup source value retains the controller path, defaults path,
  decoded controller document, and decoded defaults document as distinct data.
- Loading that value derives and requires `defaults.json` beside the selected
  controller path.
- Controller documents must use the canonical `controller_config` namespace;
  the loader rejects other namespaces instead of rewriting them.
- The source value can produce one defaults `controller_config` scope and one
  explicit controller scope without mutating either decoded document.
- Passing the scopes to `variable.NewSet` in their documented order makes an
  explicit controller declaration win over a default with the same qualified
  name.
- A default remains available when the controller document does not replace
  it, and an explicit-only declaration remains available.
- Defaults belonging to `client_config`, `worker_config`, or `project_config`
  remain retained in the defaults document but do not enter the controller
  startup scopes.
- The same unqualified key in a retained non-controller namespace does not
  collide with the controller startup scopes.
- Errors loading either document identify which source path failed.
- Existing canonical checked-in controller documents continue to load.
- Targeted source-loading and layering tests pass.

## Notes

- Preserve the two scopes rather than returning one merged map. Scope order is
  the layering rule; retaining each scope is the minimum source identity needed
  for later provenance work.
- In Go, maps and slices are reference-like values. Tests should prove scope
  construction does not rewrite the decoded declarations when selecting the
  `controller_config` subset.
- This slice removes the transitional `normalizeVariables` behavior. Silent
  namespace rewriting would make diagnostics and provenance disagree with the
  serialized controller document.
- `variable.NewSet(defaultScope, controllerScope)` currently gives the later
  controller scope precedence. Slice 009 will combine these retained scopes
  with environment, override, and runtime inputs for bounded startup decisions.

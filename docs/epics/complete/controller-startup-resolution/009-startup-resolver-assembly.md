# 009 Startup Resolver Assembly

Status: proposed

## Objective

Construct a bounded controller-startup resolver from the retained defaults and
controller documents, parsed startup overrides, generated runtime values, and
an injected controller-environment lookup. Bootstrap and validate
`resolver_max_depth` before constructing the resolver used for subsequent
startup decisions.

## Required Context

Read these files first:

- `docs/epics/controller-startup-resolution/README.md`
- `docs/epics/controller-startup-resolution/005-defaults-controller-layering.md`
- `docs/epics/controller-startup-resolution/006-controller-environment-accessor.md`
- `docs/epics/controller-startup-resolution/007-startup-override-scope.md`
- `docs/epics/controller-startup-resolution/008-generated-startup-runtime-scope.md`
- `cmd/controller/main.go`
- `cmd/controller/config.go`
- `cmd/controller/config_test.go`
- `internal/variable/resolver.go`
- `internal/variable/scope.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/config_test.go`

## Out Of Scope

- Wiring the assembled resolver into `main` or existing service constructors
- Storing a resolver on `Controller` or in package-global state
- Materialized provenance fields, provenance reporting, or provenance APIs
- Adding sensitivity metadata, propagation, redaction, or protected storage
- Database, filesystem, logging, caretaker, HTTP, or recovery contracts
- Removing the temporary live-startup rejection of `--override`
- Changing variable namespace precedence or qualified-reference semantics
- Generating `runtime.controller_recovery_started_at`
- Mutating or collapsing the retained controller and defaults documents

## Acceptance Criteria

- A dedicated function constructs a startup resolver from
  `controllerStartupSources`, an override scope, a runtime scope, and an
  injected controller-environment lookup function.
- The resolver set orders scopes from lowest to highest as defaults
  `controller_config`, explicit `controller_config`, `override`, and `runtime`.
- An unqualified matching key selects runtime over override, override over
  explicit controller config, and explicit controller config over defaults.
- A qualified lookup selects only the requested namespace; in particular,
  `controller_config.KEY`, `override.KEY`, and `runtime.KEY` retain their normal
  qualified behavior.
- A qualified `controller_env.KEY` uses the injected accessor, and an
  unqualified key consults that accessor only when no assembled scope contains
  the key.
- Startup first constructs a bootstrap resolver using
  `variable.DefaultMaxDepth`, resolves unqualified `resolver_max_depth` through
  the assembled startup sources, and requires the result to be an integer
  greater than zero.
- The returned resolver uses the validated `resolver_max_depth` for subsequent
  recursive resolution.
- An override may supply `resolver_max_depth`; a qualified
  `controller_config.resolver_max_depth` still reads the controller namespace.
- Missing, wrong-type, non-positive, or recursively unresolvable
  `resolver_max_depth` values return an error identifying that key.
- Bootstrap and final resolvers independently cache controller-environment
  lookups because they are separate bounded resolution operations.
- The retained defaults and controller documents remain unchanged and
  available through `controllerStartupSources`, preserving source identity for
  later provenance work.
- No resolver is retained beyond the caller-selected bounded startup decision,
  and no resolver is added to `Controller` or package-global state.
- Targeted startup-resolver assembly tests pass.

## Notes

- Go passes the resolver value by value, but its environment cache is shared
  by copies of that one resolver. Constructing the bootstrap and final
  resolvers separately therefore creates two intentionally separate caches.
- Use the existing `controllerScopes` helper so defaults and explicit
  controller declarations remain separate inputs until `variable.NewSet`
  applies precedence.
- Treat `resolver_max_depth` as a bootstrap exception: the built-in limit is
  used only to resolve the configured limit, after which later bounded
  resolvers use the configured value.
- Source identity is retained structurally in `controllerStartupSources` in
  this slice. Adding provenance to `variable.Variable`,
  `variable.ResolvedValue`, or resolver results requires a dedicated later
  slice.

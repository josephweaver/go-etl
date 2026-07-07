# 007 Startup Override Scope

Status: proposed

## Objective

Decode the raw canonical-JSON declarations collected from repeated
`--override` arguments into one validated `override` scope and prove its
precedence behavior without enabling overrides in live controller startup yet.

## Required Context

Read these files first:

- `docs/concepts/controller-startup-resolution/README.md`
- `docs/concepts/controller-startup-resolution/002-startup-command-line-contract.md`
- `docs/concepts/controller-startup-resolution/005-defaults-controller-layering.md`
- `cmd/controller/main.go`
- `cmd/controller/config_test.go`
- `internal/variable/variable.go`
- `internal/variable/scope.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/config_test.go`

## Out Of Scope

- Wiring the override scope into `main` or existing service constructors
- Removing the temporary live-startup rejection of `--override`
- An override-key allowlist or denylist
- Changing qualified versus unqualified lookup semantics
- Sensitivity metadata, secret transport, redaction, or persistence
- Controller environment and generated runtime scope assembly
- Resolver-depth bootstrap or complete startup resolver construction
- Adding an alternative shorthand override syntax

## Acceptance Criteria

- A dedicated function decodes each raw override string through the canonical
  `variable.Variable` JSON schema.
- Scalar, list, and object typed-expression declarations use the same recursive
  validation as serialized variable documents.
- Every declaration must explicitly use the `override` namespace; missing or
  different namespaces are rejected.
- Any valid variable key is accepted; no separate override key policy is
  introduced.
- Duplicate override keys are rejected instead of silently selecting the last
  command-line occurrence.
- No override arguments produce a valid empty override scope.
- Decode and validation errors identify the one-based override argument index
  and, when decoded safely, its qualified variable name.
- Errors do not reproduce the complete raw JSON payload or a materialized
  expression value.
- `variable.NewSet(defaultScope, controllerScope, overrideScope)` selects the
  override for an unqualified matching key.
- A qualified `controller_config.KEY` lookup still selects the controller
  declaration, while qualified `override.KEY` selects the override.
- Generated `runtime` values and `controller_env` access are not part of this
  scope and cannot be authored through an override declaration.
- Live startup continues to reject `--override` until the startup resolver
  assembly slice can consume it.
- Targeted override parsing and precedence tests pass.

## Notes

- Use `json.Unmarshal` into `variable.Variable`; its custom decoder already
  rejects unknown fields, multiple JSON values, invalid types, and malformed
  recursive expressions.
- Build the final scope with `variable.NewScope` so duplicate-key behavior is
  consistent with other variable sources.
- Keep the raw argument strings local to parsing. Return only validated
  variable declarations in the scope.
- Inline command arguments are not an approved secret transport even after
  this scope is integrated, because process inspection and shell history may
  expose them before GOET receives them.

# 004 Defaults Document Loading

Status: proposed

## Objective

Define and load the required canonical `defaults.json` document beside a
selected controller document, validating its envelope, variable definitions,
duplicate names, and allowed configuration namespaces without yet combining it
with controller declarations.

## Required Context

Read these files first:

- `docs/concepts/controller-startup-resolution/README.md`
- `docs/concepts/controller-startup-resolution/003-executable-relative-config-discovery.md`
- `cmd/controller/config.go`
- `cmd/controller/config_test.go`
- `internal/variable/namespace.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/config.go`

## Allowed New Configuration Files

- `cmd/controller/defaults.json`

The new document contains the agreed controller defaults as canonical,
fully-qualified variable declarations. It does not contain structural
execution-environment configuration.

## Allowed Test Files

- `cmd/controller/config_test.go`

## Out Of Scope

- Combining defaults with explicit controller declarations
- Changing current controller startup to require or consume `defaults.json`
- Removing controller-variable namespace normalization
- Adding provenance to `variable.ResolvedValue`
- Loading defaults from the working directory or a global installation path
- Client, environment, override, runtime, or resolver scope assembly
- Structural execution-environment defaults
- Service construction or readiness behavior

## Acceptance Criteria

- The defaults document decodes `api_version`, `kind`, and canonical variable
  declarations from JSON.
- The only initially supported envelope is `api_version: goet/v1alpha1` and
  `kind: Defaults`.
- Missing, empty, unsupported, or incorrectly cased envelope values are
  rejected before variable-definition validation.
- `defaults.json` is derived as a sibling of the selected controller document,
  including when the controller path is relative.
- The loader requires the defaults file and returns contextual read, decode,
  envelope, and variable-validation errors containing its path.
- Defaults may use only `client_config`, `controller_config`, `worker_config`,
  and `project_config` namespaces.
- Environment, override, runtime, workflow, step, work-item, deprecated global,
  and legacy namespaces are rejected with an error identifying the variable
  and namespace.
- Duplicate keys are rejected within one namespace, while the same key may
  appear in two different allowed namespaces.
- The checked-in `cmd/controller/defaults.json` contains the defaults agreed in
  the epic and passes the same loader validation as deployment documents.
- Targeted defaults-document tests pass.

## Notes

- Keep the defaults document separate from `ControllerConfig`; slice 005 owns
  retention and layering so source identity is not discarded prematurely.
- Group variables by namespace before using `variable.NewScope`, because one
  defaults document may intentionally contain the same key in different
  namespaces.
- Relative sibling derivation should use `filepath.Dir` and `filepath.Join`;
  it must not convert the selected controller path to an absolute path.
- Implementing this slice requires adding `+newfile` to the current HCI budget
  because `cmd/controller/defaults.json` is a new configuration artifact.

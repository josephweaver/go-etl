# 001 Controller Document Envelope

Status: proposed

## Objective

Require every serialized controller configuration document to declare the
supported `api_version` and `kind` metadata before GOET normalizes or validates
its variable declarations.

## Required Context

Read these files first:

- `docs/epics/complete/controller-startup-resolution/README.md`
- `cmd/controller/config.go`
- `cmd/controller/config_test.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/config.go`

## Allowed Configuration Files

- `cmd/controller/controller-default-config.json`
- `cmd/controller/demo-config.json`
- `cmd/controller/fake-hpcc-ssh-config.json`
- `cmd/controller/local-singularity-config.json`

These existing documents must receive the required envelope so they remain
valid fixtures and runnable examples. Do not otherwise rewrite their contents.

## Allowed Test Files

- `cmd/controller/config_test.go`

## Out Of Scope

- Command-line parsing or `--config` behavior
- Executable-relative default config discovery
- The separate defaults JSON document
- Command-line override declarations
- `controller_env` access
- Variable precedence, resolution, sensitivity, or provenance changes
- Controller service construction or HTTP readiness
- Removing the transitional `execution_environment` configuration structure

## Acceptance Criteria

- `ControllerConfig` decodes `api_version` and `kind` from the document root.
- The only initially supported envelope is `api_version: goet/v1alpha1` and
  `kind: Controller`.
- Loading rejects a missing, empty, unsupported, or incorrectly cased
  `api_version` with an error that identifies `api_version`.
- Loading rejects a missing, empty, unsupported, or incorrectly cased `kind`
  with an error that identifies `kind`.
- Envelope validation occurs before variable normalization, variable
  definition validation, or execution-environment validation.
- Existing checked-in controller documents contain the supported envelope and
  continue to load successfully.
- Existing malformed-JSON and missing-file errors retain their current
  contextual file-path behavior.
- Targeted controller config tests pass.

## Notes

- Keep the metadata as language-neutral serialized fields; it does not
  participate in variable namespaces or precedence.
- Prefer small named constants for the supported API version and kind so tests
  and validation do not scatter magic strings.
- This slice validates one schema version. It does not add version conversion
  or compatibility negotiation.

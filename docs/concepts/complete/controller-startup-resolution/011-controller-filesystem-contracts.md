# 011 Controller Filesystem Contracts

Status: proposed

## Objective

Resolve and validate the controller root, Git-cache, temporary, and artifact-cache
paths through a caller-supplied bounded startup resolver, then anchor relative
paths to the controller process working directory before later consumers are
constructed.

## Required Context

Read these files first:

- `docs/concepts/complete/controller-startup-resolution/README.md`
- `docs/concepts/complete/controller-startup-resolution/009-startup-resolver-assembly.md`
- `docs/concepts/complete/controller-startup-resolution/010-main-database-contract.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `cmd/controller/defaults.json`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/main_test.go`

## Out Of Scope

- Creating, removing, or checking the existence of directories
- Requiring cache, temporary, or artifact paths to remain under
  `controller_root_dir`
- Resolving logging paths or constructing a logger
- Resolving Git-cache, temporary-storage, or artifact-retention policy values
- Constructing Git-cache, temporary-storage, or artifact-cache services
- Adding path-permission, ownership, free-space, symlink, or filesystem-device
  checks
- Changing the process working directory
- Adding an aggregate controller runtime-configuration object
- Retaining the startup resolver on `Controller` or in package-global state
- Wiring filesystem paths into the existing execution-environment model

## Acceptance Criteria

- A filesystem startup consumer resolves these four variables:
  - `controller_root_dir`
  - `controller_git_cache_path`
  - `controller_temp_path`
  - `controller_artifact_cache_path`
- Each lookup uses normal unqualified startup precedence, so an authorized
  `override` declaration may replace the defaults or explicit controller value.
- Each resolved value must have variable type `path` and must not be empty.
- Missing, wrong-type, empty, or recursively unresolvable values return an
  error identifying the filesystem consumer and the affected variable.
- The caller supplies the process working directory used as the base for all
  relative resolved paths.
- A missing or relative working-directory input is rejected with filesystem
  startup context.
- Relative resolved paths are joined to the supplied working directory and
  cleaned; absolute resolved paths are cleaned without being rebased.
- Cache, temporary, and artifact paths may resolve outside
  `controller_root_dir`; the root provides derived defaults and is not a
  containment boundary.
- Resolution returns one small value containing the four normalized paths for
  use by later constructors without becoming a second serialized configuration
  authority.
- Live startup obtains the process working directory and resolves the
  filesystem contract after the main database is ready and before constructing
  the execution environment or binding the HTTP listener.
- No directory is created and no filesystem service is constructed in this
  slice.
- The bounded startup resolver is not retained after filesystem resolution.
- Targeted controller filesystem-contract tests pass.

## Notes

- The serialized/default variable type remains `path`; normalization to an
  absolute operating-system path happens at the filesystem consumer boundary.
- Injecting the working directory into the helper keeps tests deterministic and
  makes the on-demand launcher's working-directory responsibility explicit.
- Lexical cleaning does not resolve symlinks or prove that a path exists.
- Keeping the four normalized paths in a narrow constructor input is consistent
  with the epic's prohibition on a duplicate aggregate
  `ControllerRuntimeConfig`.

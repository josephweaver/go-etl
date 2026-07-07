# 003 Executable-Relative Config Discovery

Status: proposed

## Objective

Wire the startup argument parser into the controller entry point so an explicit
`--config` path selects the controller document and an omitted path selects
`controller.json` next to the running executable.

## Required Context

Read these files first:

- `docs/concepts/controller-startup-resolution/README.md`
- `docs/concepts/controller-startup-resolution/002-startup-command-line-contract.md`
- `cmd/controller/main.go`
- `cmd/controller/config_test.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/config_test.go`

## Allowed Cleanup Files

- `cmd/demo-client/main.go`
- `scripts/fake-hpcc/run-demo`
- `scripts/local-singularity/run-demo`
- `docs/fake-hpcc.md`
- `PROJECT_STATE.md`

These files may only replace the legacy positional controller config path with
the explicit `--config` form or document the resulting startup behavior.

## Out Of Scope

- Loading or layering the separate defaults document
- Decoding, validating, or applying `--override` declarations
- Controller environment access
- Generated runtime variables or startup resolver assembly
- Installing or copying `controller.json` beside a built executable
- Searching the working directory, source tree, or additional fallback paths
- Service construction, HTTP readiness, or database ownership

## Acceptance Criteria

- `main` parses startup options before loading controller configuration.
- `--config PATH` loads exactly `PATH`; a relative explicit path remains
  relative to the controller process working directory.
- With no explicit path, startup loads the filename `controller.json` from the
  directory containing the running executable.
- Default discovery uses the executable path returned by `os.Executable`; it
  does not use the process working directory.
- Failure to determine the executable path or load the selected document
  returns a contextual startup/config error.
- Until slice 006 applies overrides, live startup rejects any supplied
  `--override` argument instead of silently ignoring it.
- The legacy positional config-path form is rejected by the slice 002 parser.
- Repository-owned demo launch commands use `--config PATH` and continue to
  identify the same controller documents.
- Path-selection tests use an injected executable path or a pure path helper;
  they do not depend on the test binary's actual installation directory.
- Targeted controller config and argument tests pass.

## Notes

- `os.Executable()` returns the path of the running binary. `filepath.Dir` and
  `filepath.Join` provide platform-correct sibling discovery.
- `go run` places its temporary executable outside the repository. Developers
  and repository demos must therefore pass `--config`; installed distributions
  are responsible for placing `controller.json` beside the built controller.
- Keep argument parsing, path selection, and document loading as distinct
  steps so later slices can add defaults and overrides without changing path
  semantics.

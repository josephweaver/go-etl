# 002 Startup Command-Line Contract

Status: proposed

## Objective

Parse controller startup arguments into a small options value containing an
optional explicit config path and zero or more raw canonical-JSON override
declarations, without loading configuration or constructing services.

## Required Context

Read these files first:

- `docs/concepts/controller-startup-resolution/README.md`
- `cmd/controller/main.go`
- `cmd/controller/config_test.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/config_test.go`

## Out Of Scope

- Wiring parsed options into `main`
- Loading either the explicit or default controller document
- Executable-relative config discovery
- Decoding or validating override JSON as `variable.Variable`
- Enforcing the `override` namespace
- Building an override scope or startup resolver
- Environment access, generated runtime values, and service construction
- Removing the existing positional config-path behavior before its callers are
  migrated

## Acceptance Criteria

- A dedicated parser accepts `--config PATH` and records `PATH` as the explicit
  config path.
- The parser accepts repeated `--override JSON` arguments and preserves their
  raw JSON strings in command-line order.
- The parser accepts the `--config=PATH` and `--override=JSON` forms supported
  by Go's standard flag package.
- Omitting both flags produces an options value with no config path and no
  overrides.
- Duplicate `--config` flags are rejected instead of silently selecting one.
- An unknown flag, a missing flag value, or an unexpected positional argument
  produces a contextual argument-parsing error.
- Override payloads are not decoded, normalized, logged, or otherwise exposed
  by this slice.
- Existing controller startup behavior remains unchanged because integration
  occurs in later slices.
- Targeted command-line parser tests pass.

## Notes

- Keep parsing separate from config loading so option syntax can be tested
  without filesystem access.
- A small custom `flag.Value` can collect repeated override strings while
  preserving order.
- Raw override retention is temporary. Slice 006 must decode each declaration,
  require the `override` namespace, and reject invalid values before startup
  uses them.
- The existing positional config path is not part of the target contract. It
  remains operational only until explicit `--config` wiring and caller cleanup
  are handled by a later approved slice.

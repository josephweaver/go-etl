# 012 Controller Operational Policy Contracts

Status: proposed

## Objective

Resolve and validate the controller-owned millisecond, capacity, concurrency,
cleanup, caretaker, and log-level policy variables through a caller-supplied
bounded startup resolver, then package the normalized values for the later
constructors that need them.

## Required Context

Read these files first:

- `docs/concepts/controller-startup-resolution/README.md`
- `docs/concepts/controller-startup-resolution/009-startup-resolver-assembly.md`
- `docs/concepts/controller-startup-resolution/011-controller-filesystem-contracts.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `cmd/controller/defaults.json`

Do not read unrelated files unless a targeted test failure directly requires it.

## Allowed Production Files

- `cmd/controller/main.go`
- Additional controller production files that are required by the agreed design

Implementation remains limited to one production file per prompt.

## Allowed Test Files

- `cmd/controller/main_test.go`

## Out Of Scope

- Constructing caches, caretaker workers, or log sinks
- Resolving HTTP listener, advertised URL, shutdown, or request-size settings
- Resolving database, filesystem, or execution-environment settings
- Adding a generic policy subsystem or aggregate controller runtime config
- Retaining the startup resolver on `Controller` or in package-global state
- Adding live reload or mutable runtime policy updates
- Introducing new policy variables beyond the ones already agreed in the epic

## Acceptance Criteria

- A startup consumer resolves the agreed controller policy variables from the
  standard precedence model, using the caller-supplied bounded resolver.
- The consumer covers the following values:
  - `resolver_max_depth`
  - `caretaker_interval_schedule_milliseconds`
  - `caretaker_missed_interval_limit`
  - `controller_git_cache_max_size_mb`
  - `controller_git_cache_retention_milliseconds`
  - `controller_git_fetch_timeout_milliseconds`
  - `controller_git_fetch_concurrency`
  - `controller_temp_cleanup_age_milliseconds`
  - `controller_artifact_cache_max_size_mb`
  - `controller_artifact_cache_retention_milliseconds`
  - `controller_storage_min_free_mb`
  - `controller_filesystem_logging_enabled`
  - `controller_log_root_path`
  - `controller_log_level`
- Each lookup uses the documented namespace and type for that variable.
- Integer policy values are rejected when they are zero, negative, or not an
  integer when the epic requires a positive integer.
- Boolean and string values are validated as their declared types.
- Missing, wrong-type, or invalid values return errors that identify the
  policy consumer and the affected variable.
- The slice preserves source precedence and provenance, but it does not build
  the caches, caretaker loop, or logger yet.
- Live startup resolves the policy contract after filesystem resolution and
  before constructing the HTTP server or any policy consumers.
- The bounded startup resolver is not retained after policy resolution.
- Targeted controller operational-policy tests pass.

## Notes

- This slice is the place to gather the non-HTTP, non-database operational
  knobs that are needed before the controller can construct its long-lived
  services.
- Keep the policy result narrow so later constructors can consume the resolved
  values without introducing a second configuration authority.

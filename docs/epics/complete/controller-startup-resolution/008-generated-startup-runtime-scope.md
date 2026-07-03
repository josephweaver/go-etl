# 008 Generated Startup Runtime Scope

Status: proposed

## Objective

Construct one immutable `runtime` variable scope containing the process-stable
values generated when controller startup begins, without wiring that scope into
live controller startup or retaining a resolver.

## Required Context

Read these files first:

- `docs/epics/complete/controller-startup-resolution/README.md`
- `docs/epics/complete/controller-startup-resolution/007-startup-override-scope.md`
- `cmd/controller/main.go`
- `cmd/controller/config_test.go`
- `internal/variable/scope.go`
- `internal/variable/type.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/config_test.go`

## Out Of Scope

- Wiring the runtime scope into `main`, live startup, or existing service
  constructors
- Assembling the complete startup resolver or changing resolver precedence
- Generating `runtime.controller_recovery_started_at`
- Mutable operational runtime observations
- Reading runtime values from controller JSON, defaults, environment, or
  command-line overrides
- Adding a global scope, global resolver, or aggregate runtime-configuration
  object
- Changing the existing workflow and attempt runtime metadata paths
- Sensitivity propagation, provenance reporting, logging, or persistence

## Acceptance Criteria

- A dedicated function constructs a `variable.Scope` containing exactly these
  qualified variables:
  - `runtime.controller_process_id`
  - `runtime.controller_instance_id`
  - `runtime.controller_started_at`
  - `runtime.controller_build_version`
- `controller_process_id` is an `int` variable containing the controller
  process ID supplied at the startup boundary.
- `controller_instance_id` is a non-empty `string` variable containing a newly
  generated process-stable instance identifier.
- `controller_started_at` is a `datetime` variable containing the supplied
  startup instant normalized to UTC.
- `controller_build_version` is a non-empty `string` variable containing the
  supplied build version.
- Construction rejects a non-positive process ID, an empty instance ID, and an
  empty build version with errors that identify the invalid runtime key.
- Time, process ID, instance ID, and build version inputs can be controlled by
  tests without mutating package-global state.
- Reusing the returned scope yields the same values for the lifetime of that
  constructed scope; the function does not regenerate values during lookup.
- The scope uses the canonical `runtime` namespace and standard typed-variable
  representations.
- No generated runtime value can be authored through the defaults,
  controller-config, environment, or override source parsers.
- Targeted runtime-scope tests pass.

## Notes

- Treat scope construction as the lifecycle boundary: obtain the process ID,
  instance ID, startup time, and build version once, then store their literal
  typed expressions in the returned scope.
- Prefer explicit function inputs for nondeterministic values. This keeps the
  behavior deterministic in tests and avoids replaceable package-level hooks.
- Use `time.Time` at the Go boundary and the standard variable datetime type;
  normalize the value to UTC before constructing its expression.
- The existing `buildInfoCodeVersion` helper may supply the build version when
  this scope is wired into startup later, but this slice does not change its
  current call sites.
- Instance-ID generation may use the repository's existing UUID dependency,
  but generation itself remains outside the pure scope-construction function.
- `runtime.controller_recovery_started_at` has a later lifecycle boundary and
  remains owned by the recovery-mode admission slice.

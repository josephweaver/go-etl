# 008 Worker Launch Config Integration Proof

Status: implemented

## Objective

Add a controller-level integration test proving that recursively typed
structured variables resolve through references and interpolation into the
concrete object, path, string, and string-list values consumed by
`workerLaunchConfig`.

## Required Context

Read these files first:

- `docs/concepts/structured-variable-resolution/README.md`
- `docs/concepts/structured-variable-resolution/004-variable-typed-expression-integration.md`
- `docs/concepts/structured-variable-resolution/005-recursive-whole-value-references.md`
- `docs/concepts/structured-variable-resolution/006-string-path-interpolation.md`
- `cmd/controller/worker_launch_config.go`
- `cmd/controller/worker_launch_config_test.go`

Do not read unrelated files unless test or compile failures directly require
it.

## Allowed Production Files

None.

## Allowed Test Files

- `cmd/controller/worker_launch_config_test.go`

## Out Of Scope

- Changing `workerLaunchConfig` production behavior.
- Adding resource-constraint parsing, admission control, or scheduling.
- Adding new controller configuration fields.
- Testing every variable error already owned by `internal/variable`.
- Adding path normalization or execution-environment translation.
- Refactoring existing controller tests unrelated to this integration path.

## Acceptance Criteria

- The test constructs controller configuration variables using the recursive
  typed-expression model rather than legacy raw-JSON object expressions.
- At least one nested object field resolves a whole-value reference.
- At least one nested string or path field resolves interpolation containing a
  reference and surrounding literal text.
- At least one nested generic list is consumed through a string-list boundary,
  demonstrating that homogeneity is validated by the consumer rather than
  encoded in the list type.
- The test assembles all scopes needed by its references before invoking
  resolution.
- `workerLaunchConfig` receives and returns the expected concrete worker
  executable, arguments, configuration path, log directory, and relevant
  scheduler or transport setting.
- No unresolved `${...}` token reaches the resulting worker-launch
  configuration.
- The focused controller test passes without production-file changes.

## Notes

- This is the epic's end-to-end consumer proof. Detailed resolver edge cases
  remain in `internal/variable` tests.
- `workerLaunchConfig` is used because it is an existing structured-value
  consumer. Resource constraints remain owned by their separate epic.
- The test should remain deterministic and should not start a controller,
  worker, container, scheduler, or external process.

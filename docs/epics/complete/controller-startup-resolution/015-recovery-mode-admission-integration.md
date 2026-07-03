# 015 Recovery-Mode Admission Integration

Status: proposed

## Objective

After the controller has constructed the required startup services and loaded
durable recovery state, expose only the health and worker heartbeat/report
APIs, generate `runtime.controller_recovery_started_at`, hand control to
caretaker recovery, and keep normal submission/admission closed until the
recovery contract allows it.

## Required Context

Read these files first:

- `docs/epics/controller-startup-resolution/README.md`
- `docs/epics/controller-startup-resolution/014-exclusive-database-ownership-integration.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`

Do not read unrelated files unless a targeted test failure directly requires it.

## Allowed Production Files

- `cmd/controller/main.go`

Implementation remains limited to one production file per prompt.

## Allowed Test Files

- `cmd/controller/main_test.go`

## Out Of Scope

- Implementing the heartbeat tracker
- Implementing caretaker abandonment, fencing, or recovery policy
- Changing database ownership mechanics
- Changing schema verification or migration behavior
- Binding or unbinding the HTTP listener
- Adding new submission routes or workflow-admission behavior
- Introducing a duplicate aggregate controller runtime-config object
- Reworking the earlier startup resolver slices

## Acceptance Criteria

- Startup reaches a recovery-only phase only after the required services and
  durable recovery state are ready.
- `runtime.controller_recovery_started_at` is generated at the recovery
  boundary, not during earlier startup phases.
- During recovery, only the health and worker heartbeat/report APIs are
  reachable.
- Normal submission and admission remain closed until the recovery contract
  explicitly permits them.
- Recovery-mode failures still fail closed before normal admission.
- The bounded startup resolver is not retained as a long-lived controller
  configuration object.
- Targeted controller recovery-boundary tests pass.

## Notes

- Keep this slice at the admission boundary so `attempt-liveness-recovery`
  can own the underlying recovery policy and tracker behavior.
- The goal is to make the startup phase transition explicit, not to redesign
  controller routing or worker reporting.

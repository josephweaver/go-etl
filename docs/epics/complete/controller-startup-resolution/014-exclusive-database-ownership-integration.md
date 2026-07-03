# 014 Exclusive Database Ownership Integration

Status: proposed

## Objective

Require the controller startup path to verify or acquire the database ownership
lock or lease before recovery processing or normal API admission begins, and
fail closed if another controller already owns the configured database.

## Required Context

Read these files first:

- `docs/epics/controller-startup-resolution/README.md`
- `docs/epics/controller-startup-resolution/010-main-database-contract.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`

Do not read unrelated files unless a targeted test failure directly requires it.

## Allowed Production Files

- `cmd/controller/main.go`

Implementation remains limited to one production file per prompt.

## Allowed Test Files

- `cmd/controller/main_test.go`

## Out Of Scope

- Implementing the underlying lock or lease mechanism
- Defining stale-owner detection or takeover policy
- Adding recovery-mode heartbeat or caretaker admission logic
- Changing database connectivity, schema checks, or migrations
- Binding or unbinding the HTTP listener
- Introducing a duplicate aggregate controller runtime-config object
- Expanding the startup contract to new ownership-related settings

## Acceptance Criteria

- Live startup refuses to proceed past database ownership validation when the
  configured database is already owned by another controller.
- Successful ownership verification or acquisition allows startup to continue
  into later phases.
- Ownership failure is distinguishable from ordinary database connectivity or
  schema-validation failure.
- The startup path does not admit normal API traffic before ownership is
  established.
- The ownership check is integrated at the startup boundary rather than stored
  as a long-lived controller configuration object.
- Targeted controller startup-ownership tests pass.

## Notes

- This slice should stay at the readiness gate and not drift into the concrete
  database-lock implementation, which belongs to `controller-resilience`.
- Keep the implementation boundary small enough that the later slice can own
  the underlying ownership mechanism without undoing this integration.

# 016 Startup Integration Coverage

Status: proposed

## Objective

Exercise the complete controller startup path so the startup-resolution epic
proves its real behavior end to end: successful startup, precedence handling,
qualified lookup protection, redacted failure paths, fail-before-bind behavior,
recovery-mode boundary behavior, and the normal-readiness transition.

This slice is about verification coverage, not new startup policy. It should
demonstrate that the earlier startup slices compose correctly when the
controller runs as a whole.

## Required Context

Read these files first:

- `docs/epics/controller-startup-resolution/README.md`
- `docs/epics/controller-startup-resolution/015-recovery-mode-admission-integration.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `PROJECT_STATE.md`

Do not read unrelated files unless a targeted test failure directly requires
them.

## Allowed Production Files

- `cmd/controller/main.go`

Keep the production change surface to one file unless the current code shape
forces a bounded cleanup edit.

## Allowed Test Files

- `cmd/controller/main_test.go`

## Allowed Documentation Files

- `docs/epics/controller-startup-resolution/README.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`

## Out Of Scope

- Adding new startup configuration policy
- Changing resolver precedence rules
- Changing database ownership mechanics
- Changing recovery policy or heartbeat tracking
- Adding new controller endpoints
- Introducing a duplicate aggregate runtime-config object
- Reworking earlier slice boundaries

## Acceptance Criteria

- A targeted startup test covers the full successful startup path.
- A targeted startup test covers source precedence and qualified lookup
  behavior.
- A targeted startup test covers the redacted failure path for missing or
  invalid required values.
- Startup still fails closed before binding HTTP when required startup phases
  fail.
- Recovery-mode boundary behavior is visible in the integration coverage.
- The normal-readiness transition remains intact after recovery startup.
- Project state documentation reflects the implemented startup coverage.

## Notes

- Keep this slice focused on coverage for the agreed startup behavior already
  described in the epic README.
- The main value here is confidence that the earlier slices work together, not
  new controller policy.
- If the test harness needs a small helper inside `cmd/controller/main.go` to
  make the startup path observable, keep that helper local to the file rather
  than broadening the design.

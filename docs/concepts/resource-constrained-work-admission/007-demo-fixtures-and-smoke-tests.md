# 007 Demo Fixtures And Smoke Tests

Status: Complete

## Objective

Add small demo fixtures and smoke/integration coverage proving resource-constrained work admission across mutex, memory, and all operators.

## Current State

The repository has smoke patterns for Python work items, submission/status, logs, and dependency-aware workflows. Resource-constrained work admission has no fixture coverage yet.

## Target State

A repeatable local smoke path proves:

```text
mutex constraint:
  two queued items share ctlr/python-env:torch requested=1 <= target=1
  first claim succeeds
  second claim is blocked until first completes/fails

memory capacity:
  target:local/memory-mib running total affects later candidate admission
  candidate is admitted only when total + requested <= target

head-of-line avoidance:
  first queued item is blocked
  later queued unconstrained or eligible item can still be claimed

operator coverage:
  =, !=, <, >, <=, >= are all exercised by focused tests
```

Smoke should stay small and should not require real Torch, real memory pressure, Docker, Slurm, or HPCC.

## Concept Decision

Use fake/lightweight work items to test scheduler behavior. This concept tests admission logic, not actual resource consumption.

All operator semantics should be covered by Go unit tests. Smoke scripts only need representative end-to-end behavior.

## Required Context

Read these files first:

- `scripts/dependency-aware-workflow-smoke.ps1`
- `scripts/python-workitem-smoke.ps1`
- `cmd/controller/*_test.go`
- `internal/persistence/*_test.go`
- sibling `../go-etl-demo-project` fixtures if available
- `docs/concepts/resource-constrained-work-admission/README.md`

## Allowed Production Files

- None expected unless tests reveal a small implementation defect.

## Allowed Test Files

- `cmd/controller/*_test.go`
- `internal/persistence/*_test.go`
- `internal/model/*_test.go`

## Allowed Documentation And Script Files

- `scripts/resource-constrained-work-admission-smoke.ps1`
- sibling demo-project workflow/submission fixtures under `../go-etl-demo-project/` if available
- `docs/concepts/resource-constrained-work-admission/README.md` only for small clarifications discovered during implementation

## Out Of Scope

- Real memory allocation.
- Real Python environment creation.
- HPCC/Slurm integration tests.
- Long-running stress tests.
- CI integration unless an existing smoke convention makes it trivial.

## Acceptance Criteria

- Unit tests prove every operator's true/false behavior.
- Unit tests prove overflow handling.
- Store/controller tests prove resource-blocked work is not claimed.
- Store/controller tests prove later eligible work can bypass earlier blocked work.
- Store/controller tests prove completed/failed attempts release running resource usage.
- Smoke script or focused integration test proves a mutex-like resource limit end to end.
- Smoke script or focused integration test proves a memory-like aggregate resource limit end to end.
- Smoke script does not require external services.
- Any sibling demo-project changes are listed in the implementation report.

## Notes

- Prefer deterministic fake work item IDs and deterministic queue times.
- Do not make smoke tests depend on wall-clock sleeps except for existing controller startup polling conventions.
- Keep tests focused on scheduler admission facts.

# 008 Docs Project State And Cleanup

Status: Complete

## Objective

Finalize documentation and project-state updates for resource-constrained work admission.

## Current State

After slices 001-007, production behavior and tests should exist, but docs may still describe resource constraints as deferred or unsupported.

## Target State

Documentation explains:

- resource readiness is independent from dependency readiness;
- constraints are resolved before work enters queued/pending state;
- SQL/view code exposes resource facts but does not evaluate operators;
- Go code evaluates all six operators;
- work items with no constraints remain assignable as before;
- resource-blocked work remains queued;
- terminal completion/failure releases resource usage by removing `running_work` rows;
- `<=` is the main practical capacity operator even though all operators are supported;
- integer units such as `memory-mib` are preferred over floating-point GB.

## Concept Decision

Documentation should present this as a controller scheduling feature, not a worker execution feature. Resource admission prevents GOET from releasing too much work; it does not guarantee the operating system, Slurm, Docker, Singularity, or Python runtime will enforce the same limits.

## Required Context

Read these files first:

- `docs/concepts/resource-constrained-work-admission/README.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/demo-client/README.md`
- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/execution-observability/README.md`

## Allowed Production Files

- None expected.

## Allowed Test Files

- None expected unless final docs/smoke cleanup reveals a narrow defect.

## Allowed Documentation And Script Files

- `docs/concepts/resource-constrained-work-admission/README.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/demo-client/README.md`
- `scripts/dependency-aware-workflow-smoke.ps1`
- `scripts/python-workitem-smoke.ps1`

## Out Of Scope

- New scheduler behavior.
- New resource-policy semantics.
- New workflow dependency features.
- New worker runtime resource enforcement.

## Acceptance Criteria

- `PROJECT_STATE.md` states that resource-constrained work admission is implemented.
- Architecture docs describe `dependency ready AND resource predicate true => assignable`.
- Controller docs explain resource-blocked queued work at a high level.
- Demo-client docs mention any status output changes.
- The concept README tracker marks slices 001-008 implemented after completion.
- Any remaining deferred topics are listed clearly, such as priorities, reservations, real runtime enforcement, or multi-controller resource leases.
- Final tests and smoke commands are listed in the implementation report.

### Implementation Report

- Tests executed for slice 008: none added; no production behavior changed.
- Smoke/verification commands used for this conceptâ€™s final verification:
  - `powershell -NoProfile -File scripts/dependency-aware-workflow-smoke.ps1`
  - `powershell -NoProfile -File scripts/python-workitem-smoke.ps1`

## Notes

- Do not over-document internal SQL implementation in user-facing docs.
- Keep detailed schema/operator behavior in the concept README and OS files.
- If some raw-work wrapper support was deferred, document that explicitly rather than implying all submission surfaces support constraints.

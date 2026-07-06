# 012 Update Dependency Workflow Docs And Smoke

Status: Complete

## Objective

Document dependency-aware workflow execution and add a repeatable smoke path that proves sequential and parallel-stage workflows do not queue downstream work early.

## Current State

The controller now supports dependency-aware stage execution internally after slices 001-011, but repo docs and smoke scripts may still describe workflow submission as compiling or queueing all steps at once.

The branch uses `docs/concepts`, so any final docs updates should keep this Strategic Concept bundle under `docs/concepts/complete/dependency-aware-workflows/` and should not reintroduce the earlier mistaken `docs/epics` path.

The sibling demo project may not yet include a small workflow fixture that demonstrates sequential-by-default execution or contiguous `parallel_with` execution.

## Target State

Documentation explains the new current behavior:

- steps are sequential by default;
- `parallel_with` groups adjacent steps into one parallel stage;
- downstream stages are compiled just in time;
- `workflow.step[index]` exposes predecessor outputs;
- logical step output is stored at dependency step level while a workflow is running, not in `workflow_stages.output_json`;
- output JSON is bounded control-plane handoff data, not provenance or bulk result storage;
- `goet status` shows dependency-aware state;
- `goet logs` shows dependency transition observations when logging is enabled.

A smoke path should prove at least:

```text
submit a two-stage sequential workflow
observe only stage 0 work is initially assignable
complete or run stage 0
observe stage 1 becomes assignable afterward
submit a workflow with a valid parallel_with group
observe parallel stage work may be assignable together
submit an invalid non-contiguous parallel_with workflow
observe submission rejection before work is queued
```

Use existing local smoke patterns and demo-project conventions. Do not introduce a new external service requirement.

## Concept Decision

This slice updates documentation and smoke coverage. It should not make further production behavior changes unless a documentation/smoke test reveals a small defect in the already-implemented behavior; report any such defect rather than expanding scope silently.

## Required Context

Read these files first:

- `docs/concepts/complete/dependency-aware-workflows/README.md`
- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/execution-observability/README.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `scripts/python-workitem-smoke.ps1`
- `cmd/controller/README.md`

If the previous concepts added newer smoke scripts or CLI docs, read those instead of older demo-only docs and report the substitution.

Do not read unrelated files unless smoke failures directly require them.

## Allowed Production Files

- None expected.

## Allowed Test Files

- `cmd/controller/main_test.go`

Only modify tests if a narrow dependency-aware smoke assertion belongs in Go tests rather than script/docs.

## Allowed Documentation And Script Files

- `docs/concepts/complete/dependency-aware-workflows/README.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/demo-client/README.md`
- `scripts/dependency-aware-workflow-smoke.ps1`
- sibling demo project workflow/submission fixtures under `../go-etl-demo-project/` if that repository is available in the implementation environment

## Out Of Scope

- New production behavior.
- New resource-constraint docs.
- Python environment management docs.
- Cross-workflow dependency docs.
- CI integration unless an existing smoke-script convention already requires it.

## Acceptance Criteria

- Documentation no longer says or implies that workflow submission queues all steps immediately.
- Documentation describes sequential-by-default stage execution.
- Documentation describes valid and invalid `parallel_with` examples.
- Documentation describes `workflow.step[index]` output access and its limitations.
- Documentation says `workflow_stages.output_json` is not the canonical source for `workflow.step[index]`.
- Documentation explains that large results should be external artifacts referenced by small output JSON metadata.
- A smoke script or narrow integration test proves downstream work is not assignable before upstream completion.
- A smoke script or narrow integration test proves a valid contiguous `parallel_with` group can make sibling steps assignable in the same stage.
- A smoke script or narrow integration test proves non-contiguous label reuse is rejected.
- The smoke path uses the existing `goet submit/status/wait/logs` surfaces when practical.
- Any required sibling demo-project fixture changes are listed clearly in the implementation report.

## Notes

- Keep the smoke fixture small. It should test dependency readiness, not Python environment management or large data movement.
- If the sibling demo project is not available in the Codex environment, update in-repo docs and tests, then report the external fixture gap.

# 011 Surface Dependency State In Status And Logs

Status: Complete

## Objective

Expose dependency-aware stage and step state through the existing submission status API/CLI and emit useful dependency transition observations through the existing observability pipeline.


## Implementation Handoff Note

Use the actual file names and helper/store owners introduced by slices 001-004. Where this document names example files such as `workflow_dependency_store.go`, `workflow_completion.go`, or `workflow_stage_queue.go`, treat those as placeholders if the branch implementation chose different owners.

## Current State

The previous Submission CLI Status concept provides `goet status <submission_id>` and JSON status output. The previous Execution Observability concept provides controller-owned log observations and `goet logs <submission_id>`.

Dependency-aware execution now has internal workflow, stage, step, and work-item state, but users may not be able to see why a submission is waiting, active, completed, or failed.

## Target State

The existing status payload for a submission includes dependency-aware information without breaking existing fields.

At minimum, structured status should show:

```text
submission_id
workflow state
current stage index, when running
stage count
per-stage state summary
failed stage/step and reason, when failed
pending/active/completed/failed counts that do not confuse blocked future work with assignable pending work
```

If status exposes output-related dependency metadata, keep it to hashes, byte counts, and pruned flags. Do not include full dependency step `OutputJSON`, membership `OutputJSON`, `completed_work.output_json`, or `workflow_stages.output_json` in status responses or transition observations.

Human-readable `goet status` should remain compact. It can show a one-line summary plus a small stage summary rather than a full tree.

The controller emits log observations for important dependency transitions through the existing observability concept, such as:

```text
normalized workflow into N stages
queued stage 0
completed stage N
activated stage N+1
completed workflow
failed workflow at stage N step M: reason
```

## Concept Decision

This slice updates existing status and observability concepts. Do not create new endpoints or a new CLI command.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/execution-observability/README.md`
- `cmd/controller/main.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_completion.go`
- `cmd/demo-client/main.go`
- `internal/model/work_item.go`
- `internal/model/log_observation.go`

If Submission CLI Status renamed `cmd/demo-client` to another CLI package, read and modify that CLI owner instead.

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- the actual dependency-state owner created by 003
- the actual completion/activation helpers created by 006-010
- `cmd/demo-client/main.go`
- `internal/model/submission.go`
- `internal/model/log_observation.go`

If status or logs are owned by files with different names after the previous concepts, modify those owners and report the substitution.

## Allowed Test Files

- `cmd/controller/main_test.go`
- tests for the actual dependency-state owner created by 003
- tests for the actual completion/activation helpers created by 006-010
- `cmd/demo-client/main_test.go`
- `internal/model/submission_test.go`
- `internal/model/log_observation_test.go`

## Out Of Scope

- New public identifiers.
- New CLI commands.
- Streaming logs.
- Rich TUI status display.
- Resource-capacity status.
- Cross-workflow status.
- Worker changes.

## Acceptance Criteria

- Status JSON includes dependency stage state for a running multi-stage workflow.
- Status JSON distinguishes blocked future-stage work from assignable pending work.
- Human-readable status remains stable enough for users and does not dump large internal JSON by default.
- A failed dependency-aware workflow reports the failed stage/step and reason.
- Dependency transition observations are emitted through the existing log-observation path.
- `goet logs <submission_id>` can show dependency transition messages for a submission.
- Existing status fields used by previous CLI/status tests continue to work.
- No new endpoint is required to view dependency state.

## Notes

- Preserve backward-compatible JSON names where the previous status concept already established them.
- Do not log full workflow documents or user data in transition messages.
- Treat `workflow_stages.output_json` as unused for dependency-aware output semantics; status/log output should not imply a stage-level logical output exists.

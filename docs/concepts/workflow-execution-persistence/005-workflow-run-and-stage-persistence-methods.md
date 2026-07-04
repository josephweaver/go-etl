# 005 Workflow Run and Stage Persistence Methods

Status: proposed

## Objective

Add persistence methods for workflow runs and ordered stage plans. The methods
should let the controller create one run under an existing project/workflow,
retrieve that run, insert the run's stage plan, query individual stage state,
and list active workflow runs after restart.

This slice stores run and stage facts only. It does not compile workflows,
publish work items, or advance stage lifecycle based on work completion.

## Required Context

Read these files first:

- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/concepts/workflow-execution-persistence/004-project-and-workflow-persistence-methods.md`
- `internal/persistence/store.go`
- `internal/persistence/store_test.go`
- `internal/persistence/db_adapter_sqlite.go`

Do not read controller files unless compile or test failures directly require
it.

## Allowed Production Files

- `internal/persistence/store.go`

## Allowed Test Files

- `internal/persistence/store_test.go`

## Out Of Scope

- Workflow compilation.
- Work-item insertion.
- Queue placement.
- Attempt claim, completion, failure, or retry behavior.
- Stage completion derived from `work_items` and terminal rows.
- Dependency-aware scheduling.
- Source-control cache or GitHub behavior.
- UUIDv7 generation.
- Canonical JSON computation.
- Controller startup or HTTP handler integration.

## Acceptance Criteria

- `Store` exposes a method to create one workflow run under an existing project
  and workflow.
- Creating the same run with identical values is idempotent.
- Creating the same run with conflicting values fails.
- Creating a run for a missing project or workflow fails through database
  constraints.
- `Store` exposes a method to retrieve one workflow run by `run_id`.
- Missing run lookup returns a distinguishable not-found result.
- `Store` exposes a method to insert an ordered stage plan for a run.
- Re-inserting an identical stage plan is idempotent.
- Re-inserting a conflicting stage plan fails.
- Stage plan insertion fails for a missing run.
- `Store` exposes a method to retrieve one stage by `run_id` and `stage_index`.
- `Store` exposes a method to list active workflow runs after restart.
- Active runs exclude runs whose stages are all terminal.

## Notes

- `submission_context_json` is caller-supplied JSON. This slice should validate
  that SQLite accepts it as JSON, but it should not define the full semantic
  schema for key/variable pairs.
- Stage states should use the existing schema values: `ready`, `running`,
  `completed`, `failed`, `skipped`, and `blocked`.
- Use explicit structs for run and stage records.
- Keep stage lifecycle transitions simple. This slice may insert initial states
  and read them, but should not implement completion publication.

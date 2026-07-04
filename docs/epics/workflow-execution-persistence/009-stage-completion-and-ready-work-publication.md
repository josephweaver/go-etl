# 009 Stage Completion and Ready-Work Publication

Status: implemented

## Objective

Add a persistence method that can mark one workflow stage complete exactly once
when persisted work-placement evidence proves the stage is complete. The same
transaction may also insert and enqueue caller-supplied newly ready work for
later stages.

This feature connects terminal attempt rows from 008 to durable stage progress:

```text
completed_work for every stage work item
  -> workflow_stages.state = completed
  -> optional newly ready work_items + queued_work rows
```

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/README.md`
- `docs/epics/workflow-execution-persistence/005-workflow-run-and-stage-persistence-methods.md`
- `docs/epics/workflow-execution-persistence/006-work-item-and-queue-persistence-methods.md`
- `docs/epics/workflow-execution-persistence/008-attempt-terminal-transition-transaction.md`
- `internal/persistence/store.go`
- `internal/persistence/store_test.go`

Do not read controller files unless compile or test failures directly require
it.

## Allowed Production Files

- `internal/persistence/store.go`

## Allowed Test Files

- `internal/persistence/store_test.go`

## Documentation Files

- `docs/epics/workflow-execution-persistence/009-stage-completion-and-ready-work-publication.md`
- `PROJECT_STATE.md`
- `epi_ctl/20260703.md`

## Proposed API Shape

Use a task-oriented method rather than exposing the caller to individual SQL
updates:

```go
type CompleteStageRequest struct {
    RunID              string
    StageIndex         int
    OutputJSON         string
    OutputJSONSHA256   string
    CompletedAt        string
    ReadyWorkItems     []WorkItemRecord
    ReadyQueuedWork    []QueuedWorkRecord
}

type CompleteStageResult struct {
    Stage     WorkflowStageRecord
    Found     bool
    Completed bool
}

func (s *Store) CompleteStageIfReady(ctx context.Context, request CompleteStageRequest) (CompleteStageResult, error)
```

`Found=false` means the requested stage does not exist. `Found=true` and
`Completed=false` means the stage exists but persisted work evidence does not
yet prove completion. `Found=true` and `Completed=true` means the stage either
transitioned to completed in this call or was already completed with identical
terminal values.

## Stage Completion Rule

For one `(run_id, stage_index)`, the stage is complete only when:

- the stage exists;
- at least one `work_items` row exists for that stage;
- every `work_items` row for that stage has one matching successful
  `completed_work` row by `work_item_id`;
- no work item for that stage has a row in `queued_work`;
- no work item for that stage has a row in `running_work`;
- no work item for that stage has a row in `failed_work`.

The at-least-one-work-item rule preserves the earlier decision that no-op
stages still create deterministic skipped/no-op work evidence. An empty stage
plan is not enough evidence to complete a stage.

## Acceptance Criteria

- `Store` exposes a method to complete a stage only when persisted work rows
  prove that all work for the stage completed successfully.
- The method does not use a stored mutable counter to determine completion.
- A stage with no work items is not completed.
- A stage with queued work is not completed.
- A stage with running work is not completed.
- A stage with failed work is not completed.
- A stage with any work item lacking a matching `completed_work` row is not
  completed.
- Completing a ready stage updates `workflow_stages.state` to `completed`.
- Completing a ready stage records `completed_at`, `output_json`, and
  `output_json_sha256`.
- Repeating an identical stage completion request is idempotent.
- Repeating a conflicting stage completion request for an already completed
  stage fails.
- A missing stage returns a distinguishable result.
- If ready work is supplied, the method inserts the supplied `work_items` and
  `queued_work` rows in the same transaction as the stage completion.
- If ready-work insertion or enqueueing fails, the stage completion rolls back.
- Ready-work publication is idempotent when the supplied rows already exist
  with identical values.
- Terminal attempt rows are not modified by this method.
- Stage completion behavior is tested without controller HTTP wiring.

## Out Of Scope

- Deciding which downstream stage is ready.
- Compiling downstream work from workflow definitions.
- Dependency expression semantics.
- `parallel_with` semantics.
- Sub-workflow invocation.
- Retry, requeue, or max-retry policy.
- Stage failure transition policy.
- Controller HTTP handler integration.
- Worker scaling.
- Source-control cache or GitHub behavior.
- UUIDv7 generation.
- Canonical JSON computation.
- Retention cleanup.

## Ambiguity To Review

The epic names "ready-work publication," but the dependency-aware compiler does
not exist yet. This slice therefore treats ready work as caller-supplied rows:
the persistence layer guarantees atomic insertion and enqueueing, while later
controller/compiler code decides what work should be published.

There is also an output-shape ambiguity. The current schema stores one
`workflow_stages.output_json` and `output_json_sha256`, but the exact way to
derive stage-level output from fan-out completions is not defined here. This
slice should accept caller-supplied stage output JSON and hash, then validate
only that JSON is syntactically valid and the hash field is non-empty.

## Notes

- Stage completion should be addressed by `(run_id, stage_index)`.
- The method should use one database transaction for readiness check, stage
  update, ready-work insertion, and ready-work enqueueing.
- If the stage is already completed with identical values, the method should
  return the existing stage as a successful idempotent result and should not try
  to publish ready work again.
- If the stage is already `failed`, `skipped`, or `blocked`, completion should
  fail unless a later slice defines legal transitions from those states.

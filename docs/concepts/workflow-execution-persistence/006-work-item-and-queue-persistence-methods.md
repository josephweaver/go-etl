# 006 Work Item and Queue Persistence Methods

Status: proposed

## Objective

Add persistence methods for compiled work items and their initial queue
placement. The methods should let the controller persist the logical work items
produced for a workflow stage, enqueue those items for later assignment, and
reconstruct queued/running status counts from the database.

This slice stores logical work and current queue visibility only. It does not
claim work for a worker, create attempts, complete work, fail work, retry work,
or advance stage lifecycle.

## Required Context

Read these files first:

- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/concepts/workflow-execution-persistence/005-workflow-run-and-stage-persistence-methods.md`
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

- Work claiming or worker assignment.
- Attempt creation.
- Moving rows from `queued_work` to `running_work`.
- Completion, failure, retry, or skipped-work terminal recording.
- Stage completion derived from work-item terminal rows.
- Publishing newly ready downstream stages.
- Worker registration.
- Source-control cache or GitHub behavior.
- UUIDv7 generation.
- Canonical JSON computation.
- Controller startup or HTTP handler integration.

## Acceptance Criteria

- `Store` exposes a method to insert compiled work items for an existing
  workflow stage.
- Inserting the same work-item set with identical values is idempotent.
- Inserting the same work-item identity with conflicting values fails.
- Inserting work items for a missing run/stage fails through database
  constraints.
- `Store` exposes a method to enqueue one or more existing work items.
- Re-enqueueing an already queued item is idempotent.
- Enqueueing a missing work item fails through database constraints.
- `Store` exposes a method to retrieve one work item by `work_item_id`.
- Missing work-item lookup returns a distinguishable not-found result.
- `Store` exposes a method to list queued work items in deterministic order.
- `Store` exposes a method to count queued/running/terminal work items for a
  run and stage.
- Status counts are reconstructed from `queued_work`, `running_work`,
  `completed_work`, and `failed_work`; they are not stored as a separate mutable
  summary row.

## Notes

- `worker_payload_json` is caller-supplied compiled worker JSON. This slice
  should validate that SQLite accepts it as JSON, but it should not define the
  final worker payload schema.
- A likely payload shape is:

  ```json
  {"plugin":"plugin-name","parameters":{"param1":"param1value"}}
  ```

- `resolved_inputs_sha256` is caller-supplied. This slice should persist and
  compare it, but should not compute it.
- Use explicit structs for work-item records and queue/status views.
- Queue order should be deterministic, probably by `queued_at` and
  `work_item_id`, so restart reconstruction is stable.

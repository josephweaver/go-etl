# 001 Decouple Workers From `run_id`

Status: Complete

## Objective

Make the worker data model match the intended architecture:

```text
workers are reusable execution capacity
queued work belongs to workflow runs
workers claim any runnable queued work item, regardless of run_id
```

A worker must not be modeled as owned by a single workflow run. The run association belongs to `work_items.run_id`, not to `workers`.

## Implementation Handoff Note

This slice is intentionally written for a two-step model handoff:

1. **5.4-mini refinement pass**
   - Read this OS.
   - Inspect the current implementation.
   - Produce a concise implementation plan only.
   - Confirm exact files to modify.
   - End with `READY FOR SPARK IMPLEMENTATION` if safe.

2. **5.3-Codex-Spark implementation pass**
   - Implement the approved plan.
   - Keep the change mechanical and narrow.
   - Prefer tests over broad redesign.
   - Do not redesign scheduling, worker lifecycle, worker capabilities, or queue prioritization.

Use:

```text
EC-3 / operational slice / files(4)+test+doc
CSx(IR)x Cadence
```

Reference prompt style:

```text
..\epistemic-control\prompts\gpt5.4-mini-refine-os.md
..\epistemic-control\prompts\gpt5.3-codex-spark-impl.md
```

## Current State

The SQLite schema currently includes a nullable `run_id` column on `workers`.

That column can imply the wrong ownership model:

```text
workflow_instances
  â””â”€â”€ workers
```

But the intended model is:

```text
workflow_instances
  â””â”€â”€ work_items
        â””â”€â”€ queued_work

workers
  â””â”€â”€ claim queued_work globally
```

The queue already appears conceptually global because work claiming should operate through `queued_work -> work_items`, where `work_items.run_id` identifies the owning workflow run.

The architectural risk is not necessarily that current code is filtering workers by `run_id`; the risk is that the schema and documentation encode a misleading ownership boundary that future scheduling, recovery, or worker-start logic may accidentally depend on.

## Target State

Workers are represented as reusable executor capacity.

The implementation should make the following true:

```text
workers.run_id is not required to determine what work a worker can claim
workers.run_id is not used as an ownership boundary
work_items.run_id remains the source of truth for workflow-run ownership
queued_work remains a global queue over work_items
running_work records the current attempt and worker_id
completed/failed work derive run identity through work_items
```

Preferred target, if safe within this slice:

```text
remove run_id from the workers table/model and related persistence code
```

Acceptable narrow target, if schema removal is unsafe or too broad:

```text
keep workers.run_id nullable as a deprecated/legacy launch-context field,
but remove any ownership semantics and document that it is not authoritative
```

The 5.4-mini refinement pass should decide between these based on actual code impact.

## Concept Decision

A worker is not a child of a workflow run.

A worker may have been launched because queued demand existed, but once alive it is general execution capacity. It should be able to claim work from multiple workflow runs over its lifetime.

The run/workflow relationship is:

```text
work_item -> workflow_stage -> workflow_instance
```

not:

```text
worker -> workflow_instance
```

## Required Context

Read these files first:

```text
docs/concepts/archtectural-refinement-1/README.md
internal/persistence/db_adapter_sqlite.go
internal/persistence/store.go
internal/persistence/store_test.go
cmd/controller/main.go
cmd/controller/*worker*.go
cmd/worker/main.go
PROJECT_STATE.md
```

If the actual Strategic Concept directory is spelled differently, use the real path but keep this OS title and filename.

Also search for:

```text
workers.run_id
WorkerRecord
worker_id
execution_handle
ClaimNextWork
RegisterWorker
InsertWorker
ListRunningWork
```

Do not read unrelated files unless tests or references require them.

## Allowed Production Files

Prefer no more than four production files.

Likely candidates:

```text
internal/persistence/db_adapter_sqlite.go
internal/persistence/store.go
cmd/controller/main.go
cmd/worker/main.go
```

If worker registration/lifecycle code lives in other files, the 5.4-mini refinement pass should identify the exact replacement list.

## Allowed Test Files

Likely candidates:

```text
internal/persistence/store_test.go
cmd/controller/main_test.go
cmd/worker/main_test.go
```

Add or update only the tests needed to prove this slice.

## Required Documentation Updates

Update:

```text
docs/concepts/archtectural-refinement-1/001-decouple-workers-from-run-id.md
PROJECT_STATE.md
```

If there is a database/schema reference document in the repo, update it to clarify:

```text
workers are reusable capacity
workers do not own workflow runs
run ownership is derived from work_items.run_id
```

## Out Of Scope

Do not implement:

- Worker capability matching.
- Worker pools.
- Queue priority or fairness.
- Per-run worker reservations.
- Per-run worker isolation.
- Multi-tenant scheduling.
- Full database migration framework.
- A new scheduler abstraction.
- A worker lease/heartbeat system unless it already exists and directly depends on `workers.run_id`.
- Any dependency-aware workflow behavior unrelated to worker/run ownership.

## Implementation Guidance

### Step 1 â€” Inspect actual usage

Identify every read/write/reference to `workers.run_id`.

Classify each reference as one of:

```text
schema-only
insert/update path
scan/read path
foreign key / validation
worker start context
scheduling / claim logic
test fixture
documentation
```

### Step 2 â€” Preserve global queue semantics

Verify that `ClaimNextWork` or equivalent work-claim logic does not filter by worker run.

The expected claim shape is:

```text
SELECT queued work
JOIN work_items
ORDER BY queued_at, work_item_id
LIMIT 1
```

The selected `work_items.run_id` should travel with the claimed work item as metadata, but it should not be matched against `workers.run_id`.

### Step 3 â€” Decide schema action

If low-risk, remove `workers.run_id` from:

```text
CREATE TABLE workers
WorkerRecord
insert/select/scan logic
tests
documentation
```

If removal is too broad, keep the column nullable but explicitly rename the concept in code/docs if practical, for example:

```text
LaunchContextRunID
```

or document it as deprecated and non-authoritative.

Do not silently leave the misleading ownership model in docs.

### Step 4 â€” Add focused tests

At minimum, add a persistence/controller test proving:

```text
given queued work from run-A and run-B
and one worker identity
when the worker claims work twice
then it can claim work from both runs
and no worker run binding is required
```

Also add a regression check that queued/running/completed/failed run status is still derived through `work_items.run_id`.

## Acceptance Criteria

- Worker identity is not modeled as owned by a workflow run.
- Work claiming does not require or consult `workers.run_id`.
- A single worker can claim work items from different `run_id` values.
- Run-level work counts still work through `work_items.run_id`.
- Attempt records still preserve `worker_id`.
- Running work still records the worker currently executing the attempt.
- Existing raw work and workflow submission behavior is preserved.
- Tests cover cross-run worker reuse.
- Documentation states the intended ownership model.
- `PROJECT_STATE.md` is updated.
- This OS is marked implemented after completion.

## Safety Checks

Stop and record an issue if any of these are true:

- Existing recovery logic assumes `workers.run_id` is authoritative.
- Existing scheduler startup logic requires a one-worker-per-run invariant.
- Removing the column requires a real migration strategy beyond this slice.
- Tests reveal that worker lifecycle is currently coupled to run cleanup.
- The change would require redesigning worker pools, scheduler behavior, or dependency-aware stage activation.

If blocked, append the issue to:

```text
docs/concepts/archtectural-refinement-1/issues.md
```

## Recommended 5.4-mini Refinement Prompt

```text
Please read operational slice (OS):

docs\concepts\archtectural-refinement-1\001-decouple-workers-from-run-id.md

Analyze the operational slice and produce a concise implementation plan only.

Use "EC-3 / operational slice / files(4)+test+doc" with CSx(IR)x Cadence.

Reference:
..\epistemic-control\hci-levels.md
..\epistemic-control\hci-cadence\CSx(IR)x.md

Your task:

1. Identify the exact implementation objective.
2. Identify every current reference to workers.run_id or equivalent worker-run ownership.
3. Decide whether to remove the column/model field now or preserve it as deprecated/non-authoritative launch context.
4. Identify the likely files to modify, maximum 4 implementation files unless the OS explicitly requires more.
5. Identify required tests.
6. Identify required documentation updates.
7. Identify any ambiguity, missing prerequisite, or invariant that could make implementation unsafe.

Do not modify files yet.

If the slice is ready for implementation, end with:

READY FOR SPARK IMPLEMENTATION
```

## Recommended 5.3-Codex-Spark Implementation Prompt

```text
Please read operational slice (OS):

docs\concepts\archtectural-refinement-1\001-decouple-workers-from-run-id.md

Then implement and test the approved plan from the prior analysis.

Use "EC-3 / operational slice / files(4)+test+doc" with CSx(IR)x Cadence.

Reference:
..\epistemic-control\hci-levels.md
..\epistemic-control\hci-cadence\CSx(IR)x.md

Constraints:

* Prefer mechanical, behavior-preserving changes.
* Do not redesign beyond the operational slice.
* Modify no more than 4 implementation files unless the approved plan explicitly requires more.
* Add or update tests required by the slice.
* Keep changes small and reviewable.
* Preserve existing public behavior except where the OS requires a change.

When complete, update:

/PROJECT_STATE.md

and mark the OS document above as implemented.

If you find a major issue, blocker, or unsafe ambiguity, stop and create/append it to:

docs\concepts\archtectural-refinement-1\issues.md

After you complete the work, please git commit with a good message.

Perform an implementation review based on:

..\epistemic-control\procedures\implementation-review.md

and record results to:

..\epistemic-control\implementation-review\archtectural-refinement-1\yyyyMMdd_hhmmss.md
```

## Notes

This slice should improve the architecture without changing the user-facing workflow API.

The most important EC takeaway is:

```text
worker_id answers: who executed this attempt?
run_id answers: which workflow run owns this work item?
```

Those are separate concepts and should remain separate in the data model.

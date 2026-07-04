# 012f Remove In-memory Queue Authority

Status: implemented

## Objective

Stop using `Controller.pending`, `Controller.assigned`, and
`Controller.failed` as queue authority. The fields have now been removed from
`Controller`.

The live controller startup path configures `Controller.workflowStore`, so
normal runtime behavior uses the database for workflow admission, assignment,
status, completion, and failure. No-store queue behavior is no longer a
supported fallback.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/012-controller-integration-cutover.md`
- `docs/epics/workflow-execution-persistence/012c-persistence-backed-raw-work-submission.md`
- `docs/epics/workflow-execution-persistence/012d-persistence-backed-work-claim.md`
- `docs/epics/workflow-execution-persistence/012e-persistence-backed-terminal-reports.md`
- `docs/epics/workflow-execution-persistence/012e2-worker-observed-skip-evidence.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/persistence/store.go`

## Implemented State

The persistence-backed paths now exist for:

- `/work` raw submission
- `/work/next`
- `/work/complete`
- `/work/fail`
- `/status`
- `/workflow` source-reference admission

The intended `/workflow` boundary is not "submit work items" and not "submit
raw workflow JSON." It is "submit a workflow run" by providing immutable
source-control references to:

- project JSON documents; and
- workflow JSON documents.

Compiled work items are a controller-generated consequence of admitting that
workflow run. They are not the API payload.

Current persisted equivalents:

```text
queued_work    pending work
running_work   assigned work
failed_work    failed work
completed_work completed/skipped work
```

## Implementation Strategy

The original incremental strategy is complete. The practical source-reference
work landed through `012f2`, `012f3`, and `012f4`.

Recommended implementation atoms:

```text
012f-a Define workflow admission payload and provenance bridge
012f-b Persist admitted workflow run and initially ready compiled work
012f-c Make persisted workflow scaling demand derive from queued/running store counts
012f-d Add guard tests proving persisted paths do not mutate pending/assigned/failed [superseded by field removal]
012f-e Remove or demote in-memory queue authority after no live store path uses it [implemented]
```

## 012f-a Define Workflow Admission Payload And Provenance Bridge

When `Controller.workflowStore` is configured, `/workflow` should be treated as
workflow/project admission, not work-item submission.

The admitted request should be one of:

```text
source reference:
  project repository/ref/path
  workflow repository/ref/path
```

The current inline workflow JSON handler should not be promoted into the
persisted `/workflow` contract. If tests still need inline documents, they
should use a test helper, fixture-backed source reference, or an explicitly
separate admin/test path. The production `/workflow` design should be
source-reference first.

Acceptance criteria:

- The 012f implementation plan names the `/workflow` admission payload shape.
- `/workflow` accepts source-control references to project/workflow JSON.
- Inline project/workflow JSON is not part of the persisted `/workflow`
  contract.
- The design records where project/workflow source identity and semantic hashes
  are stored.
- No 012f text describes `/workflow` as client-submitted work items.

## 012f-b Persist Admitted Workflow Run And Initially Ready Compiled Work

After a workflow/project admission is accepted, the controller may compile the
initially ready work items and persist them in `work_items` and `queued_work`.
This is internal controller behavior, not the external `/workflow` contract.

For the first implementation, source-control resolution can be thin or
fixture/local-cache backed if the full GitHub adapter is not ready, but the
persisted run should still be created from source references and recorded
source identities. Do not use inline JSON as the persistence bridge.

Acceptance criteria:

- With `workflowStore` configured, `/workflow` does not append to
  `Controller.pending`.
- `/workflow` creates or references persisted project/workflow identities for
  the admitted source references.
- `/workflow` creates a persisted workflow run.
- Initially ready compiled work items are inserted into `work_items` and
  `queued_work` as controller-generated work.
- Existing `/work/next` can claim workflow-run work from the store.
- Store-configured duplicate checks query the store, not `hasWorkItemID`.
- In-memory fallback remains available when no store is configured.

## 012f-c Persisted Scaling Demand

When `/workflow` admits a run and starts workers with `workflowStore`
configured, scaling demand should come from persisted queued/running counts,
not `len(c.pending)` and `len(c.assigned)`.

Acceptance criteria:

- Store-configured `/workflow` computes queued/running demand from
  `queued_work` and `running_work`.
- In-memory fallback continues to use `pending` and `assigned`.
- Existing worker-start behavior remains otherwise unchanged.

## 012f-d Guard Tests

Add tests proving that store-configured workflow submission and raw work
submission do not mutate in-memory queue collections.

Acceptance criteria:

- Store-configured `/workflow` leaves `pending`, `assigned`, and `failed`
  unchanged.
- Store-configured `/work`, `/work/next`, `/work/complete`, and `/work/fail`
  still leave in-memory collections unchanged.
- Status for store-configured controllers derives from persisted rows.

## 012f-e Removal

`pending`, `assigned`, and `failed` were removed from `Controller`. Remaining
closure work is test/documentation cleanup, not queue-authority implementation.

## Out Of Scope

- Full GitHub/source-control adapter implementation.
- Final production project/workflow/run identity model for customer workflows.
- Dependency-aware stage publication.
- Retry/requeue policy.
- Worker leases, heartbeats, fencing, or abandoned-attempt recovery.
- Removing the old ledger helper path.
- Rewriting every legacy in-memory test in one prompt.

## Ambiguity To Review

The biggest ambiguity is how much source-control machinery 012f must implement
before `/workflow` can stop using inline JSON. The desired API is clear:
`/workflow` admits source references. The implementation can still be
incremental by using a narrow local source-reference resolver or fixture-backed
resolver before the full GitHub/cache implementation exists.

Recommended first implementation: define the source-reference admission
envelope and add the smallest resolver boundary needed to load project and
workflow JSON from a pinned local/source-control cache reference. If that is too
large for 012f, split a source-reference admission slice before removing
in-memory queue authority.

The in-memory fields have been removed. Tests that still mention legacy inline
workflow submission should be replaced with source-reference coverage or
retired with an explicit rationale.

## Implementation Notes

First implementation prompt:

- Store-configured `/workflow` now rejects the legacy inline JSON submission
  path with `501 Not Implemented`.
- This prevents the persisted live controller from writing workflow-submitted
  work into `Controller.pending`.
- No-store controllers still support the old inline workflow handler as a
  legacy fallback for existing tests.
- Source-reference `/workflow` admission remains unimplemented and should be
  handled by the source-reference admission/client follow-up slices.

Closure follow-up:

- Source-reference `/workflow` admission is now implemented through the
  `012f3` atoms.
- Live startup wires a local demo source adapter, performs a read-only recovery
  check, and opens normal admission before serving requests.
- Persisted source-reference admission can start local command-backed workers
  through `LocalWorkerStarter` when no configured `ExecutionEnvironment` is
  present.
- The local demo config now uses `.run/controller/workflow-execution.sqlite`,
  avoiding the older incompatible `.run/controller/ledger.sqlite` file.
- `012f4-guard-and-demotion-cleanup.md` now serves as the epic closure and
  boundary cleanup slice.

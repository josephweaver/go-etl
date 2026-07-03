# 012c Persistence-backed Raw Work Submission

Status: implemented

## Objective

Move the internal/admin `POST /work` raw work submission path onto
`internal/persistence.Store` when a workflow-execution store is configured.

This is a transitional bridge. It lets the controller persist and queue a single
raw work item before the full `/workflow` source-control-backed submission path
is cut over.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/012-controller-integration-cutover.md`
- `docs/epics/workflow-execution-persistence/012b-persistence-backed-status.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/model/work_item.go`
- `internal/persistence/store.go`

## Allowed Production Files

- `cmd/controller/main.go`

## Allowed Test Files

- `cmd/controller/main_test.go`

## Documentation Files

- `docs/epics/workflow-execution-persistence/012c-persistence-backed-raw-work-submission.md`
- `PROJECT_STATE.md`
- `epi_ctl/20260703.md`

## Transitional Raw Run Model

Raw work submission is not the final customer-facing path. It has no
source-controlled project/workflow document and no dependency-aware stage plan.
When `workflowStore` is configured, this slice should put raw work under a
controller-owned synthetic run:

```text
project_id   = "__raw_project__"
workflow_id  = "__raw_workflow__"
run_id       = "__raw_run__"
stage_index  = 0
```

These identifiers are deliberately reserved-looking and should be documented as
transitional. They allow existing `/work` tests and admin workflows to exercise
the persisted queue without pretending raw submissions have source-controlled
provenance.

## Model Conversion

`model.WorkItem` is the existing worker transport shape. `persistence.WorkItemRecord`
stores compact compiled worker payload JSON.

For 012c, store the whole `model.WorkItem` as `worker_payload_json`.

Reason:

- It preserves the current worker endpoint payload without inventing a new
  plugin payload schema inside controller cutover.
- `012d` can decode the stored payload back into `model.WorkItem` after
  `ClaimNextWork`.
- Later workflow compilation can replace this raw-work encoding with the
  agreed compact plugin payload shape.

`resolved_inputs_sha256` should be caller-independent and deterministic for
raw work. This slice may compute it as the SHA-256 of the stored
`worker_payload_json`, using the existing fingerprint helper if practical.

## Acceptance Criteria

- `POST /work` keeps the existing behavior when no workflow-execution store is
  configured.
- When `workflowStore` is configured, `POST /work` validates the incoming
  `model.WorkItem` exactly as before.
- When `workflowStore` is configured, raw submission ensures the synthetic raw
  project/workflow/run/stage records exist idempotently.
- When `workflowStore` is configured, raw submission inserts one
  `work_items` row and one `queued_work` row through persistence methods.
- Duplicate raw work item IDs return conflict rather than creating a second
  queue row.
- The persisted `worker_payload_json` can be decoded back into the original
  `model.WorkItem`.
- `/status` sees the new raw work as pending through the persistence-backed
  status path.
- This slice does not change `/work/next`, `/work/complete`, `/work/fail`, or
  `/workflow`.
- Existing tests continue to pass.

## Out Of Scope

- Full `/workflow` persistence.
- Source-control project/workflow provenance.
- Dependency-aware stage creation.
- Worker assignment from persistence.
- Completion/failure mutation through persistence.
- Worker scaling.
- Reuse/skip behavior.
- Removing in-memory queues.

## Ambiguity To Review

The synthetic raw run is intentionally not a real workflow run. It is useful for
cutover, tests, and admin submission, but it should not become the customer
workflow model. A later slice should either retire raw work submission or mark
it clearly as an internal/admin endpoint.

The raw `resolved_inputs_sha256` value is also transitional. Hashing the whole
worker transport payload is deterministic and useful for idempotency, but it is
not the final semantic input fingerprint model.

## Notes

- Prefer helper functions for ensuring the synthetic raw run and converting
  `model.WorkItem` to `persistence.WorkItemRecord`.
- Use persistence package methods rather than controller-side SQL.
- Do not update worker assignment yet; persisted raw work will become assignable
  in 012d.

# 004 Add Submission Status API

Status: Complete

## Objective

Add a controller-owned submission status endpoint:

```text
GET /submissions/{submission_id}/status
```

The endpoint reports execution status for one submitted workflow. It is the controller capability that later `goet status`, `goet submit ... --wait`, and `--json` output consume.

## Current State

Before this slice:

- Successful workflow submission returns a structured acknowledgement with `submission_id`, `workflow_id`, and `initial_work_item_count`.
- `cmd/controller/main.go` exposes aggregate `GET /status` for controller-wide counts.
- Aggregate `GET /status` is not scoped to one submitted workflow.
- `internal/model` has a submission acknowledgement type from slice 003.
- The workflow-execution persistence layer already owns queued/running/completed/failed placement facts for workflow runs.
- There is no `GET /submissions/{submission_id}/status` endpoint.
- There is no shared submission status response type.

## Target State

The controller exposes:

```text
GET /submissions/{submission_id}/status
```

The existing endpoint remains unchanged:

```text
GET /status
```

The new response includes at least:

- `submission_id`
- `workflow_id`
- `status`
- `known_work_items`
- `queued`
- `running`
- `completed`
- `failed`
- `skipped`

Example response:

```json
{
  "submission_id": "sub_1234",
  "workflow_id": "annual-report",
  "status": "running",
  "known_work_items": 47,
  "queued": 20,
  "running": 4,
  "completed": 23,
  "failed": 0,
  "skipped": 0
}
```

The status field should be derived by the controller from controller-owned facts. At minimum, support:

- `queued`
- `running`
- `completed`
- `failed`
- `unknown`

Suggested derivation for this slice:

- `failed` when failed count is greater than zero and no queued/running work remains.
- `running` when running count is greater than zero.
- `queued` when queued count is greater than zero and running count is zero.
- `completed` when known work items are greater than zero and all known work is completed or skipped.
- `unknown` only when the controller has no usable state for a valid but not-yet-classified submission.

Unknown submission IDs should return a meaningful HTTP error, preferably `404 Not Found`.

The endpoint should not expose controller filesystem paths, persistence internals, worker attempt details, or queue implementation details beyond the submission execution summary.

## Concept Decision

This slice updates the controller API surface and the public Submission model.

Use `internal/model/submission.go` for the status response type so the controller, client, and CLI share the same JSON contract. Add persistence helper methods only if existing workflow-run count methods are not sufficient.

The controller may map public `submission_id` to the existing workflow-run ID for this phase. Do not make the CLI or client infer this mapping.

## Required Context

Read these files first:

- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/submission-cli-status/003-return-submission-ack.md`
- `docs/CUSTOMER_API.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/model/submission.go`
- `internal/model/work_item.go`
- `internal/persistence/store.go`
- `internal/persistence/store_test.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `internal/model/submission.go`
- `internal/persistence/store.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `internal/model/submission_test.go`
- `internal/persistence/store_test.go`

## Out Of Scope

- Implementing the CLI status command.
- Implementing `--wait`.
- Implementing `--json` output.
- Implementing or accepting `--watch`.
- Hierarchical workflow, step, or work-item detail reporting.
- Execution observability or log streaming.
- Artifact reporting.
- Attempt detail reporting.
- SQLite schema redesign beyond a narrow query helper if needed.
- Retry behavior.
- Authentication or authorization.
- Redesigning workflow execution.
- Redesigning scheduler behavior.
- Changing worker execution behavior.

## Acceptance Criteria

- `GET /submissions/{submission_id}/status` is implemented.
- Existing `GET /status` behavior is unchanged.
- Valid submission IDs return structured submission status.
- Invalid or unknown submission IDs return a meaningful HTTP error.
- The response includes work-item counts grouped by execution state.
- The response includes a controller-derived `status` field.
- The response includes `workflow_id` when the controller can identify it from existing submission/run state.
- The endpoint is covered by unit tests.
- Any persistence helper added for submission-scoped counts is covered by unit tests.
- Existing workflow execution behavior remains unchanged.

## Notes

- The endpoint reports current known state only. Future workflow expansion may increase `known_work_items`.
- `skipped` may be zero until skipped work is represented in the same status data source.
- Do not expose internal table names, row IDs unrelated to the public submission ID, or local filesystem paths.
- This endpoint should be stable enough for CLI and SDK wrappers but does not need final hierarchical observability.


# 003 Return Submission Acknowledgement

Status: Complete

## Objective

Extend successful workflow submission so the controller returns a structured submission acknowledgement.

The acknowledgement gives the user a stable `submission_id` and basic admission facts that future status, wait, JSON output, and Python/R wrappers can use.

## Current State

Before this slice:

- The CLI can parse `goet submit` and load explicit controller/project/workflow JSON inputs.
- `internal/client.ControllerClient` submits workflow payloads to `POST /workflow`.
- The existing client submission path treats `204 No Content` as success.
- `cmd/controller/main.go` accepts `POST /workflow`, compiles or admits workflow work, persists/queues generated work items, and currently has no public submission acknowledgement body.
- There is no `submission_id` returned to the client.
- There is no shared submission acknowledgement type in `internal/model`.

## Target State

A successful `POST /workflow` returns a structured acknowledgement body.

Recommended HTTP status:

```text
202 Accepted
```

Recommended JSON shape:

```json
{
  "submission_id": "sub_1234",
  "workflow_id": "annual-report",
  "initial_work_item_count": 47
}
```

The acknowledgement fields mean:

- `submission_id`: stable public identifier for the submitted workflow run.
- `workflow_id`: the workflow definition ID accepted by the controller.
- `initial_work_item_count`: number of work items created or queued during initial admission/compilation.

The `submission_id` may be backed by the existing workflow-run ID for this phase if that is the narrowest controller-owned implementation. The public name remains `submission_id` so CLI and future SDKs do not expose internal persistence naming.

The CLI should print a simple human-readable acknowledgement after successful `goet submit`, for example:

```text
Submission: sub_1234
Workflow: annual-report
Initial work items: 47
```

Existing internal client methods that return only `error` may remain for compatibility, but the client package should expose a submission method that returns the acknowledgement for CLI use.

## Implemented State

Implemented in this slice:

- `internal/model.SubmissionAcknowledgement` owns the shared acknowledgement transport shape.
- Successful `POST /workflow` admission returns `202 Accepted` with acknowledgement JSON.
- The current `submission_id` value is the controller-generated workflow-run ID.
- `workflow_id` comes from the admitted workflow definition.
- `initial_work_item_count` comes from the number of initial compiled work items inserted/queued during admission.
- `internal/client.ControllerClient` exposes acknowledgement-returning submission methods while retaining error-only compatibility methods that discard the acknowledgement.
- `goet submit` prints the human-readable acknowledgement by default.

## Concept Decision

This slice adds the first public Submission transport concept.

Create or update `internal/model/submission.go` for submission acknowledgement types rather than placing submission concepts in `internal/model/work_item.go`. `WorkItem` and `Submission` are separate public model concepts.

The controller remains the owner of the submitted workflow identity and initial work-item count. The CLI must not synthesize `submission_id` locally.

## Required Context

Read these files first:

- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/submission-cli-status/001-cli-client-contract.md`
- `docs/concepts/submission-cli-status/002-deserialize-cli-json-inputs.md`
- `docs/CUSTOMER_API.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `cmd/demo-client/main.go`
- `cmd/demo-client/main_test.go`
- `internal/client/README.md`
- `internal/client/controller_client.go`
- `internal/client/controller_client_test.go`
- `internal/model/work_item.go`
- `internal/workflow/workflow.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/demo-client/main.go`
- `internal/client/controller_client.go`
- `internal/model/submission.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/demo-client/main_test.go`
- `internal/client/controller_client_test.go`
- `internal/model/submission_test.go`

## Out Of Scope

- Implementing `GET /submissions/{submission_id}/status`.
- Implementing `goet status` against the controller.
- Implementing submission progress reporting.
- Implementing workflow or step hierarchy in status.
- Implementing `--wait` behavior.
- Implementing `--json` output.
- Implementing or accepting `--watch`.
- Persisting a separate submissions table unless the existing workflow-run identity/state is insufficient.
- Durable queue redesign.
- Retry behavior.
- Artifact tracking.
- Authentication or authorization.
- Redesigning workflow compilation.
- Changing worker execution behavior.
- Python or R SDKs.

## Acceptance Criteria

- Successful workflow submission returns a structured acknowledgement body.
- The acknowledgement includes a stable `submission_id`.
- The acknowledgement includes the submitted `workflow_id`.
- The acknowledgement includes the number of work items created or queued during initial admission/compilation.
- The client package can return the acknowledgement to callers.
- `goet submit` prints a human-readable acknowledgement by default.
- Existing client tests are updated so intentional acknowledgement behavior is explicit.
- Any compatibility method that discards the acknowledgement remains covered if retained.
- Unit tests verify the controller acknowledgement response.
- Unit tests verify client acknowledgement decoding.
- Any internal submission tracking introduced remains controller-owned and does not change existing orchestration responsibilities.

## Notes

- Prefer using the existing workflow-run identity as the backing ID if that keeps this slice small and correct.
- Do not attempt to compute the final number of work items for workflows that may expand dynamically during future dependency-aware execution.
- `initial_work_item_count` is an admission fact, not a promise that the final workflow will never create more work.
- A later status endpoint will report `known_work_items` for the current status view.


# 003 Return Submission Acknowledgement

Status: Proposed

## Objective

Extend the existing workflow submission path so that a successful workflow submission returns a structured submission acknowledgement.

The current `POST /workflow` endpoint already accepts workflow submissions. This slice enhances that endpoint so the client receives a stable `submission_id` and enough information to identify and monitor the submitted workflow.

Any internal controller structures required to support this acknowledgement should be introduced as part of this slice.

## Required Context

Read these files first:

* docs/concepts/submission-cli-status/README.md
* docs/concepts/submission-cli-status/001-upgrade-demo-client-cli-arguments.md
* docs/concepts/submission-cli-status/002-deserialize-cli-json-inputs.md
* docs/CUSTOMER_API.md
* cmd/controller/main.go
* internal/client/local_controller.go
* internal/workflow/workflow.go
* internal/model/work_item.go

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

* cmd/controller/main.go
* internal/client/local_controller.go
* internal/model/work_item.go

## Allowed Test Files

* cmd/controller/main_test.go
* internal/client/local_controller_test.go

## Required Behavior

Update the existing workflow submission path so that a successful submission returns a structured acknowledgement.

The acknowledgement should include at least:

* `submission_id`
* `workflow_id`
* number of work items initially queued

The `submission_id` must be stable for the lifetime of the submission and will be used by future status APIs.

The controller may introduce whatever internal submission tracking is required to support this behavior, provided orchestration ownership remains with the controller.

The reported work-item count represents the work items created during the initial workflow compilation. It should not imply that the controller already knows the final number of work items that may exist after later workflow expansion.

## Out Of Scope

* Implementing `goet status`.
* Implementing submission progress reporting.
* Implementing workflow or step status.
* Implementing `--wait`.
* Implementing `--watch`.
* Persisting submissions to SQLite.
* Durable queue redesign.
* Retry behavior.
* Artifact tracking.
* Python or R SDKs.
* Authentication or authorization.
* Redesigning workflow compilation.
* Changing worker execution behavior.
* Changing scheduler behavior.

## Acceptance Criteria

* Successful workflow submission returns a structured acknowledgement.
* The acknowledgement includes a stable `submission_id`.
* The acknowledgement includes the submitted `workflow_id`.
* The acknowledgement reports the number of work items created during the initial workflow compilation.
* Existing workflow submission behavior continues to function.
* Unit tests verify the acknowledgement response.
* Any internal submission tracking introduced remains controller-owned and does not change existing orchestration responsibilities.

## Notes

* Reuse the existing `POST /workflow` endpoint rather than introducing a new submission endpoint.
* The exact implementation of submission tracking is intentionally left to the implementation.
* Do not attempt to compute the final number of work items for workflows that may expand dynamically during execution.
* This slice establishes the public acknowledgement contract that later status slices will build upon.

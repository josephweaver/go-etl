# 005 Add CLI Status Command

Status: Proposed

## Objective

Implement the `goet status` command in `cmd/demo-client`.

This slice connects the long-term CLI client to the submission-scoped controller status endpoint added in slice `004`.

The command should query a specific submission and display a human-readable status summary.

## Required Context

Read these files first:

* docs/epics/submission-cli-status/README.md
* docs/epics/submission-cli-status/001-upgrade-demo-client-cli-arguments.md
* docs/epics/submission-cli-status/004-add-submission-status-api.md
* docs/CUSTOMER_API.md
* cmd/demo-client/main.go
* internal/client/local_controller.go

Do not read unrelated files unless test failures require them.

## Allowed Production Files

* cmd/demo-client/main.go
* internal/client/local_controller.go

## Allowed Test Files

* cmd/demo-client/main_test.go
* internal/client/local_controller_test.go

## Required Behavior

Implement:

```text
goet status <submission_id>
    [--controller-url <url>]
```

Rules:

* `submission_id` is required.
* If `--controller-url` is omitted, default to:

```text
http://localhost:8080
```

* The command calls:

```text
GET /submissions/{submission_id}/status
```

* The command prints a human-readable status summary.
* Unknown submission IDs should produce a useful user-facing error.
* Controller connection failures should produce a useful user-facing error.

Expected human-readable output may be simple for this slice:

```text
Submission: sub_1234
Workflow: annual-report
Status: running

Known work items: 47
Queued: 20
Running: 4
Completed: 23
Failed: 0
Skipped: 0
```

## Out Of Scope

* Implementing `--watch`.
* Implementing `--wait`.
* Implementing `--json` output.
* Implementing hierarchical workflow or step rendering.
* Creating new controller endpoints.
* Changing submission status semantics.
* Implementing client-side remembered state.
* Python or R SDKs.
* Authentication or authorization.
* Artifact reporting.
* Retry behavior.

## Acceptance Criteria

* `goet status <submission_id>` is implemented.
* `goet status` requires a submission ID.
* `goet status` defaults to `http://localhost:8080` when no controller URL is supplied.
* The command calls the submission-scoped status endpoint.
* Successful responses are displayed in human-readable form.
* Unknown submission IDs return useful errors.
* Controller connection failures return useful errors.
* Unit tests cover argument validation and response handling.

## Notes

* Keep output simple.
* Do not add client-side state.
* Do not remember previous submissions.
* This slice should make status lookup usable before adding watch, wait, or JSON output.

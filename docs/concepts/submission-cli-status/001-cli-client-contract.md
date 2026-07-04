# 001 Upgrade Demo Client CLI Arguments

Status: Proposed

## Objective

Upgrade `cmd/demo-client` from a demo-oriented command into the first long-term GOET CLI entry point.

This slice implements the initial command-line argument structure for:

* `goet submit`
* `goet status`

The implementation should establish the public CLI shape while preserving enough existing demo-client behavior to keep current local workflow submission usable.

## Required Context

Read these files first:

* docs/concepts/submission-cli-status/README.md
* docs/CUSTOMER_API.md
* docs/ARCHITECTURE_OVERVIEW.md
* cmd/demo-client/main.go
* internal/client/local_controller.go
* internal/client/local_controller_test.go

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

* cmd/demo-client/main.go
* internal/client/local_controller.go

## Allowed Test Files

* cmd/demo-client/main_test.go
* internal/client/local_controller_test.go

## Required CLI Shape

### Submit

```text
goet submit
    --controller <controller.json>
    --controller-url <url>
    --project <project.json>
    --workflow <workflow.json>
    [--wait]
    [--watch <seconds>]
    [--json]
```

Rules:

* `submit` is the primary workflow-submission command.
* Exactly one of `--controller` or `--controller-url` must be supplied.
* Supplying both is an error.
* `--project` is required.
* `--workflow` is required.
* `--controller` means the client may start or connect to a local controller using the supplied config.
* `--controller-url` means the client submits to an already running controller.
* `--wait` is accepted by the argument parser, but full wait behavior may remain limited to existing demo-client behavior if necessary.
* `--watch` is accepted by the argument parser, but full watch behavior may be deferred.
* `--json` is accepted by the argument parser, but full JSON output may be deferred.

### Status

```text
goet status <submission_id>
    [--controller-url <url>]
    [--watch <seconds>]
    [--json]
```

Rules:

* `status` is accepted as a top-level command.
* `submission_id` is required.
* If `--controller-url` is omitted, default to:

```text
http://localhost:8080
```

* `--watch` is accepted by the argument parser.
* `--json` is accepted by the argument parser.
* Full controller-backed submission status may be deferred until the controller status model exists.

## Out Of Scope

* Implementing first-class controller Submission storage.
* Creating new controller HTTP endpoints.
* Defining final controller, project, or workflow JSON schemas.
* Implementing hierarchical workflow/step/work-item status.
* Implementing final `--wait` behavior.
* Implementing final `--watch` behavior.
* Implementing final `--json` output.
* Implementing Python or R SDKs.
* Authentication or authorization.
* Artifact management.
* Retry behavior.
* Durable queue redesign.
* Remote SSH setup automation.
* Renaming the binary outside this command source.

## Acceptance Criteria

* `cmd/demo-client` recognizes `submit` and `status` as top-level commands.
* `submit` validates that exactly one of `--controller` or `--controller-url` is supplied.
* `submit` requires `--project` and `--workflow`.
* `submit` accepts `--wait`, `--watch`, and `--json` flags.
* `status` requires a `submission_id`.
* `status` defaults to `http://localhost:8080` when `--controller-url` is omitted.
* `status` accepts `--watch` and `--json` flags.
* Invalid argument combinations return useful errors.
* Existing local demo workflow submission remains usable, either through the new `submit` command or through an explicit compatibility path.
* Tests cover argument parsing and validation behavior.

## Notes

* This slice should focus on CLI argument parsing and command dispatch.
* It should make the demo client look like the future long-term CLI without requiring the full controller submission model yet.
* Prefer small helper functions for parsing and validating command arguments so later slices can attach real controller behavior.
* Do not implement hidden client-side state.
* Do not remember prior submissions or controller URLs.
* The CLI should remain a thin client over controller behavior.

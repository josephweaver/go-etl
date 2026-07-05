# 011 Update Observability Docs And Smoke

Status: Implemented

## Objective

Update user-facing documentation and smoke coverage to describe and verify submission-addressable execution logs.

This slice closes the Execution Observability Strategic Concept after implementation slices 001 through 010 are complete.

## Current State

The implementation now has the expected observability behavior, but documentation and smoke coverage may still describe the older state:

- Execution Observability proposed docs may not match the implemented endpoint/CLI behavior.
- `PROJECT_STATE.md` may still say execution observability is deferred.
- CLI docs may mention submit/status/wait but not `goet logs`.
- Worker/controller README files may not describe controller-owned logs.
- The Python WorkItem smoke path may verify worker-local attempt logs but not controller-owned submission logs.

## Target State

Project documentation describes the implemented current state:

- `goet logs <submission_id>` is the public bounded log retrieval command.
- `GET /submissions/{submission_id}/logs` is the controller-owned log-read API.
- `POST /observations/logs` is the worker/controller log-ingestion API.
- Controller-owned JSONL filesystem logs are the durable observability store.
- Worker fallback logs are emergency diagnostics only.
- Python subprocess stdout/stderr are visible through controller-owned logs by submission ID.
- Logs are distinct from attempts, artifacts, and future execution events.
- Built-in watch/follow behavior is intentionally absent.

Smoke coverage demonstrates at least one submitted Python workflow whose stdout/stderr can be retrieved through `goet logs <submission_id>`.

## Concept Decision

This slice updates documentation and smoke-test artifacts. It should not introduce new production observability behavior beyond small smoke-script wiring needed to exercise implemented behavior.

Do not mark the Strategic Concept `Implemented` unless the human explicitly wants that status update as part of this slice's execution report. If the human has agreed all slices are complete, setting the README status to `Implemented` is appropriate.

## Required Context

Read these files first:

- `docs/concepts/execution-observability/README.md`
- `docs/concepts/execution-observability/001-logging-model.md`
- `docs/concepts/execution-observability/002-log-configuration.md`
- `docs/concepts/execution-observability/003-controller-logging-endpoint.md`
- `docs/concepts/execution-observability/004-worker-logging-client.md`
- `docs/concepts/execution-observability/005-controller-filesystem-log-sinks.md`
- `docs/concepts/execution-observability/006-worker-fallback-logging.md`
- `docs/concepts/execution-observability/007-python-subprocess-log-emission.md`
- `docs/concepts/execution-observability/008-log-levels-and-filtering.md`
- `docs/concepts/execution-observability/009-submission-log-read-api.md`
- `docs/concepts/execution-observability/010-cli-logs-command.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/worker/README.md`
- `cmd/demo-client/README.md`
- `scripts/python-workitem-smoke.ps1`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

None.

## Allowed Test Or Script Files

- `scripts/python-workitem-smoke.ps1`

## Allowed Documentation Files

- `docs/concepts/execution-observability/README.md`
- `PROJECT_STATE.md`
- `TARGET_STATE.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/worker/README.md`
- `cmd/demo-client/README.md`

## Out Of Scope

- New observability endpoints.
- New CLI flags.
- New worker/client/controller behavior except smoke-script invocation of already implemented behavior.
- Log retention or cleanup policy.
- Python SDK or R SDK documentation beyond noting that future SDKs should wrap the same API.
- Attempt Ledger redesign.
- Execution event generalization.
- Built-in watch/follow behavior.

## Acceptance Criteria

- `PROJECT_STATE.md` describes controller-owned execution observability as implemented.
- `PROJECT_STATE.md` no longer says execution observability is deferred if all implementation slices are complete.
- `TARGET_STATE.md` remains consistent with the controller-owned public API direction.
- `cmd/controller/README.md` documents log ingestion and submission log read endpoints at a high level.
- `cmd/worker/README.md` documents worker log emission and fallback boundary.
- `cmd/demo-client/README.md` documents `goet logs <submission_id>`.
- Documentation explains that `goet logs` is a bounded read and does not implement watch/follow.
- Documentation distinguishes logs from attempts, artifacts, and future execution events.
- The Python WorkItem smoke path verifies that controller-owned logs can be retrieved for the submitted Python fixture.
- The smoke path does not rely on worker fallback logs for normal success.
- The narrowest useful tests or smoke command are run and reported.

## Notes

- Keep documentation current-state oriented after implementation.
- Do not over-document future SDK behavior. The important fact is that future SDKs should use the same controller log-read API.
- If the smoke script cannot be updated without making the slice too broad, document the manual smoke command sequence in `PROJECT_STATE.md` and report the limitation.

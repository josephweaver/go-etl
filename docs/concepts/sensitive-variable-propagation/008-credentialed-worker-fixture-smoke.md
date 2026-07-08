# 008 Credentialed Worker Fixture Smoke

Status: implemented

## Objective

Prove the sensitive-variable boundary with a small worker-local credential fixture, without using real credentials, real Google Drive, real cloud secret stores, real HPCC secrets, or large data.

This slice verifies the end-to-end behavior that matters for later credentialed data assets: the controller sees only protected refs, the worker resolves a worker-local secret, the operation uses it, and controlled outputs do not leak it.

## Current State

Earlier slices should provide the model, controller payload, worker resolution, Go handler context, Python materialization, and redaction tests. There may still be no end-to-end smoke showing all pieces together.

## Target State

A tiny fixture workflow can declare a protected ref such as:

```json
{
  "name": "fixture_token",
  "type": "string",
  "sensitive": true,
  "protected_ref": {
    "provider": "worker_env",
    "key": "GOET_FIXTURE_TOKEN"
  }
}
```

The smoke run sets `GOET_FIXTURE_TOKEN` only in the worker process environment.

A trusted Go fixture handler or Python fixture script uses the secret only to prove access, for example by checking whether it equals an expected test value or by deriving a non-secret fixture result.

The controller status and persisted output must not contain the secret.

## Concept Decision

Use fake credentials and small deterministic fixtures. Do not use actual Google Drive, rclone, OAuth, service accounts, HPCC credentials, or CDL/Yan/Roy data in this slice.

Preferred smoke shape:

```text
1. start controller without GOET_FIXTURE_TOKEN
2. submit workflow containing protected ref worker_env:GOET_FIXTURE_TOKEN
3. start worker with GOET_FIXTURE_TOKEN set
4. worker resolves and uses the secret
5. script/handler deliberately prints the secret to stdout in one negative-control step
6. GOET redacts captured stdout/stderr
7. controller completion/status/persistence contain redaction labels only
```

If a deliberately printed secret makes the smoke too noisy, keep that behavior in unit tests and use a clean smoke script.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- docs and files changed by slices 001-007
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- local demo/smoke scripts if present
- `cmd/controller/*config*.json`
- `cmd/worker/*config*.json`
- demo project fixture layout if available

Do not read real HPCC, SSH, rclone, or geospatial code unless an existing smoke harness requires a narrow compatibility check.

## Allowed Production Files

Prefer no runtime production changes.

Allowed fixture/runbook files:

- `docs/concepts/sensitive-variable-propagation/credentialed-worker-fixture-smoke.md`
- `scripts/sensitive-variable-fixture-smoke.ps1`
- `scripts/sensitive-variable-fixture-smoke.sh`
- small demo workflow/submission/script files in the sibling demo project if that is the existing fixture convention

Allowed production fixes only for bugs discovered by the smoke:

- worker redactor/materialization helpers
- controller status safe rendering
- fixture-specific config helpers if already part of demo patterns

## Allowed Test Files

No new Go tests required unless the smoke exposes a production bug.

## Out Of Scope

- Real secret managers.
- Real Google Drive access.
- rclone credential setup.
- HPCC credential propagation.
- Fake HPCC unless local smoke already works and a separate follow-up is needed.
- Large data downloads.
- Artifact scanning.

## Acceptance Criteria

- A small runbook explains how to run the fixture smoke.
- Worker-local env secret is available to the worker but not the controller.
- Controller can submit/compile/assign work containing only the protected ref.
- Worker resolves the protected ref and executes the fixture.
- Captured output does not contain the raw fixture secret.
- Controller status does not contain the raw fixture secret.
- Persisted completion/failure evidence does not contain the raw fixture secret.
- The runbook states that this proves the sensitive boundary, not a real credentialed provider.

## Notes

Do not assign this slice to `5.3-codex-spark`. Smoke tests and environment choreography have been unreliable for Spark-class runs.

## Implemented State

Implemented on 2026-07-08.

- Added the runbook `docs/concepts/sensitive-variable-propagation/credentialed-worker-fixture-smoke.md`.
- Added `scripts/sensitive-variable-fixture-smoke.ps1`, which creates temporary sibling-demo fixture files, starts the controller without `GOET_FIXTURE_TOKEN`, starts the worker with `GOET_FIXTURE_TOKEN`, submits a `worker_env:GOET_FIXTURE_TOKEN` protected reference, and verifies completion.
- The smoke fixture deliberately prints the raw fixture secret to stdout and stderr, then verifies captured logs and controller logs contain `${worker_env.GOET_FIXTURE_TOKEN}` instead of the raw sentinel.
- The smoke verifies controller status, submission status, worker output, captured stdout/stderr, controller logs, and `.run/controller/workflow-execution.sqlite` do not contain `goet-fixture-secret-008-do-not-persist`.

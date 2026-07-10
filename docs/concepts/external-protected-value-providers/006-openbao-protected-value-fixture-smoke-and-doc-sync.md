# 006 OpenBao Protected-Value Fixture Smoke and Doc Sync

Status: proposed

## Objective

Add a repeatable disposable OpenBao fixture smoke proving that an externally resolved secret crosses only the worker execution boundary, is usable by a work item, and is absent from GOET-controlled controller status, logs, captured subprocess output, completion evidence, and SQLite persistence.

Synchronize project-state and concept documentation after the smoke passes.

## Current State

The implemented sensitive-variable fixture proves the boundary for `worker_env`.

After slices 001-005, unit tests should prove:

- provider-neutral protected-reference metadata;
- registry dispatch;
- resolver injection across Go and Python execution;
- OpenBao KV v2 response handling;
- worker provider config, bootstrap token sources, and TLS construction.

There is still no end-to-end workflow proving the complete external-provider path.

## Target State

A fixture script performs a flow equivalent to:

```text
1. start a disposable OpenBao instance in development/fixture mode
2. seed a KV v2 string field with a unique sentinel
3. create a narrowly scoped fixture token when practical
4. start the GOET controller without the OpenBao token or secret
5. create worker config defining provider alias fixture_openbao
6. expose only the bootstrap token to the worker by token_env or token_file
7. submit a workflow containing:
     provider: fixture_openbao
     key: goet/fixture
     field: token
8. worker resolves the value from OpenBao
9. fixture Python code deliberately prints the raw secret to stdout and stderr
10. GOET scrubs captured output and reports completion
11. smoke scans controlled surfaces and SQLite for the raw sentinel
12. stop and remove all disposable fixture resources
```

Example protected reference:

```json
{
  "name": "fixture_token",
  "type": "string",
  "sensitive": true,
  "protected_ref": {
    "provider": "fixture_openbao",
    "key": "goet/fixture",
    "field": "token"
  },
  "materialize": {
    "mode": "env",
    "target": "GOET_FIXTURE_TOKEN"
  }
}
```

Use a sentinel such as:

```text
goet-openbao-fixture-secret-006-do-not-persist
```

Use a distinct bootstrap-token sentinel.

## Concept Decision

The smoke should use a real OpenBao process or container, not merely an `httptest` server. Unit tests already own protocol simulation.

The fixture may use OpenBao development mode only because the fixture is disposable and isolated. Documentation must state that development mode, root tokens, and loopback HTTP are not production guidance.

Pin an explicit OpenBao image version or digest selected during implementation. Do not use `latest`.

Prefer a narrow fixture token or policy over the development root token for worker retrieval. If fixture setup complexity requires the root token, document that it is fixture-only and ensure it exists only in the disposable worker bootstrap surface.

The smoke must start the controller without both:

- the bootstrap token;
- the stored secret value.

This proves the controller cannot resolve or persist either value.

The fixture script should reuse the existing sensitive-variable smoke conventions where practical.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/external-protected-value-providers/README.md`
- all prior slices in this concept
- `docs/concepts/sensitive-variable-propagation/008-credentialed-worker-fixture-smoke.md`
- `docs/concepts/sensitive-variable-propagation/credentialed-worker-fixture-smoke.md`
- `scripts/sensitive-variable-fixture-smoke.ps1`
- existing worker and controller fixture config conventions
- current OpenBao container documentation used to select a pinned fixture version

Do not read LandCore production data, real HPCC credentials, or cloud provider code.

## Allowed Production Files

Prefer no production runtime changes.

Allowed documentation and fixture files:

- `docs/concepts/external-protected-value-providers/openbao-fixture-smoke.md` (new)
- `scripts/openbao-protected-value-fixture-smoke.ps1` (new)
- `scripts/openbao-protected-value-fixture-smoke.sh` (optional new)
- small temporary or checked-in fixture workflow/script/config files following the repository's existing fixture convention
- `PROJECT_STATE.md`
- `docs/IMPLEMENTED_CAPABILITIES.md`
- `docs/TEST_AND_SMOKE_STATUS.md`
- `docs/RUNTIME_RUNBOOK.md`
- `docs/concepts/external-protected-value-providers/README.md`
- `docs/concepts/sensitive-variable-propagation/README.md` only for a narrow relationship/status update

Allowed production fixes only for defects exposed by the smoke:

- files changed by slices 002-005

Report any such defect and keep the fix narrow.

## Allowed Test Files

No new Go tests are required unless the smoke exposes a production defect not already covered by unit tests.

## Out Of Scope

- Real LandCore, Rice, MSU, GitHub, Google Drive, cloud, or HPCC credentials.
- Production OpenBao deployment guidance beyond the worker-provider boundary.
- High availability, unsealing, storage backends, backups, replication, or disaster recovery.
- AppRole, OIDC, Kubernetes auth, cloud IAM, or dynamic credentials.
- Remote non-loopback plaintext HTTP.
- Artifact-content scanning.
- Network exfiltration prevention.
- Performance or load testing.
- Automatic secret rotation tests.

## Acceptance Criteria

- The fixture pins an explicit OpenBao version or digest.
- The fixture is repeatable and cleans up disposable processes, containers, files, and tokens.
- The controller starts without the bootstrap token and stored secret.
- The worker alone receives the bootstrap credential source.
- The workflow and assignment contain only provider alias, key, field, optional version, redaction label, and materialization metadata.
- The worker resolves the secret through `fixture_openbao`.
- The fixture operation proves it received the correct value.
- Deliberately printed secret plaintext is absent from final captured stdout and stderr.
- Controller logs contain no raw secret or bootstrap token.
- Worker output contains no raw secret or bootstrap token.
- Controller status and submission status contain no raw secret or bootstrap token.
- Completion and failure evidence contain no raw secret or bootstrap token.
- SQLite inspection finds neither sentinel.
- Safe redaction labels remain visible enough to diagnose which reference was used.
- The runbook clearly distinguishes fixture mode from production OpenBao deployment.
- `PROJECT_STATE.md` identifies the external protected-value provider concept as implemented only after all checks pass.
- Relevant capability, smoke-status, and runtime documentation is updated.
- Existing sensitive-variable smoke still passes.

## Minimum Implementation Model

Minimum recommended model: `Codex 5.5`, high reasoning.

Do not assign the environment choreography and persistence inspection to a Spark-class model. The smoke spans OpenBao lifecycle, controller/worker process separation, workflow submission, negative-control leakage, cleanup, and documentation synchronization.

## Notes

The fixture should scan at least:

```text
controller status JSON
submission status JSON
controller log files
worker log files
captured stdout/stderr
worker logical output
workflow-execution SQLite database
temporary fixture directory after cleanup where practical
```

The bootstrap token and stored secret must use different sentinels so the smoke can distinguish provider-auth leakage from work-item-secret leakage.

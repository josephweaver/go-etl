# Credentialed Worker Fixture Smoke

Status: implemented

## Purpose

This runbook verifies the phase-1 sensitive-variable boundary with a fake worker-local credential. It does not use Google Drive, rclone, HPCC secrets, cloud secret stores, OAuth, service accounts, private hostnames, or large data.

The proof target is narrow:

- the controller submission contains only `protected_ref` metadata for `worker_env:GOET_FIXTURE_TOKEN`;
- the controller process starts without `GOET_FIXTURE_TOKEN`;
- the worker process starts with `GOET_FIXTURE_TOKEN`;
- the Python fixture receives a materialized env var and proves it matches the expected SHA-256;
- the fixture deliberately prints the raw secret to stdout and stderr;
- GOET-controlled stdout/stderr capture, controller logs, status, completion evidence, and SQLite persistence do not contain the raw secret.

## Command

From the `go-etl` repository root:

```powershell
pwsh -NoProfile -File scripts/sensitive-variable-fixture-smoke.ps1
```

If `pwsh` is not available on the Windows host, use Windows PowerShell:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/sensitive-variable-fixture-smoke.ps1
```

The script uses `go run ./cmd/controller`, `go run ./cmd/worker`, the sibling `../go-etl-demo-project`, and the local Python interpreter found as `python3` or `python`.

## What The Script Creates

The script writes temporary fixture files into the sibling demo project:

```text
../go-etl-demo-project/workflows/sensitive-variable-fixture.json
../go-etl-demo-project/submissions/sensitive-variable-fixture-local.json
../go-etl-demo-project/scripts/sensitive_variable_fixture.py
```

These files are deleted at the end of the run. During the run, the controller admits them through the existing `local:demo` source provider and copies admitted source bytes into the controller source cache.

Runtime evidence is written under:

```text
.run/sensitive-variable-fixture-smoke/
.run/controller/workflow-execution.sqlite
cmd/worker/.run/
```

## Expected Behavior

The smoke should finish with:

```text
Smoke path completed.
```

The worker output file should be:

```text
cmd/worker/.run/data/sensitive-variable-fixture-check.json
```

That JSON should show:

- `fixture_token_available: true`
- `fixture_token_sha256_matches: true`
- `secret_reached_argv: false`
- `secret_reached_input_json: false`

The raw fixture secret is:

```text
goet-fixture-secret-008-do-not-persist
```

The script fails if that exact value appears in:

- the submitted JSON payload;
- controller `/status`;
- submission status JSON;
- worker process output;
- worker logical output file;
- captured worker stdout/stderr logs;
- controller log endpoint JSON;
- `.run/controller/workflow-execution.sqlite`.

The script also checks that captured stdout, captured stderr, controller logs, and SQLite persistence include the redaction label:

```text
${worker_env.GOET_FIXTURE_TOKEN}
```

## Boundary Proven

This smoke proves that a protected reference can pass through controller admission, compilation, assignment, worker resolution, Python materialization, captured output redaction, controller log recording, status, and SQLite persistence without storing or reporting the raw worker-local secret.

It does not prove a real credentialed provider. It does not prove Google Drive, HPCC, rclone, OAuth, service accounts, external secret managers, transformed leak detection, artifact-content scanning, or malicious script containment.

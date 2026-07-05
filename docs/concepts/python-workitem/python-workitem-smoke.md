# Python WorkItem Smoke Path

This runbook proves the local Python WorkItem path from the sibling demo project through controller admission, worker staging, Python execution, output promotion, and controller idle state.

## Prerequisites

- `go-etl` checked out at the repo root.
- `../go-etl-demo-project` checked out next to this repository.
- `python3` on `PATH`.
- `pwsh` available for the smoke script.

The smoke path depends on `cmd/worker/demo-config.json`, which defaults the worker Python executable to `python3`.

## Repository Layout Assumptions

- Controller config: `cmd/controller/demo-config.json`
- Worker config: `cmd/worker/demo-config.json`
- Demo project root: `../go-etl-demo-project`
- Demo workflow: `../go-etl-demo-project/workflows/python-hello.json`
- Demo submission: `../go-etl-demo-project/submissions/python-hello-local.json`
- Demo entrypoint: `../go-etl-demo-project/scripts/hello.py`
- Demo environment: `../go-etl-demo-project/environments/system-python.json`

The worker config resolves `.run/...` relative to `cmd/worker/`, so the output file lands under:

```text
cmd/worker/.run/data/python-hello-hello.json
```

The controller writes its SQLite ledger under the repo root `.run/controller/` path.

## Smoke Command

Run this from the `go-etl` repository root:

```powershell
pwsh -NoProfile -File scripts/python-workitem-smoke.ps1
```

Use this to print usage:

```powershell
pwsh -NoProfile -File scripts/python-workitem-smoke.ps1 -Help
```

## Expected Controller Behavior

The script starts the controller with `go run ./cmd/controller ./cmd/controller/demo-config.json`.

After startup:

- `GET /healthz` should return `204`.
- `GET /status` should return JSON with `pending` and `assigned` counters.
- After the workflow submission is processed and the worker finishes, `pending` and `assigned` should both reach `0`.
- `POST /shutdown` should stop the controller cleanly when the smoke run is done.

The workflow submission uses the controller's local source-reference admission against `../go-etl-demo-project`.
The demo workflow also carries the worker launch settings, so the controller can start the local worker from the workflow variables.

## Expected Output File

The Python demo workflow fans out one work item with the deterministic output filename:

```text
cmd/worker/.run/data/python-hello-hello.json
```

That file should contain canonical JSON with these deterministic fields:

- `status: "ok"`
- `message: "python hello fixture completed"`
- `operation: "scripts/hello.py"`
- `observed_input_keys: ["work_item"]`
- `support_input_exists: true`

## Expected Evidence

The worker runner writes the admitted Python source bundle into an attempt-local staging tree under:

```text
cmd/worker/.run/tmp/attempts/<attempt-id>/
```

Expected logs:

- `cmd/worker/.run/tmp/attempts/<attempt-id>/logs/stdout.log`
- `cmd/worker/.run/tmp/attempts/<attempt-id>/logs/stderr.log`

The script also writes the captured controller logs to:

- `.run/python-workitem-smoke/controller.stdout.log`
- `.run/python-workitem-smoke/controller.stderr.log`

## What The Script Validates Automatically

- sibling demo project exists
- required demo fixture files exist
- JSON fixtures parse
- `hello.py` compiles with `python3 -m py_compile`
- controller becomes reachable
- workflow submission returns `204`
- controller becomes idle
- output file exists
- output JSON contains the expected deterministic fields
- worker attempt logs exist
- controller shuts down cleanly

## What Remains Manual

If `python3` is missing, the smoke path cannot complete because the worker config defaults to `python3`.

If the script fails before idle, inspect the controller log files above first.
The worker attempt log directory is the next place to check because it captures the Python subprocess stdout and stderr.

If the controller start or submission shape changes, run the same commands manually:

```powershell
go run ./cmd/controller ./cmd/controller/demo-config.json
go run ./cmd/worker ./cmd/worker/demo-config.json
```

The controller launch is normally enough because the workflow's worker settings already point at the worker command above.

## Troubleshooting

- If `GET /healthz` never succeeds, check `.run/python-workitem-smoke/controller.stderr.log`.
- If the workflow submits but never reaches idle, check the worker attempt logs under `cmd/worker/.run/tmp/attempts/<attempt-id>/logs/`.
- If the output file is missing, inspect `cmd/worker/.run/data/` and confirm the worker used the expected `python3` executable.
- If the demo repository fixture files are missing, stop and restore `../go-etl-demo-project` before rerunning the smoke path.

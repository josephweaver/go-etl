# Python WorkItem Smoke Path

This runbook proves the local Python WorkItem path from the sibling demo project through controller admission, worker staging, Python execution, output promotion, and controller idle state.

## Prerequisites

- `go-etl` checked out at the repo root.
- `../go-etl-demo-project` checked out next to this repository.
- a Windows `python3` or `python` on `PATH`, or a standard Windows `python.exe` install location.
- `pwsh` available for the smoke script.

The smoke path resolves `python3` first and then `python`. It writes a temporary worker config under `.run/` with `python_executable` set explicitly for the smoke run.
WSL Python is not used for this slice because the worker is a Windows process and the staged attempt paths are Windows filesystem paths.

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
The smoke script creates `cmd/worker/.run/logs`, `cmd/worker/.run/tmp`, and
`cmd/worker/.run/data` before it expects the worker to start.

## Smoke Command

Run this from the `go-etl` repository root:

```powershell
go build -o .run/goetl-controller.exe ./cmd/controller
go build -o .run/goetl-worker.exe ./cmd/worker
New-Item -ItemType Directory -Force -Path cmd/worker/.run/logs,cmd/worker/.run/tmp,cmd/worker/.run/data | Out-Null
pwsh -NoProfile -File scripts/python-workitem-smoke.ps1
```

Use this to print usage:

```powershell
pwsh -NoProfile -File scripts/python-workitem-smoke.ps1 -Help
```

## Expected Controller Behavior

The script starts the controller with a prebuilt `.run/goetl-controller.exe`
when it exists. If that file is absent, it falls back to
`go run ./cmd/controller --config ./cmd/controller/demo-config.json`.
The controller listens on `http://127.0.0.1:8080` for the smoke run.

After startup:

- `GET /healthz` should return `204`.
- `GET /status` should return JSON with `pending` and `assigned` counters.
- After the workflow submission is processed and the worker finishes, `pending` and `assigned` should both reach `0`.
- `POST /shutdown` should stop the controller cleanly when the smoke run is done.

The workflow submission uses the controller's local source-reference admission against `../go-etl-demo-project`.
For the smoke run, the script disables controller auto-start by overriding `worker_max_count` at submission time and then starts the worker explicitly with an absolute config path. This avoids the current relative-config-path failure mode in the workflow's built-in local worker launch settings.

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

- `.run/python-workitem-smoke/<run-id>/controller.stdout.log`
- `.run/python-workitem-smoke/<run-id>/controller.stderr.log`

## What The Script Validates Automatically

- sibling demo project exists
- required demo fixture files exist
- JSON fixtures parse
- `hello.py` compiles with the resolved Python interpreter
- controller becomes reachable
- workflow submission returns `204`
- controller becomes idle
- output file exists
- output JSON contains the expected deterministic fields
- worker attempt logs exist
- controller shuts down cleanly

## What Remains Manual

If neither `python3` nor `python` is available, the smoke path cannot complete unless the interpreter is installed at one of the standard Windows `python.exe` locations the script checks.

If the script fails before idle, inspect the controller log files above first.
The worker attempt log directory is the next place to check because it captures the Python subprocess stdout and stderr.

If the controller start or submission shape changes, run the same commands manually:

```powershell
go build -o .run/goetl-controller.exe ./cmd/controller
go build -o .run/goetl-worker.exe ./cmd/worker
New-Item -ItemType Directory -Force -Path cmd/worker/.run/logs,cmd/worker/.run/tmp,cmd/worker/.run/data | Out-Null
.run/goetl-controller.exe --config ./cmd/controller/demo-config.json
.run/goetl-worker.exe (Resolve-Path ./cmd/worker/demo-config.json)
```

The worker command should use an absolute config path. The relative workflow-owned `./cmd/worker/demo-config.json` path is not yet reliable for Python work because the worker currently resolves its runtime directories relative to the config path text it was given.

Use `http://127.0.0.1:8080` when checking the running controller by hand.

## Troubleshooting

- If `GET /healthz` never succeeds, check the per-run controller stderr log under `.run/python-workitem-smoke/<run-id>/`.
- If the workflow submits but never reaches idle, check the worker attempt logs under `cmd/worker/.run/tmp/attempts/<attempt-id>/logs/`.
- If the output file is missing, inspect `cmd/worker/.run/data/` and confirm the worker used the expected resolved Python executable.
- If the demo repository fixture files are missing, stop and restore `../go-etl-demo-project` before rerunning the smoke path.

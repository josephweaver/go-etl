# Operational Observability State

Last updated: 2026-07-07

Strategic Concept docs: [`../execution-observability/README.md`](../execution-observability/README.md)

This file preserves operational-observability current-state excerpts moved out of the root `PROJECT_STATE.md`.

## Current-State Excerpts
Dependency-aware workflow slice `011-surface-dependency-state-in-status-and-logs` is now implemented: `GET /submissions/{submission_id}/status` returns an optional dependency summary with workflow state, current stage index, stage count, per-stage/per-step state summaries, assignable-vs-blocked dependency counts, and failed stage/step/work-item reason without exposing retained dependency output JSON. `goet status --json` now prints the structured status response, human-readable `goet status` adds compact dependency stage lines, and controller-owned dependency transitions are emitted through the existing log-observation path so `goet logs <submission_id>` can show normalization, queued-stage, activation, completion, and failure messages.

Operational observability slice 010 (`010-cli-logs-command`) is now implemented: the demo CLI now supports `goet logs <submission_id> [--controller-url <url>] [--tail <n>] [--level <level>] [--stream <stream>] [--attempt-id <id>] [--json]`, with bounded, submission-scoped log retrieval via `internal/client` and compact/default rendering.
Operational observability slice 009 (`009-submission-log-read-api`) is now implemented: controller now exposes `GET /submissions/{submission_id}/logs` with bounded reads, optional level/stream/attempt filtering, known-submission validation, and bounded, deterministic tail metadata.

Operational observability slice 003 (`003-controller-logging-endpoint`) is now implemented: controller now registers `POST /observations/logs` with bounded request-size handling, JSON decode/validation behavior, and a success response that does not mutate queue/work state.

Operational observability slice 004 (`004-worker-logging-client`) is now implemented: the worker runtime has a dedicated log client that posts one `internal/model.LogObservation` to `POST /observations/logs`, validates each observation before send, and returns `*LogDeliveryError` on non-2xx responses, transport failures, encoding failures, and validation failures.

Operational observability slice 005 (`005-controller-filesystem-log-sinks`) is now implemented: the controller now persists accepted `internal/model.LogObservation` payloads to controller-owned JSONL files under `controller_log_root_path`, routing by controller-wide, submission, and attempt paths using path-safe IDs and serialized file appends.

Operational observability slice 006 (`006-worker-fallback-logging`) is now implemented: the worker logging client can now write emergency fallback diagnostics as JSONL to `<log_dir>/fallback-observations.jsonl` when `POST /observations/logs` delivery fails, while keeping controller ownership of normal GOET logs and preserving work execution flow.

The submission-cli-status documentation slice is now implemented in the living docs. `README.md`, `docs/CUSTOMER_API.md`, `cmd/demo-client/README.md`, `internal/client/README.md`, and `cmd/controller/README.md` now describe the current `goet submit` and `goet status` contract, including `--wait`, `--json`, and shell-driven repeated status display.
Operational Slice 001 (`001-logging-model`) is implemented in `internal/model/log_observation.go` as a transport type (`LogObservation`) with role-neutral validation and simple log-level helpers used by later ingestion/read APIs.

Operational Slice 008 records the repeatable local smoke path for that fixture.
`scripts/python-workitem-smoke.ps1` validates the sibling demo project, compiles
`scripts/hello.py`, starts the controller from
`cmd/controller/demo-config.json` with `--config`, waits for `/status`, submits
`submissions/python-hello-local.json` with a `worker_max_count=0` override,
starts the local worker explicitly with an absolute config path, verifies
the promoted output JSON at `cmd/worker/.run/data/python-hello-hello.json`,
verifies that `goet logs <submission_id>` returns controller-owned submission logs.
The smoke path checks the controller on `http://localhost:8080`, matching the
demo controller configuration.
It now resolves `python3` first and then `python`, and writes a temporary
worker config with `python_executable` set to a smoke-only wrapper that emits
one stdout line and one stderr line before delegating to the selected Python
interpreter.
The validated smoke path expects a Windows Python interpreter; WSL Python is
not used because the worker and its staged attempt paths are Windows-native.
The smoke script now writes per-run controller logs under
`.run/python-workitem-smoke/<run-id>/` to avoid reusing locked log files across
retries.

Operational observability slice 007 is now implemented: Python subprocess stdout
and stderr are replayed from the captured attempt logs into `internal/model`
`LogObservation` records via the worker logging client, with best-effort
delivery and fallback on failure.

# Execution Observability

Status: Complete

## Purpose

This Strategic Concept makes GOET execution logs controller-owned, structured, durable, and addressable by the public `submission_id` introduced by the completed Submission CLI Status Strategic Concept.

GOET can now submit a workflow, return a stable submission acknowledgement, query submission status, wait for terminal status, and emit JSON suitable for scripts. The next product gap is execution explanation: after a user runs a submitted workflow, the user needs a stable way to answer:

```text
What happened during this submission?
Which step or attempt produced this line?
Was this stdout, stderr, or GOET runtime output?
Can a future SDK fetch the same logs without scraping worker files?
```

This concept introduces that boundary through:

```text
POST /observations/logs
GET /submissions/{submission_id}/logs
goet logs <submission_id>
```

The core product gain is a controller-owned execution-observation path that keeps workers disposable, keeps log storage independent from worker local files, and lets users inspect execution output through the same submission identity used by `goet status` and `goet submit --wait`.

## Assumption From Previous Strategic Concept

This concept assumes `docs/concepts/submission-cli-status` is complete before implementation begins.

That means the following capabilities already exist and should be treated as stable inputs rather than redesigned here:

- `goet submit` submits an explicit project/workflow pair to a controller.
- Successful submission returns a structured acknowledgement containing `submission_id`.
- `goet status <submission_id>` calls a controller-owned status endpoint.
- `goet submit ... --wait` polls submission status until terminal state.
- `--json` emits machine-readable output without human text on standard output.
- The controller, not the CLI, owns submission status facts.

Execution Observability builds on that submission boundary. It must not redefine submission identity, status semantics, terminal states, or the `--wait` contract.

## Goals

The completed Strategic Concept should enable GOET to:

- Define a shared structured log-observation transport model in `internal/model`.
- Keep log observations distinct from work-item payloads, attempt ledger records, customer artifacts, and future execution events.
- Accept worker-submitted log observations through a bounded controller HTTP endpoint:

  ```text
  POST /observations/logs
  ```

- Use ordinary bounded HTTP request/response exchanges for log delivery. “Line-by-line streaming” means workers send completed log lines promptly; it does not mean a long-lived HTTP stream.
- Store accepted observations in controller-managed durable filesystem sinks.
- Use structured JSON Lines as the durable filesystem representation so later APIs and SDKs can return machine-readable logs without parsing human-rendered strings.
- Physically separate controller-wide, submission-level, and attempt-level log files.
- Route logs primarily by `submission_id`, then by attempt and stream metadata when available.
- Allow workers to submit observations without knowing controller filesystem layout.
- Treat logging as best-effort: logging failure must never fail a work item, mark a submission failed, or corrupt attempt status.
- Emit Python subprocess stdout/stderr as structured observations while preserving existing Python WorkItem output promotion and evidence behavior.
- Provide a controller-owned read API for submission logs:

  ```text
  GET /submissions/{submission_id}/logs
  ```

- Add a CLI command for bounded log retrieval:

  ```text
  goet logs <submission_id>
  goet logs <submission_id> --tail 100
  goet logs <submission_id> --json
  ```

- Apply configured minimum log levels before durable filesystem writes.
- Provide worker-side fallback logging only for emergency diagnostics when controller log delivery is unavailable.
- Keep fallback logs local, explicit, and non-authoritative.
- Update user-facing documentation and smoke coverage so a submitted Python fixture can be inspected by submission ID.

## Non-Goals

This Strategic Concept does not include:

- General lifecycle event system.
- Structured execution events, progress events, dependency events, or scheduler events beyond log observations.
- Attempt Ledger redesign.
- Durable database storage for log observations.
- Metrics, tracing, OpenTelemetry, dashboards, monitoring UI, or alerting.
- Authentication, authorization, redaction, or multi-tenant log policy.
- Log retention, compaction, archival, compression, rotation, or cleanup policy.
- Cross-controller log aggregation.
- Real-time browser UI or terminal TUI.
- Long-lived HTTP streaming, server-sent events, WebSockets, or `tail -f` semantics.
- Built-in `--watch` or `--follow` behavior. Repeated display should remain an operating-system composition concern.
- Artifact browsing or artifact download commands.
- Attempt detail commands beyond filtering logs by attempt ID where already available.
- Reconciliation or upload of worker fallback logs into controller-owned logs.
- Removing Python WorkItem's existing attempt-local stdout/stderr capture if it is still needed for wrapper evidence. This concept makes controller-owned logs the public observability path; it does not force a same-slice evidence redesign.
- Python SDK or R SDK log APIs. Future SDKs should wrap the same controller read API introduced here.

## Architectural Context

GOET's enduring architecture places the controller between public clients and workers. The controller owns orchestration state, submission admission, queue transitions, status facts, artifacts, and attempts. Workers are disposable execution processes that pull concrete assignments, execute them, and report outcomes.

Execution Observability follows the same ownership rule:

```text
CLI / future SDKs
        |
        | GET /submissions/{submission_id}/logs
        v
Controller
        |
        | controller-owned JSONL log sinks
        v
Filesystem

Workers / subprocesses
        |
        | POST /observations/logs
        v
Controller
```

Workers produce observations, but they do not own normal GOET log storage. The controller validates, routes, filters, and stores observations. The CLI and future SDKs read logs from the controller by `submission_id`; they do not inspect worker directories.

### Distinct concepts

This concept keeps four records separate:

- **Log observation**: a structured record carrying timestamp, level, component, stream, submission/attempt metadata, and one message line.
- **Attempt ledger record**: durable execution outcome and evidence facts owned by existing attempt/queue persistence.
- **Customer artifact**: user or workflow output such as promoted data files.
- **Execution event**: a future structured lifecycle/progress event stream; not implemented here.

A log observation may mention an attempt ID, work-item ID, step name, or stream, but it is not the authoritative attempt outcome.

### Delivery semantics

“Stream logs line-by-line” describes producer cadence, not HTTP transport shape. When a worker or subprocess has a completed log line, the worker sends a structured log observation to the controller through a normal bounded HTTP request. HTTP keep-alive may reuse connections, and later batching may be introduced, but no request or response remains open for the duration of a work item in this concept.

### Storage semantics

The first durable storage implementation is controller-managed filesystem JSON Lines. Each line is one validated log observation encoded as JSON. This choice preserves structure for the read API and future SDKs, avoids inventing a log database, and avoids parsing human-rendered lines back into fields.

Human-readable formatting belongs at read time or CLI presentation time. Filesystem paths are controller internals and must not be exposed as public API facts.

### Controller configuration defaults

Slice 002 publishes the controller-owned logging defaults that later slices must treat as stable inputs:

| Variable | Default |
| --- | ---: |
| `controller_config.controller_filesystem_logging_enabled` | `true` |
| `controller_config.controller_log_root_path` | `${controller_root_dir}/logs` |
| `controller_config.controller_log_level` | `info` |
| `controller_config.controller_log_read_default_tail_lines` | `100` |
| `controller_config.controller_log_read_max_tail_lines` | `1000` |

The source of truth is `cmd/controller/defaults.json`. `cmd/controller/controller-default-config.json` and `cmd/controller/demo-config.json` should inherit these values rather than duplicate them. This keeps the default controller configuration small and ensures validation tests exercise the inherited default path.

## Current State

### Strategic current state

GOET has a controller/worker runtime, a completed first Python WorkItem execution path, and a submission/status CLI contract. A user can submit work and monitor a submission by `submission_id`, but execution logs are not yet a controller-owned public capability.

The existing Execution Observability concept directory now records the completed controller-owned logging boundary, including the submission/status contract, the `goet logs` customer surface, the controller log-ingestion endpoint, the controller read API, and the worker fallback path.

### Operational current state

- `internal/model` currently owns shared controller/worker/client transport shapes such as work items and status payloads. It does not yet own a `LogObservation` transport shape.
- `cmd/controller` owns the current HTTP API surface for workflow submission, work assignment/reporting, status, source bundle delivery, and shutdown. It does not yet expose a log-ingestion endpoint or submission-log read endpoint.
- `cmd/controller` owns startup configuration and resolved controller operational policy. If logging-related configuration variables already exist in the current branch, they should be reused rather than duplicated.
- `cmd/worker` owns worker configuration, controller communication, work dispatch, local runtime directories, output promotion, and completion/failure reporting. It does not yet have a controller log-delivery client.
- `cmd/worker/work_python.go` owns Python work-item execution. Python stdout/stderr are currently captured under the worker attempt-local log directory as part of the completed Python WorkItem behavior. Those files are useful for current smoke/evidence behavior, but they are not a controller-owned public log path.
- `cmd/demo-client` is assumed to have been upgraded by the previous Strategic Concept into the `goet submit`, `goet status`, `--wait`, and `--json` CLI boundary.
- `internal/client` is assumed to have submission/status client helpers from the previous Strategic Concept, but it does not yet fetch submission logs.
- No implemented user command currently provides `goet logs <submission_id>`.

## Target State

### Strategic target state

GOET has a controller-owned execution-observability path. Users and future SDKs can inspect logs for one submission by calling the controller with the same `submission_id` used for status and wait. Workers and subprocesses emit structured observations upward; they do not expose normal logs through worker-local paths.

### Operational target state

- `internal/model/log_observation.go` defines a small shared `LogObservation` transport type and validation rules.
- A valid log observation carries enough metadata for routing and display, including:
  - timestamp;
  - level;
  - component;
  - stream;
  - message;
  - `submission_id` when the log belongs to a submitted workflow;
  - workflow/run/step metadata when available;
  - work-item, attempt, and worker metadata when available.
- `cmd/controller` resolves logging configuration from controller config/defaults, including:
  - whether filesystem logging is enabled;
  - controller-owned log root path;
  - minimum durable log level;
  - safe bounded defaults for log-read requests.
- `cmd/controller` exposes:

  ```text
  POST /observations/logs
  ```

  for worker-to-controller log ingestion.

- The ingestion endpoint validates each observation and forwards it to a controller-owned log sink.
- The first durable sink writes JSONL files under the controller log root, using a layout equivalent to:

  ```text
  logs/
    controller/
      controller.jsonl
    submissions/
      <submission_id>/
        submission.jsonl
        attempts/
          <attempt_id>.jsonl
  ```

- The sink uses safe path construction and never trusts submitted IDs as raw filesystem paths.
- The sink serializes concurrent writes enough to avoid corrupting JSONL files.
- The controller accepts log observations best-effort. Sink failures become warnings; they do not fail work execution or mutate submission status.
- `cmd/worker` can submit one structured observation to the controller log endpoint using the existing controller URL.
- Worker log delivery failures are returned or recorded as warnings and do not fail work execution.
- Worker fallback logging writes local emergency JSONL diagnostics only when controller log delivery fails.
- Python subprocess stdout and stderr are emitted as structured observations while preserving existing Python WorkItem output and evidence behavior.
- `cmd/controller` exposes:

  ```text
  GET /submissions/{submission_id}/logs
  ```

  returning a bounded JSON response of structured observations.

- `internal/client` can fetch submission logs from that endpoint.
- `cmd/demo-client` provides:

  ```text
  goet logs <submission_id> [--controller-url <url>] [--tail <n>] [--json]
  ```

- `goet logs` defaults to `http://localhost:8080` when `--controller-url` is omitted.
- `goet logs` has no built-in `--watch` or `--follow` option.
- Human CLI output is compact and readable by default.
- `--json` emits valid JSON to standard output and writes diagnostics to standard error.
- Documentation describes how `submit`, `status`, `wait`, and `logs` compose around `submission_id`.

## Public API Shape

The exact JSON structs should be defined in `internal/model`, but the intended external shape is:

```http
POST /observations/logs
Content-Type: application/json
```

```json
{
  "timestamp": "2026-07-05T11:00:00Z",
  "level": "info",
  "component": "worker",
  "stream": "stdout",
  "submission_id": "sub_1234",
  "workflow_id": "python-hello",
  "step_name": "hello",
  "work_item_id": "work_5678",
  "attempt_id": "att_9012",
  "worker_id": "worker-local-1",
  "message": "hello from python"
}
```

```http
GET /submissions/sub_1234/logs?tail=100
Accept: application/json
```

If `tail` is omitted, the controller uses `controller_log_read_default_tail_lines = 100`. Requests above `controller_log_read_max_tail_lines = 1000` should return a client error rather than silently clamping.

```json
{
  "submission_id": "sub_1234",
  "entries": [
    {
      "timestamp": "2026-07-05T11:00:00Z",
      "level": "info",
      "component": "worker",
      "stream": "stdout",
      "attempt_id": "att_9012",
      "message": "hello from python"
    }
  ],
  "tail": 100,
  "truncated": false
}
```

## Proposed Slices

These Operational Slices should be implemented one at a time. Each slice is intentionally bounded so Codex can operate from the slice document plus the listed context files.

| Slice | Document | Purpose |
| --- | --- | --- |
| 001 | `001-logging-model.md` | Define the shared structured log observation model and validation rules in `internal/model`. |
| 002 | `002-log-configuration.md` | Resolve controller logging configuration and defaults without opening files or endpoints. |
| 003 | `003-controller-logging-endpoint.md` | Add the controller log-ingestion endpoint for one structured observation. |
| 004 | `004-worker-logging-client.md` | Add worker-side best-effort delivery of one log observation to the controller. |
| 005 | `005-controller-filesystem-log-sinks.md` | Add controller-managed JSONL filesystem sinks and connect accepted observations to durable storage. |
| 006 | `006-worker-fallback-logging.md` | Add worker emergency fallback JSONL logging when controller delivery fails. |
| 007 | `007-python-subprocess-log-emission.md` | Emit Python subprocess stdout/stderr as structured observations through the worker logging path. |
| 008 | `008-log-levels-and-filtering.md` | Apply configured minimum log levels and routing rules before durable writes. |
| 009 | `009-submission-log-read-api.md` | Add `GET /submissions/{submission_id}/logs` for bounded controller-owned log retrieval. |
| 010 | `010-cli-logs-command.md` | Add `goet logs <submission_id>` and client support for reading submission logs. |
| 011 | `011-update-observability-docs-and-smoke.md` | Update docs and smoke coverage for submission-addressable execution logs. |

## Suggested Codex Prompt Pattern

Use one slice per Codex session:

```text
please read docs/concepts/complete/execution-observability/001-logging-model.md and implement exactly that slice
```

Then continue with the next numbered slice after the previous one is committed and accepted.

## Completion Criteria

This Strategic Concept is complete when:

- All agreed Operational Slices are implemented and accepted.
- `internal/model` contains a validated structured log-observation transport type.
- The controller exposes `POST /observations/logs` and accepts valid observations.
- Invalid JSON and invalid observations produce client errors from the ingestion endpoint.
- Controller-managed JSONL filesystem sinks exist for controller-wide, submission-level, and attempt-level logs.
- Filesystem sink writes are safe under concurrent log submissions.
- Logging failures produce warnings only and do not fail work execution or submission status.
- Workers can submit structured observations to the controller log endpoint.
- Workers write fallback diagnostics only when controller log delivery is unavailable.
- Python stdout/stderr lines are visible through controller-owned logs for the submitted workflow.
- The controller exposes `GET /submissions/{submission_id}/logs` and returns bounded structured log entries.
- `goet logs <submission_id>` displays logs for one submission.
- `goet logs <submission_id> --json` emits valid JSON with no human-readable text mixed into standard output.
- No built-in `--watch` or `--follow` behavior is added.
- Public interfaces remain consistent with the controller-owned orchestration architecture.
- Documentation explains the distinction between logs, attempt records, artifacts, and future execution events.
- Documentation describes how users compose `goet submit`, `goet status`, `goet submit --wait`, and `goet logs` around `submission_id`.

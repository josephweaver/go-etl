# Execution Observability

Status: Proposed

## Purpose

Define GOET's logging and execution observation model. This epic focuses on collecting, streaming, routing, and storing execution logs. Structured execution events are covered by a separate Execution Events epic.

## Goals

- Stream worker and subprocess logs to the Controller line-by-line.
- Centralize GOET-owned logging in the Controller.
- Avoid unmanaged log files.
- Support configurable filesystem log sinks and verbosity.
- Keep log producers independent from storage layout and line formatting.
- Allow worker-side fallback logging when the Controller is unavailable.

## Non-Goals

- General lifecycle event system.
- Attempt Ledger redesign.
- Metrics, tracing, or monitoring UI.
- Treating logging failures as work-item failures.

## Architectural Context

The Controller is the authoritative owner of execution observations. Workers, runtimes, plugins, and subprocesses emit log observations upward. The Controller routes those observations to configured sinks.

Workers may also write fallback logs when they are separated from the Controller. Worker fallback logs are emergency diagnostics, not a second authoritative logging system.

## HTTP Delivery Semantics

"Stream logs line-by-line" describes the observation cadence, not a long-lived
HTTP streaming connection. As a worker or subprocess produces a completed log
line, the worker sends a structured log observation through a normal bounded
HTTP request/response exchange. The HTTP client may reuse connections through
ordinary keep-alive behavior, but no request or response remains open for the
duration of the work item.

Later batching may combine several completed lines in one bounded request, but
it must preserve line boundaries and useful local ordering. This transport
optimization does not change the controller-owned logging model.

## Core Principles

- Controller owns GOET logging.
- Components emit logs rather than managing normal log files.
- Logs stream upward.
- Logging is best-effort.
- Failed logging must never fail work execution.
- Logging failures should produce warnings only.
- Customer artifacts, attempt records, execution events, and logs are distinct.
- Transfer metadata is structured, but rendered log lines are controlled by log configuration.

## Log Observation Model

Log producers should emit structured log observations. The structured object should carry enough metadata for routing, filtering, and formatting.

Example metadata:

```text
workflow_id
run_id
step_name
work_item_id
attempt_id
worker_id
component
level
stream
timestamp
message
```

The transfer object should not decide the final rendered line. The log sink configuration decides what gets written, similar to configurable Apache-style log formats.

## Filesystem Log Sinks

The first durable logging implementation should use controller-managed filesystem sinks.

The expected design is physically separated log files rather than one global append-only file with logical views.

Three initial filesystem sinks are expected:

```text
logs/controller/<date-time>.log
```

Controller-wide log stream.

```text
logs/workflows/<workflow-name>/<run-id>/<date-time>.log
```

Workflow/run-level log stream.

```text
logs/workflows/<workflow-name>/<run-id>/steps/<step-name>/<work-item-id>/<attempt-id>.log
```

Attempt-level log stream for focused debugging.

The exact root path and line format should be configurable.

## Ordering

Strict global ordering is not required. Logs should preserve useful local ordering within a run, work item, or attempt where practical.

Good hierarchy, metadata, and physical separation are more important than total ordering across the whole Controller.

## Worker Fallback Logging

If a worker cannot reach the Controller, it may write local fallback logs. These logs exist to diagnose Controller separation, network failure, or worker crash scenarios.

Fallback logging should not become the normal logging path. When the Controller is available, GOET-owned logs should stream to the Controller.

## Proposed Slices

001 Logging Model
002 Log Configuration
003 Controller Logging Endpoint
004 Worker Logging Client
005 Streaming Stdout/Stderr
006 Controller Filesystem Log Sinks
007 Worker Fallback Logging
008 Log Levels and Filtering

## Completion Criteria

- Streaming logging implemented.
- GOET avoids unmanaged log files during normal execution.
- Controller is the authoritative logging endpoint.
- Controller-managed filesystem sinks exist for controller, workflow/run, and attempt logs.
- Rendered log lines are configurable from structured log observations.
- Logging failures produce warnings only and do not fail work execution.

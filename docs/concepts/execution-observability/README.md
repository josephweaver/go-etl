# Execution Observability

Status: Proposed

## Purpose

Define GOET's logging and execution observation model. This epic focuses on collecting, streaming, routing, and storing execution logs. Structured execution events are covered by a separate Execution Events epic.

## Goals

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

## Concurrency

The Controller should expect concurrent log submissions from many workers.

- Concurrent log submission should be safe.
- Sink implementations should serialize writes appropriately.
- Logging should avoid unnecessary global locks where practical.
- The implementation does not require strict global ordering of log messages.
- Logging throughput should not become a bottleneck for work execution.

## Log Observation Model

Log producers should emit structured log observations. The structured object should carry enough metadata for routing, filtering, and formatting.

## Filesystem Log Sinks

The first durable logging implementation should use controller-managed filesystem sinks with physically separated files.

## Ordering

Strict global ordering is not required. Good hierarchy, metadata, and physical separation are more important than total ordering.

## Worker Fallback Logging

Fallback logging exists only for Controller separation and should never replace Controller-owned logging during healthy execution.

## Proposed Slices

001 Logging Model
002 Log Configuration
003 Controller Logging Endpoint
004 Worker Logging Client
005 Controller Filesystem Log Sinks
006 Worker Fallback Logging
007 Log Levels and Filtering

## Completion Criteria

- GOET avoids unmanaged log files during normal execution.
- Controller is the authoritative logging endpoint.
- Controller-managed filesystem sinks exist for controller, workflow/run, and attempt logs.
- Rendered log lines are configurable from structured log observations.
- Logging failures produce warnings only and do not fail work execution.
- Controller safely handles concurrent log submissions from multiple workers.
# Execution Observability

Status: Proposed

## Purpose

Define GOET's logging and execution observation model. This epic focuses on collecting, streaming, routing, and storing execution logs. Structured execution events are covered by a separate Execution Events epic.

## Goals
- Stream worker and subprocess logs to the Controller line-by-line.
- Centralize GOET-owned logging in the Controller.
- Avoid unmanaged log files.
- Support configurable log sinks and verbosity.
- Keep log producers independent from storage.

## Non-Goals
- General lifecycle event system.
- Attempt Ledger redesign.
- Metrics, tracing, or monitoring UI.

## Architectural Context
The Controller is the authoritative owner of execution observations. Workers, runtimes, plugins, and subprocesses emit log observations upward. The Controller routes those observations to configured sinks.

## Core Principles
- Controller owns GOET logging.
- Components emit logs rather than managing log files.
- Logs stream upward.
- Customer artifacts, attempt records, and logs are distinct.

## Proposed Slices
001 Logging Model
002 Controller Logging Endpoint
003 Worker Logging Client
004 Streaming Stdout/Stderr
005 Controller Log Sinks
006 Log Levels and Filtering

## Completion Criteria
- Streaming logging implemented.
- GOET avoids unmanaged log files.
- Controller is the authoritative logging endpoint.

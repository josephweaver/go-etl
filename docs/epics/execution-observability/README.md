# Execution Observability

Status: Proposed

## Purpose

Define GOET's execution observability model. The Controller is the authoritative collector of execution observations. Workers, runtimes, plugins, and subprocesses emit observations upward rather than creating GOET-owned log files throughout the system.

## Goals

- Establish a single architectural model for execution observations.
- Stream worker and subprocess output to the Controller line-by-line.
- Keep observation producers independent from observation storage.
- Support configurable observation sinks.
- Distinguish execution observations from attempt records and customer artifacts.
- Support configurable observation verbosity.

## Non-Goals

- Replace the Attempt Ledger.
- Define metrics or distributed tracing implementations.
- Define a monitoring UI.
- Require plugins or customer code to use GOET logging.

## Architectural Context

Execution Observability belongs to the Controller. The Controller owns orchestration state and therefore owns the authoritative execution history. Workers remain lightweight producers of observations.

## Core Principles

- The Controller is the authority for execution observations.
- Observations stream upward through the execution hierarchy.
- GOET should avoid unmanaged log files.
- Observation producers do not know where observations are stored.
- Artifacts, attempts, and observations are separate concepts.

## Execution Observation Flow

Client
→ Controller
→ Worker
→ Worker Plugin
→ Subprocess

Observations propagate upward to the Controller, which forwards them to configured sinks.

## Observation Levels

- ERROR
- WARN
- INFO
- VERBOSE
- DEBUG
- TRACE

## Proposed Slices

001 Observation Model
002 Controller Observation Endpoint
003 Worker Observation Client
004 Worker Log Streaming
005 Subprocess Stdout/Stderr Streaming
006 Controller Log Sink
007 Attempt-linked Observation Metadata
008 Configurable Observation Levels

## Completion Criteria

- Execution observations stream to the Controller.
- Components no longer create unmanaged GOET-owned log files.
- Observations can be routed to configurable sinks.
- Attempt records, artifacts, and observations remain architecturally distinct.
- The implementation aligns with GOET's controller-owned orchestration architecture.

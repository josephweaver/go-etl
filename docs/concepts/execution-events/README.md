# Execution Events

Status: Proposed

## Purpose

Generalize communication between GOET components around a common event model. Replace specialized worker messages such as completed and failed with typed execution events while preserving the Controller as the authoritative owner of orchestration state.

## Goals
- Define a common execution event schema.
- Generalize worker-to-controller communication.
- Support lifecycle events across Client, Controller, Worker, and Runtime.
- Enable future subscribers without requiring them.

## Non-Goals
- Logging and stdout/stderr streaming.
- Metrics and tracing.
- Attempt Ledger redesign.

## Architectural Context
Events represent structured state transitions. The Controller consumes events, updates orchestration state, and may dispatch them internally. This epic does not define log streaming.

## Proposed Slices
001 Event Model
002 Worker Event API
003 Controller Event Ingestion
004 Migrate Complete/Fail Messages
005 Additional Lifecycle Events
006 Internal Event Dispatch

## Completion Criteria
- Complete/fail replaced by generic events.
- Controller processes typed execution events.
- Existing orchestration behavior preserved.

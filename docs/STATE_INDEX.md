# State Index

Last updated: 2026-07-07

This index replaces the long-form root `PROJECT_STATE.md`. The exact pre-split file is archived at [`docs/history/PROJECT_STATE_2026-07-07_pre-split.md`](history/PROJECT_STATE_2026-07-07_pre-split.md).

## Root Index

[`../PROJECT_STATE.md`](../PROJECT_STATE.md) is intentionally short. Keep it as an entry point only.

## Split Documents

- [`CURRENT_FOCUS.md`](CURRENT_FOCUS.md): moved `Current Focus` and `Likely Next Step` content.
- [`IMPLEMENTED_CAPABILITIES.md`](IMPLEMENTED_CAPABILITIES.md): capability-oriented index into current and concept-local state.
- [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md): moved architecture, package layout, controller, model, client, fake-HPCC, demo-work, and design-direction sections.
- [`RUNTIME_RUNBOOK.md`](RUNTIME_RUNBOOK.md): moved `How To Run` commands and expected runtime outputs.
- [`TEST_AND_SMOKE_STATUS.md`](TEST_AND_SMOKE_STATUS.md): moved `Tests` coverage and smoke-test notes.
- [`DEVELOPMENT_GOVERNANCE.md`](DEVELOPMENT_GOVERNANCE.md): moved `Development Governance`.

## Concept-Local State

- [`concepts/data-assets-and-materialized-outputs/STATE.md`](concepts/data-assets-and-materialized-outputs/STATE.md): data asset, materialized input, artifact, publication, and explicit data-operator state.
- [`concepts/dependency-aware-workflows/STATE.md`](concepts/dependency-aware-workflows/STATE.md): dependency-stage compilation, activation, output capture, and related architectural refinement state.
- [`concepts/resource-constrained-work-admission/STATE.md`](concepts/resource-constrained-work-admission/STATE.md): resource-constraint model, persistence, claim, status, and cleanup state.
- [`concepts/workflow-execution-persistence/STATE.md`](concepts/workflow-execution-persistence/STATE.md): moved workflow execution persistence and cutover state.
- [`concepts/operational-observability/STATE.md`](concepts/operational-observability/STATE.md): log model, ingestion, filesystem sinks, read API, CLI logs, and Python subprocess log emission state.
- [`concepts/source-control-resolution-and-cache/STATE.md`](concepts/source-control-resolution-and-cache/STATE.md): repository-source, source-reference admission, cache, and restart verification state.

## Pre-Split Section Homes

| Pre-split section | New home |
| --- | --- |
| `Current Focus` | [`CURRENT_FOCUS.md`](CURRENT_FOCUS.md), with concept excerpts in `docs/concepts/*/STATE.md` |
| `Development Governance` | [`DEVELOPMENT_GOVERNANCE.md`](DEVELOPMENT_GOVERNANCE.md) |
| `Current Layout` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Runtime Flow` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Controller` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Workflow Execution Persistence` | [`concepts/workflow-execution-persistence/STATE.md`](concepts/workflow-execution-persistence/STATE.md) |
| `SQLite Ledger` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Worker Config` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Shared Models` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Variable Model` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Workflow Compilation` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Local Client` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Fake HPCC And Dockerized Slurm` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Demo Work` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Tests` | [`TEST_AND_SMOKE_STATUS.md`](TEST_AND_SMOKE_STATUS.md) |
| `How To Run` | [`RUNTIME_RUNBOOK.md`](RUNTIME_RUNBOOK.md) |
| `Design Direction` | [`ARCHITECTURE_STATE.md`](ARCHITECTURE_STATE.md) |
| `Workflow Execution Persistence Cutover` | [`concepts/workflow-execution-persistence/STATE.md`](concepts/workflow-execution-persistence/STATE.md) |
| `Likely Next Step` | [`CURRENT_FOCUS.md`](CURRENT_FOCUS.md) |

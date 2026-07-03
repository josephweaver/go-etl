# GOET Epic Navigation

This directory contains GOET's capability plans. Each epic README describes the
problem boundary, goals, non-goals, architectural decisions, and proposed
implementation slices. Use this page as a quick index; follow an epic link for
its authoritative scope.

## Proposed Epics

- [Attempt Liveness and Caretaker Recovery](attempt-liveness-recovery/README.md) — Detect workers that stop reporting, recover abandoned attempts, and requeue their logical work.
- [Controller Resilience](controller-resilience/README.md) — Define controller process identity and restart behavior for work, reports, and orchestration state.
- [Controller Retention and Cleanup](controller-retention-cleanup/README.md) — Bound controller-owned disk and database growth without deleting data still needed for active work, recovery, or audit.
- [Dependency-Aware Workflow Execution](dependency-aware-workflows/README.md) — Compile and queue workflow steps only when their predecessor steps have completed successfully.
- [Execution Events](execution-events/README.md) — Replace specialized worker messages with a shared typed event model while keeping orchestration state controller-owned.
- [Execution Observability](execution-observability/README.md) — Collect, route, stream, and store execution logs through the controller.
- [Resource Constraints](resource-constraint/README.md) — Add controller-owned admission limits for work items that share named resources.
- [Sensitive Variable Metadata and Propagation](sensitive-variable-propagation/README.md) — Preserve secret sensitivity through variable resolution, diagnostics, persistence, and execution boundaries.
- [Submission CLI Status](submission-cli-status/README.md) — Establish a production-oriented CLI and a queryable submission model for workflow execution.
- [Workflow Dependency Resolution](workflow-dependency-resolution/README.md) — Resolve dependencies between complete workflows and delay dependent execution until prerequisites succeed.
- [Workflow Execution Persistence](workflow-execution-persistence/README.md) — Make the database authoritative for workflow runs, steps, work items, attempts, resolver inputs, and outputs across controller restarts.

## Early Concepts

These documents describe intended capabilities but do not yet declare the full
epic status and planning structure used by the proposed epics above.

- [Logging Framework](logging/README.md) — Sketches hierarchical logging from clients, controllers, workers, and worker subprocesses.
- [Python WorkItem](python-workitem/README.md) — Sketches a worker plugin for preparing an environment and executing generic Python scripts.

## Completed Epics

- [Controller Startup Resolution](complete/controller-startup-resolution/README.md) — Builds and validates controller startup through the standard typed-variable system and fails before normal API admission when startup requirements are not met.
- [SSH Transport](complete/ssh-transport/README.md) — Adds remote command execution and file transfer through controller transport implementations.
- [Structured Variable Resolution](complete/structured-variable-resolution/README.md) — Adds recursively resolved, explicitly typed object and list expressions.

## Planning Guides

- [Epic procedure](epic-procedure.md) — Defines how to frame and approve an epic.
- [Epic slice procedure](epic-slice-procedure.md) — Defines how an approved epic is decomposed into implementation slices.

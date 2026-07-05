# GOET Strategic Concept Navigation

This directory contains GOET's capability plans. Each Strategic Concept README
describes the problem boundary, goals, non-goals, architectural decisions, and
proposed Operational Slices. Use this page as a quick index; follow a Strategic
Concept link for its authoritative scope.

## Proposed Strategic Concepts

- [Attempt Liveness and Caretaker Recovery](attempt-liveness-recovery/README.md) - Detect workers that stop reporting, recover abandoned attempts, and requeue their logical work.
- [Controller Resilience](controller-resilience/README.md) - Define controller process identity and restart behavior for work, reports, and orchestration state.
- [Controller Retention and Cleanup](controller-retention-cleanup/README.md) - Bound controller-owned disk and database growth without deleting data still needed for active work, recovery, or audit.
- [Dependency-Aware Workflow Execution](dependency-aware-workflows/README.md) - Compile and queue workflow steps only when their predecessor steps have completed successfully.
- [Execution Events](execution-events/README.md) - Replace specialized worker messages with a shared typed event model while keeping orchestration state controller-owned.
- [Resource Constraints](resource-constraint/README.md) - Add controller-owned admission limits for work items that share named resources.
- [Sensitive Variable Metadata and Propagation](sensitive-variable-propagation/README.md) - Preserve sensitivity metadata through variable resolution, diagnostics, persistence, and execution boundaries.
- [Workflow Compilation Resolution](workflow-compilation-resolution/README.md) - Define how workflow submission and ready-step compilation use short-lived resolvers with durable recipes and resolved snapshots.
- [Workflow Dependency Resolution](workflow-dependency-resolution/README.md) - Resolve dependencies between complete workflows and delay dependent execution until prerequisites succeed.
- [Workflow Execution Persistence](workflow-execution-persistence/README.md) - Make the database authoritative for workflow runs, steps, work items, attempts, resolver inputs, and outputs across controller restarts.

## Early Concepts

These documents describe intended capabilities but do not yet declare the full
Strategic Concept status and planning structure used by the proposed concepts
above.

- [Logging Framework](logging/README.md) - Sketches hierarchical logging from clients, controllers, workers, and worker subprocesses.

## Completed Strategic Concepts

- [Execution Observability](complete/execution-observability/README.md) - Collects, routes, streams, and stores execution logs through the controller.
- [Controller Startup Resolution](complete/controller-startup-resolution/README.md) - Builds and validates controller startup through the standard typed-variable system and fails before normal API admission when startup requirements are not met.
- [SSH Transport](complete/ssh-transport/README.md) - Adds remote command execution and file transfer through controller transport implementations.
- [Structured Variable Resolution](complete/structured-variable-resolution/README.md) - Adds recursively resolved, explicitly typed object and list expressions.
- [Python WorkItem](complete/python-workitem/README.md) - Phase 1 complete for admitted-source system Python execution, including end-to-end smoke-path validation.
- [Submission CLI Status](complete/submission-cli-status/README.md) - Establishes a production-oriented CLI and queryable submission status model for workflow execution.

## Planning Guides

- [Strategic Concept procedure](../../../epistemic-control/procedures/strategic-concept-design.md) - Defines how to frame and approve a Strategic Concept.
- [Operational Slice procedure](../../../epistemic-control/procedures/operational-slice.md) - Defines how an approved Strategic Concept is decomposed into Operational Slices.

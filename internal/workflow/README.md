# internal/workflow

This directory owns the early workflow compilation boundary.

It is not the controller scheduler, durable workflow state store, variable resolver, worker runtime, or customer-facing pipeline language. Its job is to turn a small in-memory workflow definition into concrete work-item assignments that the controller can queue.

## Files

- `workflow.go` owns the top-level workflow shape and the package-level compilation result.
- `step.go` owns the step boundary and dispatch from a workflow step to the supported step compiler.
- `fanout.go` owns the current fan-out compilation concept that expands one list expression into many concrete work items.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Workflow definition shape used by the current Go runtime.
- Step-level compilation boundary.
- Fan-out expansion from a resolved list into multiple work items.
- Generated work-item uniqueness within one compiled workflow.
- Workflow and step traceability attached to compiled work items.

## Concepts Owned Elsewhere

- Typed variables, precedence, expression resolution, and structured access belong in `internal/variable`.
- Work-item transport shape and structural assignment validity belong in `internal/model`.
- Queue ownership, scheduling, worker scaling, and completion handling belong in the controller.
- Durable attempt history and future skip evidence belong in `internal/ledger`.
- Worker operation execution and output promotion belong in `cmd/worker`.
- Client submission and controller bootstrap belong in `internal/client`.

## Invariants

- Workflow compilation produces concrete work items; it does not execute them.
- The controller decides when compiled work becomes pending, assigned, complete, failed, or skipped.
- Fan-out is explicit and driven by a resolved list expression.
- Object fan-out values require explicit accessors for generated IDs, filenames, or parameters.
- Generated work-item IDs must be unique inside one compiled workflow.
- Workers should receive concrete parameters, not unresolved workflow expressions.

## Major Dependencies

- `internal/variable` for resolved values and fan-out expression evaluation.
- `internal/model` for concrete work-item transport shapes.
- The Go standard library for validation and string formatting.

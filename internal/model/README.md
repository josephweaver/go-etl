# internal/model

This directory owns shared transport models used between the controller, workers, clients, and tests.

It is not the place for controller scheduling, workflow compilation, worker execution, ledger persistence, or variable resolution. Its job is to define small data shapes that can cross package and HTTP boundaries without making one runtime role depend on another role's internals.

## Files

- `work_item.go` owns the shared work assignment, completion, failure, parameter, and controller status shapes.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Work-item transport shape.
- Concrete worker parameter transport shape.
- Completion and failure report shapes.
- Controller status summary shape.
- Shared structural validity rules for work assignments.

## Concepts Owned Elsewhere

- Workflow definitions and compilation belong in `internal/workflow`.
- Typed variables, precedence, and resolution belong in `internal/variable`.
- Queue state, assignment lifecycle, scheduling, and ledger writes belong in the controller.
- Worker operation dispatch, execution, and output promotion belong in `cmd/worker`.
- Durable attempt history belongs in `internal/ledger`.
- Client submission, polling, startup, and shutdown behavior belong in `internal/client`.

## Invariants

- Models in this directory should remain role-neutral transport contracts.
- Workers should receive already-resolved concrete parameters rather than workflow expressions.
- Work-item IDs, types, and output filenames are required for executable assignments.
- Output filenames are filenames only; directory ownership belongs to runtime configuration and worker storage paths.
- Runtime identity and fingerprint fields mirror controller-generated metadata and should not become a separate configuration system.

## Major Dependencies

- The Go standard library only.
- `fmt` for validation errors.
- `path/filepath` for filename boundary checks.

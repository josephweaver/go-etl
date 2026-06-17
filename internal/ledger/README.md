# internal/ledger

This directory owns the local durable attempt ledger.

It is not the controller queue, workflow state machine, scheduler, retry system, or worker runtime. Its job is to persist attempt history and the variable snapshot attached to each attempt so later controller decisions can compare current work against prior completed work.

## Files

- `sqlite.go` owns the SQLite-backed ledger schema, attempt records, attempt variable records, and read/write access for local attempt history.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Durable attempt history for local execution.
- Attempt status as recorded history.
- Attempt identity and traceability fields mirrored from runtime variables.
- Attempt variable snapshots with namespace, type, source, and lifecycle.
- SQLite schema ownership for the local ledger.

## Concepts Owned Elsewhere

- Queue state, assignment, completion handling, scheduling, and skip decisions belong in the controller.
- Workflow definitions and workflow compilation belong in `internal/workflow`.
- Work-item transport models belong in `internal/model`.
- Typed variable construction, precedence, and resolution belong in `internal/variable`.
- Worker execution and output promotion belong in `cmd/worker`.
- Client startup, submission, polling, and shutdown belong in `internal/client`.

## Invariants

- The controller is the process that writes to the ledger; workers and clients do not talk to SQLite directly.
- Ledger rows record facts about attempts; they do not decide whether new work should run.
- Common identity and fingerprint columns mirror typed runtime variables instead of replacing the variable model.
- Attempt variable snapshots preserve enough context to support future correctness checks.
- A previous output filename alone is not sufficient evidence for skipping work.

## Major Dependencies

- `database/sql` for the database boundary.
- `modernc.org/sqlite` as the SQLite driver.
- `encoding/json` for storing variable values.
- `context` for database operation lifetimes.
- `time` for attempt timestamps.

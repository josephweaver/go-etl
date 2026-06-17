# cmd/worker

This directory owns the local Go worker executable.

The worker is the current execution process for concrete work items. It loads local runtime paths, validates its filesystem environment, repeatedly pulls work from the controller, runs supported operations, writes completed outputs through the temp-to-data pattern, and reports completion or failure.

It is not the workflow compiler, scheduler, queue owner, ledger writer, client bootstrapper, or variable resolver. The worker should stay relatively dumb: receive already-resolved assignments, execute them, and report what happened.

## Files

- `main.go` owns the worker process entry point and pull-execute-report loop.
- `config.go` owns loading and validating worker runtime configuration.
- `worker.go` owns worker environment validation and dispatch to supported work operations.
- `state.go` owns HTTP communication with the controller for fetching work and reporting outcomes.
- `work_demo.go` owns the demo output-producing operation.
- `work_summary.go` owns the input-file summary operation.
- `demo-config.json` is the local demo worker configuration.
- `demo-item.json` is a local demo work-item fixture.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Worker process boundary.
- Local runtime directories for logs, temporary output, and completed data.
- Pull-based work retrieval from the controller.
- Work-item execution dispatch.
- Temp-to-data output promotion.
- Worker-side completion and failure reporting.
- Worker-generated attempt timing for completed work.

## Concepts Owned Elsewhere

- Queue state, scheduling, worker startup, and ledger writes belong in `cmd/controller`.
- Workflow definitions and compilation belong in `internal/workflow`.
- Work-item, completion, and failure payload shapes belong in `internal/model`.
- Typed variables and expression resolution belong in `internal/variable`.
- Durable attempt history belongs in `internal/ledger`.
- Client submission, polling, and controller bootstrap belong in `internal/client`.

## Invariants

- Workers pull work; they do not receive a preloaded queue.
- Workers execute concrete assignments and should not resolve workflow expressions locally.
- Required runtime directories must exist before work runs.
- Incomplete output is written under the temporary directory before completed output appears under the data directory.
- Completion and failure are reported to the controller over HTTP.
- Unsupported work-item types are rejected by the worker dispatch boundary.

## Major Dependencies

- `net/http` for controller communication.
- `os` and `path/filepath` for local filesystem work.
- `encoding/json` for controller payloads.
- `time` for attempt timing.
- `internal/model` for shared work and status payloads.

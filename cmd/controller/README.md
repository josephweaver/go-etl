# cmd/controller

This directory owns the local Go controller executable.

The controller is the current control plane for local workflow runs. It accepts workflow submissions, compiles them into concrete work items, owns queue state, starts local workers when configured, records completed attempts, and exposes HTTP endpoints used by clients and workers.

It is not the worker runtime, Python-facing API, reusable workflow language, variable system, or ledger implementation. It coordinates those pieces from the process boundary.

## Files

- `main.go` owns the controller process, HTTP API surface, durable queue lifecycle, workflow submission handling, runtime metadata attachment, ledger write coordination, status reporting, and shutdown hook.
- `config.go` owns loading controller startup configuration into typed variables.
- `local_worker.go` owns the local worker-starting adapter used by the controller.
- `worker_scaler.go` owns the small worker-start planning state used when pending work exists.
- `demo-config.json` is the local demo controller configuration.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Controller process boundary.
- HTTP API for work assignment, completion, failure, workflow submission, status, and shutdown.
- Durable queued, running, completed, and failed work state through the workflow-execution store.
- Workflow submission orchestration.
- Controller-generated runtime metadata for work items.
- Local worker startup decisions.
- Controller-owned ledger write coordination.
- Local status summary for clients and demos.

## Concepts Owned Elsewhere

- Workflow shape and fan-out compilation belong in `internal/workflow`.
- Work-item, completion, failure, and status transport shapes belong in `internal/model`.
- Typed variables, precedence, and resolution belong in `internal/variable`.
- SQLite schema and durable attempt storage belong in `internal/ledger`.
- Worker execution, output promotion, and reporting belong in `cmd/worker`.
- Client startup, submission, polling, and shutdown behavior belong in `internal/client`.

## Invariants

- The controller owns queue transitions; workers pull assignments and report outcomes.
- Direct SQLite access stays inside the controller process through `internal/ledger`.
- Workflow submission is the target boundary; raw work submission is local administrative/test support.
- Runtime configuration is resolved through typed variables rather than a separate hidden config authority.
- Workers should receive concrete work-item parameters and metadata, not unresolved workflow intent.
- Worker startup is bounded by configured scaling limits and queued work.
- Queue state is stored in the workflow-execution database; a controller without a workflow store rejects queue endpoints.

## Major Dependencies

- `net/http` for the controller API.
- `sync` for protecting controller admission and recovery flags.
- `database/sql` through the configured ledger handle.
- `internal/workflow` for workflow compilation.
- `internal/model` for shared HTTP payloads.
- `internal/variable` for runtime configuration and submission variables.
- `internal/ledger` for durable attempt recording and prior-attempt lookup.
- `os/exec` for local worker startup.

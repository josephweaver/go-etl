# cmd/demo-client

This directory owns the local demo client executable.

The demo client is a runnable example of the current local workflow path. It builds a small typed-variable resolver, starts or contacts the local controller through `internal/client`, submits a workflow-run source-reference file, waits for the controller to become idle, asks for shutdown, and prints the final status.

It is not the reusable Python-facing API, the controller, the worker, the workflow compiler, or the variable system. It wires existing package boundaries together for a local demonstration.

## Files

- `main.go` owns the demo executable entry point, demo runtime variables, workflow-run submission file selection, final status formatting, and local client wiring.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Local demo execution path.
- Demo defaults for controller contact and startup.
- Demo workflow-run source-reference file selection.
- Final local status display.
- Example wiring between the reusable client helper and the controller executable.

## Concepts Owned Elsewhere

- Client submission, controller reachability, polling, startup, and shutdown behavior belong in `internal/client`.
- Controller queue ownership, workflow submission handling, worker startup, and ledger writes belong in `cmd/controller`.
- Worker execution and reporting belong in `cmd/worker`.
- Workflow shape and compilation belong in `internal/workflow`.
- Typed variables, namespaces, and resolution belong in `internal/variable`.
- Shared status and work payload shapes belong in `internal/model`.

## Invariants

- This executable should stay small and demonstrative.
- Demo startup values are expressed as typed variables.
- The demo submits workflow-run source-reference files, not raw work items or inline workflow JSON.
- The demo waits for pending and assigned work to reach zero before requesting controller shutdown.
- Reusable client behavior should live in `internal/client`, not be duplicated here.

## Major Dependencies

- `internal/client` for workflow submission and local controller bootstrap.
- `internal/variable` for demo runtime variables.
- `internal/model` for final controller status formatting.
- The Go standard library for process arguments and console output.

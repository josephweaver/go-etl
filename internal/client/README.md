# internal/client

This directory owns the local Go client boundary for submitting workflow runs to a controller.

It is not the workflow compiler, scheduler, ledger, or worker runtime. Its job is to translate a client-side submission request into controller HTTP calls, and to handle the local-controller bootstrap path when the configured controller is not already reachable.

## Files

- `controller_client.go` owns source-reference workflow-run submission, submission acknowledgement handling, submission status retrieval, wait polling, submission file loading, controller reachability checks, and client-initiated shutdown after the controller becomes idle.
- `cli_inputs.go` owns the first CLI-side loading boundary for explicit `controller.json`, `project.json`, and `workflow.json` paths.
- `local_controller.go` owns the local process-starting adapter used when a client is allowed to start a controller on the same machine.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Client-side workflow-run source-reference submission envelope.
- Submission acknowledgement and submission status transport.
- CLI JSON input loading for the current public `goet submit` shape.
- Controller reachability from the client's point of view.
- Optional local controller startup before submission.
- Client-side polling for idle controller state.
- Client-side shutdown request for a controller that is no longer doing pending or assigned work.

## Concepts Owned Elsewhere

- Workflow structure and compilation belong in `internal/workflow`.
- Work-item and controller status transport models belong in `internal/model`.
- Typed variables, precedence, and reference resolution belong in `internal/variable`.
- Queue ownership, scheduling, worker startup decisions, and ledger writes belong in the controller.
- Worker execution, output promotion, and failure reporting belong in `cmd/worker`.
- Durable attempt history belongs in `internal/ledger`.

## Invariants

- The controller URL and client polling interval come from typed variables, not from a separate client config system.
- Workflow-run submission targets the controller workflow API, not raw worker execution.
- The demo compatibility path submits project/workflow source references.
- `goet submit` loads explicit controller/project/workflow JSON inputs and submits them through the current controller workflow admission boundary.
- `goet status` reads submission-scoped status from the controller.
- `goet submit --wait` polls submission status until the controller reports a terminal state.
- `--json` output stays machine-readable and separate from diagnostics.
- The client may start a local controller, but it does not manage controller internals after startup.
- Local controller startup is best-effort coordinated so concurrent clients do not intentionally start duplicate controllers.
- Shutdown is requested only after the client observes no pending or assigned work.

## Major Dependencies

- `net/http` for controller API calls.
- `os` and JSON encoding for loading serialized workflow submissions.
- `os/exec` for local controller startup.
- `internal/variable` for resolving runtime control values.
- `internal/workflow` only for the temporary legacy inline workflow submission helpers.
- `internal/model` for controller status.

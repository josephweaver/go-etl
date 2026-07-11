# cmd/worker

This directory owns the Go worker executable.

The worker executes concrete work items through two process modes. Controller
mode repeatedly pulls work, executes it, and reports completion or failure.
Direct mode is a development-only harness that loads one resolved work item from
a local JSON file, executes it through the same `Worker.Run` method, and writes a
local result without polling or terminal reporting.

It is not the workflow compiler, scheduler, queue owner, ledger writer, client bootstrapper, or variable resolver. The worker should stay relatively dumb: receive already-resolved assignments, execute them, and report what happened.

## Files

- `main.go` owns the worker process entry point and pull-execute-report loop.
- `config.go` owns loading and validating worker runtime configuration.
- `direct.go` owns direct command parsing, work-item loading, attempt identity,
  one-shot execution, and local result writing.
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
- Development-only direct execution of one resolved work item.
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

## Process modes

Controller mode remains backward compatible:

```bash
worker [worker-config.json]
```

Direct development mode is:

```bash
worker execute \
  --config ./worker.json \
  --work-item ./work-item.json \
  [--result ./worker-result.json]
```

Direct config uses the same worker path and executable settings but may omit
`controller_url`. The configured log, temporary, and data directories must
already exist. Relative config paths remain relative to the config file.

The work-item file contains exactly one resolved `model.WorkItem` JSON document.
Direct mode supplies a missing attempt ID as
`direct-attempt-<short-random-id>`, but does not compile workflows, resolve
expressions, manufacture operation parameters, or execute dependencies.

The result defaults to `worker-result.json`. Once options identify the result
path, any prior result is removed before config and work-item loading. Completed
and failed executions overwrite the path with
`gorc/worker-direct-result/v1` JSON. Exit status is `0` for completed work and
`1` for any failure.

OS-001 supports operations that do not require controller source retrieval,
including `write_demo_output`, `summarize_input_file`, `cache_data`, and
`commit_data` when their normal inputs and provider configuration exist. Direct
mode does not maintain a separate operation allow-list. Local source-bundle and
Python direct execution are added by OS-002; do not use OS-001 direct mode for
`python_script`.

Direct mode is intentionally unsuitable for production use or production
credentials.

## Invariants

- Controller mode pulls work; direct mode consumes exactly one explicit item.
- Workers execute concrete assignments and should not resolve workflow expressions locally.
- Required runtime directories must exist before work runs.
- Incomplete output is written under the temporary directory before completed output appears under the data directory.
- Controller mode reports completion and failure over HTTP; direct mode writes
  only its local result artifact.
- Unsupported work-item types are rejected by the worker dispatch boundary.

## Major Dependencies

- `net/http` for controller communication.
- `os` and `path/filepath` for local filesystem work.
- `encoding/json` for controller payloads.
- `time` for attempt timing.
- `internal/model` for shared work and status payloads.

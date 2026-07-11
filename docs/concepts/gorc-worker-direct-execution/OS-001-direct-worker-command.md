# OS-001: Direct Worker Command

## Status

Ready for implementation.

## Objective

Add a development-only `worker execute` command that reads one resolved work
item, supplies missing bookkeeping identifiers, passes the item to `Worker.Run`
exactly once, overwrites a local result document, and exits without controller
communication.

OS-001 establishes the command with source-free execution. Local source bundles
and Python execution are completed by OS-002 and proven by OS-003.

## Current state

- `cmd/worker/main.go:main` loads full controller-mode config and constructs
  `NewWorkerControllerClient` before selecting behavior.
- `workerConfigPath` treats the first positional argument as a config path.
- `runWorkerLoop` owns work fetch, `Worker.Run`, and terminal reporting.
- `Config.Validate` requires `controller_url`.
- `Worker.Validate` checks the configured log, temporary, and data directories.
- `Worker.Run` is the shared single-item dispatcher and currently accepts
  `write_demo_output`, `summarize_input_file`, `cache_data`, `commit_data`, and
  `python_script`.
- `workCompletion` is a controller transport shape with demo fallbacks and copied
  parameters; it is not a direct result contract.

## Target state

Production invocation remains compatible:

```text
worker
worker ./worker-config.json
```

Direct development invocation is:

```text
worker execute --config PATH --work-item PATH [--result PATH]
```

Direct mode removes any previous result, loads runtime configuration without
requiring a controller URL, loads and normalizes one work item, constructs a
local-only worker, calls `Worker.Run` once, and writes a completed or failed
result directly to the result path.

Direct mode has no separate work-item-type allow-list. Every item type accepted
by `Worker.Run` reaches that method. In OS-001, a source-staged operation may
return a clear missing-source-provider error; OS-002 supplies the local provider.

## Concept decision

This slice adds a new worker CLI execution mode and direct result artifact.
`cmd/worker/direct.go` owns the independently testable options, input
normalization, orchestration, and result behavior. Process selection remains in
`main.go`; config validation remains in `config.go`.

The direct command is intentionally a thin harness around `Worker.Run`, not a
second dispatcher, plugin registry, workflow compiler, or production runtime.

## Required context

Read these files first:

- `docs/concepts/gorc-worker-direct-execution/README.md`
- `cmd/worker/main.go`
- `cmd/worker/main_test.go`
- `cmd/worker/config.go`
- `cmd/worker/config_test.go`
- `cmd/worker/worker.go`
- `cmd/worker/state.go`
- `cmd/worker/evidence.go`

Read operation-specific files only when a focused direct-command test requires
their existing fixture shape.

## Allowed production files

- `cmd/worker/main.go`
- `cmd/worker/config.go`
- `cmd/worker/direct.go` (new)
- `cmd/worker/evidence.go` only if a small JSON helper can be reused without
  retaining atomic-replacement semantics

## Allowed test files

- `cmd/worker/main_test.go`
- `cmd/worker/config_test.go`
- `cmd/worker/direct_test.go` (new)

## Allowed documentation files

- `cmd/worker/README.md`
- `PROJECT_STATE.md`

Do not modify controller, workflow compiler, variable resolver, ledger, plugin,
or `internal/model` code.

## CLI parsing

The literal first argument `execute` selects direct mode before config loading
or controller-client construction. Any other first positional argument retains
its production meaning as a config path.

Use a dedicated `flag.FlagSet` with `ContinueOnError`. Direct flags in this
slice are:

```text
--config       required
--work-item    required
--result       optional; default worker-result.json
```

Reject unexpected positional arguments. OS-002 adds `--source-bundle`.

## Config validation split

Refactor without weakening production validation:

```go
func (c Config) ValidateRuntime() error
func (c Config) ValidateControllerMode() error
func (c Config) Validate() error
```

- `ValidateRuntime` checks log/tmp/data and non-controller fields such as
  nonnegative asset limits.
- `ValidateControllerMode` checks controller URL, scheme, token requirements,
  and external HTTP rules.
- `Validate` calls both and preserves production behavior.
- `loadConfig(path)` remains the full production loader.
- A direct loader decodes the same `Config`, resolves relative paths in the same
  way, and applies runtime validation only.
- A controller URL present in a direct config is ignored.

## Work-item loading and normalization

The loader must:

- read a regular file;
- decode exactly one JSON document into `model.WorkItem`;
- reject trailing non-whitespace JSON;
- retain the existing assignment decoder's unknown-field behavior;
- reject a wrapper object;
- avoid workflow-expression or variable resolution; and
- normalize permitted bookkeeping fields before final `item.Validate()`.

Behavior-affecting fields remain required. Direct mode does not invent the item
ID, type, output filename, operation parameters, resolved provider payload,
publish target, artifact declaration, or dependency output.

When `AttemptID` is missing, generate:

```text
direct-attempt-<short-random-id>
```

The generated value must match `[A-Za-z0-9._-]+`, must be neither `.` nor `..`,
and must be unique enough that repeated direct runs do not share an attempt
directory. Preserve an explicitly supplied nonempty attempt ID only when it
meets the same portable-component rule; otherwise reject it. OS-002 owns missing
source bookkeeping defaults for source-staged work.

## Operation pass-through

Do not switch on work-item type to decide whether direct execution is allowed.
After generic input preparation, call `Worker.Run(item)`. Operation-specific
validation and behavior remain owned by the existing worker operation.

This includes `cache_data` and `commit_data`. Their normal resolved payloads,
provider configuration, cached inputs, and referenced artifacts remain the
developer's responsibility; direct mode does not manufacture prerequisites.

## Result lifecycle

After successful flag parsing determines the result path, remove that path if it
exists. If config or work-item preparation then fails, print the diagnostic to
stderr and leave no stale result. A flag-parse failure reports to stderr and
does not alter a result because a reliable custom result path may not be known.

After `Worker.Run` starts, create the result parent directory if needed and write
a human-readable direct result using a normal overwrite/truncating file write.
Atomic publication and a temp-and-rename helper are not required.

The result has stable explicit snake_case JSON fields:

```go
type DirectExecutionResult struct {
    Schema         string                  `json:"schema"`
    Status         string                  `json:"status"`
    WorkItemID     string                  `json:"work_item_id"`
    AttemptID      string                  `json:"attempt_id"`
    WorkItemType   string                  `json:"work_item_type"`
    OutputFilename string                  `json:"output_filename"`
    StartedAt      string                  `json:"started_at"`
    FinishedAt     string                  `json:"finished_at"`
    DataOutputPath string                  `json:"data_output_path,omitempty"`
    AttemptDir     string                  `json:"attempt_dir,omitempty"`
    Evidence       *DirectExecutionEvidence `json:"evidence,omitempty"`
    Error          string                  `json:"error,omitempty"`
}
```

Define `DirectExecutionEvidence` with explicit tags rather than serializing
untagged `WorkEvidence` directly. Convert the returned evidence without copying
the complete work item or parameters.

Use schema `gorc/worker-direct-result/v1` and status values `completed` and
`failed`. `data_output_path` and `attempt_dir` are included only when created by
the current run. A failed result must not claim that a pre-existing output was
produced by the failed run.

Timestamps and generated IDs make result bytes variable. “Stable” means the JSON
schema and status mapping are stable.

## Exit and diagnostic behavior

Use conventional process status only:

```text
0  work completed and result writing succeeded
1  invocation, validation, work execution, or result writing failed
```

Print concise diagnostics to the supplied stderr writer. The local result and
worker files are the developer-facing diagnostic contract; do not create a
multi-code exit API.

## Controller independence

The direct command must not initialize `Worker.Controller`, call
`worker.controllerClient`, enter `runWorkerLoop`, fetch work, or report terminal
state. OS-002 completes the same invariant for source retrieval and Python log
observations.

A direct test must configure a sentinel controller URL, count all requests, run
representative source-free work, and assert that the total request count is
zero. A second successful test with no controller URL proves it is optional.

## Tests

Add focused coverage for:

```text
TestDirectOptionsRequireConfig
TestDirectOptionsRequireWorkItem
TestDirectOptionsRejectUnexpectedArguments
TestLoadDirectConfigAllowsMissingControllerURL
TestLoadConfigStillRequiresControllerURL
TestLoadDirectWorkItemReadsModelWorkItem
TestLoadDirectWorkItemRejectsTrailingJSON
TestNormalizeDirectWorkItemPreservesAttemptID
TestNormalizeDirectWorkItemCreatesSafeUniqueAttemptID
TestRunDirectCommandExecutesDemoItem
TestRunDirectCommandExecutesSummaryItem
TestRunDirectCommandPassesCacheDataToWorkerRun
TestRunDirectCommandPassesCommitDataToWorkerRun
TestRunDirectCommandWritesFailureResult
TestRunDirectCommandRemovesStaleResultBeforeInvalidConfigOrWorkItem
TestRunDirectCommandOverwritesExistingResult
TestRunDirectCommandSendsZeroControllerRequests
TestWriteDirectExecutionResultUsesSnakeCaseEvidence
```

Cache and commit tests need only prove there is no direct-mode allow-list and
that execution reaches their normal worker behavior. Reuse existing operation
fixtures when a compact successful case is available; do not recreate those
operation test suites here.

## Out of scope

- Local source-bundle acquisition and successful Python direct execution.
- Workflow compilation or unresolved variable resolution.
- Automatic dependency execution.
- HTTP receive mode.
- Generic assignment-source or result-sink interfaces.
- Controller behavior changes.
- `internal/model.WorkItem` schema changes.
- Direct-mode security hardening or production-credential guarantees.
- GOET-to-GORC identifier renaming.

## Implementation sequence

1. Split config validation and prove production validation is unchanged.
2. Add direct options and stale-result removal.
3. Add exact work-item loading and bookkeeping normalization.
4. Add the explicit direct result and evidence shapes.
5. Select local-only direct mode and call `Worker.Run` once.
6. Add pass-through, overwrite, failure, and zero-request tests.
7. Run `go test ./cmd/worker`.
8. Update worker README and project state.

## Acceptance criteria

OS-001 is complete when:

1. Existing `worker [config.json]` behavior and tests remain valid.
2. Direct config may omit the controller URL; production config may not.
3. Direct mode does not maintain a work-item-type allow-list.
4. Current source-free types reach their normal `Worker.Run` behavior.
5. Missing attempt identity receives a safe unique direct default.
6. A stale result is removed before new input preparation.
7. Direct success and work failure overwrite the result with valid JSON using
   explicit snake_case evidence fields.
8. The simple zero/single-nonzero exit contract is tested.
9. A configured sentinel controller observes zero requests.
10. `Worker.Run` is called exactly once for an executable item.
11. `go test ./cmd/worker` passes.

# Project State

Last updated: 2026-06-02

## Current Focus

We now have a minimal local Go controller and worker runtime. The controller owns an in-memory work queue. The worker loads local runtime config, repeatedly pulls assigned work over HTTP, dispatches supported work-item types, writes completed output through mounted-style local directories, and reports completion or failure.

The target product still has a reusable Python interface that submits external pipeline/config files to a Go controller on backends such as HPCC. The current implementation is a local runtime foundation, not the intended user-facing API.

Project guidance is in `AGENTS.md`. The longer product and architecture direction is in `TARGET_STATE.md`.

## Current Layout

```text
go.mod
demo-workflow.json
internal/
  client/
    local_controller.go
    local_controller_test.go
    workflow.go
    workflow_test.go
  model/
    work_item.go
    work_item_test.go
  workflow/
    fanout.go
    fanout_test.go
    step.go
    step_test.go
    workflow.go
    workflow_test.go
  variable/
    literal.go
    literal_test.go
    accessor.go
    accessor_test.go
    name.go
    name_test.go
    namespace.go
    namespace_test.go
    reference.go
    reference_test.go
    resolver.go
    resolver_test.go
    scope.go
    scope_test.go
    type.go
    type_test.go
    variable.go
    variable_test.go
cmd/
  controller/
    main.go
    main_test.go
  worker/
    main.go
    main_test.go
    config.go
    config_test.go
    state.go
    state_test.go
    worker.go
    worker_test.go
    work_demo.go
    work_demo_test.go
    demo-config.json
    .run/
      logs/
      tmp/
      data/
```

The repository uses one root Go module:

```go
module goetl
```

Shared controller-worker JSON contracts live in `internal/model`.

## Runtime Flow

The local runtime flow is:

1. Start the controller on `:8080`.
2. The controller creates an in-memory queue with one demo work item.
3. Start the worker from `cmd/worker`.
4. The worker loads `demo-config.json`.
5. The worker validates required runtime directories.
6. The worker requests `GET /work/next`.
7. The controller moves one item from `pending` to `assigned`.
8. The worker validates and dispatches the item by `Type`.
9. The demo handler writes temporary output under `TmpDir`.
10. The demo handler renames completed output into `DataDir`.
11. The worker reports success with `POST /work/complete`, or failure with `POST /work/fail`.
12. The worker asks for more work.
13. The worker exits cleanly when `GET /work/next` returns `204 No Content`.

## Controller

The controller stores three in-memory collections:

```text
pending    work that can be assigned
assigned   work currently owned by a worker
failed     failed work and its error text
```

Access is protected by `sync.Mutex` so later concurrent workers can safely use the queue.

Current endpoints:

```text
GET  /work/next      assign the next pending item, or return 204
POST /work/complete  mark an assigned item complete
POST /work/fail      record failure for an assigned item
POST /work           submit one raw work item
POST /workflow       submit one tiny workflow and variables
POST /shutdown       ask the controller process to shut down
GET  /status         return queue counts
```

Completed items are removed from `assigned`. Failed items are removed from `assigned` and stored in `failed`. Queue state is currently process-local and is lost when the controller exits.

`POST /work` is useful for internal testing and local administration, but it is not the intended customer-facing submission boundary. The target submission boundary is workflow submission. The controller will eventually compile submitted workflows into concrete work items.

`POST /workflow` currently accepts JSON containing a workflow and optional submitted variables. Workflow-scope variables live inside the workflow object. Top-level submitted variables are reserved for overrides and runtime/config variables. The controller builds variable scopes from workflow variables and submitted variables, compiles the workflow through `internal/workflow`, checks generated work-item IDs against the existing queue state, and appends generated work items to the pending queue.

After workflow submission creates pending work, the controller resolves `worker_target_environment` through the same variable resolver. If that variable is present and a `WorkerStarter` is configured, the controller asks the starter to launch one worker for that target environment. The current `LocalWorkerStarter` supports only `local` and starts a background worker process from typed `worker_start_executable` and `worker_start_args` variables.

`GET /status` currently reports pending, assigned, and failed counts.

`POST /shutdown` currently invokes a controller shutdown hook. In local client-started runs, the client should poll `GET /status` and call this endpoint when pending and assigned counts both reach zero.

## Worker Config

The worker loads `cmd/worker/demo-config.json`:

```json
{
  "log_dir": ".run/logs",
  "tmp_dir": ".run/tmp",
  "data_dir": ".run/data",
  "controller_url": "http://localhost:8080"
}
```

The matching Go model is:

```go
type Config struct {
	LogDir        string `json:"log_dir"`
	TmpDir        string `json:"tmp_dir"`
	DataDir       string `json:"data_dir"`
	ControllerURL string `json:"controller_url"`
}
```

The local paths are relative to the directory where the worker is run.

## Shared Models

`internal/model/work_item.go` defines:

```go
type WorkItem struct {
	ID             string       `json:"id"`
	Type           WorkItemType `json:"type"`
	OutputFilename string       `json:"output_filename"`
}

type WorkCompletion struct {
	ID string `json:"id"`
}

type WorkFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}
```

`WorkItem.Validate()` checks structural validity:

- A non-empty ID.
- A non-empty type.
- A non-empty output filename.
- An output filename without directory components.

Operation support is separate from structural validity. The worker dispatcher rejects unsupported operation types.

## Variable Model

`internal/variable` contains the early typed-variable model and resolver foundation.

Current variable namespaces are:

```text
client_env
controller_env
worker_env
global
backend
project
workflow
override
```

Unqualified references use precedence lookup. Qualified references such as `worker_env.GDAL_DATA` explicitly select a namespace.

Current variable types include:

```text
string
int
bool
datetime
path
object
list[T]
```

`list[T]` currently supports lists of scalar types and lists of `object`. `list[list[T]]` is intentionally rejected for now.

Current resolver behavior supports:

- Typed scalar literal parsing.
- JSON object and list literal parsing into explicit resolved values.
- Variable precedence merging.
- Qualified and unqualified references.
- Recursive resolution with a configurable maximum depth.
- Escaped variable references such as `\${year}`.
- Scalar structured access in reference expressions, such as `${record.year}` and `${years[0]}`.
- Fan-out list access through `Resolver.ResolveFanOutExpression("${years[*]}")`.

Structured value support is intentionally small. Object literals are JSON objects with inferred field value types. List literals use their declared `list[T]` element type. Scalar access supports `.field` and `[index]`. Fan-out supports only `[*]` and returns a list of resolved values for later workflow compilation.

Runtime configuration must flow through the variable subsystem. Controller settings, worker settings, backend choices, command-line flags, API arguments, and client overrides should be represented as typed variables with clear namespaces and sources. Config structs and HTTP JSON fields are transport surfaces, not a separate configuration authority.

Important near-term runtime variables include:

```text
backend.controller_url
backend.controller_start_executable
backend.controller_start_args
backend.controller_start_lock_path
backend.worker_target_environment
backend.worker_start_executable
backend.worker_start_args
backend.worker_min_count
backend.worker_max_count
backend.worker_count_per_start
backend.worker_min_elapsed_time_between_starts
backend.client_status_poll_interval
override.controller_url
override.controller_start_executable
override.controller_start_args
override.controller_start_lock_path
override.worker_target_environment
override.worker_start_executable
override.worker_start_args
override.worker_min_count
override.worker_max_count
override.worker_count_per_start
override.worker_min_elapsed_time_between_starts
override.client_status_poll_interval
```

The local Go client may submit override variables such as `override.worker_target_environment = "local"` and `override.client_status_poll_interval = "5s"`. The controller should resolve these variables before choosing local worker bootstrap behavior.

The next controller scheduler should use a conservative organic worker-scaling model:

- Start at most `worker_count_per_start` workers in one decision.
- Keep started/active workers at or above `worker_min_count` while pending work exists.
- Never exceed `worker_max_count`.
- For organic scale-up above the minimum floor, wait at least `worker_min_elapsed_time_between_starts`.
- For organic scale-up above the minimum floor, wait until the previous worker start is confirmed by assigned work increasing.

Early local defaults should be `worker_min_count = 0`, `worker_max_count = 2`, `worker_count_per_start = 1`, and `worker_min_elapsed_time_between_starts = "30s"`. A workflow known to require fast parallel startup can set `worker_min_count = 10`, `worker_max_count = 10`, and `worker_count_per_start = 10` to request ten workers immediately, still bounded by pending work and the hard maximum.

`cmd/controller/worker_scaler.go` contains the first worker-scaling decision state. It plans how many workers to start from pending count, assigned count, started-worker count, elapsed time, and the scaling config. `submitWorkflowHandler` now uses that plan instead of starting exactly one worker. Current controller defaults are `worker_min_count = 0`, `worker_max_count = 2`, `worker_count_per_start = 1`, and `worker_min_elapsed_time_between_starts = "30s"`. Submitted variables can override these defaults.

## Workflow Compilation

`internal/workflow` contains the first local workflow-compilation helper. It does not expose HTTP workflow submission yet.

Current workflow model:

- A `Workflow` has an ID and an ordered list of steps.
- Workflow-scope variables live on `Workflow.Variables`.
- A `Step` has an ID and currently supports one compiler path: `FanOut`.
- A `FanOutStep` wraps the fan-out work-item template.
- A step ID becomes the default generated work-item ID prefix when the fan-out template does not provide one.
- `CompiledWorkItem` carries workflow ID and step ID metadata next to the generated `model.WorkItem`.
- `CompileResult` carries the workflow ID, step count, and compiled work items.
- Workflow compilation rejects duplicate step IDs.
- Workflow compilation rejects duplicate generated work-item IDs.

Current fan-out compilation behavior:

- Wraps fan-out work-item templates in a minimal `FanOutStep` with a required step ID.
- Resolves one fan-out expression with `Resolver.ResolveFanOutExpression`.
- Expands each fan-out value into one `model.WorkItem`.
- Builds stable item IDs and output filenames from a template plus the fan-out value.
- Reuses `WorkItem.Validate()` so generated work items obey the shared controller-worker contract.
- Supports scalar fan-out tokens for `string`, `path`, and `int`.
- Supports explicit token accessors for object fan-out values, such as `.year`.
- Supports separate token accessors for work-item IDs and output filenames.

Object fan-out values must use an explicit token accessor. The compiler does not guess which object field should become the work-item ID or output filename.

`CompileWorkflow` still returns plain `[]model.WorkItem` for compatibility. `CompileWorkflowItems` returns `[]CompiledWorkItem` when the caller needs workflow and step traceability. `CompileWorkflowResult` returns the richer compile result.

`demo-workflow.json` contains the first serialized workflow submission payload. It keeps workflow-scope variables inside the workflow object and defines a one-step fan-out workflow that produces `write_demo_output` work items for demo years.

## Local Client

`internal/client` contains the first Go local workflow client helper.

Current client behavior:

- Resolves `controller_url` through the variable resolver.
- Checks controller reachability through `GET /status` before submission.
- Can call an injected `ControllerStarter` when the controller is not reachable, then retry the reachability check.
- Provides a `LocalControllerStarter` that resolves `controller_start_executable` plus `controller_start_args` and starts them as a background process.
- Waits for a newly started controller to become reachable through repeated `GET /status` checks.
- Sends workflow submissions to `POST /workflow`.
- Loads serialized workflow submission files from disk.
- Can submit a serialized workflow submission file directly.
- Can fetch controller status and call `POST /shutdown` when pending and assigned work are both zero.
- Uses `client_status_poll_interval` as the typed variable for delay between non-idle status checks.
- Uses JSON containing a workflow plus optional submitted override/runtime variables.
- Treats the controller URL as a typed variable, not a separate config path.

The local controller starter is intentionally minimal. It resolves structured executable and argument variables and starts them as a background process. If `controller_start_lock_path` is configured, it uses an atomic lock file to avoid multiple clients starting duplicate local controllers at the same time. A pre-existing lock is treated as "another client is starting the controller," so the client continues into readiness polling. The client waits for readiness with a bounded number of status checks.

## Demo Work

The worker currently supports one operation:

```go
WorkItemTypeWriteDemoOutput WorkItemType = "write_demo_output"
```

`cmd/worker/work_demo.go`:

- Logs that the item is starting.
- Writes a small file under `TmpDir`.
- Logs the temporary output path.
- Uses `os.Rename` to promote the completed file into `DataDir`.
- Logs that the item completed.

This models the intended mounted-storage pattern: incomplete output stays temporary, while completed output appears in persistent data storage.

## Tests

The project uses Go's standard `testing` package. Run all tests from the repository root:

```powershell
go test ./...
```

Current coverage includes:

- Shared work-item validation.
- Variable type validation, including scalar, object, and list types.
- Variable literal parsing for scalar, object, and list types.
- Variable object field, list index, and fan-out accessors.
- Variable precedence merging and reference lookup.
- Recursive variable resolution, scalar structured access, fan-out expression resolution, and max-depth failure behavior.
- Local workflow fan-out compilation into validated draft work items.
- Local client workflow submission HTTP behavior.
- JSON config loading and validation.
- Runtime directory validation.
- Demo temporary-output promotion and logging.
- Worker dispatch validation.
- Worker HTTP fetch, completion, and failure clients.
- Empty-queue handling.
- Worker looping across multiple items.
- Worker failure reporting.
- Controller assignment, completion, and failure endpoints.
- Controller raw work submission and status endpoint behavior.
- Controller workflow submission into the pending queue.
- Controller worker-start hook selection from submitted variables.
- Controller local worker command resolution.
- Controller worker-scaling decision state.
- Controller shutdown endpoint behavior.
- Controller rejection of invalid methods and payloads.

Norton antivirus may briefly lock Go's temporary test executables after tests finish. If that happens, assertions still report `PASS`, but Go may print a cleanup error. Re-running the command usually succeeds.

## How To Run

Run the local workflow demo from the repository root:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl"
go run ./cmd/demo-client
```

The demo client:

- Starts a local controller if `http://localhost:8080` is not reachable.
- Submits `demo-workflow.json`.
- Lets the controller start local workers using variables from the submitted workflow file.
- Polls controller status.
- Calls `POST /shutdown` when pending and assigned work reach zero.

The worker can still be run manually:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl"
go run ./cmd/worker ./cmd/worker/demo-config.json
```

Expected worker output after exhausting the queue:

```text
worker starting
log dir: .run/logs
no work available
```

Expected completed demo output:

```text
cmd/worker/.run/data/cdl-demo-2024.txt
cmd/worker/.run/data/cdl-demo-2025.txt
```

## Design Direction

The controller now owns queue semantics. The worker stays relatively dumb: pull, execute, report, repeat.

The current in-memory queue is intentionally small. Do not add database persistence, retry rules, workflow parsing, HPCC integration, or Docker orchestration until the local controller state is observable and its basic transitions are clear.

## Likely Next Step

Add cleanup for generated demo outputs or a repeatable demo reset command.

# Project State

Last updated: 2026-06-02

## Current Focus

We now have a minimal local Go controller and worker runtime. The controller owns an in-memory work queue. The worker loads local runtime config, repeatedly pulls assigned work over HTTP, dispatches supported work-item types, writes completed output through mounted-style local directories, and reports completion or failure.

The target product still has a reusable Python interface that submits external pipeline/config files to a Go controller on backends such as HPCC. The current implementation is a local runtime foundation, not the intended user-facing API.

Project guidance is in `AGENTS.md`. The longer product and architecture direction is in `TARGET_STATE.md`.

## Current Layout

```text
go.mod
internal/
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
GET  /status         return queue counts
```

Completed items are removed from `assigned`. Failed items are removed from `assigned` and stored in `failed`. Queue state is currently process-local and is lost when the controller exits.

`POST /work` is useful for internal testing and local administration, but it is not the intended customer-facing submission boundary. The target submission boundary is workflow submission. The controller will eventually compile submitted workflows into concrete work items.

`GET /status` currently reports pending, assigned, and failed counts.

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

## Workflow Compilation

`internal/workflow` contains the first local workflow-compilation helper. It does not expose HTTP workflow submission yet.

Current workflow model:

- A `Workflow` has an ID and an ordered list of steps.
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
- Controller rejection of invalid methods and payloads.

Norton antivirus may briefly lock Go's temporary test executables after tests finish. If that happens, assertions still report `PASS`, but Go may print a cleanup error. Re-running the command usually succeeds.

## How To Run

In one terminal:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl"
go run ./cmd/controller
```

In a second terminal:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl\cmd\worker"
go run .
```

Expected worker output after exhausting the queue:

```text
worker starting
log dir: .run/logs
no work available
```

Expected completed output:

```text
cmd/worker/.run/data/local-demo-001.txt
```

## Design Direction

The controller now owns queue semantics. The worker stays relatively dumb: pull, execute, report, repeat.

The current in-memory queue is intentionally small. Do not add database persistence, retry rules, workflow parsing, HPCC integration, or Docker orchestration until the local controller state is observable and its basic transitions are clear.

## Likely Next Step

Add the first compile diagnostic field only when there is a concrete warning or non-fatal condition to report. Otherwise, keep the next slice inside `internal/workflow` and continue shaping the local workflow model before controller HTTP submission.

# Project State

Last updated: 2026-06-02

## Current Focus

We are building the Go ETL worker from the main entry point inward. The current worker loads local JSON runtime config and one local JSON work item, validates them, dispatches the work item by type, writes output through mounted-style local directories, and logs progress.

The target product has a reusable Python interface that submits external pipeline/config files to a Go controller on backends such as HPCC. The Python layer may also start a Go controller instance. The current Go worker is an early runtime component, not the intended user-facing API.

Project guidance is in `AGENT.md`. The longer product and architecture direction is in `TARGET_STATE.md`.

## Current Layout

```text
cmd/worker/
  go.mod
  main.go
  config.go
  config_test.go
  state.go
  state_test.go
  worker.go
  worker_test.go
  work_demo.go
  work_demo_test.go
  demo-config.json
  demo-item.json
  .run/
    logs/
    tmp/
    data/
```

## Current Startup Flow

The worker startup flow is:

1. Load `demo-config.json`.
2. Decode and validate its `Config`.
3. Construct a `Worker` with that config.
4. Validate that required local directories exist.
5. Load `demo-item.json`.
6. Decode and validate its `WorkItem`.
7. Run the worker with that item.
8. Dispatch the item by `Type`.
9. Write temporary output under `TmpDir`.
10. Rename the completed output into `DataDir`.
11. Append progress messages to `worker.log` under `LogDir`.

`main.go` is intentionally startup wiring. Runtime config lives in `config.go`. Work-item data lives in `state.go`. Worker lifecycle and dispatch live in `worker.go`. The concrete demo operation lives in `work_demo.go`.

## Config

Runtime config is loaded from `demo-config.json`:

```json
{
  "log_dir": ".run/logs",
  "tmp_dir": ".run/tmp",
  "data_dir": ".run/data",
  "controller_url": "https://controller.local"
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

`loadConfig(path)` reads JSON, decodes it, and calls `Config.Validate()`.

The local paths are relative to the directory where `go run .` is executed. From `cmd/worker`, the expected directories are:

```text
cmd/worker/.run/logs
cmd/worker/.run/tmp
cmd/worker/.run/data
```

## Work Items

The worker currently supports one concrete work-item type:

```go
type WorkItemType string

const (
	WorkItemTypeWriteDemoOutput WorkItemType = "write_demo_output"
)
```

A work item loaded from JSON has:

```go
type WorkItem struct {
	ID             string       `json:"id"`
	Type           WorkItemType `json:"type"`
	OutputFilename string       `json:"output_filename"`
}
```

`loadWorkItem(path)` reads JSON, decodes it, and calls `WorkItem.Validate()`.

`WorkItem.Validate()` checks structural validity:

- A non-empty ID.
- A non-empty type.
- A non-empty output filename.
- An output filename without directory components.

Rejecting directory components prevents a work item such as `../outside.txt` from writing outside configured directories.

## Validation Pattern

There are three validation layers:

- `Config.Validate()` checks whether runtime settings are present.
- `Worker.Validate()` checks whether required runtime directories exist and are directories.
- `WorkItem.Validate()` checks whether an assigned item is structurally valid.

Operation support is separate from structural validity. `Worker.runWorkItem(item)` uses a `switch` on `item.Type` and rejects unsupported operation types.

## Current Worker Behavior

`Worker.Run(item)`:

- Prints startup messages to stdout.
- Appends `worker starting` to the persistent log.
- Passes the item to the dispatcher.

`Worker.runWorkItem(item)`:

- Validates the work item.
- Dispatches supported types.
- Rejects unsupported types.

`Worker.writeDemoOutput(item)` in `work_demo.go`:

- Logs that the item is starting.
- Writes a small demo output file under `TmpDir`.
- Logs the temporary output path.
- Uses `os.Rename` to promote the completed file into `DataDir`.
- Logs that the item completed.

This models the intended container-mounted storage pattern: incomplete output stays temporary, while completed output appears in persistent data storage.

## Tests

The worker uses Go's standard `testing` package. Test files are colocated with their implementation files and end in `_test.go`.

Current tests cover:

- Loading valid JSON config.
- Missing, malformed, and invalid JSON config files.
- Required config fields.
- Loading valid JSON work items.
- Missing, malformed, and invalid JSON work-item files.
- Required work-item fields.
- Rejection of unsafe output filenames.
- Acceptance of structurally valid unknown work-item types.
- Runtime directory validation.
- Dispatch rejection of unsupported operation types.
- Rejection of structurally invalid items before dispatch.
- Demo temporary-output promotion into `DataDir`.
- Demo operation logging.
- The top-level `Worker.Run(item)` flow.

Run tests from `cmd/worker`:

```powershell
go test -v ./...
```

Norton antivirus may briefly lock Go's temporary `worker.test.exe` after tests finish. If that happens, test assertions still report `PASS`, but Go may print a cleanup error. Re-running the command usually succeeds.

## Go Concepts Introduced

- `struct` types for `Config`, `Worker`, and `WorkItem`.
- Named string types and constants for supported work-item types.
- JSON struct tags and `encoding/json`.
- Methods with receivers, such as `func (item WorkItem) Validate() error`.
- Idiomatic `(value, error)` handling.
- Wrapping errors with `fmt.Errorf("context: %w", err)`.
- `switch` dispatch by work-item type.
- `os.Stat`, `os.ReadFile`, `os.WriteFile`, and `os.OpenFile`.
- `os.Rename` for promoting completed output.
- `defer` for cleanup.
- Table-driven tests, subtests, and `t.TempDir()`.
- Colocated files in one Go package, including `work_demo.go`.

## How To Run

From the worker directory:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl\cmd\worker"
go run .
```

Expected stdout:

```text
worker starting
log dir: .run/logs
```

Expected completed output:

```text
cmd/worker/.run/data/local-demo-001.txt
```

Expected persistent log:

```text
cmd/worker/.run/logs/worker.log
```

## Design Direction

For now, `LogDir`, `TmpDir`, and `DataDir` are explicitly local directories. That matches the target container-mounted filesystem model described in `TARGET_STATE.md`.

The local JSON files are temporary stand-ins for externally supplied runtime values:

- `demo-config.json` supplies worker runtime config.
- `demo-item.json` supplies assigned work.

Keep the worker concrete and local until the execution path is clear enough to wrap with a controller API.

## Likely Next Step

Add command-line flags for the config and work-item file paths.

The executable currently hard-codes `demo-config.json` and `demo-item.json`. Flags such as `-config` and `-item` will keep those files as useful local defaults while allowing the worker executable to run other inputs without code changes.

# Project State

Last updated: 2026-06-02

## Current Focus

We are building the Go ETL worker from the main entry point inward. The current worker can load one local JSON work item, validate it, execute it against local mounted-style directories, and log its progress.

The target product has a reusable Python interface that submits external pipeline/config files to a Go controller on backends such as HPCC. The Python layer may also start a Go controller instance. The current Go worker is an early runtime component, not the intended user-facing API.

Project guidance is in `AGENT.md`. The longer product and architecture direction is in `TARGET_STATE.md`.

## Current Layout

```text
cmd/worker/
  go.mod
  main.go
  config.go
  config_test.go
  worker.go
  worker_test.go
  state.go
  state_test.go
  demo-item.json
  .run/
    logs/
    tmp/
    data/
```

## Current Startup Flow

The worker startup flow is:

1. Build a default `Config`.
2. Validate required config fields.
3. Construct a `Worker` with that config.
4. Validate that required local directories exist.
5. Load `demo-item.json`.
6. Decode and validate its `WorkItem`.
7. Run the worker with that item.
8. Write temporary output under `TmpDir`.
9. Rename the completed output into `DataDir`.
10. Append progress messages to `worker.log` under `LogDir`.

`main.go` is intentionally startup wiring. Worker-specific behavior lives in `worker.go`. Work-item data and validation live in `state.go`.

## Config

`Config` currently contains:

```go
type Config struct {
	LogDir        string
	TmpDir        string
	DataDir       string
	ControllerURL string
}
```

The default config is local-development oriented:

```go
LogDir:        ".run/logs"
TmpDir:        ".run/tmp"
DataDir:       ".run/data"
ControllerURL: "https://controller.local"
```

These paths are relative to the directory where `go run .` is executed. From `cmd/worker`, the expected directories are:

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

`WorkItem.Validate()` requires:

- A non-empty ID.
- A supported type.
- A non-empty output filename.
- An output filename without directory components.

Rejecting directory components prevents a work item such as `../outside.txt` from writing outside configured directories.

## Validation Pattern

There are three validation layers:

- `Config.Validate()` checks whether config values are present.
- `Worker.Validate()` checks whether required runtime directories exist and are directories.
- `WorkItem.Validate()` checks whether an assigned item is structurally valid and supported.

This pattern separates invalid settings, invalid runtime environments, and invalid assigned work.

## Current Worker Behavior

`Worker.Run(item)`:

- Prints startup messages to stdout.
- Appends `worker starting` to the persistent log.
- Executes the supplied item.

`Worker.runWorkItem(item)`:

- Validates the work item.
- Logs that the item is starting.
- Writes a small demo output file under `TmpDir`.
- Logs the temporary output path.
- Uses `os.Rename` to promote the completed file into `DataDir`.
- Logs that the item completed.

This models the intended container-mounted storage pattern: incomplete output stays temporary, while completed output appears in persistent data storage.

## Tests

The worker uses Go's standard `testing` package. Test files are colocated with their implementation files and end in `_test.go`.

Current tests cover:

- Required config fields.
- Required and supported work-item fields.
- Rejection of unsafe output filenames.
- Loading valid JSON work items.
- Missing, malformed, and invalid JSON work-item files.
- Runtime directory validation.
- Temporary-output promotion into `DataDir`.
- Worker logging.
- Rejection of invalid items before execution.
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
- `os.Stat`, `os.ReadFile`, `os.WriteFile`, and `os.OpenFile`.
- `os.Rename` for promoting completed output.
- `defer` for cleanup.
- Table-driven tests, subtests, and `t.TempDir()`.
- Passing a `WorkItem` into `Worker.Run(item)` instead of constructing it inside the worker.

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

The local JSON work-item file is a temporary stand-in for controller-supplied data. Keep the worker concrete and local until the execution path is clear enough to wrap with a controller API.

## Likely Next Step

Make `runWorkItem` dispatch on `item.Type` before executing the demo operation.

There is currently only one supported operation, but an explicit `switch` will establish the boundary where future CDL-oriented operations are selected. Keep the existing demo write behavior as the first concrete handler. Do not add controller API polling yet.

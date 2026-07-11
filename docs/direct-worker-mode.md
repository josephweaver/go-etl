# Direct Worker Mode for Agents

Use direct worker mode to test one already-resolved work item in the real worker
runtime without starting a client, controller, queue, polling loop, or reporting
cycle.

This mode is for worker and plugin development only. Do not use production data
or credentials.

## Inputs

Prepare:

1. A worker config whose `log_dir`, `tmp_dir`, and `data_dir` already exist.
   `controller_url` may be omitted.
2. One JSON document matching `model.WorkItem`. Do not provide workflow
   expressions or a wrapper object.
3. For `python_script`, a ZIP containing the source files at the archive root.

Direct mode supplies a missing safe attempt ID. For Python it also supplies
missing `source.run_id = direct-run-dummy` and
`source.manifest_path = source-manifest.json`.

## Command

From the repository root:

```powershell
go run ./cmd/worker execute `
  --config .run/direct/worker.json `
  --work-item .run/direct/work-item.json `
  --source-bundle .run/direct/source-bundle.zip `
  --result .run/direct/worker-result.json
```

Omit `--source-bundle` for work that does not stage source. Direct mode accepts
every operation dispatched by `Worker.Run`; it has no separate operation
allow-list.

Minimal config:

```json
{
  "log_dir": "logs",
  "tmp_dir": "tmp",
  "data_dir": "data",
  "python_executable": "python"
}
```

Config-relative paths resolve relative to the config file. The result path must
not be the config, work-item, or source-bundle path. An old result is removed
before config/work-item loading.

## Result

- Exit `0`: work completed and the result was written.
- Exit `1`: invocation, validation, execution, or result writing failed.
- Inspect `worker-result.json` plus the configured data and temporary
  directories.
- Python logs are under
  `tmp/attempts/<attempt_id>/logs/{stdout,stderr}.log`.
- Python artifacts are promoted through the normal worker artifact path.

Direct mode does not fetch work, retrieve source from a controller, send log
observations or heartbeats, or report completion/failure. Use the normal
controller-driven path when testing compilation, dependencies, scheduling,
claims, reporting, or ledger state.

## Checked-in Python fixture

Fixture inputs are under `cmd/worker/testdata/direct-python`. Build the ZIP at
test time or with `Compress-Archive`; do not commit a generated ZIP.

Run the focused verification with:

```powershell
go test ./cmd/worker -run 'TestRunDirectPythonTargetFixture' -count=1 -v
```

See `docs/RUNTIME_RUNBOOK.md` for container, Apptainer, and HPCC examples.

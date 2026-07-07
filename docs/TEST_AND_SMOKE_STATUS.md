# Test And Smoke Status

Last updated: 2026-07-07

This file preserves the moved test coverage and smoke-test status section from the pre-split root state file.

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
- Demo temporary-output promotion, deterministic overwrite, and logging.
- Worker dispatch validation.
- Worker HTTP fetch, completion, and failure clients.
- Empty-queue handling.
- Worker looping across multiple items.
- Worker failure reporting.
- Controller assignment, completion, and failure endpoints.
- Controller raw work submission and status endpoint behavior.
- Controller submission status endpoint behavior.
- Controller source-bundle endpoint behavior for admitted Python source files,
  including missing-run, missing-source-context, unsafe-path, and cache
  miss/corruption errors.
- Controller workflow submission into the pending queue.
- Controller worker-start hook selection from submitted variables.
- Controller local worker command resolution.
- Controller worker-scaling decision state.
- Controller shutdown endpoint behavior.
- Controller rejection of invalid methods and payloads.
- Controller config loading and namespace normalization.
- Controller default config loading when no config path is supplied.
- Controller execution-environment config validation and construction.
- Controller startup assembly coverage for precedence, recovery mode, qualified lookup protection, and fail-closed startup.
- Docker transport command construction for `exec` and `cp` behavior.
- SSH transport config validation, key loading, host-key checking, connect/close behavior, command execution, copy/list behavior, filesystem helpers, reconnect behavior, and end-to-end in-process SSH/SFTP fixture coverage.
- Fake HPCC SSH controller config construction.
- Client SSH setup key generation, existing-key config generation, and required host-key confirmation behavior.
- Bash shell dialect newline, quoting, path localization, copy command, and remove command behavior.
- Slurm scheduler script writing, copy, and submit behavior.
- WorkerRuntime path derivation, remote directory preparation, worker config upload, and optional worker artifact upload.
- Optional `Preparer` helper behavior for components that need setup hooks.
- Controller workflow submission using `Controller.env` to prepare the runtime and submit scheduled worker jobs.
- Required controller SQLite initialization from the qualified main-database driver and connection-string variables.
- SQLite schema creation, strict version-1 validation, parent-directory creation, and attempt snapshot insertion.
- Controller-owned attempt recording adapter.
- Controller completion handling that records full completion metadata when present and still accepts legacy `id`-only completions.
- Explicit data-operator fixture smoke coverage for `cache_data -> compute -> commit_data`, including materialized input manifest hydration into compute, terminal records for all three operator families, source-transfer resource serialization, and publish-location write serialization.
- Worker use of controller-provided `materialized_data_assets` manifests without reacquiring provider data.

Norton antivirus may briefly lock Go's temporary test executables after tests finish. If that happens, assertions still report `PASS`, but Go may print a cleanup error. Re-running the command usually succeeds.

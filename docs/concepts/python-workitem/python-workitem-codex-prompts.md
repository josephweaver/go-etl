# Codex Prompts: Python WorkItem Queue

## Prompt 1: docs-only Strategic Concept

Read:

```text
AGENTS.md
PROJECT_STATE.md
TARGET_STATE.md
docs/concepts/README.md
docs/concepts/python-workitem/README.md
docs/concepts/source-control-resolution-and-cache/README.md
```

Task:

Replace `docs/concepts/python-workitem/README.md` with a full Strategic Concept titled `Python WorkItem and Staged Source Execution`.

Use the design decisions from the supplied planning text:

- Python WorkItem is a worker-side operation, not the Python SDK.
- Python source files come only from admitted `source_manifest` files.
- Workers fetch a controller-provided admitted source bundle.
- Add an optional controller-generated work-item source locator in a later slice.
- Use `model.Parameters` for Python-specific inputs: `python_entrypoint`, optional `python_environment`, and optional `python_args`.
- First runner uses configured/system Python; environment creation is later.
- Script reads `GOET_INPUT_JSON` and writes `GOET_OUTPUT_JSON`.
- Output evidence must keep `input_sha256` and `output_sha256` visible in `WorkCompletion.OutputJSON`.

Do not edit production or test code. Update `docs/concepts/README.md` only if the concept should move from Early Concepts to Proposed Strategic Concepts.

Run no Go tests unless documentation tooling exists. Report changed documentation files only.

## Prompt 2: OS 001 WorkItem Source and Python Operation Contract

Read:

```text
AGENTS.md
PROJECT_STATE.md
docs/concepts/python-workitem/README.md
internal/model/work_item.go
internal/model/work_item_test.go
cmd/worker/worker.go
```

Task:

Implement Operational Slice 001 from the Python WorkItem concept.

Allowed production files:

```text
internal/model/work_item.go
```

Allowed test files:

```text
internal/model/work_item_test.go
```

Expected behavior:

- Add `WorkItemTypePythonScript` with JSON value `python_script`.
- Add optional `Source *WorkItemSource` to `model.WorkItem`.
- Define `WorkItemSource` with `schema`, `run_id`, and `manifest_path` JSON fields.
- Validate `WorkItemSource` when present.
- Require source locator for `python_script` either in `WorkItem.Validate()` or in a new operation-specific validation helper. Prefer not to break unknown-type structural validity unless needed.
- Preserve existing JSON compatibility and tests.

Run:

```powershell
go test ./internal/model
```

Update `PROJECT_STATE.md` only if the current implementation state changed enough to record.

## Prompt 3: OS 002 Controller Source Bundle API

Wait until source-control OS 010 and OS 011 have landed.

Read:

```text
AGENTS.md
PROJECT_STATE.md
docs/concepts/python-workitem/README.md
docs/concepts/source-control-resolution-and-cache/010-controller-admission-integration.md
docs/concepts/source-control-resolution-and-cache/011-restart-reload-verification.md
internal/reposource/cache_access.go
internal/reposource/cache_verify.go
internal/reposource/materialize.go
internal/persistence/store.go
cmd/controller/main.go
```

Task:

Implement the read-only controller source-bundle API from Python WorkItem OS 002.

Allowed production files:

```text
cmd/controller/main.go
cmd/controller/source_bundle.go
```

Allowed test files:

```text
cmd/controller/source_bundle_test.go
cmd/controller/main_test.go
```

Expected behavior:

- Add `GET /workflow-runs/<run_id>/source-bundle.zip` or an equivalently documented controller endpoint that returns the admitted source files for one active run as a safe `application/zip` bundle.
- Locate source-admission context from the durable workflow run record.
- Read every bundled file through `internal/reposource` verified cache access.
- Do not read provider source directly.
- Include only files in the admitted manifest.
- Preserve repository-relative paths safely.
- Return clear errors for missing run, missing manifest, cache miss, and corrupted cache.

Run targeted controller/source tests.

## Prompt 4: OS 003 Worker Source Bundle Client and Staging

Read:

```text
AGENTS.md
PROJECT_STATE.md
docs/concepts/python-workitem/README.md
cmd/worker/config.go
cmd/worker/worker.go
cmd/worker/state.go
internal/model/work_item.go
```

Task:

Implement the worker source-bundle downloader and safe attempt staging helper.

Allowed production files:

```text
cmd/worker/source_bundle.go
cmd/worker/config.go
cmd/worker/worker.go
```

Allowed test files:

```text
cmd/worker/source_bundle_test.go
cmd/worker/config_test.go
cmd/worker/worker_test.go
```

Expected behavior:

- Given `ControllerURL` and `WorkItem.Source`, request the controller source bundle.
- Create `<TmpDir>/attempts/<attempt_id>/source`, `work`, and `logs` directories.
- Safely extract bundle entries into `source`.
- Reject traversal, absolute paths, Windows drive paths, backslashes, duplicate entries, and symlink-like entries.
- Keep worker validation and existing tests passing.

Run:

```powershell
go test ./cmd/worker
```

## Prompt 5: OS 004 Python Subprocess Runner

Read:

```text
AGENTS.md
PROJECT_STATE.md
docs/concepts/python-workitem/README.md
cmd/worker/worker.go
cmd/worker/state.go
cmd/worker/work_demo.go
cmd/worker/work_summary.go
internal/model/work_item.go
```

Task:

Implement the first `python_script` runner without environment creation.

Allowed production files:

```text
cmd/worker/work_python.go
cmd/worker/worker.go
cmd/worker/config.go
```

Allowed test files:

```text
cmd/worker/work_python_test.go
cmd/worker/worker_test.go
cmd/worker/config_test.go
```

Expected behavior:

- Dispatch `model.WorkItemTypePythonScript` to a new Python runner.
- Require and validate `python_entrypoint` parameter.
- Optionally validate `python_environment` parameter.
- Optionally accept `python_args` as a list of strings.
- Write `GOET_INPUT_JSON` into attempt work dir.
- Set `GOET_*` environment variables.
- Execute the configured/default Python interpreter with cwd at staged source dir.
- Capture stdout/stderr to attempt log files.
- Fail clearly on missing entrypoint, non-zero exit, or missing output JSON.

Run:

```powershell
go test ./cmd/worker
```

Skip subprocess execution tests only when no Python executable is available in the test environment.

## Prompt 6: OS 005 Python Output Evidence Contract

Read:

```text
AGENTS.md
PROJECT_STATE.md
docs/concepts/python-workitem/README.md
cmd/worker/work_python.go
cmd/worker/work_demo.go
cmd/worker/state.go
cmd/controller/main.go
```

Task:

Complete the Python output/evidence behavior.

Allowed production files:

```text
cmd/worker/work_python.go
cmd/worker/work_demo.go
```

Allowed test files:

```text
cmd/worker/work_python_test.go
cmd/worker/work_demo_test.go
cmd/worker/state_test.go
```

Expected behavior:

- Decode `GOET_OUTPUT_JSON` as exactly one JSON document.
- Canonicalize script logical output.
- Atomically promote canonical script logical output to `DataDir/<output_filename>`.
- Return `WorkEvidence` with valid `OutputJSON`, `PreStateJSON`, `PostStateJSON`, and deterministic hashes.
- Ensure `OutputJSON` includes top-level `input_sha256` and `output_sha256` so current controller reuse-candidate extraction still works.
- Do not implement Python skip/reuse yet.

Run:

```powershell
go test ./cmd/worker ./cmd/controller
```

## Prompt 7: OS 006 Workflow Compilation Integration

Wait until OS 010/011 and Python WorkItem slices 001-005 land.

Read:

```text
AGENTS.md
PROJECT_STATE.md
docs/concepts/python-workitem/README.md
docs/concepts/source-control-resolution-and-cache/010-controller-admission-integration.md
cmd/controller/main.go
internal/model/work_item.go
internal/reposource/model.go
```

Task:

Attach controller-generated source locators to compiled Python work items during source-reference workflow admission and validate Python source path roles.

Allowed production files:

```text
cmd/controller/main.go
```

Allowed test files:

```text
cmd/controller/main_test.go
```

Allowed fixture files:

```text
../go-etl-demo-project/workflows/*.json
../go-etl-demo-project/submissions/*.json
```

Expected behavior:

- For `python_script` work items, attach `WorkItem.Source` from the run source-admission context.
- Validate `python_entrypoint` exists in the admitted manifest with role `python_entrypoint`.
- Validate `python_environment` exists in the admitted manifest with role `python_environment` when present.
- Reject undeclared or wrong-role source paths before inserting work items.
- Leave non-Python work items unchanged.
- Add a small Python demo workflow fixture if available.

Run targeted controller admission tests.

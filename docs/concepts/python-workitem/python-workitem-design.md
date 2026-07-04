# Python WorkItem and Staged Source Execution

Status: Proposed

Cadence: CSxIx

## Purpose

Add a GOET worker operation that executes an admitted Python entrypoint from a workflow source manifest. The operation should use controller-admitted source files, a worker-local staging directory, resolved work-item parameters, and a structured output contract.

This is not the Python client or Python SDK. The Python client remains an interface for starting or calling the Go controller and submitting workflow/project files. The Python WorkItem is a worker-side execution operation inside the Go runtime.

## Strategic Context

The current GOET state already has the prerequisites for this concept:

- `internal/reposource` defines admitted source manifests, roles, cache layout, verified cached reads, and materialization.
- Workflow source documents can declare supplemental files through `source_manifest` roles: `python_entrypoint`, `python_environment`, and `support_file`.
- The controller persists compiled worker payload JSON in `work_items.worker_payload_json` and workers receive `model.WorkItem` values through `/work/next`.
- `model.WorkItem.Parameters` is the existing transport for concrete resolved worker inputs.
- The worker currently dispatches by `WorkItem.Type` and supports `write_demo_output` and `summarize_input_file`.
- The worker reports `WorkEvidence` through `WorkCompletion`, including `output_json`, state hashes, and attempt metadata.

The missing boundary is the source-to-worker execution contract: how a worker obtains admitted Python files, stages them, invokes Python, captures logs, and reports typed output evidence.

## Goals

- Add a `python_script` work-item type.
- Treat Python source files as admitted source-manifest files, not runtime discoveries.
- Keep client-submitted paths as repository-relative source paths; workers never resolve GitHub or local repository sources directly.
- Let the controller expose admitted run source files to workers through a bounded source-bundle API backed by verified cache reads.
- Let the worker download/extract the source bundle into an attempt-local staging directory.
- Execute one declared Python entrypoint as a subprocess.
- Pass concrete resolved inputs through an input JSON file and explicit environment variables.
- Optionally pass command-line arguments from resolved work-item parameters.
- Capture stdout and stderr without treating them as typed outputs.
- Require the Python script to write a structured output JSON file.
- Promote the structured output to `DataDir/<output_filename>` atomically.
- Return `WorkEvidence` compatible with the existing completion path and reuse-candidate hash extraction.
- Keep environment creation incremental: first support the configured/system Python interpreter, then add environment-spec parsing, then add cached JIT environment creation.

## Non-Goals

- Python SDK or Python client implementation.
- Moving workflow compilation, scheduling, dependency readiness, or queue semantics into Python.
- Letting workers scan imports or discover undeclared source files.
- Fetching GitHub or local repository sources from the worker.
- Allowing workflow authors to submit controller cache paths.
- Secret propagation or credential injection.
- Multi-repository source manifests.
- Arbitrary network package installation in the first implementation.
- Worker-side skip/reuse for arbitrary Python scripts in the first implementation.
- Full controller-owned logging framework implementation; this concept should integrate with execution observability when that epic lands.

## Core Decisions

### 1. Python WorkItem is a worker operation, not a client API

The future Python package should submit project/workflow files to the Go controller. It should not own work queues, retries, scheduler policy, source-cache repair, or worker orchestration. The Python WorkItem belongs to `cmd/worker` and consumes `model.WorkItem` like the existing demo operations.

### 2. Admitted source is the only executable source

A Python work item may execute only files that were admitted before run creation through the workflow `source_manifest`. If the script later needs an undeclared helper file, that is a workflow authoring error.

Accepted source-manifest roles remain:

```text
python_entrypoint
python_environment
support_file
```

The work-item payload chooses an entrypoint by repository/cache path, but the controller must validate that the path is present in the admitted manifest with role `python_entrypoint`.

### 3. Workers should fetch source bundles from the controller

Initial endpoint shape: `GET /workflow-runs/<run_id>/source-bundle.zip`, returning `application/zip`. The zip should contain staged repository-relative files, plus a GOET manifest metadata entry such as `.goet/source-manifest.json`.

The worker should not need direct access to the controller repository-cache filesystem. This matters for remote HPCC workers and keeps the source cache controller-owned.

Target source flow:

```text
/workflow admission
  -> resolve source refs
  -> publish admitted files into repository cache
  -> persist source-admission context
  -> compile python_script work items with controller-generated source locator

worker /work/next
  -> receive WorkItem with source locator and python parameters
  -> request admitted source bundle from controller
  -> extract into attempt-local staging directory
  -> run Python entrypoint from staged source
```

### 4. WorkItem needs a controller-generated source locator

Add an optional source field to `model.WorkItem`:

```go
type WorkItemSource struct {
    Schema       string `json:"schema,omitempty"`
    RunID        string `json:"run_id"`
    ManifestPath string `json:"manifest_path"`
}

type WorkItem struct {
    ...
    Source *WorkItemSource `json:"source,omitempty"`
}
```

`Source` is optional so raw/admin work and non-source work items remain valid. For `python_script`, it is required. The controller generates this field after source admission. Workflow authors must not provide it as a workflow parameter.

### 5. Python-specific payload lives in parameters first

Use the existing `model.Parameters` map for operation-specific inputs. Required and optional parameters:

```json
{
  "python_entrypoint": {
    "type": "path",
    "value": "scripts/run_step.py"
  },
  "python_environment": {
    "type": "path",
    "value": "environments/python.json"
  },
  "python_args": {
    "type": "list",
    "value": ["--year", "2024", "--tile", "tile-001"]
  }
}
```

Rules:

- `python_entrypoint` is required and must be a `path` or `string`.
- `python_entrypoint` must refer to a bundled file with role `python_entrypoint`.
- `python_environment` is optional and must refer to a bundled file with role `python_environment` when present.
- `python_args` is optional and must be a list of non-empty strings when present.
- Other parameters are treated as user inputs and copied into the worker-created input JSON.

### 6. Worker staging is attempt-local

For an assigned work item with attempt ID `attempt-abc`, the worker creates:

```text
<TmpDir>/attempts/attempt-abc/
  source/
    scripts/run_step.py
    scripts/lib/helper.py
    environments/python.json
  work/
    input.json
    output.json
  logs/
    stdout.log
    stderr.log
```

Extraction must reject absolute paths, `..` segments, backslashes, drive-qualified paths, symlinks, duplicate entries, and any path escaping the staging root.

### 7. Script interface

The worker runs the Python entrypoint with:

```text
cwd = <attempt>/source
python <entrypoint> <python_args...>
```

The worker sets environment variables:

```text
GOET_WORK_ITEM_ID
GOET_ATTEMPT_ID
GOET_INPUT_JSON
GOET_OUTPUT_JSON
GOET_SOURCE_DIR
GOET_WORK_DIR
GOET_DATA_DIR
GOET_TMP_DIR
GOET_LOG_DIR
GOET_PYTHON_ENTRYPOINT
GOET_PYTHON_ENVIRONMENT_JSON   # only when supplied
```

The worker writes `GOET_INPUT_JSON` before launching the process:

```json
{
  "schema": "goet/python-workitem-input/v1",
  "work_item": {
    "id": "run-001:python-2024",
    "attempt_id": "attempt-abc",
    "workflow_definition_id": "workflow-python-demo",
    "workflow_instance_id": "workflow-python-demo-instance-...",
    "step_definition_id": "run-python",
    "code_version": "..."
  },
  "parameters": {
    "year": {"type": "int", "value": 2024},
    "tile": {"type": "string", "value": "tile-001"}
  },
  "paths": {
    "source_dir": ".../source",
    "work_dir": ".../work",
    "data_dir": ".../data",
    "tmp_dir": ".../tmp",
    "log_dir": ".../logs"
  }
}
```

The script writes JSON to `GOET_OUTPUT_JSON`. Stdout and stderr are logs only, not typed outputs.

### 8. Output and evidence contract

On success, the worker validates `GOET_OUTPUT_JSON` as a single JSON document, canonicalizes it, and atomically promotes it to:

```text
<DataDir>/<output_filename>
```

`WorkCompletion.OutputJSON` should be a GOET evidence wrapper that still exposes `input_sha256` and `output_sha256` for existing reuse-candidate extraction:

```json
{
  "schema": "goet/python-workitem-output/v1",
  "work_item_id": "run-001:python-2024",
  "operation": "python_script",
  "entrypoint": "scripts/run_step.py",
  "environment": "environments/python.json",
  "exit_code": 0,
  "logical_output": {
    "status": "ok",
    "outputs": {
      "summary_path": {"type": "path", "value": "/data/goetl/data/result.json"}
    }
  },
  "input_sha256": "...",
  "output_sha256": "...",
  "pre_state_sha256": "...",
  "post_state_sha256": "...",
  "stdout_sha256": "...",
  "stderr_sha256": "..."
}
```

Hash definitions:

- `input_sha256`: canonical hash of operation type, resolved parameters, source manifest file roles/paths/raw hashes used by the work item, selected entrypoint, selected environment file, command args, and Python runner contract version.
- `output_sha256`: canonical hash of the script-produced logical output JSON only, before wrapping it in GOET evidence.
- `pre_state_sha256` / `post_state_sha256`: canonical hash of the state of `DataDir/<output_filename>` before and after promotion.
- `stdout_sha256` / `stderr_sha256`: raw byte hash of captured streams.

The first Python runner should not attempt worker-side skip/reuse because arbitrary Python scripts cannot predict their output before execution. It should still produce input/output/state hashes so future controller decisions have evidence.

### 9. Environment handling is phased

First implementation:

- Use a configured/system Python executable, defaulting to `python3`.
- Validate that `python_environment`, when present, is bundled and readable.
- Pass the environment spec path through `GOET_PYTHON_ENVIRONMENT_JSON`.
- Do not create a virtual environment yet.

Later implementation:

- Define `PythonEnvironment` schema.
- Support preinstalled environment selection.
- Add worker-side environment cache root.
- Add JIT environment creation from lock-file-style specs.
- Add resource constraints around environment creation so many workers do not build the same environment concurrently.

## Example workflow source fragment

This uses the current exported Go struct JSON style for workflow fields and the existing lower-case `source_manifest` field.

```json
{
  "workflow": {
    "ID": "python-demo",
    "Variables": [
      {
        "name": {"namespace": "workflow", "key": "years"},
        "type": "list",
        "expression": [
          {"type": "int", "expression": 2024}
        ]
      }
    ],
    "Steps": [
      {
        "ID": "run-python",
        "FanOut": {
          "WorkItem": {
            "FanOutExpression": "${years[*]}",
            "Type": "python_script",
            "IDPrefix": "python",
            "OutputPrefix": "python-result",
            "OutputExtension": ".json",
            "Parameters": {
              "python_entrypoint": {
                "type": "path",
                "value": "scripts/run_step.py"
              },
              "python_environment": {
                "type": "path",
                "value": "environments/python.json"
              },
              "python_args": {
                "type": "list",
                "value": ["--year", "${year}"]
              },
              "year": {
                "type": "int",
                "value": 0
              }
            },
            "ParameterAccessors": {
              "year": ""
            }
          }
        }
      }
    ]
  },
  "source_manifest": {
    "files": [
      {
        "role": "python_entrypoint",
        "path": "scripts/run_step.py",
        "content_type": "text/x-python"
      },
      {
        "role": "python_environment",
        "path": "environments/python.json",
        "content_type": "application/json"
      },
      {
        "role": "support_file",
        "path": "scripts/lib/helper.py",
        "content_type": "text/x-python"
      }
    ]
  },
  "variables": []
}
```

Note: the current fan-out compiler does not yet resolve string interpolation inside arbitrary `model.Parameters` values except through `ParameterAccessors`. A clean first demo can use static `python_args` or pass dynamic values through the generated input JSON instead of command-line arguments.

## Proposed Operational Slices

### 001 WorkItem Source and Python Operation Contract

Objective: add the optional source locator and Python work-item type without execution behavior.

Allowed production files:

```text
internal/model/work_item.go
```

Allowed test files:

```text
internal/model/work_item_test.go
```

Acceptance criteria:

- `model.WorkItemTypePythonScript` exists with JSON value `python_script`.
- `model.WorkItem` has optional `Source *WorkItemSource`.
- `WorkItemSource` validates non-empty `run_id` and `manifest_path` when present.
- `python_script` work items require `Source` during validation or during a new operation-specific validation helper.
- Existing non-source work items remain valid.
- JSON round-trip tests cover `Source` and `python_script`.

### 002 Controller Source Bundle API

Objective: let a worker download admitted files for one run from the controller through verified cache reads.

Depends on source-control OS 010 and OS 011.

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

Acceptance criteria:

- Controller exposes a read-only source-bundle endpoint for active admitted runs.
- The endpoint locates the run source-admission context from durable workflow-run state.
- The endpoint reads files through `internal/reposource` verified cache access.
- The endpoint never reads provider source directly.
- The response contains only admitted manifest files.
- Bundle paths preserve repository-relative layout and reject unsafe paths.
- Missing or corrupted cache returns a clear error rather than silently reading local source paths.
- Tests cover successful bundle, missing run, missing manifest, corrupted cache, and unsafe manifest entry rejection.

### 003 Worker Source Bundle Client and Staging

Objective: give the worker a reusable source-bundle downloader and safe extractor.

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

Acceptance criteria:

- Worker can request a source bundle using `ControllerURL` and `WorkItem.Source`.
- Worker creates an attempt-local staging directory under `TmpDir`.
- Worker safely extracts the source bundle into `source/`.
- Extraction rejects path traversal, absolute paths, backslashes, drive-qualified paths, duplicate entries, and symlinks.
- Worker staging cleanup behavior is explicit: either retain failed attempts for diagnostics or clean successful staging after output promotion.
- Tests use an in-process HTTP server and malicious bundle fixtures.

### 004 Python Subprocess Runner, No Environment Creation

Objective: execute a staged Python entrypoint using the configured/system Python interpreter.

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

Acceptance criteria:

- Worker dispatch supports `python_script`.
- Runner requires `python_entrypoint` and validates it is inside staged source.
- Runner optionally accepts `python_environment` and validates it is inside staged source.
- Runner writes `GOET_INPUT_JSON` before process launch.
- Runner sets the agreed `GOET_*` environment variables.
- Runner runs the entrypoint with `cwd` set to the staged source directory.
- Runner captures stdout/stderr into bounded attempt log files.
- Non-zero exit status returns work failure with a clear error.
- Missing output JSON returns work failure.
- Tests execute a tiny Python script available on the test host or skip only when Python is unavailable.

### 005 Python Output Evidence Contract

Objective: convert script output into GOET `WorkEvidence` compatible with persisted completion.

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

Acceptance criteria:

- Script output JSON is decoded as exactly one JSON document.
- Worker canonicalizes script output before promotion.
- Worker writes the canonical script output to `DataDir/<output_filename>` atomically.
- Worker returns `OutputJSON` wrapper containing `input_sha256` and `output_sha256`.
- Worker returns `PreStateJSON` and `PostStateJSON` compatible with existing completion validation.
- Hashes are deterministic across equivalent JSON formatting.
- Existing `workerObservedHashesFromOutputJSON` can extract Python output hashes.

### 006 Workflow Compilation Integration for Python Source WorkItems

Objective: have source-reference workflow admission inject source locators into compiled Python work items.

Depends on OS 010/011 and slice 001.

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

Acceptance criteria:

- When compiling source-reference workflow runs, controller-generated `WorkItem.Source` is attached to each `python_script` work item.
- The source locator references the admitted run and manifest from source-admission context.
- The controller validates `python_entrypoint` against the admitted manifest role before queueing the work item.
- The controller validates `python_environment` against the admitted manifest role when present.
- Non-Python work items do not receive source locators unless a later concept needs them.
- A Python demo workflow admits and queues successfully.
- A Python workflow referencing an undeclared entrypoint fails before work-item insertion.

### 007 Python Environment Specification V1

Objective: define the Python environment file schema without JIT environment creation.

Allowed production files:

```text
cmd/worker/python_environment.go
```

Allowed test files:

```text
cmd/worker/python_environment_test.go
```

Acceptance criteria:

- Environment file decodes `api_version: goet/v1alpha1` and `kind: PythonEnvironment`.
- First supported mode is `system`, selecting the worker-configured/default interpreter.
- Unsupported modes fail clearly.
- Environment file must not contain secrets.
- Tests cover valid system mode, unsupported mode, missing kind/version, and invalid JSON.

### 008 Cached Python Environment Creation

Objective: add JIT environment creation using worker-local environment cache.

Defer until Python runner, source bundle, and resource-constraint design are stable.

Initial acceptance direction:

- Add `worker_config.python_env_cache_dir` and possibly `worker_config.python_package_cache_dir`.
- Compute environment fingerprint from environment spec file plus runner version.
- Create environments under cache root atomically.
- Reuse existing environment when fingerprint matches.
- Avoid concurrent duplicate builds on the same worker host.
- Do not install packages from the network unless explicitly configured.

### 009 Execution Observability Integration

Objective: replace direct stdout/stderr log files with the execution-observability logging client when that epic is implemented.

Defer until `execution-observability` slices provide shared log observations, worker logging client, and fallback logging.

## Recommended Codex Queue

Do not run controller-source Python implementation concurrently with OS 010/011 because those tasks touch the same admission and source-cache files.

Safe queue order after OS 010/011 lands:

```text
1. Docs-only: replace docs/concepts/python-workitem/README.md with this Strategic Concept.
2. OS 001: WorkItem Source and Python Operation Contract.
3. OS 002: Controller Source Bundle API.
4. OS 003: Worker Source Bundle Client and Staging.
5. OS 004: Python Subprocess Runner, No Environment Creation.
6. OS 005: Python Output Evidence Contract.
7. OS 006: Workflow Compilation Integration and demo fixture.
```

A later queue can handle environment-spec parsing, cached JIT environments, resource locks, and observability.

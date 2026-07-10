# OS-002: Data Asset Python Environment Selection

Status: Proposed

## Purpose

Let `python_script` work items select a pre-created Python `.venv` that has been
materialized through the Data Assets infrastructure before executing the Python
entrypoint.

## Scope

This slice consumes environment assets that already exist or have already been
materialized for the attempt. It does not create or install dependencies; that
belongs to `python-create-env`.

## Proposed Contract

Worker config declares a Python environment asset materialization root:

```json
{
  "python_environment_asset_root": "<PYTHON_ENV_ASSET_ROOT>"
}
```

A Python work item may reference a materialized environment asset:

```json
{
  "type": "python_script",
  "python_environment": {
    "asset_key": "<ENVIRONMENT_ASSET_KEY>"
  }
}
```

The controller or worker supplies the selected environment through the existing
materialized Data Assets manifest shape, extended with Python environment
metadata where needed:

```text
schema = goet/materialized-data-assets/v1
asset_key = <ENVIRONMENT_ASSET_KEY>
kind = python_environment
local_path = <python_environment_asset_root>/<ENVIRONMENT_ASSET_KEY>/.venv
```

The worker validates `local_path`, confirms it points under the configured
environment asset root, and runs:

```text
<local_path>/bin/python
```

on POSIX runtimes, with equivalent path handling for Windows workers if needed.

The selected asset's evidence must include enough compatibility data for the
worker to reject incompatible environments before execution:

```text
environment spec hash
python major.minor
platform and architecture
worker image digest or runtime fingerprint
GDAL version when relevant
installer and installer version
```

## Safety Requirements

- Reject empty environment asset keys.
- Reuse Data Assets path validation for materialized asset paths.
- Resolve and validate the final `.venv` path before execution.
- Fail clearly if the selected `.venv` or Python executable is missing.
- Do not let ordinary Python execution create or mutate shared environments.
- Fail before script execution when the environment asset compatibility evidence
  does not match the current worker runtime.

## Execution Requirements

When an environment is selected, the worker must:

- use the selected environment's Python executable;
- preserve the existing `GOET_INPUT_JSON` and `GOET_OUTPUT_JSON` contract;
- preserve admitted-source staging behavior;
- consume environment selection through Data Assets materialization evidence;
- capture stdout and stderr through the existing worker log path;
- include the selected environment asset key in completion evidence or attempt
  metadata.

## Validation

Add a smoke that:

1. creates a tiny `.venv` under a temporary environment asset root;
2. records it as a materialized `python_environment` Data Asset with compatible
   evidence;
3. installs or writes a small marker package/module;
4. runs a `python_script` work item that imports that marker;
5. verifies the script used the selected `.venv` rather than system Python.

## Stop Conditions

- Environment path validation cannot be made deterministic across supported
  worker platforms.
- Existing Python work-item execution regresses when no environment is selected.
- The worker can escape the configured environment asset root.
- Environment compatibility cannot be verified from compact evidence.

## Completion Criteria

- Worker config can declare the environment asset root.
- `python_script` can select a pre-created environment asset.
- Missing, unsafe, or incompatible environments fail with actionable errors.
- A local smoke proves environment selection.

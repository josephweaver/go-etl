# OS-002: Mounted Python Environment Selection

Status: Proposed

## Purpose

Let `python_script` work items select a pre-created Python `.venv` from a
configured worker environment root before executing the Python entrypoint.

## Scope

This slice consumes environments that already exist. It does not create or
install dependencies; that belongs to `python-create-env`.

## Proposed Contract

Worker config declares a Python environment root:

```json
{
  "python_environment_root": "<PYTHON_ENV_ROOT>"
}
```

A Python work item may reference an environment:

```json
{
  "type": "python_script",
  "python_environment": {
    "id": "<ENVIRONMENT_ID>"
  }
}
```

The worker resolves the selected environment to:

```text
<python_environment_root>/<ENVIRONMENT_ID>/.venv
```

and runs:

```text
<python_environment_root>/<ENVIRONMENT_ID>/.venv/bin/python
```

on POSIX runtimes, with equivalent path handling for Windows workers if needed.

## Safety Requirements

- Reject empty environment IDs.
- Reject absolute paths, `..`, path separators, shell metacharacters, or any
  environment ID that cannot be represented as a single safe directory name.
- Resolve and validate the final `.venv` path before execution.
- Fail clearly if the selected `.venv` or Python executable is missing.
- Do not let ordinary Python execution create or mutate shared environments.

## Execution Requirements

When an environment is selected, the worker must:

- use the selected environment's Python executable;
- preserve the existing `GOET_INPUT_JSON` and `GOET_OUTPUT_JSON` contract;
- preserve admitted-source staging behavior;
- capture stdout and stderr through the existing worker log path;
- include the selected environment ID in completion evidence or attempt
  metadata.

## Validation

Add a smoke that:

1. creates a tiny `.venv` under a temporary environment root;
2. installs or writes a small marker package/module;
3. runs a `python_script` work item that imports that marker;
4. verifies the script used the selected `.venv` rather than system Python.

## Stop Conditions

- Environment path validation cannot be made deterministic across supported
  worker platforms.
- Existing Python work-item execution regresses when no environment is selected.
- The worker can escape the configured environment root.

## Completion Criteria

- Worker config can declare the environment root.
- `python_script` can select a pre-created environment.
- Missing or unsafe environments fail with actionable errors.
- A local smoke proves environment selection.

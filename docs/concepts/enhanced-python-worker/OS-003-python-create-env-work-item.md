# OS-003: Python Create Environment Work Item

Status: Proposed

## Purpose

Add a `python-create-env` work-item type that creates or refreshes a shared
Python `.venv` under the configured worker environment root.

Environment creation should be explicit workflow work. Ordinary `python_script`
items should execute against a selected environment but should not install
packages as a side effect.

## Proposed Work-Item Type

Initial shape:

```json
{
  "type": "python_create_env",
  "python_environment": {
    "id": "<ENVIRONMENT_ID>",
    "spec_path": "<ADMITTED_ENV_SPEC_PATH>",
    "installer": "pip"
  }
}
```

The exact workflow JSON naming can be adjusted to match existing work-item
schema conventions during implementation.

## Environment Spec

The first slice should pick one minimal admitted spec format. A conservative
starting point is a controller-admitted `requirements.txt` file from the
workflow source manifest.

Later slices may add lock files, `pyproject.toml`, offline wheelhouses, or
GOET-owned JSON environment specs.

## Worker Behavior

The worker must:

- validate the environment ID with the same rules used by Python execution;
- stage the admitted environment spec from the controller-provided source
  bundle;
- create `<python_environment_root>/<ENVIRONMENT_ID>/.venv`;
- install dependencies using the selected installer;
- write an environment marker file containing the spec hash and creation
  metadata;
- report evidence that includes the environment ID, spec hash, installer, and
  created environment path relative to the environment root.

## Idempotency

The preferred first behavior is:

- if the environment exists and the marker spec hash matches, report success
  without reinstalling;
- if the environment exists and the marker spec hash differs, fail unless an
  explicit replace policy is set;
- if another worker is creating the same environment, fail fast or wait behind a
  simple lock, depending on the available shared-filesystem locking primitive.

The implementation slice should choose the simplest reliable locking behavior
and document it.

## Safety Requirements

- Never install into system Python.
- Never install outside the configured environment root.
- Never execute package-manager commands from unvalidated paths.
- Do not print secrets in installer logs.
- Do not allow workflow-authored shell fragments as install commands in the
  first version.

## Validation

Add a local smoke that:

1. submits a `python_create_env` work item with a tiny admitted requirements
   file;
2. verifies the environment marker and `.venv` are created under the configured
   root;
3. runs a dependent `python_script` work item using that environment;
4. verifies idempotent rerun behavior for the same spec.

## Stop Conditions

- Environment creation cannot be made safe against path escape.
- Dependency installation requires credentials or private package indexes in the
  default smoke.
- Concurrent creation can corrupt the environment.

## Completion Criteria

- `python_create_env` is a recognized work-item type.
- The worker can create a `.venv` under a configured environment root.
- A later `python_script` work item can use the created environment.
- Evidence records the environment ID and spec hash.
- Local smoke coverage proves create-env plus execute-env sequencing.

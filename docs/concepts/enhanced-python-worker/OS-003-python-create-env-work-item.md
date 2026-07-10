# OS-003: Python Create Environment Work Item

Status: Proposed

## Purpose

Add a `python-create-env` work-item type that creates or refreshes a shared
Python `.venv` and records it as a materialized Data Asset under the configured
Python environment asset root.

Environment creation should be explicit workflow work. Ordinary `python_script`
items should execute against a selected environment asset but should not install
packages as a side effect.

## Proposed Work-Item Type

Initial shape:

```json
{
  "type": "python_create_env",
  "python_environment": {
    "asset_key": "<ENVIRONMENT_ASSET_KEY>",
    "spec_path": "<ADMITTED_ENV_SPEC_PATH>",
    "installer": "pip",
    "target_environment_id": "<TARGET_ENVIRONMENT_ID>"
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

The environment asset identity must include the environment spec hash and the
runtime compatibility fingerprint, not only a user-facing name:

```text
environment spec hash
python major.minor
platform and architecture
worker image digest or runtime fingerprint
GDAL version when relevant
installer and installer version
package index or wheelhouse identity when relevant
```

## Worker Behavior

The worker must:

- validate the environment asset key with Data Assets cache-key/path rules;
- stage the admitted environment spec from the controller-provided source
  bundle;
- create `<python_environment_asset_root>/<ENVIRONMENT_ASSET_KEY>/.venv`;
- install dependencies using the selected installer;
- write an environment marker file containing the spec hash, runtime
  fingerprint, and creation metadata;
- report a compact materialized Data Assets manifest entry with
  `kind = python_environment`, the environment asset key, relative environment
  path, directory manifest hash, spec hash, installer, and runtime compatibility
  evidence.

## Idempotency

The preferred first behavior is:

- if the environment asset exists and its Data Asset evidence plus marker match
  the requested spec and runtime fingerprint, report success without
  reinstalling;
- if the environment asset exists and its evidence differs, fail unless an
  explicit replace policy is set;
- if another worker is creating the same environment, fail fast or wait behind a
  simple lock, depending on the available shared-filesystem locking primitive.

The implementation slice should choose the simplest reliable locking behavior
and document it. Reuse Data Assets immutable cache conflict behavior where it
fits.

## Safety Requirements

- Never install into system Python.
- Never install outside the configured environment asset root.
- Never execute package-manager commands from unvalidated paths.
- Do not print secrets in installer logs.
- Do not allow workflow-authored shell fragments as install commands in the
  first version.
- Do not claim a materialized environment asset until installation and evidence
  generation are complete.

## Validation

Add a local smoke that:

1. submits a `python_create_env` work item with a tiny admitted requirements
   file;
2. verifies the environment marker and `.venv` are created under the configured
   environment asset root;
3. verifies the work item returns a materialized `python_environment` Data Asset
   manifest entry;
4. runs a dependent `python_script` work item using that environment asset;
5. verifies idempotent rerun behavior for the same spec and runtime
   fingerprint.

## Stop Conditions

- Environment creation cannot be made safe against path escape.
- Dependency installation requires credentials or private package indexes in the
  default smoke.
- Concurrent creation can corrupt the environment.
- The resulting environment cannot be represented as compact Data Asset
  evidence.

## Completion Criteria

- `python_create_env` is a recognized work-item type.
- The worker can create a `.venv` under a configured environment asset root.
- A later `python_script` work item can use the created environment asset.
- Evidence records the environment asset key, spec hash, directory manifest
  hash, and runtime compatibility fingerprint.
- Local smoke coverage proves create-env plus execute-env sequencing.

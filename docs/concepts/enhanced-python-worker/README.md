# Enhanced Python Worker

Status: Proposed

## Purpose

Add a production-oriented Python worker runtime that can run Python work items
with a controlled dependency environment instead of relying only on system
Python.

The immediate target is an enhanced worker image with the usual scientific and
geospatial runtime tools, plus a Data Assets-backed materialization path for
reusable Python `.venv` directory assets. A Python work item selects the correct
environment asset before executing its entrypoint. A new `python-create-env`
work item creates or refreshes those environment assets as controller-managed
work.

## Goals

- Provide an enhanced Python worker image with common runtime tools.
- Reuse the Data Assets infrastructure for environment identity, materialized
  directory evidence, cache roots, reuse, and lifecycle instead of creating a
  separate Python-environment registry.
- Support a worker-mounted Python environment asset root that stores
  materialized `.venv` directory assets outside individual attempt directories.
- Let each Python work item select one environment by materialized environment
  asset identity or resolved environment requirement.
- Add a `python-create-env` work-item type that creates a `.venv` from an
  admitted and validated environment specification and records it as a
  materialized Data Asset.
- Make environment creation explicit workflow work, not an implicit side effect
  of ordinary Python script execution.
- Preserve the current admitted-source boundary: workers use controller-served
  source bundles and do not fetch mutable user code directly.
- Keep controller-owned evidence for environment creation, including the input
  spec hash, runtime compatibility fingerprint, and resulting environment asset
  identity.

## Non-Goals

- Replacing the existing `python_script` work-item type.
- Implementing a full Python package manager abstraction in the first slice.
- Letting arbitrary work items mutate shared environments while another attempt
  may be using them.
- Letting workers install packages from undeclared files or live source trees.
- Solving all container publication and registry automation.
- Making Google Drive, HPCC, Slurm, or SSH behavior specific to Python
  environments.
- Allowing untrusted workflow authors to escape configured Data Asset roots.

## Current State

The completed `python-workitem` concept supports admitted-source Python
execution with configured or system Python. It intentionally does not create
virtual environments, install packages, or manage dependency caches.

The Data Assets concept already provides cache policies, materialized directory
manifests, `cache_data` work items, compact evidence, and safe path validation
for large execution-environment data. Python environments fit that model better
than a bespoke registry: a `.venv` is a directory asset with provenance,
compatibility constraints, and reuse rules.

LandCore and HPCC workflows now need Python workers that include common runtime
tools such as Python, NumPy, GDAL command-line tools, Python GDAL bindings, and
archive utilities. They also need a repeatable way to select task-specific
Python dependencies without baking every package combination into one image.

## Target State

An enhanced Python worker runs inside a configured worker image or host runtime
with:

```text
python3
venv support
pip or a selected installer
numpy
GDAL command-line tools
Python osgeo.gdal bindings
7z/7za/7zr
basic shell utilities needed by worker scripts
```

The worker receives a configured environment asset materialization root, for
example:

```text
/mnt/scratch/weave151/etl/python-envs
```

That root is mounted read/write for `python-create-env` work and read-only for
ordinary Python execution where the runtime allows separate mount modes. Under
that root, each materialized environment asset lives in a deterministic directory
derived from Data Assets identity, such as:

```text
<python_env_asset_root>/<environment_asset_key>/.venv
```

Ordinary `python_script` work items reference a materialized environment asset
or a resolved environment requirement. Before execution, the worker resolves that
reference through the Data Assets materialized manifest, validates that the
resulting `.venv` is inside the configured environment asset root, and executes
the script with that environment's Python executable.

`python-create-env` work items create the environment directory from an admitted
environment spec, record compact Data Asset evidence, and fail clearly when
dependencies cannot be installed.

Because `.venv` directories are not portable across arbitrary runtimes, the
environment asset identity must include runtime compatibility inputs, not just
the dependency spec:

```text
environment spec hash
python major.minor
platform and architecture
worker image digest or runtime fingerprint
GDAL version when relevant
installer and installer version
package index or wheelhouse identity when relevant
```

## Proposed Slices

- [OS-001 Enhanced Python Worker Runtime Image](OS-001-enhanced-python-worker-runtime-image.md)
- [OS-002 Data Asset Python Environment Selection](OS-002-data-asset-python-environment-selection.md)
- [OS-003 Python Create Environment Work Item](OS-003-python-create-env-work-item.md)

## Open Design Questions

- What is the first supported environment spec format: `requirements.txt`,
  `pyproject.toml`, lock file, or a small GOET-owned JSON spec?
- Should `environment_id` be user-supplied, controller-derived from a spec hash,
  or both?
- Should Python environments use a new Data Assets provider kind, a specialized
  cache-data payload, or the existing materialized directory manifest with
  Python-specific metadata?
- Should `python-create-env` be idempotent when an environment already exists
  and matches the requested spec hash?
- How should environment locking work when multiple workers try to create the
  same environment concurrently on a shared filesystem?
- Should ordinary Python execution require read-only environment mounts where
  the backend supports that mode?
- Which package indexes or offline wheelhouses are allowed in controlled HPCC
  environments?

## Completion Criteria

- An enhanced Python worker image or runtime definition is documented and
  buildable.
- Worker config can declare a Python environment Data Asset materialization root.
- `python_script` work items can select a pre-created `.venv` through
  Data Assets materialization evidence.
- `python-create-env` can create a `.venv` from an admitted environment spec and
  report durable Data Asset evidence.
- Environment paths are validated so work items cannot escape the configured
  environment asset root.
- Local smoke coverage proves create-env followed by Python execution in that
  environment.

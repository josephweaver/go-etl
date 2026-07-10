# Enhanced Python Worker

Status: Proposed

## Purpose

Add a production-oriented Python worker runtime that can run Python work items
with a controlled dependency environment instead of relying only on system
Python.

The immediate target is an enhanced worker image with the usual scientific and
geospatial runtime tools, plus a mounted environment root containing reusable
Python `.venv` directories. A Python work item selects the correct environment
before executing its entrypoint. A new `python-create-env` work item creates or
refreshes those `.venv` directories as controller-managed work.

## Goals

- Provide an enhanced Python worker image with common runtime tools.
- Support a worker-mounted Python environment root that stores named `.venv`
  directories outside individual attempt directories.
- Let each Python work item select one environment by stable environment ID or
  resolved environment reference.
- Add a `python-create-env` work-item type that creates a `.venv` from an
  admitted and validated environment specification.
- Make environment creation explicit workflow work, not an implicit side effect
  of ordinary Python script execution.
- Preserve the current admitted-source boundary: workers use controller-served
  source bundles and do not fetch mutable user code directly.
- Keep controller-owned evidence for environment creation, including the input
  spec hash and resulting environment identity.

## Non-Goals

- Replacing the existing `python_script` work-item type.
- Implementing a full Python package manager abstraction in the first slice.
- Letting arbitrary work items mutate shared environments while another attempt
  may be using them.
- Letting workers install packages from undeclared files or live source trees.
- Solving all container publication and registry automation.
- Making Google Drive, HPCC, Slurm, or SSH behavior specific to Python
  environments.
- Allowing untrusted workflow authors to escape the configured environment root.

## Current State

The completed `python-workitem` concept supports admitted-source Python
execution with configured or system Python. It intentionally does not create
virtual environments, install packages, or manage dependency caches.

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

The worker receives a configured environment root, for example:

```text
/mnt/scratch/weave151/etl/python-envs
```

That root is mounted read/write for environment creation work and read-only for
ordinary Python execution where the runtime allows separate mount modes. Under
that root, each environment lives in a stable directory such as:

```text
<python_env_root>/<environment_id>/.venv
```

Ordinary `python_script` work items reference an environment identity. Before
execution, the worker resolves that identity to a `.venv`, validates that it is
inside the configured environment root, and executes the script with that
environment's Python executable.

`python-create-env` work items create the environment directory from an admitted
environment spec, record evidence, and fail clearly when dependencies cannot be
installed.

## Proposed Slices

- [OS-001 Enhanced Python Worker Runtime Image](OS-001-enhanced-python-worker-runtime-image.md)
- [OS-002 Mounted Python Environment Selection](OS-002-mounted-python-environment-selection.md)
- [OS-003 Python Create Environment Work Item](OS-003-python-create-env-work-item.md)

## Open Design Questions

- What is the first supported environment spec format: `requirements.txt`,
  `pyproject.toml`, lock file, or a small GOET-owned JSON spec?
- Should `environment_id` be user-supplied, controller-derived from a spec hash,
  or both?
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
- Worker config can declare a Python environment root mount.
- `python_script` work items can select a pre-created `.venv` by environment
  identity.
- `python-create-env` can create a `.venv` from an admitted environment spec and
  report durable evidence.
- Environment paths are validated so work items cannot escape the configured
  environment root.
- Local smoke coverage proves create-env followed by Python execution in that
  environment.

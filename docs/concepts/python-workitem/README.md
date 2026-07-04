# Python WorkItem and Staged Source Execution Strategic Concept

Status: Proposed  
Cadence: CSxIx

## Purpose

Add a GOET worker operation that can execute an admitted Python entrypoint from a workflow source manifest.

The completed concept should let a workflow run declare Python source files before admission, let the controller admit and cache those files, let a worker stage only those admitted files, and let the worker execute a Python script through a bounded subprocess contract.

This Strategic Concept is not the future Python SDK. The Python SDK remains a client/interface layer for starting or calling the Go controller and submitting project/workflow files. Python WorkItem is worker-side execution inside the Go runtime.

Naming note: this document uses `GOET` because the current repository and code use that name. If the project/product name moves to `GORC`, this concept should be read as part of GORC's governed orchestration runtime.

## Goals

- Add a `python_script` work-item type.
- Execute Python source only when it was declared before run admission.
- Treat `source_manifest` files as the authority for executable Python source, environment specifications, and support files.
- Keep workers from fetching GitHub, local filesystem repositories, or controller cache paths directly.
- Expose admitted source files to workers through a controller-owned source-bundle boundary.
- Stage admitted source files under an attempt-local worker directory.
- Execute one declared Python entrypoint as a subprocess.
- Pass resolved work-item inputs through a worker-written input JSON document.
- Let the Python script write one structured output JSON document.
- Capture stdout and stderr as logs, not typed outputs.
- Produce `WorkEvidence` compatible with the existing completion path.
- Preserve visible `input_sha256` and `output_sha256` values in completion evidence so current reuse-candidate extraction continues to work.
- Keep environment creation incremental:
  - first use configured/system Python;
  - then define a Python environment spec;
  - then add cached environment creation in a later slice.

## Non-Goals

- Implementing the Python SDK or Python client API.
- Moving workflow compilation into Python.
- Moving scheduling, queue semantics, retry policy, or dependency readiness into Python.
- Letting workers scan imports to discover extra required source files.
- Letting workers fetch GitHub files directly.
- Letting workers reread mutable local source files.
- Letting workflow authors provide controller cache paths.
- Implementing secret propagation.
- Implementing arbitrary package installation in the first Python runner.
- Implementing virtualenv, conda, uv, pip, or lock-file environment creation in the first runner.
- Implementing worker-side skip/reuse for arbitrary Python scripts.
- Implementing dependency-aware workflow scheduling.
- Implementing a general plugin marketplace.
- Implementing execution-observability infrastructure before that Strategic Concept exists.
- Renaming the whole project from GOET to GORC.

## Architectural Context

GOET is evolving into a controller-led orchestration runtime. The controller owns workflow admission, source provenance, queue state, worker assignment, attempt records, and completion/failure decisions. Workers remain relatively simple: they request one work item, validate local execution requirements, perform the assigned work, write output, report evidence, and ask for more work.

The source-control/cache boundary is now a prerequisite for Python execution. Source-reference `/workflow` admission reads project, workflow, and workflow-declared supplemental files through `internal/reposource`, publishes admitted bytes into the controller repository cache, and compiles workflow work from verified cached workflow bytes.

The Python WorkItem concept consumes that admitted source boundary. It should not create a second source-admission path. It should not make the worker a source-control client. It should turn already-admitted source files into an executable worker-side staging directory.

The target product direction still keeps the Python package as an interface layer. Python users should eventually submit workflow/project files through a Python-facing API, but the Go controller remains the orchestration authority.

## Ownership Boundary

### Controller owns

- Source admission.
- Provider selection.
- Repository identity and source revision identity.
- Admitted source manifest construction.
- Repository cache publication.
- Verified cached reads.
- Workflow compilation.
- Work-item source locator generation.
- Work-item queueing.
- Attempt completion/failure interpretation.

### Worker owns

- Receiving one assigned `model.WorkItem`.
- Requesting a controller-provided source bundle for that work item.
- Creating attempt-local staging directories.
- Safely extracting admitted source files.
- Writing `GOET_INPUT_JSON`.
- Running the Python subprocess.
- Capturing stdout/stderr.
- Validating `GOET_OUTPUT_JSON`.
- Promoting canonical output into `DataDir`.
- Reporting completion or failure.

### Python script owns

- Reading `GOET_INPUT_JSON` if it needs structured inputs.
- Performing the user-authored computation.
- Writing exactly one JSON document to `GOET_OUTPUT_JSON`.
- Returning a zero exit code only when the work succeeded.

### Future Python SDK owns

- User-facing project/workflow submission.
- Controller startup/contact behavior.
- Backend selection at the API layer.
- Client-side convenience commands.

The future Python SDK must not own controller queue state, worker orchestration, scheduling policy, source-cache repair, or retry semantics.

## Current State

### Strategic current state

GOET has a working local controller/worker runtime, a source-reference workflow admission path, a repository-source/cache package, and a restart verification path for admitted workflow source.

The system can already treat project and workflow files as admitted source-controlled or local-source inputs. Workflow source documents can declare supplemental files through a top-level `source_manifest` with roles intended for Python execution:

```text
python_entrypoint
python_environment
support_file
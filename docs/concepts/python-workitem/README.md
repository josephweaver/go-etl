# Python WorkItem and Staged Source Execution Strategic Concept

Status: Proposed
Cadence: CSxIx

## Purpose

Add a GOET worker operation that can execute an admitted Python entrypoint from a workflow source manifest.

This concept lets a workflow run declare Python source files before admission, lets the controller admit and cache those files, lets a worker stage only the admitted files, and lets the worker execute a Python script through a bounded subprocess contract.

This is not the future Python SDK. The Python SDK remains a client and interface layer for starting or calling the Go controller and submitting project and workflow files. Python WorkItem is worker-side execution inside the Go runtime.

Naming note: this document uses `GOET` because the repository and code still use that name. If the project name changes later, this concept should be read as part of the same governed orchestration runtime.

## Goals

- Add a `python_script` work-item type.
- Require Python execution only after the controller has admitted the source files.
- Treat `source_manifest` files as the authority for executable Python source, environment specifications, and support files.
- Keep workers from fetching GitHub, local filesystem repositories, or controller cache paths directly.
- Expose admitted source files through a controller-owned source bundle boundary.
- Stage admitted source files under an attempt-local worker directory.
- Execute one declared Python entrypoint as a subprocess.
- Pass resolved work-item inputs through a worker-written input JSON document.
- Let the Python script write one structured output JSON document.
- Capture stdout and stderr as logs, not typed outputs.
- Produce `WorkEvidence` compatible with the existing completion path.
- Preserve visible `input_sha256` and `output_sha256` values in completion evidence so reuse-candidate extraction continues to work.
- Keep environment creation incremental:
  - first use configured or system Python;
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
- Implementing worker-side skip or reuse for arbitrary Python scripts.
- Implementing dependency-aware workflow scheduling.
- Implementing a general plugin marketplace.
- Implementing execution-observability infrastructure before that Strategic Concept exists.
- Renaming the project from GOET to GORC.

## Architectural Context

GOET is evolving into a controller-led orchestration runtime. The controller owns workflow admission, source provenance, queue state, worker assignment, attempt records, and completion and failure decisions. Workers remain relatively simple: they request one work item, validate local execution requirements, perform the assigned work, write output, report evidence, and ask for more work.

The source-control and cache boundary is a prerequisite for Python execution. Source-reference `/workflow` admission already reads project, workflow, and workflow-declared supplemental files through `internal/reposource`, publishes admitted bytes into the controller repository cache, and compiles workflow work from verified cached workflow bytes.

The Python WorkItem concept consumes that admitted source boundary. It must not create a second source-admission path. It must not make the worker a source-control client. It must turn already-admitted source files into an executable worker-side staging directory.

The target product direction still keeps the Python package as an interface layer. Python users should eventually submit workflow and project files through a Python-facing API, but the Go controller remains the orchestration authority.

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
- Attempt completion and failure interpretation.

### Worker owns

- Receiving one assigned `model.WorkItem`.
- Requesting a controller-provided source bundle for that work item.
- Creating attempt-local staging directories.
- Safely extracting admitted source files.
- Writing `GOET_INPUT_JSON`.
- Running the Python subprocess.
- Capturing stdout and stderr.
- Validating `GOET_OUTPUT_JSON`.
- Promoting canonical output into `DataDir`.
- Reporting completion or failure.

### Python script owns

- Reading `GOET_INPUT_JSON` if it needs structured inputs.
- Performing the user-authored computation.
- Writing exactly one JSON document to `GOET_OUTPUT_JSON`.
- Returning a zero exit code only when the work succeeded.

### Future Python SDK owns

- User-facing project and workflow submission.
- Controller startup and contact behavior.
- Backend selection at the API layer.
- Client-side convenience commands.

The future Python SDK must not own controller queue state, worker orchestration, scheduling policy, source-cache repair, or retry semantics.

## Current State

GOET already has a working local controller and worker runtime, a source-reference workflow admission path, a repository-source and cache package, and a restart verification path for admitted workflow source.

The shared work-item model now includes:

- `WorkItemTypePythonScript = "python_script"`
- `WorkItem.Source`
- `WorkItemSource`

`python_script` work items now require source validation in the shared model contract.

The controller already serves a read-only source bundle for an admitted workflow run at `GET /workflow-runs/{run_id}/source-bundle.zip`. That endpoint reads the run's persisted source-admission context, reloads the admitted manifest from the controller-owned cache reference, and returns a zip containing only worker-stageable `source_manifest` files: `python_entrypoint`, `python_environment`, and `support_file`.

That bundle is built from verified repository-cache reads. It does not reread provider source files, and it does not expose controller cache filesystem paths in the HTTP response.

## Target State

The system should support this flow:

1. A workflow run declares Python source files before admission.
2. The controller admits those files and records the manifest.
3. The controller compiles a `python_script` work item with a source locator.
4. The worker receives the work item and asks the controller for the source bundle.
5. The worker stages only the admitted files under the attempt directory.
6. The worker writes `GOET_INPUT_JSON`, runs Python, captures stdout and stderr, and validates `GOET_OUTPUT_JSON`.
7. The worker promotes the output artifact and reports completion evidence.

Later slices may add environment-spec interpretation and cached environment creation, but those belong after the basic admitted-source execution path is stable.

## Notes

- The numbered slice files in this directory hold the implementation details for this concept.
- `planning-note.md` keeps the slice-planning guidance and should remain the place for prompt-by-prompt process notes.
- The worker should never need controller cache paths to execute admitted Python source.

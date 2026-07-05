# Python WorkItem and Staged Source Execution Strategic Concept

Status: Complete
Cadence: CSxIx

## Purpose

Add a GOET worker operation that can execute an admitted Python entrypoint from a workflow source manifest.

This concept lets a workflow run declare Python source files before admission, lets the controller admit and cache those files, lets a worker stage only the admitted files, and lets the worker execute a Python script through a bounded subprocess contract.

This is not the future Python SDK. The Python SDK remains a client and interface layer for starting or calling the Go controller and submitting project and workflow files. Python WorkItem is worker-side execution inside the Go runtime.

Phase 1 is implemented. The concept remains open only for later environment-management, observability, CLI, SDK, dependency, and resource work.

Naming note: this document uses `GOET` because the repository and code still use that name. If the project name changes later, this concept should be read as part of the same governed orchestration runtime.

## Implemented Slices

- `001` - shared `python_script` work-item source contract.
- `002` - controller source-bundle endpoint for admitted source files.
- `003` - worker source-bundle download and safe staging.
- `004` - Python subprocess execution with no environment creation.
- `005` - Python output validation, promotion, and evidence wrapping.
- `006` - workflow compilation validation and controller-generated source locators.
- `007` - sibling demo-project fixture for the Python vertical slice.
- `008` - repeatable local smoke path and runbook.

## Current State

GOET now supports admitted-source Python execution end to end:

- the shared work-item contract includes `python_script` and `WorkItem.Source`;
- the controller validates admitted `source_manifest` roles for Python work items;
- the controller serves a read-only source bundle at `GET /workflow-runs/{run_id}/source-bundle.zip`;
- the worker downloads that bundle and stages it safely under the attempt directory;
- the worker runs a declared Python entrypoint with configured or system Python;
- the worker validates and promotes canonical JSON output;
- the worker returns completion evidence with top-level `input_sha256` and `output_sha256`;
- the sibling demo project proves the source-admission-to-Python-execution path;
- the smoke runbook documents the repeatable local verification path.

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

## Smoke / Runbook

The repeatable local verification path is documented in:

- [`python-workitem-smoke.md`](python-workitem-smoke.md)

## Deferred Work

The following work remains outside this first phase and should be owned by later concepts or later phases:

- Python Environment Management - environment-spec interpretation, creation, caching, and package installation policy.
- Execution Observability - controller-owned log routing, streaming, and retention.
- Submission CLI Status - production CLI ergonomics and queryable submission status.
- Dependency-Aware Workflows - workflow scheduling across upstream and downstream dependencies.
- Resource Constraints - controller-owned admission limits for named resources.
- Python SDK/client behavior - the user-facing package and API layer for starting or calling the controller.

## Notes

- The numbered slice files in this directory hold the implementation details for this concept.
- `planning-note.md` keeps the slice-planning guidance and should remain the place for prompt-by-prompt process notes.
- The worker should never need controller cache paths to execute admitted Python source.



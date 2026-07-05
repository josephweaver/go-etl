# Python WorkItem Concept Closure and Documentation Sync

Status: Complete

## Objective

Close the first phase of the Python WorkItem Strategic Concept after the implementation and smoke slices are complete.

This slice should update documentation so the repository accurately states what Python WorkItem now supports, what remains deliberately deferred, and which future Strategic Concepts should own the deferred work.

## Current State

The Python WorkItem Strategic Concept describes worker-side execution of admitted Python source. The current implementation path includes:

- `python_script` as a shared work-item type;
- `WorkItem.Source` / `WorkItemSource` as the source locator contract;
- strict source validation for queued/raw `python_script` work items;
- a compile-time validation boundary that allows source-less `python_script` items only as intermediate workflow-compiler output;
- controller source-bundle delivery for admitted `source_manifest` files;
- worker source-bundle download and safe attempt-local staging;
- Python subprocess execution using configured or system Python;
- `GOET_INPUT_JSON` and `GOET_OUTPUT_JSON` subprocess boundaries;
- stdout/stderr capture;
- strict JSON output validation;
- canonical output promotion;
- Python evidence wrapping with top-level `input_sha256` and `output_sha256`;
- controller workflow-admission validation of `python_entrypoint` and `python_environment` roles;
- a planned or completed sibling demo-project fixture;
- a planned or completed smoke path.

The Strategic Concept README may still say `Status: Proposed` and may still contain target-state language that describes implemented behavior as future work. `PROJECT_STATE.md` may also need a concise update after the smoke path lands.

The broader future work remains intentionally outside this concept's first phase:

- Python environment specification and creation;
- cached Python environments;
- package installation policy;
- execution-observability integration;
- submission CLI/status ergonomics;
- dependency-aware workflow execution;
- resource constraints;
- Python SDK/client behavior.

## Target State

The Python WorkItem documentation reflects the completed first phase:

- the Strategic Concept README accurately distinguishes implemented behavior from future/deferred work;
- completed slice files are marked consistently;
- deferred work is explicitly assigned to future Strategic Concepts or later phases;
- `PROJECT_STATE.md` states the current Python WorkItem capability without overstating environment management or SDK support;
- concept indexes, if present, classify Python WorkItem as implemented for the admitted-source system-Python vertical slice;
- smoke/runbook notes are linked from the Strategic Concept README;
- no runtime code changes are made.

## Concept Decision

This slice updates the existing Python WorkItem concept. It does not create a new concept and does not implement new runtime behavior.

The closure should use precise status language. If the first phase is complete but future Python environment management is not complete, do not imply that all Python runtime management is done.

Preferred status language:

```text
Status: Complete
```

or:

```text
Status: Complete
```

Avoid status language such as `Complete` if it would hide intentionally deferred environment, observability, CLI, or SDK work.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/python-workitem/planning-note.md`
- `docs/concepts/python-workitem/001-workitem-source-and-python-operation-contract.md`
- `docs/concepts/python-workitem/002-controller-source-bundle-api.md`
- `docs/concepts/python-workitem/003-worker-source-bundle-client-and-staging.md`
- `docs/concepts/python-workitem/004-python-subprocess-runner-no-environment-creation.md`
- `docs/concepts/python-workitem/005-python-output-evidence-contract.md`
- `docs/concepts/python-workitem/006-workflow-compilation-integration-for-python-source-workitems.md`
- `docs/concepts/python-workitem/007-python-demo-project-fixture.md`
- `docs/concepts/python-workitem/008-python-workitem-end-to-end-smoke-path.md`
- `docs/concepts/python-workitem/python-workitem-smoke.md`
- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/complete/execution-observability/README.md`
- `docs/concepts/resource-constraint/README.md`
- `docs/concepts/dependency-aware-workflows/README.md`

Do not read unrelated files unless documentation references or status checks directly require it.

## Allowed Production Files

- `docs/concepts/python-workitem/README.md`
- `docs/concepts/python-workitem/planning-note.md`
- `docs/concepts/python-workitem/001-workitem-source-and-python-operation-contract.md`
- `docs/concepts/python-workitem/002-controller-source-bundle-api.md`
- `docs/concepts/python-workitem/003-worker-source-bundle-client-and-staging.md`
- `docs/concepts/python-workitem/004-python-subprocess-runner-no-environment-creation.md`
- `docs/concepts/python-workitem/005-python-output-evidence-contract.md`
- `docs/concepts/python-workitem/006-workflow-compilation-integration-for-python-source-workitems.md`
- `docs/concepts/python-workitem/007-python-demo-project-fixture.md`
- `docs/concepts/python-workitem/008-python-workitem-end-to-end-smoke-path.md`
- `docs/concepts/python-workitem/python-workitem-smoke.md`
- `docs/concepts/README.md`
- `PROJECT_STATE.md`
- `README.md`

## Allowed Test Files

None.

This is a documentation-only closure slice. If runtime code appears to need changes, stop and report the mismatch instead of implementing code.

## Out Of Scope

- Do not change Go production code.
- Do not change Go tests.
- Do not change worker execution behavior.
- Do not change controller admission behavior.
- Do not add Python environment creation.
- Do not add virtualenv, conda, uv, pip, package install, or environment caching.
- Do not add execution-observability infrastructure.
- Do not add a submission CLI or status command.
- Do not add Python SDK/client behavior.
- Do not add dependency-aware workflow scheduling.
- Do not add resource constraints.
- Do not rename GOET to GORC in code or docs as part of this slice.
- Do not rewrite unrelated concept documents except to add minimal cross-links.
- Do not claim future/deferred features are implemented.

## Acceptance Criteria

- `docs/concepts/python-workitem/README.md` states the implemented first-phase capability in current-state language.
- The README no longer describes already-implemented 001 through 008 behavior only as future target state.
- The README lists implemented slices and their outcomes.
- The README links to the smoke/runbook document when present.
- The README has an explicit `Deferred Work` or equivalent section.
- Deferred work is assigned to future concepts or later phases, including:
  - Python Environment Management;
  - Execution Observability;
  - Submission CLI Status;
  - Dependency-Aware Workflows;
  - Resource Constraints;
  - Python SDK/client behavior.
- Slice files 001 through 008 have consistent status notes if this repository's convention is to update slice status after completion.
- `PROJECT_STATE.md` accurately summarizes the completed Python WorkItem vertical slice.
- `PROJECT_STATE.md` does not claim Python environment creation, package installation, log streaming, CLI status, or SDK support exists.
- `docs/concepts/README.md` is updated if it indexes concept status.
- `README.md` is updated only if there is already a concise project-feature section that should mention Python WorkItem.
- No runtime code is modified.
- The final report names any documentation files intentionally left unchanged.

## Notes

- This slice should close the first Python WorkItem phase, not extend it.
- Use conservative status language if 008's smoke path is manual rather than fully automated.
- If `docs/concepts/README.md` does not exist or does not track concept status, do not create a broad index just for this slice.
- If `README.md` does not already describe runtime capabilities, do not add a marketing section here.
- The Strategic Concept may remain open for later environment work only if the README clearly marks the admitted-source system-Python path as implemented and moves environment work to a later phase.
- Future Codex work after this closure should probably move to the Submission CLI Status Strategic Concept or the Execution Observability Strategic Concept rather than adding more Python WorkItem slices.



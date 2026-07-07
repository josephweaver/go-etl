# 007 Python Demo Project Fixture

Status: Complete

## Objective

Add a minimal Python workflow fixture to the sibling `go-etl-demo-project` repository to prove the source-admission-to-Python-execution vertical slice.

This slice creates client-facing demo project files. It should not change GOET production code unless a missing test hook is discovered and approved before implementation.

## Current State

`PROJECT_STATE.md` says client-facing demo project artifacts now live in the sibling repository:

```text
../go-etl-demo-project
```

That repository is intended to hold source-control-style customer files such as:

```text
project.json
workflows/
submissions/
data/
```

Completion note: the sibling `go-etl-demo-project` now contains the minimal `python-hello` fixture described below.

After slices 001 through 006, GOET should be able to admit source-reference workflows that declare Python source files, compile `python_script` work items with source locators, let workers stage admitted source bundles, execute Python, and report evidence.

## Target State

The sibling demo project contains a minimal Python workflow example.

Suggested fixture layout:

```text
../go-etl-demo-project/
  project.json
  workflows/
    python-hello.json
  submissions/
    python-hello-local.json
  scripts/
    hello.py
  environments/
    system-python.json
  data/
    input.json
```

The workflow declares a top-level `source_manifest` containing:

```text
scripts/hello.py                  role python_entrypoint
environments/system-python.json   role python_environment
```

The workflow contains at least one `python_script` work item that:

- points `python_entrypoint` to `scripts/hello.py`;
- optionally points `python_environment` to `environments/system-python.json`;
- writes a small JSON result;
- uses only standard-library Python.

The submission JSON targets local filesystem source admission so the demo can run without GitHub credentials.

## Concept Decision

This slice adds a demo-project fixture concept, not a GOET runtime concept.

The customer-facing demo repository owns these files because they represent source documents a client or researcher would submit, not internal worker fixtures.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/source-control-resolution-and-cache/README.md`
- `../go-etl-demo-project/README.md`
- `cmd/controller/controller-default-config.json`
- `cmd/worker/worker-default-config.json`

Do not read unrelated scheduler, transport, SSH, Docker, or low-level worker fixture files unless the demo command or test failures directly require it.

## Allowed Production Files

None.

## Allowed Test Files

None.

## Allowed Fixture Files

- `../go-etl-demo-project/README.md`
- `../go-etl-demo-project/project.json`
- `../go-etl-demo-project/workflows/python-hello.json`
- `../go-etl-demo-project/submissions/python-hello-local.json`
- `../go-etl-demo-project/scripts/hello.py`
- `../go-etl-demo-project/environments/system-python.json`
- `../go-etl-demo-project/data/input.json`

## Allowed Documentation Files

- `PROJECT_STATE.md`

## Out Of Scope

- GOET production code changes.
- GOET test code changes.
- Python SDK/client implementation.
- Python package installation.
- Virtualenv or conda creation.
- Dependency-aware workflows.
- Resource constraints.
- Secret propagation.
- GitHub-backed demo submission unless local submission already works.

## Acceptance Criteria

- Demo project has a minimal `project.json` suitable for local source admission.
- Demo project has a workflow file under `workflows/`.
- Workflow file declares `source_manifest` entries for the Python entrypoint and environment spec.
- Workflow file has a `python_script` work item using the declared entrypoint.
- Python script uses only the standard library.
- Python script reads `GOET_INPUT_JSON` when present or tolerates its absence.
- Python script writes exactly one JSON document to `GOET_OUTPUT_JSON`.
- Environment file is valid JSON and represents system-Python behavior or a placeholder compatible with the current runner.
- Submission file points to the demo workflow using local source-reference admission.
- README documents the demo structure and the intended command or manual steps.

## Notes

- This slice may be run after slices 001 through 006, because the fixture depends on the runtime accepting `python_script`.
- If the implementation cannot prove execution end-to-end because local controller/worker orchestration needs manual setup, the report should state the exact manual command sequence and which parts were tested.
- Keep the fixture intentionally small. It should prove the Python WorkItem path, not demonstrate a realistic research pipeline yet.
- Do not include secrets, credentials, hostnames, usernames, or private paths in the demo project.


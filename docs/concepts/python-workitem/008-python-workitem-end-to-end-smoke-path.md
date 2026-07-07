# Python WorkItem End-to-End Smoke Path

Status: Complete

## Objective

Add a repeatable local smoke path that proves the Python WorkItem vertical slice from the sibling demo project through controller admission, worker staging, Python execution, and completed output.

The smoke path should let a developer run the Python fixture added by Operational Slice 007 and verify that:

- a source-reference workflow submission is accepted;
- the controller admits and caches the declared Python source files;
- the controller compiles a `python_script` work item with a controller-generated `WorkItem.Source` locator;
- the worker downloads the admitted source bundle;
- the worker stages the bundle under its attempt directory;
- the worker runs the declared Python entrypoint;
- the worker writes output under its configured `DataDir`;
- stdout and stderr are captured under the attempt log directory;
- the controller reaches an idle state after the run.

This slice is a smoke/runbook slice. It should not add new runtime behavior unless the smoke path exposes a small missing command-line or script hook that is required to run the already-implemented feature.

## Current State

The Python WorkItem Strategic Concept now has implementation slices for:

- the `python_script` work-item type and source locator model;
- the controller source-bundle endpoint;
- worker source-bundle download and safe staging;
- worker-side Python subprocess execution;
- Python output validation, canonical promotion, and evidence wrapping;
- controller workflow-admission validation for `python_script` work items;
- a planned sibling demo-project fixture.

`PROJECT_STATE.md` describes the controller endpoint:

```text
GET /workflow-runs/{run_id}/source-bundle.zip
```

and the worker path that downloads the bundle and runs `python_script` through a subprocess. The sibling repository `../go-etl-demo-project` owns client-facing project, workflow, submission, script, environment, and data fixtures.

The repeatable smoke path now exists as `scripts/python-workitem-smoke.ps1` with the companion runbook `docs/concepts/python-workitem/python-workitem-smoke.md`.

## Target State

The repository contains a small smoke script and runbook that can be followed from the GOET repository root with the sibling demo project checked out next to it.

The smoke path should:

1. Validate the expected demo-project fixture files exist.
2. Validate the demo JSON files are syntactically valid.
3. Validate the Python script compiles when `python3` is available.
4. Start the controller using the local demo controller configuration.
5. Wait for the controller status endpoint to become available.
6. Submit the demo project's Python workflow submission to `POST /workflow`.
7. Start a local worker using the local worker configuration.
8. Wait until the controller reports no pending or assigned work.
9. Verify the expected worker output file exists in the configured `DataDir`.
10. Verify the output file contains valid JSON and expected deterministic content from the demo Python script.
11. Verify stdout/stderr log files exist under the worker attempt staging tree when practical.
12. Shut down the controller cleanly through the existing shutdown endpoint.

The smoke path is automated through the controller's existing local worker-launch settings, so no new runtime behavior or submission CLI is required.

## Concept Decision

This slice updates the existing Python WorkItem concept. It does not create a new runtime concept.

The smoke script/runbook belongs near developer scripts and concept documentation because it verifies a cross-package vertical slice without adding new controller, worker, source-cache, or workflow-compilation behavior.

The first implementation should prefer a simple PowerShell script because the current development environment and recent Codex paths are Windows-oriented. A Bash script may be added only if it is straightforward and does not increase the slice scope.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/python-workitem/007-python-demo-project-fixture.md`
- `cmd/controller/demo-config.json`
- `cmd/worker/demo-config.json`
- `cmd/controller/main.go`
- `cmd/worker/main.go`
- `../go-etl-demo-project/README.md`
- `../go-etl-demo-project/project.json`
- `../go-etl-demo-project/workflows/python-hello.json`
- `../go-etl-demo-project/submissions/python-hello-local.json`
- `../go-etl-demo-project/scripts/hello.py`
- `../go-etl-demo-project/environments/system-python.json`

Do not read unrelated files unless smoke-path implementation or validation failures directly require it.

## Allowed Production Files

- `scripts/python-workitem-smoke.ps1`
- `scripts/python-workitem-smoke.sh`
- `docs/concepts/python-workitem/python-workitem-smoke.md`
- `docs/concepts/python-workitem/008-python-workitem-end-to-end-smoke-path.md`
- `PROJECT_STATE.md`

## Allowed Test Files

None.

This slice is a smoke/runbook slice. It should validate behavior by running commands, not by adding Go unit tests.

## Out Of Scope

- Do not change controller production code.
- Do not change worker production code.
- Do not change `internal/model`.
- Do not change `internal/workflow`.
- Do not change `internal/reposource`.
- Do not change the source-bundle endpoint.
- Do not change Python evidence wrapping.
- Do not add Python environment creation.
- Do not add virtualenv, conda, uv, pip, package install, or dependency caching.
- Do not add dependency-aware workflow scheduling.
- Do not add a Python SDK or client API.
- Do not add a general submission CLI.
- Do not add long-running log streaming or execution-observability infrastructure.
- Do not rewrite the demo project fixture created by Operational Slice 007 unless the smoke path proves a fixture bug.
- Do not introduce private paths, hostnames, usernames, credentials, or secrets.

## Acceptance Criteria

- A developer-facing smoke script and runbook exist.
- The smoke path names the required sibling repository layout.
- The smoke path validates the Python fixture JSON files.
- The smoke path validates `scripts/hello.py` with `python3 -m py_compile` when `python3` is available.
- The smoke path starts the controller with the existing local demo controller configuration or documents the exact command to do so.
- The smoke path submits the Python demo workflow to `POST /workflow` or documents the exact HTTP request.
- The smoke path starts a local worker with the existing worker configuration or documents the exact command to do so.
- The smoke path waits for the controller to become idle or documents how to check idle state with `GET /status`.
- The smoke path verifies the expected output JSON file in the worker `DataDir`.
- The smoke path verifies the expected deterministic fields written by the demo Python script.
- The smoke path captures or points to controller and worker logs for debugging failed smoke runs.
- The smoke path shuts down the controller cleanly when automation is possible.
- If full automation is not possible, the runbook states the missing blocker in concrete terms.
- `PROJECT_STATE.md` is updated to record the smoke path.
- No runtime behavior is changed by this slice.

## Notes

- Keep this slice boring and practical. Its job is to prove the feature can be used, not to improve user ergonomics.
- The smoke path should use local source-reference admission against `../go-etl-demo-project`.
- The worker output file path depends on `cmd/worker/demo-config.json` and the process working directory. The script or runbook must make the working directory explicit.
- If `python3` is not available, skip only Python compile/execution validation and report that clearly.
- If the controller or worker command-line shape differs from the assumptions above, follow the code and document the exact commands that work.
- If a future workflow needs broader submission ergonomics, record that as a reason to advance the Submission CLI Status Strategic Concept.


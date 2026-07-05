# 002 Controller Source Bundle API

Status: Complete

## Objective

Add a read-only controller endpoint that returns the admitted source files for a workflow run as a safe source bundle for worker staging.

The endpoint packages files from the controller-owned repository cache using the run's admitted source manifest. It does not make the worker read repository cache paths, does not reread source providers, and does not execute Python.

## Current State

`internal/reposource` already defines admitted source manifests, file roles, cache publication, verified cache reads, and materialization from verified cache reads.

Source-reference `/workflow` admission now uses `internal/reposource` to read project, workflow, and workflow-declared supplemental files, publish those admitted files into the repository cache, and compile workflow work from verified cached workflow bytes.

Controller startup recovery now verifies active run source caches before normal admission. GitHub-backed cache misses or corruptions are repaired from the recorded immutable revision and admitted source paths. Local-backed cache misses or corruptions fail recovery without rereading mutable local source files.

The controller does not yet expose a worker-facing source-bundle endpoint.

## Target State

The controller exposes a route equivalent to:

```text
GET /workflow-runs/{run_id}/source-bundle.zip
```

If the existing route style suggests a better local convention, the implementation may use that convention, but the route must be clearly named and tested.

For a valid run with admitted source context, the endpoint returns a zip file containing only admitted source-manifest files needed for worker staging. The zip preserves safe repository-relative paths.

The endpoint reads through `internal/reposource` cache access or materialization logic. It does not read provider source directly. It does not reread mutable local files. It does not expose repository cache filesystem paths.

The endpoint returns clear HTTP errors for:

- missing run;
- missing source-admission context;
- missing or unreadable admitted manifest;
- missing cached file;
- corrupted cached file;
- unsafe admitted file path;
- zip construction failure.

## Concept Decision

This slice adds a controller-owned source-bundle concept because packaging admitted cached source files for workers is separate from workflow admission and separate from worker extraction.

Prefer a new controller file such as:

```text
cmd/controller/source_bundle.go
```

if the endpoint requires more than a small handler wrapper. Keep route registration near existing controller route setup in `cmd/controller/main.go`.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `docs/concepts/source-control-resolution-and-cache/README.md`
- `internal/reposource/model.go`
- `internal/reposource/cache_access.go`
- `internal/reposource/cache_verify.go`
- `internal/reposource/materialize.go`
- `internal/persistence/store.go`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`

Do not read worker, scheduler, transport, SSH, Docker, or client setup files unless compile or test failures directly require it.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/source_bundle.go`
- `internal/reposource/materialize.go`
- `internal/reposource/cache_access.go`
- `internal/persistence/store.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/source_bundle_test.go`
- `internal/reposource/materialize_test.go`
- `internal/reposource/cache_access_test.go`
- `internal/persistence/store_test.go`

## Allowed Documentation Files

- `PROJECT_STATE.md`

## Out Of Scope

- Worker source-bundle download.
- Worker safe extraction.
- Python subprocess execution.
- Python output/evidence contract.
- Workflow compiler changes for `python_script`.
- Environment creation.
- Source admission redesign.
- Provider rereads for local filesystem sources.
- Adding providers beyond GitHub and local filesystem.
- Retention cleanup.
- Authentication/authorization redesign.

## Acceptance Criteria

- A controller source-bundle endpoint exists and is covered by tests.
- The endpoint can locate the admitted source manifest for a run using durable controller state.
- The endpoint reads admitted files from verified repository cache content.
- The endpoint does not read GitHub or local filesystem provider source directly.
- The endpoint does not expose repository cache paths in the response body.
- The zip contains only admitted source-manifest files selected by the admitted manifest.
- Zip entries use safe slash-separated relative paths.
- Unsafe paths are rejected before writing zip entries.
- Tests cover successful bundle creation.
- Tests cover missing run or missing source context.
- Tests cover missing or corrupt cached content if the existing test harness can construct those states locally.
- `go test ./cmd/controller` passes.

## Notes

- The source-bundle endpoint is a worker packaging boundary, not a client source-inspection API.
- If the controller already has a helper for loading an admitted source manifest from run context, reuse it rather than duplicating persistence parsing.
- If there is no clean persistence API for admitted source context, add the smallest necessary query/helper inside the allowed persistence files and test it narrowly.
- The route name should be recorded in `PROJECT_STATE.md` if this slice lands.
- Do not add `python_script` workflow compilation in this slice.


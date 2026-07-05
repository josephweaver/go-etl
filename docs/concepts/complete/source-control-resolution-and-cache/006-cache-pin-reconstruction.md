# 006 Cache Pin Reconstruction

Status: Complete

## Objective

Reconstruct operational repository cache pin files for active workflow runs from
durable workflow execution state after controller restart.

This slice makes cache pins rebuildable state. The workflow execution database
remains authoritative for which runs are active and which source files they
require; pin files only protect cache entries from future cleanup.

## Current State

After Operational Slice 004, `internal/reposource` published admitted source
manifests into the repository cache and read cached files with verification.

After Operational Slice 005, `internal/reposource` materialized admitted
manifests into local staging directories.

The Strategic Concept defines cache pin files as operational state under cache
entries. `internal/reposource` can now write deterministic workflow-run cache
pin files from admitted manifests and reconstruct pins from admitted-manifest
paths. Controller startup still does not call this behavior.

The current persistence package already has recovery-oriented methods and
records:

- `Store.ListActiveWorkflowRuns`;
- `Store.GetProject`;
- `Store.GetWorkflow`;
- `ProjectRecord`;
- `WorkflowRecord`;
- `WorkflowRunRecord`.

Those records still use `SourceCommit`/`source_commit` vocabulary. This slice
does not rename persistence schema fields. Future controller wiring should map
the current durable field to the repository-source `RevisionID` concept when
constructing pin inputs.

## Target State

`internal/reposource` can write deterministic cache pin files for active runs
based on admitted source manifests or equivalent durable run source facts.

A reconstructed pin records:

- pin schema;
- pin ID derived from the workflow run ID;
- reason `workflow_run`;
- workflow run ID;
- source identity;
- source revision identity, when present;
- manifest path;
- pinned file cache paths.

Pin files are written to the cache entry for the admitted source. If a pin file
already exists with the same content, reconstruction is idempotent.

The controller-facing reconstruction flow is documented but not wired into live
startup in this slice:

```text
ListActiveWorkflowRuns -> load project/workflow source facts -> locate admitted manifest -> write pin file
```

## Concept Decision

This slice updates the repository cache concept by adding reconstructable
operational pin state.

Pin files should be owned by `internal/reposource` because they refer to cache
layout paths and admitted source manifests. The workflow execution database
remains the authority for active run state. Pin files must not become a second
database for run liveness or source provenance.

## Required Context

Read these files first:

- `docs/concepts/complete/source-control-resolution-and-cache/README.md`
- `docs/concepts/complete/source-control-resolution-and-cache/003-repository-cache-access-layer-and-layout.md`
- `docs/concepts/complete/source-control-resolution-and-cache/004-cached-admission-and-verified-reads.md`
- `internal/reposource/model.go`
- `internal/reposource/cache_layout.go`
- `internal/reposource/cache_access.go`
- `internal/persistence/store.go`

Do not read unrelated controller files unless compile or test failures directly
require it.

## Allowed Production Files

- `internal/reposource/cache_pin.go`
- `internal/reposource/cache_pin_reconstruction.go`

This slice needs new production files. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/reposource/cache_pin_test.go`
- `internal/reposource/cache_pin_reconstruction_test.go`

## Out Of Scope

- Wiring reconstruction into controller startup.
- Changing `internal/persistence` schemas or renaming `source_commit`.
- Adding new persistence store methods.
- Deciding retention cleanup policy.
- Deleting unpinned cache files.
- Verifying cached file contents.
- Fetching missing files from GitHub or local filesystem providers.
- Reimporting corrupt GitHub cache entries.
- Materializing files into worker staging directories.
- Supporting multi-source or multi-repository admitted manifests.

## Acceptance Criteria

- `internal/reposource` defines a cache pin model with schema
  `goet/source-cache-pin/v1`.
- Pin IDs are deterministic for workflow runs, recommended `run-<run-id>`.
- Pin files record reason `workflow_run`, workflow run ID, source identity,
  source revision identity when present, manifest path, and pinned file
  `cache_path` values.
- Pin path derivation uses the cache layout from OS 003.
- Local filesystem admissions can derive a pin path under
  `<cache-root>/local/runs/<run-id>/pins/<pin-id>.json` or the equivalent
  OS 003 local cache entry.
- GitHub admissions can derive a pin path under
  `<cache-root>/github/repos/<repository-key>/<commit-id>/pins/<pin-id>.json`.
- Reconstructing the same pin twice is idempotent.
- Reconstructing a pin writes through a temporary file and promotes it to the
  final pin path.
- Pin reconstruction treats durable workflow execution state as authoritative
  and does not infer active runs from existing pin files.
- Pin reconstruction fails clearly when the admitted manifest required for an
  active run is missing.
- Tests cover local pin path/content, GitHub pin path/content, idempotent
  rewrite, missing manifest behavior, and pin content for multiple manifest
  files.
- No controller startup, persistence schema, provider fetch, cache verification,
  materialization, or retention behavior changes.

## Notes

- This slice should not create a retention cleanup worker. It only creates the
  operational pin files that a later cleanup slice can honor.
- Pin files are reconstructable cache state. They are useful for cleanup and
  inspection, but they must not be treated as the source of truth for active
  runs.
- If current persistence records expose `SourceCommit`, treat that as the
  current storage name for source revision identity. Do not broaden this slice
  into a database migration.
- If OS 003's local layout does not yet include a `pins/` directory, this slice
  may add pin path derivation to the cache layout without changing file content
  publication behavior.

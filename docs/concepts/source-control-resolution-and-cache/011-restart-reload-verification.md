# 011 Restart Reload Source Verification

Status: implemented

## Objective

Verify cached project and workflow source documents when the controller reloads
active workflow runs after restart, and repair only GitHub-backed cache entries
when the recorded source revision can be reimported.

## Current State

Before this slice, the controller persisted project, workflow, and workflow-run
records with enough source metadata to identify the submitted project and
workflow documents. OS 009 defines the persistence contract for nullable source
revision identity and durable source-admission context.

The controller also stored canonical JSON SHA-256 evidence for project and
workflow documents after loading them. This slice adds the reload-time
comparison between persisted canonical hashes and cached bytes.

OS 010 admits source-reference workflow submissions into the repository cache
and persists the workflow run after reading project and workflow JSON back from
verified cache. OS 011 verifies that admitted state during startup recovery.

## Target State

On controller restart, active workflow runs are reloaded only after the
controller verifies that the cached project and workflow JSON still match the
canonical JSON SHA-256 values stored in durable project and workflow records.

If cached GitHub-backed source files are missing or fail verification, the
controller reimports the exact recorded GitHub source revision and
manifest-declared source paths, republishes the cache entry, and verifies the
cache again before treating the run as reloadable.

If cached local-backed source files are missing or fail verification, the
controller reports a clear reload error for that run. Local sources have null
source revision identity and are not a provenance technique, so the controller
must not silently reread arbitrary local files during restart recovery.

## Concept Decision

Reload verification is based on the GOET canonical JSON SHA-256 of the project
and workflow documents, because that is the representation the controller uses
after parsing and loading those documents.

GitHub repair is allowed only against the recorded immutable source revision
identity and the already admitted manifest-declared paths. It must not expand
the source manifest, discover worker files at runtime, or resolve a moving
branch during repair.

Local repair is intentionally unsupported. The reload error should point to the
run ID and source identity and explain that local source files do not provide
source-control authenticity.

## Required Context

Before implementing this slice, read:

- `docs/concepts/source-control-resolution-and-cache/README.md`
- `docs/concepts/source-control-resolution-and-cache/002-provider-reads-and-admission-manifest.md`
- `docs/concepts/source-control-resolution-and-cache/004-cached-admission-and-verified-reads.md`
- `docs/concepts/source-control-resolution-and-cache/009-persistence-source-revision-and-admission-context.md`
- `docs/concepts/source-control-resolution-and-cache/010-controller-admission-integration.md`
- Existing controller persistence methods for active workflow runs, projects,
  and workflows.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/source_reload.go` (new file)
- `cmd/controller/source_control.go`
- `internal/reposource/cache_access.go`
- `internal/reposource/cache_publish.go`
- `internal/reposource/cache_verify.go`
- `internal/reposource/provider.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/source_reload_test.go` (new file)
- `internal/reposource/cache_verify_test.go`
- `internal/reposource/provider_test.go`

## Out Of Scope

- Worker restart behavior.
- Python executor materialization.
- Runtime discovery of supplemental files.
- Changing persistence schema field names or nullability.
- Source cache retention cleanup.
- Cache pin reconstruction from durable state.
- Multi-source workflow manifests.
- Compatibility aliases for old `controller_git_cache_*` config names.
- Repairing local source cache entries by rereading local filesystem paths.

## Acceptance Criteria

- Controller restart reload enumerates active workflow runs from durable state.
- For each active run, the controller loads the persisted project and workflow
  records.
- The reload path locates the admitted source manifest and cached
  project/workflow file entries for the run from the OS 009 source-admission
  context.
- Cached project and workflow bytes are read through the verified cache reader,
  not directly from caller-supplied paths.
- The reload path recomputes GOET canonical JSON SHA-256 for the cached project
  and workflow documents and compares them with the durable project/workflow
  hashes.
- If the cache exists and both canonical hashes match, the run is considered
  source-verified without contacting the provider.
- If a GitHub-backed cache entry is missing or fails verification, the
  controller reimports the exact recorded GitHub source revision and admitted
  manifest-declared paths, republishes the cache, and verifies the
  project/workflow canonical hashes again.
- If GitHub repair cannot read the recorded revision or the repaired cache still
  mismatches, the run reload fails with a clear cache/provenance error.
- If a local-backed cache entry is missing or fails verification, the run reload
  fails with a clear error and does not reread local source paths.
- Tests cover verified cache hit, GitHub missing-cache repair, GitHub
  corrupted-cache repair, GitHub repair failure, local missing-cache failure,
  and local corrupted-cache failure.

## Notes

- This slice depends on OS 009 for nullable source revision identity and the
  durable source-admission context shape.
- The repair path should avoid resolving mutable GitHub refs during restart. It
  should use the resolved commit ID captured at admission time.
- A reload failure for one run should be reported precisely enough to identify
  the affected `run_id`, source provider, source identity, and failed document
  role.

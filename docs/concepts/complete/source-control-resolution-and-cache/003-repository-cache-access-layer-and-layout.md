# 003 Repository Cache Access Layer and Layout

Status: Complete

## Objective

Define the repository cache access layer and deterministic on-disk layout for
admitted source manifests and file contents.

This slice establishes how code derives internal cache paths for local
filesystem admissions and GitHub admissions. It does not publish provider-read
bytes into the cache, verify cached file contents, reconstruct pins, or
materialize files into worker staging directories.

## Current State

After Operational Slice 001, `internal/reposource` defined the
repository-source model and path validation helpers.

After Operational Slice 002, `internal/reposource` defined provider reads and
admitted source manifest construction, but provider reads remain independent
from cache layout.

`internal/reposource` now has a repository cache layout and access layer. The
controller startup code now uses `controller_repo_cache_path`, and existing
source-reference behavior in `cmd/controller/source_control.go` reads local
files directly. The new shared code maps:

```text
cache root + admitted source manifest + manifest file path -> internal cache path
```

The new code also defines the GitHub repository-key sanitizer and
provider-content directory rule for GitHub commit-backed cache entries. It does
not create directories, write cache files, verify cache files, or change
controller admission.

## Target State

`internal/reposource` has cache layout code that can derive, validate, and
describe cache locations without reading or writing provider file bytes.

The cache access layer supports these physical layouts:

```text
<cache-root>/
  local/
    runs/
      <run-id>/
        files/
          <cache-path>
        manifest.json
```

```text
<cache-root>/
  github/
    repos/
      <repository-key>/
        <content-key>/
          files/
            <cache-path>
          manifests/
            <run-id>.json
        locks/
        tmp/
```

For GitHub, `<content-key>` is the resolved immutable commit ID. The GitHub
content directory is a revision cache entry, not a full repository checkout.
Run-specific admitted source manifests remain authoritative about which
requested subset of files each admitted run requires.

The access layer exposes a logical lookup contract:

```text
run_id + admitted manifest file cache_path -> local file path under cache root
```

Callers do not build cache paths by string concatenation outside the access
layer.

## Concept Decision

This slice updates the repository-source concept by adding the repository cache
layout concept.

The layout code should live in `internal/reposource` because it consumes the
admitted source manifest and path validation types from OS 001 and OS 002. It
should be separate from provider reads so GitHub and local filesystem providers
do not need to know where bytes are stored after admission.

## Required Context

Read these files first:

- `docs/concepts/complete/source-control-resolution-and-cache/README.md`
- `docs/concepts/complete/source-control-resolution-and-cache/001-repository-source-model-and-path-safety.md`
- `docs/concepts/complete/source-control-resolution-and-cache/002-provider-reads-and-admission-manifest.md`
- `internal/reposource/model.go`
- `internal/reposource/path.go`
- `internal/reposource/manifest.go`
- `cmd/controller/main.go`
- `cmd/controller/config_test.go`

Do not read unrelated controller files unless compile or test failures directly
require it.

## Allowed Production Files

- `internal/reposource/cache_layout.go`
- `internal/reposource/cache_access.go`

This slice needs new production files. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/reposource/cache_layout_test.go`
- `internal/reposource/cache_access_test.go`

## Out Of Scope

- Renaming controller startup config fields from `controller_git_cache_*` to
  `controller_repo_cache_*`.
- Changing `cmd/controller/main.go` startup behavior.
- Changing controller defaults, config fixtures, or client submission format.
- Publishing admitted provider bytes into the cache.
- Reading file contents from the cache.
- Verifying cached file hashes.
- Reimporting corrupt GitHub cache entries.
- Creating, deleting, or reconstructing pin files.
- Retention cleanup.
- Materializing files into worker staging directories.
- Supporting multi-source or multi-repository admitted manifests.
- Whole-repository checkout, clone, recursive copy, or Git object database
  layout.

## Acceptance Criteria

- The cache layout code accepts a configured cache root and rejects an empty
  cache root.
- The cache layout code treats the cache root as controller-owned internal
  storage and never accepts client-supplied absolute cache paths.
- Local filesystem manifests map to
  `<cache-root>/local/runs/<run-id>/files/<cache-path>`.
- GitHub manifests map to
  `<cache-root>/github/repos/<repository-key>/<content-key>/files/<cache-path>`.
- GitHub `<content-key>` is the resolved immutable commit ID from the admitted
  source manifest.
- Repository keys are deterministic, contain provider namespace, contain no
  credentials, and use only safe filename characters.
- The recommended GitHub repository key shape is
  `github.com_<owner>_<repo>`.
- Cache path resolution rejects empty paths, `.`, absolute paths,
  drive-qualified paths, backslash paths, and any path containing an original
  `..` segment.
- Resolved file paths are proven to remain under the provider's `files/` root.
- Repository-relative directory structure is preserved under `files/`.
- The layout exposes local manifest paths and GitHub run-specific manifest
  paths.
- The layout exposes per-repository `locks/` and `tmp/` paths for GitHub cache
  entries, without implementing locking or temp publish behavior.
- Tests prove local and GitHub path derivation, repository-key sanitization,
  unsafe path rejection, and no whole-repository checkout assumptions.
- No provider read, controller admission, cache publication, persistence,
  retention, pinning, or materialization behavior changes.

## Notes

- This slice is about path derivation and layout contracts only. OS 004 owns
  writing admitted files, reading cached files, and verifying hashes.
- Keep all returned paths under the configured cache root.
- Use the path validator from OS 001 for manifest `cache_path` values.
- Do not use branch or tag names in GitHub cache entry paths. Use the resolved
  immutable commit ID.
- Do not deduplicate project or workflow documents in the filesystem layout.
  Semantic deduplication belongs to the persistence tables.
- It is acceptable for this slice to expose simple structs such as
  `CacheLayout`, `CacheEntry`, or `CachePaths`; avoid a broad cache manager.

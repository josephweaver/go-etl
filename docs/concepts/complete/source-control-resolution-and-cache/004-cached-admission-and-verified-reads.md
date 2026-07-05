# 004 Cached Admission and Verified Reads

Status: Complete

## Objective

Publish admitted source files into the repository cache and read them back
through the cache access layer with verification against the admitted source
manifest.

This slice turns the layout from OS 003 into usable cache behavior. It writes
only manifest-declared files, preserves repository-relative directory structure
under `files/`, and verifies cached bytes before returning them. It does not
fetch from GitHub or local filesystem sources itself; provider reads remain
owned by OS 002.

## Current State

After Operational Slice 001, `internal/reposource` defined the
repository-source model and path validation helpers.

After Operational Slice 002, `internal/reposource` defined provider reads and
admitted source manifest construction.

After Operational Slice 003, `internal/reposource` defined cache layout and
path derivation for local filesystem and GitHub cache entries.

`internal/reposource` now has a cache publisher and verified cache reader.
Admitted source manifests can be written to disk, manifest-declared file bytes
can be copied into the repository cache, and cached bytes are checked against
raw file SHA-256, GOET canonical JSON SHA-256, and recorded byte size before
use.

## Target State

`internal/reposource` can publish an admitted manifest and the corresponding
file bytes into the repository cache.

For local filesystem admissions, the cache publisher writes:

```text
<cache-root>/local/runs/<run-id>/
  files/
    <cache-path>
  manifest.json
```

For GitHub admissions, the cache publisher writes:

```text
<cache-root>/github/repos/<repository-key>/<commit-id>/
  files/
    <cache-path>
  manifests/
    <run-id>.json
```

GitHub commit cache entries are shared by repository and immutable commit ID,
but they are not full checkouts. They may accumulate additional
manifest-declared files for the same commit over time when later admitted
manifests declare those files. Writes are append-only by validated `cache_path`:
an existing file with matching verification evidence is reused, and an existing
file with mismatched evidence is treated as cache corruption.

The cache reader can load a cached file by admitted manifest and `cache_path`,
verify the file, and return local bytes. Verification includes:

- raw file-byte SHA-256 when `raw_sha256` is present;
- GOET canonical JSON SHA-256 when `canonical_json_sha256` is present;
- size in bytes when `size_bytes` is present in the manifest model.

## Concept Decision

This slice updates the repository cache concept by adding cache publication and
verified reads.

The behavior should live in `internal/reposource` because it consumes the
admitted source manifest, file content, path validation, and cache layout types
from earlier slices. It should remain independent from controller admission and
provider fetching so later slices can decide when the controller starts using
the cache.

## Required Context

Read these files first:

- `docs/concepts/complete/source-control-resolution-and-cache/README.md`
- `docs/concepts/complete/source-control-resolution-and-cache/001-repository-source-model-and-path-safety.md`
- `docs/concepts/complete/source-control-resolution-and-cache/002-provider-reads-and-admission-manifest.md`
- `docs/concepts/complete/source-control-resolution-and-cache/003-repository-cache-access-layer-and-layout.md`
- `internal/reposource/model.go`
- `internal/reposource/path.go`
- `internal/reposource/manifest.go`
- `internal/reposource/cache_layout.go`
- `internal/reposource/cache_access.go`
- `internal/fingerprint/canonical_json.go`

Do not read unrelated controller files unless compile or test failures directly
require it.

## Allowed Production Files

- `internal/reposource/cache_access.go`
- `internal/reposource/cache_publish.go`
- `internal/reposource/cache_verify.go`

This slice needs new production files. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/reposource/cache_access_test.go`
- `internal/reposource/cache_publish_test.go`
- `internal/reposource/cache_verify_test.go`

## Out Of Scope

- Fetching missing files from GitHub or local filesystem providers.
- Changing `/workflow` admission behavior.
- Replacing `cmd/controller/source_control.go` call sites.
- Renaming controller startup config fields. OS 007 owns the completed
  `controller_repo_cache_*` names.
- Materializing files into worker staging directories.
- Creating, deleting, or reconstructing cache pin files.
- Retention cleanup.
- Reimporting corrupt GitHub cache entries after verification failure.
- Supporting multi-source or multi-repository admitted manifests.
- Whole-repository checkout, clone, recursive copy, or Git object database
  layout.

## Acceptance Criteria

- Cache publication writes only files named by the admitted source manifest.
- Cache publication preserves each admitted file's validated `cache_path` under
  the cache entry `files/` root.
- Cache publication writes parent directories as needed.
- Cache publication writes files through temporary paths and promotes them to
  their final paths without exposing partially written final files.
- Cache publication writes the admitted source manifest.
- Local filesystem admissions store their manifest at
  `<cache-root>/local/runs/<run-id>/manifest.json`.
- GitHub admissions store run-specific manifests under
  `<cache-root>/github/repos/<repository-key>/<commit-id>/manifests/<run-id>.json`.
- GitHub cache entries may add newly manifest-declared files for the same
  commit, but must not overwrite an existing file whose verification evidence
  differs.
- Reading a cached file verifies raw SHA-256 when present.
- Reading a cached project or workflow file verifies GOET canonical JSON
  SHA-256 when present.
- Reading a cached file verifies byte size when the manifest records size.
- Verification failure returns a clear cache-corruption error and does not
  mutate or repair the cache.
- Missing cached files return a clear cache-miss error and do not fetch from a
  provider.
- Tests cover local publish/read, GitHub publish/read, nested paths, cache miss,
  raw hash mismatch, canonical JSON hash mismatch, size mismatch, and conflict
  on an existing GitHub cached file.
- No controller, provider fetch, persistence, pinning, retention, or
  materialization behavior changes.

## Notes

- This slice depends on the OS 003 layout rule that GitHub commit cache
  directories are shared revision cache entries and may be append-only for newly
  manifest-declared files.
- The admitted manifest is the authority for the files required by a run. A
  GitHub commit cache entry may contain extra files requested by other runs at
  the same commit.
- Use `internal/fingerprint.CanonicalJSONSHA256` for canonical JSON
  verification.
- Do not attempt provider repair in this slice. GitHub reimport on mismatch and
  local-source reload failure belong to later controller integration behavior.
- Keep cache publication independent from cache pins. OS 006 owns pin
  reconstruction.

# 005 Manifest Materialization

Status: Complete

## Objective

Materialize an admitted source manifest from the repository cache into a local
destination directory.

This slice copies already-admitted cached files into a staging directory using
their manifest `cache_path` values. It verifies cached bytes through the cache
access layer before writing them, preserves repository-relative directory
structure, and does not decide worker packaging policy.

## Current State

After Operational Slice 001, `internal/reposource` defined the
repository-source model and path validation helpers.

After Operational Slice 003, `internal/reposource` defined cache layout and
path derivation.

After Operational Slice 004, `internal/reposource` published admitted files into
the repository cache and read them back with verification.

`internal/reposource` now has manifest materialization behavior. Worker runtime
code is still not wired to this helper, but repository-source code can take an
admitted source manifest and produce a local filesystem tree containing the
declared Python scripts, Python environment specification, and support files.

## Target State

`internal/reposource` can materialize an admitted source manifest into a local
destination directory.

For each manifest file, materialization:

- reads the cached file through the verified cache reader from OS 004;
- writes the file under the destination directory at the validated
  manifest `cache_path`;
- creates parent directories as needed;
- preserves slash-separated repository-relative directory structure;
- uses temporary output paths before final promotion so partial final files are
  not exposed;
- handles existing destination files using a deterministic overwrite rule.

The materializer is a filesystem operation only. It does not know whether the
file will later be used by the Python executor, another worker type, packaging
code, or a test.

## Concept Decision

This slice updates the repository cache concept by adding materialization from
cache to a caller-selected destination directory.

The behavior should live in `internal/reposource` because it consumes the
admitted source manifest, cache layout, cache reader, and path validation types.
It should not live in worker runtime code because the same source materializer
may be used by Python executor staging, diagnostics, tests, or future packaging
steps.

## Required Context

Read these files first:

- `docs/concepts/complete/source-control-resolution-and-cache/README.md`
- `docs/concepts/complete/source-control-resolution-and-cache/003-repository-cache-access-layer-and-layout.md`
- `docs/concepts/complete/source-control-resolution-and-cache/004-cached-admission-and-verified-reads.md`
- `internal/reposource/model.go`
- `internal/reposource/path.go`
- `internal/reposource/cache_layout.go`
- `internal/reposource/cache_access.go`
- `internal/reposource/cache_verify.go`

Do not read unrelated controller, worker, or transport files unless compile or
test failures directly require it.

## Allowed Production Files

- `internal/reposource/materialize.go`

This slice needs a new production file. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/reposource/materialize_test.go`

## Out Of Scope

- Fetching missing files from GitHub or local filesystem providers.
- Publishing admitted files into the repository cache.
- Changing `/workflow` admission behavior.
- Replacing `cmd/controller/source_control.go` call sites.
- Creating Python executor worker payloads.
- Deciding which files belong in a Python worker package.
- Copying files to remote workers, containers, SSH targets, or Slurm staging
  locations.
- Creating artifact bundles or writing to `controller_artifact_cache_path`.
- Creating, deleting, or reconstructing cache pin files.
- Retention cleanup.
- Supporting multi-source or multi-repository admitted manifests.

## Acceptance Criteria

- Materialization accepts an admitted source manifest and destination directory.
- Materialization reads source bytes only through the verified cache reader from
  OS 004.
- Materialization writes only files named by the admitted source manifest.
- Materialization writes each file to
  `<destination>/<manifest cache_path>`.
- Materialization creates required parent directories.
- Materialization preserves repository-relative directory structure.
- Materialization rejects destination paths that are empty or cannot be proven
  safe for local filesystem writes.
- Materialization rejects manifest `cache_path` values that escape the
  destination directory.
- Materialization uses temporary files and promotes them to final paths so a
  partial final file is not exposed.
- Existing destination files are overwritten only after the replacement file has
  been fully written and verified.
- A cache miss or cache-corruption error from the verified cache reader is
  returned clearly and does not trigger provider fetch or repair.
- Tests cover nested files, parent directory creation, deterministic overwrite,
  unsafe path rejection, cache miss propagation, verification failure
  propagation, and partial-write protection.
- No controller, provider, persistence, remote transport, artifact cache,
  pinning, or retention behavior changes.

## Notes

- This slice materializes source files for local staging only. Remote transfer
  and worker launch behavior belong to later worker-runtime or packaging slices.
- The materializer should not filter by role unless the caller passes a filtered
  manifest or file list. Packaging policy lives outside this slice.
- Use the repository path validator from OS 001 for manifest `cache_path`
  values.
- Destination root validation may use normal filesystem path rules, but
  manifest paths must remain slash-separated repository-relative paths.
- Keep the API small, such as `MaterializeManifest(ctx, manifest, destination)`,
  unless existing OS 004 cache-reader types require a different shape.

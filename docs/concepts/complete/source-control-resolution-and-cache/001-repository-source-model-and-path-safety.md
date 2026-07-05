# 001 Repository Source Model and Path Safety

Status: implemented

## Objective

Define the repository-source model and repository-relative path validation that
later provider, cache, and materialization slices will use.

This slice introduces types for repository identity, resolved source revision
identity, requested file content, admitted source manifest entries, and safe
slash-separated repository paths. It does not read from GitHub, read from local
filesystem roots, publish cache files, or change controller workflow admission.

## Current State

`internal/reposource` now exists and defines the shared repository-source
model for this Strategic Concept. It includes repository identity, resolved
source reference, source file request and content types, admitted manifest
types, file-role constants, and reusable slash-separated path validation.

The controller-local source-control code in `cmd/controller/source_control.go`
still exists and still owns the current runtime behavior. It continues to use
Git-specific vocabulary such as `ResolvedCommit`, resolve one document at a
time, and keep its own `safeRepositoryPath` helper. Later Operational Slices
will move controller call sites onto the shared model and then replace the
controller-local source logic.

## Target State

A new `internal/reposource` package defines the shared model for this Strategic
Concept:

- repository identity and display name;
- resolved source reference with requested ref and explicit nullable revision
  identity;
- file reference addressed by repository, revision identity, and
  repository-relative source path;
- file content with bytes and explicit nullable provider object ID;
- admitted source manifest with one source and a list of admitted files;
- manifest file roles for `project`, `workflow`, `python_entrypoint`,
  `python_environment`, and `support_file`;
- reusable path validation for slash-separated repository-relative path strings
  and cache-relative path strings under a future `files/` root.

The package compiles independently and is covered by focused unit tests. No
existing controller call site is moved in this slice.

## Concept Decision

This slice adds a new concept: the repository-source model.

The concept should live in new files under `internal/reposource` because it has
its own structs, role constants, validation helpers, and test surface. Keeping
these types in `cmd/controller/source_control.go` would preserve the current
controller-local coupling and make later GitHub, local filesystem, cache, and
materialization slices depend on the controller `main` package.

## Required Context

Read these files first:

- `docs/concepts/source-control-resolution-and-cache/README.md`
- `cmd/controller/source_control.go`
- `cmd/controller/source_control_test.go`
- `internal/fingerprint/canonical_json.go`

Do not read unrelated controller files unless compile or test failures directly
require it.

## Allowed Production Files

- `internal/reposource/model.go`
- `internal/reposource/path.go`

This slice needs new production files. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/reposource/model_test.go`
- `internal/reposource/path_test.go`

## Out Of Scope

- Moving or deleting `cmd/controller/source_control.go`.
- Changing `SourceControlAdapter`, `SourceDocumentReference`, or
  `ResolvedSourceDocument`.
- Changing `/workflow` admission behavior.
- Adding GitHub API calls.
- Reading local filesystem source roots through the new package.
- Publishing files into the repository cache.
- Materializing files into worker staging directories.
- Computing canonical JSON SHA-256 for project or workflow documents.
- Changing persistence schema fields such as `source_commit`.
- Implementing local provenance warnings.
- Inferring or negotiating local Git provenance.

## Acceptance Criteria

- `internal/reposource` defines repository-source model types without importing
  `cmd/controller`.
- The model uses `RevisionID` terminology rather than `CommitID` or
  `ResolvedCommit`.
- The model represents nullable fields with `*string`, not empty-string
  sentinels.
- The model permits local filesystem sources to represent null revision
  identity with `RevisionID *string`.
- The admitted source manifest model includes schema, run ID, one source, and
  admitted files.
- Manifest files record role, source path, cache path, explicit nullable
  provider object ID, size in bytes, explicit nullable raw SHA-256, explicit
  nullable canonical JSON SHA-256, and content type.
- Initial role constants cover `project`, `workflow`, `python_entrypoint`,
  `python_environment`, and `support_file`.
- Path validation accepts clean slash-separated repository-relative paths such
  as `project.json`, `workflows/train.json`, and `scripts/lib/helpers.py`.
- Path validation rejects empty paths, `.`, absolute paths, Windows
  drive-qualified paths, backslash paths, and any path containing an original
  `..` segment.
- Validated paths preserve repository-relative directory structure using `/`.
- Path values are represented as strings after validation. This slice does not
  introduce a custom path type.
- Tests cover valid model construction and path validation failure cases.
- No controller, provider, cache, persistence, or materialization behavior is
  changed.

## Notes

- Prefer concrete structs and small validation helpers over broad interfaces.
- `source_path` and `cache_path` should use the same path validator in this
  slice. A later slice may add stricter cache-specific checks if needed.
- Use `*string` for nullable fields such as `RevisionID`, `ObjectID`,
  `RawSHA256`, and `CanonicalJSONSHA256`.
- Keep path values as strings. The path validator provides the safety boundary.
- Do not add a provider interface until Operational Slice 002 needs one.
- Do not make local filesystem sources pretend to have a revision identity.
- Do not introduce a full repository checkout concept. The model is for
  explicitly requested files only.
- Old controller-local source types may be removed in a later implementation
  slice once their call sites have moved. OS 001 should not delete them merely
  because the new model exists.

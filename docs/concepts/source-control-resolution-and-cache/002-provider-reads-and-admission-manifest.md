# 002 Provider Reads and Admission Manifest

Status: implemented

## Objective

Implement repository-source provider reads for GitHub and local filesystem
sources, then build an admitted source manifest from an explicitly declared
file set.

This slice proves GOET can read project, workflow, Python entrypoint, Python
environment, and support files without checking out or copying an entire
repository. It does not publish files into the controller repository cache and
does not change `/workflow` admission behavior.

## Current State

After Operational Slice 001, `internal/reposource` contained the shared
repository-source model and path validation helpers.

Current controller source-reference behavior still lives in
`cmd/controller/source_control.go`. That controller-local adapter reads local
files directly, uses Git-specific `ResolvedCommit` vocabulary, and may infer Git
provenance from a local checkout. This slice does not modify that controller
adapter.

`internal/reposource` now has a narrow provider abstraction, a GitHub provider,
a local filesystem provider, and an admitted-manifest builder. Provider reads
validate repository-relative paths, read only caller-requested files, compute
raw file-byte SHA-256 values, preserve GitHub object IDs when present, and leave
local filesystem revision and object IDs null.

## Target State

`internal/reposource` has provider-read behavior that can:

- resolve GitHub refs to immutable commit IDs;
- read only explicitly requested GitHub files at the resolved commit;
- read only explicitly requested local filesystem files under a configured local
  source root;
- preserve slash-separated repository-relative source paths;
- compute raw file-byte SHA-256 for read files;
- populate provider object IDs for GitHub when available;
- leave local filesystem `RevisionID` and `ObjectID` values null;
- produce the local provenance warning for local filesystem admissions;
- build an admitted source manifest from caller-supplied file requests and file
  contents.

The admitted manifest includes project, workflow, `python_entrypoint`,
`python_environment`, and `support_file` roles. Required file discovery remains
outside this slice: callers must provide the declared file set before admission.

## Concept Decision

This slice updates the repository-source concept introduced in OS 001.

Provider reads should live in `internal/reposource` beside the model because
GitHub, local filesystem, and later cache publication all depend on the same
source identity, path validation, and manifest types. The provider abstraction
should stay narrow: resolve a source, read named files, and return enough
evidence to build the admitted manifest.

## Required Context

Read these files first:

- `docs/concepts/source-control-resolution-and-cache/README.md`
- `docs/concepts/source-control-resolution-and-cache/001-repository-source-model-and-path-safety.md`
- `internal/reposource/model.go`
- `internal/reposource/path.go`
- `internal/fingerprint/canonical_json.go`
- `cmd/controller/source_control.go`

Do not read unrelated controller files unless compile or test failures directly
require it.

## Allowed Production Files

- `internal/reposource/provider.go`
- `internal/reposource/local_provider.go`
- `internal/reposource/github_provider.go`
- `internal/reposource/manifest.go`

This slice needs new production files. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/reposource/provider_test.go`
- `internal/reposource/local_provider_test.go`
- `internal/reposource/github_provider_test.go`
- `internal/reposource/manifest_test.go`

## Out Of Scope

- Changing `cmd/controller/source_control.go` or `/workflow` admission.
- Replacing `SourceControlAdapter` call sites.
- Publishing admitted files into the repository cache.
- Reading from the repository cache.
- Materializing files into worker staging directories.
- Parsing workflow JSON to discover required Python files.
- Inferring imports from Python scripts.
- Supporting multi-source or multi-repository admitted manifests.
- Storing credentials or adding controller credential configuration.
- Retention cleanup, cache pins, locks, or temp directories.
- Changing persistence schema fields such as `source_commit`.

## Acceptance Criteria

- A narrow provider abstraction exists in `internal/reposource`.
- GitHub provider resolves a requested branch, tag, or commit-like ref to a full
  immutable commit ID before file reads.
- GitHub provider reads only explicitly requested repository-relative files from
  the resolved commit.
- GitHub provider preserves repository-relative paths and records provider
  object IDs when the API response provides them.
- GitHub provider tests use a local HTTP test server or equivalent fake; tests
  do not depend on live GitHub access.
- Local filesystem provider uses a configured alias such as `local:demo` as the
  durable repository identity.
- Local filesystem provider reads only explicitly requested validated paths
  under the configured root.
- Local filesystem provider sets `RevisionID`, `ObjectID`, and source-control
  provenance fields to null rather than inferring local Git identity.
- Local filesystem provider returns or exposes this warning during admission:

```text
Local source files do not provide source-control authenticity. Use a source-control provider when provenance must be verifiable.
```

- Provider reads compute raw file-byte SHA-256 for admitted files.
- Manifest construction uses caller-supplied file roles and paths; it does not
  inspect workflow or Python contents to discover more files.
- Manifest construction fails when a required declared file cannot be read.
- The admitted source manifest uses
  `goet/admitted-source-manifest/v1`, includes one top-level source, and records
  every admitted file with role, source path, cache path, content type, size,
  raw SHA-256, optional canonical JSON SHA-256, and nullable provider object ID.
- Project and workflow manifest entries can carry caller-supplied canonical JSON
  SHA-256 values.
- No whole-repository checkout, clone, or recursive copy occurs.
- No controller, cache, persistence, or materialization behavior changes.

## Notes

- Treat the requested file list as authoritative. If a Python script imports an
  undeclared helper file, that is a later admission or authoring error, not a
  runtime cache expansion behavior.
- GitHub authentication may be represented as an optional in-memory token or
  request hook if needed for the provider constructor, but credential discovery
  and controller configuration are outside this slice.
- Prefer `net/http` and local test servers over adding a third-party GitHub
  client dependency unless the implementation slice explicitly justifies that
  dependency.
- Do not use `git` commands for local filesystem sources.
- Do not make local filesystem sources pretend to have commits, blobs, or object
  IDs.
- Keep provider reads independent from cache layout. OS 004 owns cache
  publication and verified cached reads.

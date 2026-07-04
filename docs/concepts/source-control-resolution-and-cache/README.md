# Source-Control Resolution and Cache Strategic Concept

Status: Proposed

Cadence: CSxIx

## Purpose

Provide the source-control boundary GOET needs to resolve mutable repository
references into immutable source identities, read pinned files, verify semantic
content, and materialize explicitly requested files into local cache or staging
directories.

Workflow execution persistence records source locators and semantic hashes, but
source-control behavior is broader than database persistence. This Strategic
Concept owns ref resolution, repository-relative path safety, GitHub-backed
retrieval, local cache layout, cache pins, materialization, and offline restart
behavior.

## Goals

- Define a controller-facing source-control abstraction.
- Resolve mutable refs such as branches or tags into immutable commit IDs before
  admission.
- Read files by repository identity, commit ID, and repository-relative path.
- Keep source locators separate from GOET semantic canonical SHA-256 values.
- Reject unsafe source and materialization paths before provider or filesystem
  code sees them.
- Provide a GitHub-backed implementation as the first source-control provider.
- Maintain a local source-control cache that can serve already admitted pinned
  documents after controller restart.
- Materialize explicit file manifests into staging directories when packaging or
  worker runtime code needs filesystem files.
- Avoid embedding credentials in cache paths, remotes, logs, errors, or durable
  source identities.

## Non-Goals

- Storing source-controlled JSON documents wholesale in SQLite.
- Defining workflow dependency semantics.
- Deciding which files belong in worker artifacts.
- Implementing controller retention cleanup policy.
- Supporting non-GitHub providers before GitHub behavior is proven.
- Replacing GOET canonical JSON hashes with Git object IDs.

## Architectural Context

The controller admits workflow runs from source-controlled project and workflow
documents. The `workflow-execution-persistence` Strategic Concept stores the
durable source facts for admitted runs, but it deliberately does not implement
remote source-control behavior or a local source cache.

This Strategic Concept adds the source-control boundary that runs before and
beside workflow admission. The boundary resolves mutable source references into
immutable commit identities, reads pinned files, and provides local cached bytes
for already admitted source documents when remote source control is unavailable.

## Relationship To Workflow Execution Persistence

`workflow-execution-persistence` stores source-control references as durable
facts:

- repository identity;
- resolved commit ID;
- repository-relative path;
- source object ID when available;
- canonical GOET SHA-256;
- document schema/version metadata.

This Strategic Concept owns how those references are created, refreshed,
verified, cached, and materialized. Persistence remains the database authority
for admitted runs; source control remains the provenance and file-retrieval
authority for pinned source documents.

Remote source control is needed to create or refresh pins. It is not required to
resume an already admitted run when the local cache has verified content for the
recorded repository, commit, path, and canonical hash.

## Current State

Strategically, GOET has planned persistence fields for source-control facts, but
no reusable source-control boundary that owns ref resolution, pinned file reads,
GitHub retrieval, cache layout, or materialization.

Operationally, controller source handling currently lives near controller
workflow admission code. The repository has `cmd/controller/source_control.go`
and `cmd/controller/source_control_test.go`, and the existing local source path
behavior is useful for local-only execution and tests. There is no dedicated
`internal/sourcecontrol` package, no GitHub-backed source-control provider, and
no controller-owned source cache layout.

## Target State

Strategically, GOET has a controller-facing source-control boundary that keeps
source provenance separate from workflow persistence while still producing the
immutable facts persistence needs for admitted runs.

Operationally, implementation will introduce a small source-control package,
GitHub-backed ref and file reads, a deterministic local cache layout, cached
pinned reads with verification, explicit manifest materialization, and cache pin
reconstruction from durable workflow execution state.

## Design Principles

- Branches and tags are discovery inputs, not durable execution identities.
- Every admitted run uses immutable commit IDs.
- Repository identity must not contain secrets.
- Provider object identity, such as a Git blob SHA, is separate from GOET
  canonical JSON SHA-256.
- Path validation belongs at the source-control boundary.
- Materialization consumes an explicit manifest; packaging policy lives
  elsewhere.
- Cache cleanup must not remove files required by active or recoverable admitted
  runs.

## Proposed Operational Slices

These are candidate Operational Slices. They are not implementation
authorization until explicitly selected.

```text
001 Source-Control Abstraction

Define the controller-facing interface, source identity structs, file-content
records, materialization request structs, and repository-relative path-safety
helper. No GitHub, git command, credential, or cache behavior.

002 GitHub Source-Control Implementation

Implement ref resolution, commit identity lookup, pinned file reads, and object
identity reporting for GitHub while preserving the locator-vs-semantic-hash
distinction.

003 Local Source-Control Cache Layout

Define deterministic cache paths, repository/commit pin records, cache root
validation, and collision rules without full retention cleanup.

004 Cached Pinned File Reads

Read pinned files from the local cache when available and verify content against
stored expectations. Fetch only missing exact commits or objects.

005 Manifest Materialization

Materialize an explicit source manifest into a destination directory with path
safety, deterministic overwrite behavior, and no packaging-policy decisions.

006 Cache Pin Reconstruction

Reconstruct active cache pins from durable workflow execution state after
controller restart.
```

## Operational Slice 001 Candidate: Source-Control Abstraction

### Objective

Define the controller-facing source-control boundary used to resolve mutable
repository references into immutable source identities, read pinned files, and
materialize explicitly requested files into a local cache or staging directory.

This Operational Slice creates the abstraction that later GitHub and local-cache
implementations must satisfy. It does not contact GitHub, run `git`, clone
repositories, or decide which project/workflow files a submission needs.

### Proposed API Shape

Introduce a small package, likely `internal/sourcecontrol`, with data structs
and an interface shaped around immutable source retrieval:

```go
type RepositoryRef struct {
    Identity string
    DisplayName string
}

type ResolvedRef struct {
    Repository RepositoryRef
    RequestedRef string
    CommitID string
}

type FileRef struct {
    Repository RepositoryRef
    CommitID string
    Path string
}

type FileContent struct {
    Ref FileRef
    Bytes []byte
    ObjectID string
}

type MaterializeRequest struct {
    Repository RepositoryRef
    CommitID string
    Files []MaterializeFile
    Destination string
}

type MaterializeFile struct {
    SourcePath string
    DestinationPath string
}

type Client interface {
    ResolveRef(ctx context.Context, repository RepositoryRef, ref string) (ResolvedRef, error)
    ReadFile(ctx context.Context, ref FileRef) (FileContent, error)
    Materialize(ctx context.Context, request MaterializeRequest) error
    GetCommitIdentity(ctx context.Context, repository RepositoryRef, commitID string) (ResolvedRef, error)
}
```

Names are design candidates. The implementation should prefer short, explicit
structs over maps or provider-specific JSON blobs.

### Path Safety Boundary

The abstraction owns repository-relative path validation. It should provide a
helper such as:

```go
func CleanRelativePath(path string) (string, error)
```

The helper must reject:

- empty paths;
- absolute paths;
- drive-qualified Windows paths;
- paths containing any original `..` segment;
- paths that clean to `.` or escape the repository root;
- paths with backslashes if repository paths are standardized on `/`.

This helper is used for source paths and materialized destination paths. Later
implementations may add provider-specific validation, but unsafe paths should
fail before reaching GitHub, `git`, or filesystem copy code.

### Acceptance Criteria

- A new source-control package defines the controller-facing interface.
- The interface distinguishes mutable requested refs from immutable commit IDs.
- The interface distinguishes source locator identity from semantic canonical
  JSON hashes.
- Repository identity is represented without embedding credentials.
- File reads are addressed by repository identity, immutable commit ID, and
  repository-relative path.
- File read results may include provider object identity separately from GOET
  canonical SHA-256.
- Materialization accepts an explicit file manifest and destination rather than
  deciding packaging policy.
- Path validation rejects absolute, escaping, empty, and platform-specific
  unsafe paths.
- Path validation accepts clean repository-relative paths.
- Unit tests cover path validation and struct/interface compile-time use.

### Out Of Scope

- GitHub API implementation.
- Local bare Git cache implementation.
- Running `git` commands.
- Fetching, cloning, pruning, or garbage collection.
- Credential lookup, token storage, or secret handling.
- Controller submission integration.
- Workflow/project JSON parsing.
- Canonical JSON SHA-256 computation.
- Cache directory naming and collision policy.
- Retention cleanup.

## Operational Slice 003 Candidate: Local Source-Control Cache Layout

### Objective

Define the on-disk contract for the controller source-control cache.

The cache must make this true:

```text
source adapter resolves a source reference into a local pinned source document
```

That means provider-specific source-control work happens before workflow
admission reads files. `/workflow` should receive local bytes plus source
identity facts, not know whether those bytes came from GitHub, a local checkout,
or an already-populated cache entry.

### Cache Root

The cache root comes from controller configuration:

```text
controller_config.controller_git_cache_path
```

Existing defaults resolve this under the controller root:

```text
${controller_root_dir}/git_cache
```

The cache root must be a controller-owned directory. It may be outside the
`go-etl` repo. It must not contain credentials in any path segment.

### Directory Layout

Use a deterministic provider/repository/commit layout:

```text
<cache-root>/
  repositories/
    <provider>/
      <repository-key>/
        objects/
        commits/
          <commit-sha>/
            files/
              <repo-relative-path>
            manifest.json
            pins/
              <pin-id>.json
        locks/
        tmp/
```

Example:

```text
git_cache/
  repositories/
    github/
      github.com_openai_go-etl-demo-project/
        objects/
        commits/
          3f2b0a7.../
            files/
              project.json
              workflows/demo-workflow.json
            manifest.json
            pins/
              run-018f....json
        locks/
        tmp/
```

This is a file-materialized pinned-document cache. A later implementation may
also store a bare/partial Git object database under `objects/`, but workflow
admission reads the pinned files under `commits/<commit-sha>/files/`.

### Repository Key

`<repository-key>` is a sanitized stable repository identity. It must:

- be deterministic for the provider repository identity;
- contain no credentials, tokens, query strings, or user-specific auth data;
- use only safe filename characters, recommended:

```text
[a-zA-Z0-9._-]
```

- avoid case collisions by normalizing provider-owned case rules where possible;
- include enough provider namespace to avoid ambiguity.

Recommended GitHub key:

```text
github.com_<owner>_<repo>
```

If the provider exposes a numeric immutable repository ID, store that in
metadata but do not require it in the path until the GitHub adapter is designed.

### Commit Directory

`commits/<commit-sha>/` is immutable after publish.

Rules:

- `<commit-sha>` must be a full immutable commit ID, not a branch or tag.
- Files are written under `tmp/` first, then atomically published into
  `commits/<commit-sha>/`.
- Once published, file contents under that commit directory must not be edited
  in place.
- If verification finds a mismatch, mark the commit cache entry corrupt and
  rebuild it through a new temp directory rather than mutating files in place.

### Pinned Files

Pinned source files live under:

```text
commits/<commit-sha>/files/<repo-relative-path>
```

Path rules:

- source paths are repository-relative and slash-separated;
- empty paths, absolute paths, Windows drive-qualified paths, and paths with
  `..` segments are rejected before filesystem access;
- cleaned paths must stay under the `files/` root;
- the original repository-relative path is preserved in metadata.

Only requested files must be materialized. The cache does not need to checkout
the entire repository for a workflow admission.

### Manifest

Each published commit directory has:

```text
manifest.json
```

Initial shape:

```json
{
  "schema": "goet/source-cache/v1",
  "provider": "github",
  "repository_identity": "github.com/owner/repo",
  "repository_key": "github.com_owner_repo",
  "requested_refs": ["main"],
  "commit_sha": "3f2b0a7...",
  "created_at": "2026-07-04T12:00:00Z",
  "files": [
    {
      "path": "project.json",
      "object_id": "...",
      "size_bytes": 144,
      "sha256": "..."
    }
  ]
}
```

`sha256` here is the raw file byte SHA-256 for cache integrity. It is not the
GOET canonical JSON SHA-256 stored with workflow/project provenance.

### Pins

Pins prevent cleanup from removing files needed by active or recoverable runs.

Pin files live under:

```text
commits/<commit-sha>/pins/<pin-id>.json
```

Initial pin shape:

```json
{
  "schema": "goet/source-cache-pin/v1",
  "pin_id": "run-018f...",
  "reason": "workflow_run",
  "created_at": "2026-07-04T12:00:00Z",
  "workflow_run_id": "run-018f...",
  "files": [
    "project.json",
    "workflows/demo-workflow.json"
  ]
}
```

The workflow execution database remains the durable authority for admitted run
source facts. Pin files are operational cache state reconstructed from the
database after restart if missing.

### Locks And Temp

Per-repository operations use files under:

```text
locks/
tmp/
```

Rules:

- one repository-level lock prevents concurrent fetch/materialization from
  corrupting the same repository cache;
- temp directories include a random suffix and are safe to delete after crash;
- publish uses atomic rename where the filesystem supports it;
- cleanup may remove stale temp directories that are not locked.

### Local Adapter Relationship

The current `local` adapter can be treated as a pre-populated cache source:

```text
local:demo -> ../go-etl-demo-project
```

That path is useful for local-only execution and tests, but it is not the
primary cache layout. The primary GitHub adapter should materialize pinned files
into the cache layout above, then return the same resolved-document shape the
local adapter returns today.

### Acceptance Criteria

- The cache root, repository key, commit directory, files directory, manifest,
  pins, locks, and temp directories are specified.
- The layout records immutable commit IDs, not mutable refs, as execution
  lookup keys.
- The cache stores raw file-byte hashes separately from GOET canonical JSON
  hashes.
- Path safety rules prevent source files from escaping `files/`.
- Pin files are explicitly operational cache state, reconstructable from the
  workflow execution database.
- The layout supports reading pinned project/workflow files without remote
  source control when the cache entry is present and verified.

### Out Of Scope

- Implementing the directories in code.
- GitHub API calls.
- Fetching/cloning strategy.
- Bare Git object database design.
- Retention cleanup implementation.
- Schema migration for source-cache pins.
- Worker artifact packaging.

## Open Questions

- What exact format should `RepositoryRef.Identity` use for GitHub stable
  repository identity?
- Should materialization write files by copying from a bare cache, using a
  temporary worktree, or using provider-specific file reads?
- Should cache pin state live only in workflow-execution tables or have its own
  operational table?
- How should cache corruption be detected and repaired before retrying remote
  fetch?

## Completion Criteria

- All agreed Operational Slices for this Strategic Concept are written and
  approved before implementation begins.
- The implemented source-control boundary resolves mutable refs into immutable
  commit IDs before workflow admission.
- Pinned file reads preserve the distinction between source locator identity,
  provider object identity, raw file-byte hashes, and GOET canonical JSON
  SHA-256 values.
- Unsafe source and destination paths are rejected before provider or filesystem
  operations.
- Already admitted pinned files can be read from verified local cache content
  after controller restart without requiring remote source control.
- Source-control cache pins can be reconstructed from durable workflow execution
  state.
- Documentation describes the completed current state after implementation.

# Source-Control Resolution and Cache Epic

Status: Proposed

## Purpose

Provide the source-control boundary GOET needs to resolve mutable repository
references into immutable source identities, read pinned files, verify semantic
content, and materialize explicitly requested files into local cache or staging
directories.

Workflow execution persistence records source locators and semantic hashes, but
source-control behavior is broader than database persistence. This epic owns
ref resolution, repository-relative path safety, GitHub-backed retrieval, local
cache layout, cache pins, materialization, and offline restart behavior.

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

## Relationship To Workflow Execution Persistence

`workflow-execution-persistence` stores source-control references as durable
facts:

- repository identity;
- resolved commit ID;
- repository-relative path;
- source object ID when available;
- canonical GOET SHA-256;
- document schema/version metadata.

This epic owns how those references are created, refreshed, verified, cached,
and materialized. Persistence remains the database authority for admitted runs;
source control remains the provenance and file-retrieval authority for pinned
source documents.

Remote source control is needed to create or refresh pins. It is not required to
resume an already admitted run when the local cache has verified content for the
recorded repository, commit, path, and canonical hash.

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

## Proposed Slices

These are candidate slices. They are not implementation authorization until
explicitly selected.

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

## Slice 001 Candidate: Source-Control Abstraction

### Objective

Define the controller-facing source-control boundary used to resolve mutable
repository references into immutable source identities, read pinned files, and
materialize explicitly requested files into a local cache or staging directory.

This slice creates the abstraction that later GitHub and local-cache
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
- paths containing `..` segments after cleaning;
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

## Open Questions

- What exact format should `RepositoryRef.Identity` use for GitHub stable
  repository identity?
- Should materialization write files by copying from a bare cache, using a
  temporary worktree, or using provider-specific file reads?
- Should cache pin state live only in workflow-execution tables or have its own
  operational table?
- How should cache corruption be detected and repaired before retrying remote
  fetch?

# Repository Source Resolution and Cache Strategic Concept

Status: In Progress

Cadence: CSxIx

## Purpose

Provide the repository-source boundary GOET needs to resolve project, workflow,
and explicitly requested supplemental files from GitHub or the local filesystem,
publish the admitted bytes into a controller-owned repository cache, verify
cached content, and materialize explicitly requested files into staging
directories.

Workflow execution persistence records source locators and semantic hashes, but
repository-source behavior is broader than database persistence. This Strategic
Concept owns ref resolution where a provider has refs, repository-relative path
safety, GitHub-backed retrieval, local filesystem-backed retrieval, admitted
source manifests, repository cache layout, cache pins, materialization, and
offline restart behavior.

## Goals

- Define a controller-facing repository-source abstraction.
- Resolve mutable refs such as branches or tags into stable revision identities
  before admission when the provider has mutable refs.
- Read files by repository identity, revision identity, and repository-relative
  path.
- Keep source locators separate from GOET semantic canonical SHA-256 values.
- Reject unsafe source and materialization paths before provider or filesystem
  code sees them.
- Provide GitHub and local filesystem providers behind the same abstraction.
- Rename the controller cache configuration from Git-specific names to
  repository-cache names:
  `controller_repo_cache_path`, `controller_repo_cache_max_size_mb`, and
  `controller_repo_cache_retention_milliseconds`.
- Maintain a local repository cache that can serve already admitted pinned
  files after controller restart, regardless of
  whether they originally came from GitHub or the local filesystem.
- Cache only files explicitly declared by the workflow/project source manifest.
  Preserve each cached file's repository-relative directory structure under the
  cache `files/` root.
- Prevent clients from addressing repository cache paths directly. Clients name
  provider source paths such as `workflow.json`; the controller chooses where
  admitted bytes are stored under the cache root.
- Treat local filesystem source files as volatile. After admission, local files
  are read from the controller cache copy, not from the original local path.
- Materialize explicit file manifests into staging directories when packaging or
  worker runtime code needs filesystem files, such as Python scripts and Python
  environment specifications for the Python executor.
- Avoid embedding credentials in cache paths, remotes, logs, errors, or durable
  source identities.

## Non-Goals

- Storing source-referenced JSON documents wholesale in SQLite.
- Checking out, copying, or caching an entire project repository when only a
  subset of files is needed.
- Defining workflow dependency semantics.
- Deciding which files belong in worker artifacts.
- Implementing controller retention cleanup policy.
- Supporting providers other than GitHub and local filesystem before those two
  providers are proven.
- Replacing GOET canonical JSON hashes with Git object IDs.
- Inferring Git identity from local filesystem paths. A local path is treated as
  a local filesystem source unless a future Operational Slice explicitly asks
  for local Git behavior.

## Architectural Context

The controller admits workflow runs from source-referenced project and workflow
documents. The `workflow-execution-persistence` Strategic Concept stores the
durable source facts for admitted runs, but it deliberately does not implement
GitHub retrieval behavior, local filesystem source reads, or a repository cache.

This Strategic Concept adds the repository-source boundary that runs before and
beside workflow admission. The boundary resolves source references, reads
project/workflow files and explicitly declared supplemental files, writes
admitted bytes into the controller repository cache, and provides local cached
bytes for already admitted source files when the original source is unavailable.

## Relationship To Workflow Execution Persistence

`workflow-execution-persistence` stores repository-source references as durable
facts:

- repository identity;
- resolved source revision identity;
- repository-relative path;
- source object ID when available;
- canonical GOET SHA-256;
- document schema/version metadata.

This Strategic Concept owns how those references are created, refreshed,
verified, cached, and materialized. Persistence remains the database authority
for admitted runs. GitHub remains the provenance authority for GitHub-backed
source files. Local filesystem sources provide no source-control provenance and
are recoverable only from the admitted cache copy plus recorded content hashes.

The project and workflow persistence tables keep the original source locator
facts for audit and provenance. The repository cache provides the controller's
operational reload source. A cache access layer maps a run's admitted source
manifest to physical cache files. For local filesystem admissions, the physical
files live under a run-scoped local cache area. For GitHub admissions, the
physical files may live under a provider/repository content cache and be reached
through the same cache access layer.

Source revision identity is nullable in durable persistence. GitHub-backed
records store the resolved immutable commit ID. Local filesystem records store
null because local files are not a source-control provenance technique.

Workflow-run submission context must include a durable source-admission context:
the admitted manifest reference, provider, repository identity, repository key,
requested ref, nullable source revision identity, and admitted file roles and
paths needed for restart reload and GitHub repair.

The original provider is needed to create or refresh pins. It is not required to
resume an already admitted run when the repository cache has verified content
for the recorded provider, repository identity, source revision identity, path,
and expected hashes.

Clients never submit repository cache paths. Submitted paths like `project.json`,
`workflow.json`, or `workflows/demo.json` are interpreted relative to the
selected provider repository. A client may normalize user-facing path input
before submission. The controller does not normalize unsafe submitted paths into
safe basenames; if a submitted path contains `..`, is absolute, is
drive-qualified, targets the repository cache, targets another protected
controller area, or otherwise cannot be proven to stay inside the provider
repository, the controller rejects it. The controller then chooses the
segregated cache destination for admitted bytes.

## Current State

Strategically, GOET has planned persistence fields for repository-source facts,
and a reusable `internal/reposource` package now exists for the shared model,
path validation, provider reads, admitted source manifest construction, and
deterministic repository cache path derivation. It can also publish admitted
files into that cache and read cached files back with manifest verification.
It can materialize admitted cached files into a local destination directory.
It can write reconstructable workflow-run cache pin files. Source-reference
`/workflow` admission now uses `internal/reposource` to read project, workflow,
and workflow-declared supplemental source files, publish them into the
repository cache, and compile from verified cached workflow bytes.
Workflow-execution persistence now uses nullable `source_revision_id` fields for
project and workflow rows, and workflow-run submission context now has a
repository-source admission context with source identity, nullable revision
identity, a manifest reference, and admitted file roles/paths.

Operationally, `cmd/controller/source_control.go` now only defines the
source-reference request shape. The old controller-local source adapter path has
been removed. The controller has a repository-source provider registry and a
repository cache layout; local demo admission uses `reposource.LocalProvider`,
which does not infer local Git provenance and records the local provenance
warning in workflow-run submission context. Workflow source documents can
declare supplemental source files through a validated top-level
`source_manifest`, and persistence no longer requires fake commit identity for
local filesystem source rows.

## Target State

Strategically, GOET has a controller-facing repository-source boundary that
keeps source provenance separate from workflow persistence while still producing
the immutable facts persistence needs for admitted runs.

Operationally, implementation will introduce a small repository-source package,
GitHub-backed ref and file reads, local filesystem file reads, deterministic
repository cache layout, cached pinned reads with verification, explicit
manifest materialization, and cache pin reconstruction from durable workflow
execution state.

The target state uses a cache access layer in front of physical cache storage.
The access layer resolves an admitted run's source manifest into local files
without exposing cache paths to clients or worker policy code. Local filesystem
providers publish admitted files under a run-scoped cache area such as
`<cache-root>/local/runs/<run-id>/files/...`. GitHub providers may publish
repository contents under a provider cache such as
`<cache-root>/github/repos/<repository-key>/<content-key>/...`. The access layer
keeps materialization code independent of these physical storage choices.

## Design Principles

- Branches and tags are discovery inputs, not durable execution identities, when
  the provider supports branches and tags.
- Every admitted run uses a stable source revision identity. For GitHub this is
  an immutable commit ID. Local filesystem sources do not provide provenance, so
  their source revision identity is null; the run-scoped cache copy is the
  operational reload mechanism.
- `RepositoryRef.Identity` is provider-qualified. GitHub repositories use
  `github.com/<owner>/<repo>`. Local filesystem repositories use
  controller-defined aliases such as `local:demo`; the local path behind the
  alias is controller configuration, not durable run identity.
- Durable persistence vocabulary should use source revision identity rather than
  source commit. Git commits are one provider-specific revision identity, not
  the generic source identity shape.
- The repository cache configuration rename is immediate because GOET is still
  pre-production. The controller should use `controller_repo_cache_path`,
  `controller_repo_cache_max_size_mb`, and
  `controller_repo_cache_retention_milliseconds` without compatibility aliases
  for the old `controller_git_cache_*` names.
- The repository cache is controller-owned internal storage. User-facing source
  paths never target cache directories.
- Clients may normalize user-facing paths before submission. The controller
  validates submitted paths and rejects traversal, absolute paths, cache paths,
  and other protected controller paths.
- Repository-relative source paths are slash-separated on every platform. Local
  filesystem providers convert slash-separated source paths to OS paths only
  after source-boundary validation succeeds.
- Repository identity must not contain secrets.
- Provider object identity, such as a Git blob SHA, is separate from GOET
  canonical JSON SHA-256.
- GOET canonical JSON SHA-256 is the required project/workflow restart
  verification hash because the controller reads and loads those documents as
  canonical JSON. Raw file-byte SHA-256 may be stored as optional cache/audit
  evidence, but it is not the source of execution correctness for JSON
  documents.
- Restart verification happens when the controller reloads an active `run_id`.
  The controller parses cached `project.json` and `workflow.json`, computes GOET
  canonical JSON SHA-256, and compares those values with the durable
  project/workflow rows for the run. If a GitHub-backed run mismatches, the
  controller should reimport the pinned GitHub source. If a local filesystem run
  mismatches, the controller should fail reload with a clear error because local
  source provenance cannot be recovered.
- Cache corruption is detected by the same active-run reload verification. For a
  GitHub-backed run, the controller may delete or quarantine the corrupt cache
  files before reimporting the recorded GitHub repository, revision, and paths.
  For a local filesystem run, the controller does not attempt repair and reports
  the mismatch.
- Workflow-execution persistence is authoritative for active and recoverable
  runs. Cache-local pin files are optional operational state that may speed
  cleanup or help inspection, but they are reconstructable from durable
  workflow-execution rows and must not become a second authority.
- Materialization always copies through the repository cache access layer, not
  directly from provider reads. This ensures packaging and staging use admitted
  bytes and do not depend on GitHub availability or mutable local filesystem
  sources.
- Cache population is file-granular. Providers fetch or copy only files declared
  by the workflow/project source manifest and recorded in the admitted source
  manifest, not the whole repository. The cache preserves repository-relative
  directory structure so later materialization can recreate the expected layout.
- Required worker files are declared before run admission, not discovered by
  workers at execution time. For the Python executor, Python scripts, helper
  files, and environment specifications must be named by the workflow/project
  source manifest before the run starts. If execution later needs an undeclared
  source file, that is a workflow authoring error, not a cache miss to repair
  automatically.
- The cache access layer owns physical cache lookup. Callers provide an admitted
  run identity and manifest paths; they do not know whether bytes live under
  local run-scoped storage or GitHub repository-content storage.
- Path validation belongs at the repository-source boundary.
- Materialization consumes an explicit manifest; packaging policy lives
  elsewhere.
- Cache cleanup must not remove files required by active or recoverable admitted
  runs.

## Operational Slice Progress

OS implementation order and status:

```text
001 Repository Source Model And Path Safety        [implemented]
002 Provider Reads And Admission Manifest         [implemented]
003 Repository Cache Access Layer And Layout       [implemented]
004 Cached Admission And Verified Reads            [implemented]
005 Manifest Materialization                       [implemented]
006 Cache Pin Reconstruction                       [implemented]
007 Controller Repo Cache Config Rename            [implemented]
008 Workflow Source Manifest Declaration           [implemented]
009 Persistence Source Revision and Admission Context
010 Controller Admission Integration
011 Restart Reload Source Verification
```

The approved Operational Slice charters are separate files in this directory
and remain authoritative for detailed scope.

## Open Questions

No open questions remain for this Strategic Concept draft.

## Completion Criteria

- All agreed Operational Slices for this Strategic Concept are written and
  approved before implementation begins.
- The implemented source-control boundary resolves source references into stable
  source revision identities before workflow admission.
- Pinned file reads preserve the distinction between source locator identity,
  provider object identity, optional raw file-byte hashes, and GOET canonical
  JSON SHA-256 values.
- Unsafe source and destination paths are rejected before provider or filesystem
  operations.
- Already admitted pinned files can be read from verified repository cache
  content after controller restart without requiring remote source control.
- Local filesystem admissions read from cached admitted bytes after admission,
  not from the original local source path.
- Worker-required source files are declared before run admission; workers do not
  discover and expand the required source file set during execution.
- Source-control cache pins can be reconstructed from durable workflow execution
  state.
- Documentation describes the completed current state after implementation.

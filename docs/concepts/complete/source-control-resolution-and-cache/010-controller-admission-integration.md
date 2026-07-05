# 010 Controller Admission Integration

Status: Complete

## Objective

Wire source-reference `/workflow` admission to use the repository-source
provider, admitted source manifest, and repository cache path.

This slice replaces the controller-local source admission path for workflow-run
submissions. Admission reads project, workflow, and workflow-declared
supplemental files through `internal/reposource`, publishes admitted files into
the repository cache, persists project/workflow source facts, and compiles the
workflow from admitted cached bytes.

## Current State

Before this slice, `cmd/controller/main.go` admitted source-reference workflow
runs through `Controller.sourceControl`, `SourceControlAdapter`, and
`ResolvedSourceDocument` in the controller `main` package.

That path:

- reads only project and workflow documents;
- resolves local files through `LocalSourceControlAdapter`;
- may infer local Git provenance;
- uses `ResolvedCommit` terminology;
- computes project/workflow canonical JSON hashes;
- persists project/workflow rows and the workflow run;
- compiles workflow stages and work items from the workflow source document;
- does not publish files into the repository cache;
- does not read supplemental files declared by `source_manifest`.

After OS 001-009, `internal/reposource` is expected to provide model
types, provider reads, admitted manifest construction, cache layout, cache
publication, verified cached reads, materialization, pin reconstruction,
repo-cache config names, workflow source manifest declaration, and persistence
support for nullable source revision identity plus durable source-admission
context.

## Target State

Source-reference `/workflow` admission uses `internal/reposource` for source
admission.

For each submitted run, the controller:

1. Resolves the project source file.
2. Resolves the workflow source file.
3. Decodes the workflow source document and validates any top-level
   `source_manifest`.
4. Builds the required file set:
   - implicit project file;
   - implicit workflow file;
   - workflow-declared supplemental files.
5. Reads all required files through the selected repository-source provider.
6. Computes GOET canonical JSON SHA-256 for project and workflow documents.
7. Builds the admitted source manifest.
8. Publishes admitted files and manifest into the repository cache.
9. Reads project and workflow bytes back through the verified cache reader.
10. Persists project, workflow, and run records using admitted source facts.
11. Compiles workflow stages and work items from cached workflow bytes.

The old controller-local source adapter path is removed or no longer used by
source-reference workflow admission.

## Concept Decision

This slice updates the controller admission concept to consume the
repository-source cache boundary.

The controller remains responsible for admission orchestration: choosing the
provider from submission source identity, applying workflow declaration rules,
persisting run facts, and compiling work. `internal/reposource` remains
responsible for provider reads, admitted manifests, cache publication, and
verified cache reads.

## Required Context

Read these files first:

- `docs/concepts/complete/source-control-resolution-and-cache/README.md`
- `docs/concepts/complete/source-control-resolution-and-cache/001-repository-source-model-and-path-safety.md`
- `docs/concepts/complete/source-control-resolution-and-cache/002-provider-reads-and-admission-manifest.md`
- `docs/concepts/complete/source-control-resolution-and-cache/004-cached-admission-and-verified-reads.md`
- `docs/concepts/complete/source-control-resolution-and-cache/007-controller-repo-cache-config-rename.md`
- `docs/concepts/complete/source-control-resolution-and-cache/008-workflow-source-manifest-declaration.md`
- `docs/concepts/complete/source-control-resolution-and-cache/009-persistence-source-revision-and-admission-context.md`
- `internal/reposource/model.go`
- `internal/reposource/provider.go`
- `internal/reposource/manifest.go`
- `internal/reposource/cache_access.go`
- `internal/reposource/cache_publish.go`
- `internal/reposource/cache_verify.go`
- `cmd/controller/main.go`
- `cmd/controller/source_control.go`
- `cmd/controller/main_test.go`
- `internal/persistence/store.go`

Do not read unrelated controller runtime, scheduler, transport, or worker files
unless compile or test failures directly require it.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/source_control.go`
- `internal/reposource/provider.go`
- `internal/reposource/manifest.go`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `internal/reposource/provider_test.go`
- `internal/reposource/manifest_test.go`

## Allowed Fixture Files

- `../go-etl-demo-project/workflows/*.json`
- `../go-etl-demo-project/submissions/*.json`

## Out Of Scope

- Controller restart reload verification.
- GitHub reimport after cache mismatch.
- Local filesystem reload failure behavior after restart.
- Python executor implementation.
- Materializing source files into worker staging directories during admission.
- Remote worker transfer, artifact bundles, or package creation.
- Retention cleanup.
- Cache pin reconstruction wiring into controller startup.
- Multi-source or multi-repository workflow admissions.
- Supporting old `controller_git_cache_*` config aliases.

## Acceptance Criteria

- Source-reference `/workflow` admission uses `internal/reposource` provider
  reads instead of the controller-local `SourceControlAdapter` path.
- Local filesystem admissions do not infer local Git provenance.
- Local filesystem admissions produce or expose the agreed local provenance
  warning.
- Project and workflow files are implicit required manifest files.
- Workflow `source_manifest` supplemental files are included in the admitted
  source manifest.
- Admission fails before run creation if a declared supplemental file cannot be
  read.
- Admission publishes project, workflow, and supplemental files into the
  repository cache.
- Admission reads project and workflow bytes back through the verified cache
  reader before decoding/compiling.
- Project and workflow rows continue to persist repository identity, nullable
  source revision identity, path, source object ID where available, and
  canonical JSON SHA-256.
- Workflow run submission context records the source-admission context defined
  by OS 009, including the admitted manifest reference needed by restart reload.
- Existing source-reference demo workflow submission still admits successfully.
- A workflow with a valid `source_manifest` admits and publishes supplemental
  files into the cache.
- A workflow with an unsafe supplemental source path is rejected before provider
  file access.
- A workflow with no `source_manifest` remains valid.
- Old controller-local source types are deleted if no longer used; otherwise
  they are left only where still required by tests or transitional code.
- Targeted controller admission and reposource tests pass.

## Notes

- Keep this slice focused on admission. Restart behavior belongs to OS 011.
- Persistence now uses nullable `SourceRevisionID`/`source_revision_id`.
  Controller admission should map repository-source `RevisionID` into that
  field and leave it null for local filesystem admissions.
- The controller should not let a client submit cache paths. Submitted paths
  remain provider repository-relative paths.
- Compilation should use cached workflow bytes after verification so the
  controller does not compile from mutable local filesystem content.
- Do not materialize Python files for execution in this slice. Publishing them
  into the repository cache proves admission captured them.

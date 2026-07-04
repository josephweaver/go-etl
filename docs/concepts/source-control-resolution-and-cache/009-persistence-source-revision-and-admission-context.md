# 009 Persistence Source Revision and Admission Context

Status: implemented

## Objective

Update workflow-execution persistence so admitted repository-source facts can
represent nullable source revision identity and durable admitted-manifest lookup
information.

This slice removes the implementation blocker that local filesystem sources
have no source-control revision identity while current persistence requires a
non-empty `source_commit` value.

## Current State

Before this slice, the persistence schema stored project and workflow source
revision data in `source_commit TEXT NOT NULL` columns. `ProjectRecord` and
`WorkflowRecord` exposed `SourceCommit string`, and validation rejected empty
values.

That shape works for Git commits, but it does not match this Strategic Concept:
GitHub-backed source has a resolved source revision identity, while local
filesystem source must use null source revision identity.

Workflow runs persist `SubmissionContextJSON`. This slice adds a structured
repository-source admission context with a schema, manifest reference, source
identity, nullable source revision identity, and admitted file roles/paths. OS
010 still owns replacing the transitional manifest reference with the concrete
cache manifest written by `internal/reposource`.

## Target State

Persistence records can represent source revision identity as nullable data:

- GitHub records store the resolved immutable commit ID.
- Local filesystem records store null.

The persistence vocabulary uses source revision identity rather than source
commit in new code and schema names. Existing implementation code may bridge
from old names only inside this slice while the schema and public record fields
are updated.

Workflow-run submission context stores a repository-source admission context
with enough information to locate the admitted manifest and retry GitHub repair
without resolving a mutable ref.

Implemented submission context shape:

```json
{
  "schema": "goet/workflow-run-submission-context/v1",
  "source_admission": {
    "schema": "goet/admitted-source-manifest/v1",
    "manifest_ref": "workflows/demo.json#source_manifest",
    "source": {
      "repository_identity": "github:owner/repo",
      "requested_ref": "main"
    },
    "source_revision_id": "3f2b0a7...",
    "files": [
      {
        "role": "project_config",
        "source_path": "project.json",
        "source_object_id": "...",
        "sha256": "..."
      },
      {
        "role": "workflow",
        "source_path": "workflows/demo.json",
        "source_object_id": "...",
        "sha256": "..."
      }
    ]
  },
  "variables": []
}
```

The current `manifest_ref` is a transitional workflow-source manifest reference.
OS 010 must replace it with the concrete admitted cache manifest reference once
controller admission publishes files through `internal/reposource`.

## Concept Decision

This slice updates the workflow-execution persistence concept that the
repository-source Strategic Concept depends on.

It is intentionally separate from OS 010 because controller admission should not
hide a persistence schema change inside an integration slice. Once this slice
exists, OS 010 can persist local filesystem admissions without inventing a fake
revision identity, and OS 011 can locate the admitted manifest without guessing
from scattered project/workflow rows.

## Required Context

Read these files first:

- `docs/concepts/source-control-resolution-and-cache/README.md`
- `docs/concepts/source-control-resolution-and-cache/001-repository-source-model-and-path-safety.md`
- `docs/concepts/source-control-resolution-and-cache/010-controller-admission-integration.md`
- `docs/concepts/source-control-resolution-and-cache/011-restart-reload-verification.md`
- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/db_adapter_sqlite_test.go`
- `cmd/controller/main.go`

Do not read unrelated worker, transport, scheduler, or artifact files unless
compile or test failures directly require it.

## Allowed Production Files

- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`
- `cmd/controller/main.go`

## Allowed Test Files

- `internal/persistence/store_test.go`
- `internal/persistence/db_adapter_sqlite_test.go`
- `cmd/controller/main_test.go`

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `docs/concepts/source-control-resolution-and-cache/README.md`
- `docs/concepts/source-control-resolution-and-cache/010-controller-admission-integration.md`
- `docs/concepts/source-control-resolution-and-cache/011-restart-reload-verification.md`

## Out Of Scope

- Implementing repository-source provider reads.
- Publishing repository cache files.
- Changing controller admission to use `internal/reposource`.
- Restart reload verification and GitHub repair behavior.
- Cache retention cleanup.
- Supporting multi-source workflow manifests.
- Supporting compatibility with old persisted development databases.

## Acceptance Criteria

- Persistence schema stores project source revision identity in a nullable
  column using source revision vocabulary.
- Persistence schema stores workflow source revision identity in a nullable
  column using source revision vocabulary.
- Project and workflow record structs represent source revision identity as
  explicit nullable data, not an empty-string sentinel.
- Persistence validation allows null source revision identity for local
  filesystem records.
- Persistence validation still requires non-empty repository identity,
  source path, canonical JSON SHA-256, and created-at fields.
- GitHub-backed project/workflow records can still store a non-null resolved
  commit ID as source revision identity.
- Store tests cover local records with null revision identity and GitHub records
  with non-null revision identity.
- Workflow-run `SubmissionContextJSON` has a documented repository-source
  admission context shape with schema, manifest reference, source identity,
  source revision identity, and admitted file roles/paths.
- Controller helper tests prove the run submission context can carry the
  admitted manifest reference needed by OS 011.
- No provider read, cache publication, controller admission integration,
  materialization, pin reconstruction, or restart reload behavior changes.

## Notes

- GOET is still pre-production, so this slice does not need to preserve old
  development database compatibility unless the implementation session
  deliberately chooses to add a small migration.
- If the schema version is bumped, update strict schema-version tests and
  `PROJECT_STATE.md` in the same slice.
- Prefer source revision identity names in new structs, schema columns, and
  JSON.

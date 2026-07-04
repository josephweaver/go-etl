# 012f3 Controller Source-reference Workflow Admission

Status: proposed

## Objective

Implement the new persisted `/workflow` controller behavior: admit a workflow
run from project/workflow source references, load the referenced JSON documents
through a source-control adapter, persist project/workflow/run provenance, and
enqueue initially ready compiled work without using `Controller.pending`.

This slice replaces the current persisted-mode `501 Not Implemented` response
with source-reference admission. It does not reintroduce direct inline workflow
JSON submission.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/012f-remove-in-memory-queue-authority.md`
- `docs/epics/workflow-execution-persistence/012f2-client-source-reference-submission.md`
- `docs/epics/workflow-execution-persistence/README.md`
- `FUTURE.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `internal/client/workflow.go`
- `internal/persistence/store.go`
- `internal/workflow`
- `internal/fingerprint`

## API Contract

`POST /workflow` accepts a source-reference workflow run submission:

```json
{
  "project": {
    "repository": "local:demo",
    "ref": "main",
    "path": "project.json"
  },
  "workflow": {
    "repository": "local:demo",
    "ref": "main",
    "path": "workflows/demo-workflow.json"
  },
  "variables": []
}
```

The controller must not treat this as submitted work items. The controller:

1. Resolves the project reference to an immutable source identity.
2. Resolves the workflow reference to an immutable source identity.
3. Reads and validates the project/workflow JSON documents.
4. Computes canonical semantic hashes for those documents.
5. Creates or reuses immutable project/workflow rows.
6. Creates a new workflow run.
7. Builds a resolver from project/workflow/submission variables.
8. Compiles initially ready work.
9. Persists compiled work items and queues them.
10. Starts worker capacity based on persisted queued/running demand.

## Source-control Adapter Boundary

Introduce a narrow controller-owned adapter boundary before adding GitHub:

```go
type SourceControlAdapter interface {
    Resolve(ctx context.Context, ref SourceDocumentReference) (ResolvedSourceDocument, error)
}

type SourceDocumentReference struct {
    Repository string
    Ref        string
    Path       string
}

type ResolvedSourceDocument struct {
    RepositoryIdentity string
    RequestedRef       string
    ResolvedCommit     string
    Path               string
    SourceObjectID     string
    Data               []byte
}
```

Names are candidates. The important behavior is that the controller receives
bytes plus immutable source identity. The database stores the identity and
semantic canonical SHA-256, not a copy of the source document.

## First Adapter: `local`

Implement a `local` source-control adapter first.

The local adapter is valid only when the controller has filesystem access to the
referenced repository or local source-control cache. It should:

- accept a local repository identity such as `local:demo`;
- map that identity to a configured or test-provided local repository root;
- resolve `ref` to an immutable commit identity when the root is a Git repo;
- reject unsafe paths before reading;
- read repository-relative files;
- return the same `ResolvedSourceDocument` shape future remote adapters return.

For tests, the adapter may use a fixture-backed repository root and a fixed
resolved commit value if a real Git repository fixture would be too large. The
controller API should not know whether the adapter is fixture-backed, local
Git-backed, or remote.

## Submission Context

`workflow_instances.submission_context_json` should remain a bounded list of
facts, not a copy of the submitted documents.

Initial shape:

```json
{
  "project": {
    "repository_identity": "...",
    "requested_ref": "main",
    "resolved_commit": "...",
    "path": "project.json",
    "source_object_id": "...",
    "config_sha256": "..."
  },
  "workflow": {
    "repository_identity": "...",
    "requested_ref": "main",
    "resolved_commit": "...",
    "path": "workflows/demo-workflow.json",
    "source_object_id": "...",
    "workflow_sha256": "..."
  },
  "variables": []
}
```

The exact JSON can evolve, but it must preserve source identity and the
submission variables needed to reconstruct resolver inputs.

## Persistence Mapping

Project source document:

```text
projects.repository_identity = resolved project repository identity
projects.source_commit       = resolved project commit
projects.config_path         = project path
projects.source_object_id    = source object identity when available
projects.config_sha256       = canonical project JSON SHA-256
```

Workflow source document:

```text
workflows.project_id          = persisted project identity
workflows.repository_identity = resolved workflow repository identity
workflows.source_commit       = resolved workflow commit
workflows.workflow_path       = workflow path
workflows.source_object_id    = source object identity when available
workflows.workflow_sha256     = canonical workflow JSON SHA-256
```

Run:

```text
workflow_instances.project_id
workflow_instances.workflow_id
workflow_instances.submission_context_json
workflow_instances.created_at
```

Initially ready work:

```text
work_items.run_id
work_items.stage_index
work_items.worker_payload_json
work_items.resolved_inputs_sha256
queued_work
```

## Workflow JSON Loading

The workflow source document is decoded into `workflow.Workflow` and compiled
using the existing compiler.

Project JSON handling is intentionally narrower for the first slice. If the
current project model is not yet implemented, the controller may:

- validate that the referenced project JSON is syntactically valid canonical
  JSON;
- compute and persist its canonical SHA-256;
- include it in submission provenance;
- defer semantic project-variable merging to a later workflow-resolution slice.

This preserves the source-control-first contract without pretending the project
model is complete.

## Acceptance Criteria

- Store-configured `/workflow` accepts the source-reference submission envelope.
- Store-configured `/workflow` rejects legacy inline workflow JSON.
- The controller loads project/workflow documents through a source-control
  adapter boundary.
- The first adapter supports local/fixture-backed references such as
  `local:demo`.
- Unsafe source paths are rejected.
- Project and workflow rows are persisted with source identity and canonical
  SHA-256 values.
- A workflow run row is created with bounded submission context JSON.
- Initially ready compiled work is inserted into `work_items` and `queued_work`.
- `/workflow` does not mutate `Controller.pending`, `assigned`, or `failed`
  when `workflowStore` is configured.
- Existing `/work/next` can claim work created by source-reference `/workflow`.
- Scaling demand for store-configured `/workflow` is derived from persisted
  queued/running counts.
- No-store legacy inline `/workflow` behavior remains available only as a
  fallback until tests are migrated.

## Out Of Scope

- GitHub adapter implementation.
- Full source-control cache retention or pinning policy.
- Full semantic project model and project-variable merging.
- Dependency-aware stage publication beyond initially ready work.
- Retry/requeue policy.
- Leases, heartbeats, and stale-worker fencing.
- Removing all in-memory queue fields.
- Python/R clients.

## Ambiguity To Review

`project_id` and `workflow_id` generation need a concrete rule. Options:

- UUIDv7 identities plus stored SHA-256 values.
- deterministic IDs derived from source identity and canonical SHA-256.

The epic generally prefers controller-generated UUIDv7 IDs with semantic hashes
stored beside them. If UUIDv7 helper code is not available yet, the first slice
may use a temporary opaque ID generator and mark it for replacement.

The local adapter needs a repository identity to filesystem-root mapping. That
mapping could come from controller configuration, a fixed demo/test map, or a
future source-control config section. For the first implementation, prefer an
injected/testable resolver map and keep live defaults conservative.

It is also open whether project and workflow references must point to the same
repository/commit. The client currently sends both references explicitly. The
controller should support that shape, but may reject cross-repository or
cross-commit submissions in the first implementation if resolver semantics are
not ready.

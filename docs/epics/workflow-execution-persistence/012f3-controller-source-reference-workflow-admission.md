# 012f3 Controller Source-reference Workflow Admission

Status: designed

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

## Implementation Shape

This feature is too large for one EC-3 prompt. Implement it as small atoms on
the existing epic branch:

```text
012f3-a Local source document adapter [implemented]
012f3-b Source-reference request decode and persisted-mode guard tests [implemented]
012f3-c Source document canonicalization and provenance records [implemented]
012f3-d Compile workflow source into persisted stage/work/queue rows [implemented]
012f3-e Persisted scaling demand after workflow admission [implemented]
012f3-f End-to-end demo submission test
```

Each atom should leave `go test ./cmd/controller ./internal/client ./internal/persistence`
passing, or the narrowest smaller package set if the atom does not touch all
three.

## 012f3-a Local Source Document Adapter

Introduce a narrow controller-owned adapter boundary before adding GitHub. The
adapter resolves source references into bytes plus immutable-ish source
identity. It must not write to the database and it must not know about workflow
compilation.

Candidate types:

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

Names are candidates. If the implementation can reuse
`internal/client.SourceDocumentReference` without creating an import cycle, do
that. If not, duplicate the transport shape in the controller package for this
slice and defer shared API consolidation.

First concrete adapter: `local`.

The local adapter is valid only when the controller has filesystem access to the
referenced repository or local source-control cache. It should:

- accept a local repository identity such as `local:demo`;
- map that identity to a configured or test-provided local repository root;
- resolve `ref` to an immutable commit identity when the root is a Git repo;
- reject unsafe paths before reading;
- read repository-relative files;
- return the same `ResolvedSourceDocument` shape future remote adapters return.

For this slice, `local:demo` should map to `../go-etl-demo-project` in tests and
the development demo path. The mapping should be injectable on `Controller`, not
hard-coded inside `/workflow`.

Path safety rule:

```text
clean repository-relative path must stay inside the configured repository root
```

Reject absolute paths, `..` escape paths, empty paths, and paths that resolve
outside the repository root.

Git identity rule for this slice:

- If the repository root is a Git repo, resolve the requested ref with
  `git -C <root> rev-parse <ref>^{commit}` and use that full commit SHA.
- If the root is not a Git repo, use a deterministic local placeholder such as
  `local-unversioned` and set `source_object_id` to the canonical document hash
  or an empty value.

Ambiguity:

- Shelling out to `git` from the adapter is acceptable for the local adapter, but
  the future GitHub adapter should not inherit that implementation.
- The source-control-cache directory shape is still future work. This slice only
  needs root mapping for `local:demo`.

Acceptance criteria for 012f3-a:

- Resolves `local:demo` project and workflow paths from the sibling demo repo.
- Rejects path traversal and unknown repository identities.
- Produces bytes, repository identity, requested ref, resolved commit, path, and
  source object identity.
- Has focused tests around resolution and path rejection.

Implementation note:

- `cmd/controller/source_control.go` defines the first source-control adapter
  boundary and `LocalSourceControlAdapter`.
- `cmd/controller/source_control_test.go` verifies local demo resolution,
  unknown repository rejection, unsafe path rejection, and unversioned
  repository fallback identity.
- The adapter shells out to `git` only to resolve local Git roots. Non-Git roots
  use `local-unversioned` and a SHA-256 of document bytes as `source_object_id`.

## 012f3-b Request Decode And Persisted-Mode Guard

Change the store-configured `/workflow` branch from unconditional `501` to
decoding the source-reference envelope. Keep the legacy inline `WorkflowSubmission`
path only when `workflowStore == nil`.

The handler should distinguish:

```text
source-reference submission -> continue persisted admission
inline workflow submission   -> reject with 501 or 400 in persisted mode
invalid JSON                 -> 400
```

Acceptance criteria for 012f3-b:

- Store-configured `/workflow` accepts the source-reference envelope shape far
  enough to call the admission helper.
- Store-configured `/workflow` still rejects legacy inline workflow JSON.
- No-store `/workflow` behavior remains unchanged.
- Tests assert that persisted-mode decode does not mutate
  `pending`, `assigned`, or `failed`.

Implementation note:

- Store-configured `/workflow` now decodes `WorkflowRunSubmission` instead of
  immediately returning `501`.
- Valid source-reference payloads reach `submitWorkflowRunToStore`, which still
  returns the explicit not-implemented sentinel until later atoms wire
  resolution and persistence.
- Legacy inline workflow JSON is rejected as an invalid source-reference shape
  in persisted mode.
- Tests cover both valid source-reference decode and legacy inline rejection
  while proving in-memory queue fields stay unchanged.

## 012f3-c Canonicalization And Provenance Records

Resolve both source documents through the adapter, then compute canonical JSON
and SHA-256 using `internal/fingerprint`.

Project JSON handling remains syntactic in this slice:

- decode with `json.Decoder.UseNumber`;
- reject malformed JSON;
- canonicalize the decoded value;
- persist its canonical SHA-256;
- do not yet merge project variables into the resolver.

Workflow JSON handling:

- decode the referenced workflow file as the existing `WorkflowSubmission`
  document shape, because current workflow fixtures contain:

```json
{
  "workflow": { "...": "..." },
  "variables": []
}
```

- canonicalize the full workflow source document for `workflow_sha256`;
- use `submission.Workflow` and `submission.Variables` for compilation.

Persisted IDs:

- Use controller-generated UUIDv7 for new workflow run IDs when UUIDv7 helper
  code exists.
- For 012f3 implementation, if no UUIDv7 helper exists yet, create a tiny local
  opaque ID helper and mark replacement with UUIDv7 as follow-up.
- Project and workflow rows may use deterministic IDs derived from their source
  identity and canonical hash for idempotent `UpsertProject`/`UpsertWorkflow`.
  Example candidate:

```text
project:<sha256(repository_identity + "\n" + source_commit + "\n" + config_path + "\n" + config_sha256)>
workflow:<sha256(project_id + "\n" + repository_identity + "\n" + source_commit + "\n" + workflow_path + "\n" + workflow_sha256)>
```

This avoids duplicate project/workflow rows for the same pinned source document
while still giving each submitted run its own run ID.

Ambiguity:

- The epic earlier preferred UUIDv7 for durable entity IDs. Deterministic
  project/workflow IDs are a pragmatic exception for source-document upsert
  idempotence. If this feels wrong during review, use UUIDv7 for all three
  entity types and rely on future unique indexes or lookup methods instead.

Acceptance criteria for 012f3-c:

- Project and workflow source documents are resolved only through the adapter.
- Canonical JSON hashes are computed from decoded JSON, not raw bytes.
- Project and workflow rows are upserted with repository identity, commit/path,
  source object ID, and canonical hashes.
- Submission context JSON records source identity, canonical hashes, and
  submitted variables, but not full source documents.

Implementation note:

- Store-configured `/workflow` now resolves project and workflow source
  documents through `Controller.sourceControl`.
- The controller decodes source JSON with `UseNumber`, canonicalizes through
  `internal/fingerprint`, and upserts project/workflow provenance rows before
  returning the not-yet-implemented sentinel for run/work admission.
- Project/workflow IDs are deterministic source-derived IDs.
- Because current `UpsertProject`/`UpsertWorkflow` compares the entire row,
  including `created_at`, source-document rows use a stable source-derived
  `created_at` token. A later store-method refinement should preserve a true
  first-seen timestamp without making idempotent source upsert conflict.

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

## 012f3-d Compile And Persist Initially Ready Work

Build the resolver from:

```text
workflow source variables
submitted workflow-run variables
```

Project variables are out of scope until a semantic project model exists.

Compile with `workflow.CompileWorkflowResult` so the controller has both stage
count and per-work-item step IDs. Persist:

- one workflow run row;
- one stage plan row per workflow step, initially `ready`;
- one `work_items` row per compiled work item;
- one `queued_work` row per compiled work item.

Worker payload rule:

```text
worker_payload_json stores the existing model.WorkItem JSON transport shape
```

This is a transitional but explicit choice. It keeps `/work/next` compatible
with existing workers. A later worker-payload-model slice can replace it with a
smaller plugin payload once the worker contract is ready.

Resolved input hash rule:

```text
resolved_inputs_sha256 = sha256(canonical JSON of the model.WorkItem payload)
```

This is also transitional. It is deterministic and consistent with the current
claim/reuse stand-in, but it is not the final semantic input hash.

Stage/work indexing:

- `stage_index` is the zero-based workflow step index from the compiler result.
- `work_item_index` is the zero-based order of compiled work items within that
  stage.
- `stage_source_reference` should include the workflow source path and step ID,
  for example `workflows/demo-workflow.json#write-demo`.

Acceptance criteria for 012f3-d:

- `/workflow` creates a run and queues compiled work without touching
  `Controller.pending`.
- Existing persisted `/work/next` can claim the queued rows and return worker
  assignments.
- Duplicate compiled work item IDs in the same workflow still fail through the
  existing compiler.
- Tests can inspect store state after `/workflow` and see queued rows.

Implementation note:

- Store-configured `/workflow` now decodes the resolved workflow source as the
  existing inline `WorkflowSubmission` document shape.
- It creates an opaque `run-<hex>` workflow run ID, stores source-reference
  submission context JSON, inserts ready stage rows, inserts compiled work-item
  rows, and enqueues them.
- Persisted `work_item_id` values are run-scoped as `runID:generatedID` because
  generated workflow item IDs can repeat across workflow runs. The worker
  payload still contains the original `model.WorkItem` JSON shape.
- Controller tests verify source-reference `/workflow` persists provenance,
  creates an active run, persists a stage, queues two demo work items, leaves
  in-memory queue fields unchanged, and can claim one queued item through the
  existing persisted `/work/next` path.

## 012f3-e Persisted Scaling Demand

After persisted admission succeeds, derive worker start demand from persisted
queued/running counts rather than in-memory `pending`/`assigned`.

Use the existing persisted status/count helpers if possible. Do not introduce a
separate counter table.

Acceptance criteria for 012f3-e:

- Store-configured workflow submission plans worker starts using persisted
  queued/running counts.
- No-store workflow submission continues to use in-memory pending/assigned
  counts.
- Tests cover at least the no-env case and one injected-env/scaler case if the
  current test seams make that practical.

Implementation note:

- After source-reference workflow admission inserts and enqueues persisted work,
  the controller now derives demand from `ListQueuedWorkItems` and
  `ListRunningWork`.
- Store-configured admission uses the same scaler state and worker-start path as
  the legacy in-memory path, but passes persisted queued/running counts.
- Tests cover the no-env persisted admission case and a configured execution
  environment case that starts one scheduled worker from persisted demand.

## 012f3-f End-To-End Demo Submission Test

Purpose: verify the implemented 012f3 path against the real consumer-facing
demo repo layout. This is a verification slice, not a new runtime capability
slice.

Add a focused controller test that uses the real sibling demo project files:

```text
../go-etl-demo-project/submissions/demo-workflow-run.json
../go-etl-demo-project/project.json
../go-etl-demo-project/workflows/demo-workflow.json
```

Test setup:

- load the workflow-run submission JSON from the sibling demo project;
- create a temporary workflow-execution persistence store;
- create a controller with that store and a `LocalSourceControlAdapter`;
- map `local:demo` to `../go-etl-demo-project`;
- do not configure an execution environment for this test, so worker scaling is
  not part of the assertion;
- POST the loaded submission body to `/workflow`.

Assertions:

- HTTP response is `204 No Content`;
- exactly one active workflow run exists;
- project row exists and has:
  - `repository_identity = "local:demo"`;
  - `config_path = "project.json"`;
  - non-empty `source_commit`, `source_object_id`, and `config_sha256`;
- workflow row exists and has:
  - `repository_identity = "local:demo"`;
  - `workflow_path = "workflows/demo-workflow.json"`;
  - non-empty `source_commit`, `source_object_id`, and `workflow_sha256`;
- submission context JSON is valid and includes project/workflow source facts;
- one ready stage exists for `write-demo`;
- two queued work rows exist for the two years in `demo-workflow.json`;
- queued persisted work item IDs are run-scoped;
- queued worker payload JSON decodes as `model.WorkItem`;
- one queued item can be claimed through `/work/next`;
- `Controller.pending`, `assigned`, and `failed` remain empty.

Recommended test name:

```go
func TestSubmitWorkflowHandlerAdmitsDemoProjectWorkflowRun(t *testing.T)
```

Out of scope for 012f3-f:

- Starting a real controller process.
- Starting workers.
- Running the demo client executable.
- Completing the claimed work.
- Verifying output files.
- GitHub/cache download behavior.
- Retrying the same workflow run submission for idempotency.

Ambiguity:

- This test intentionally depends on the sibling `../go-etl-demo-project`
  checkout. If that coupling becomes too brittle, convert it to an integration
  test behind an environment flag and keep the existing temp-directory source
  adapter tests as the default unit coverage.
- The current local adapter reads the working tree and resolves the requested
  ref through Git when possible. If `go-etl-demo-project` has uncommitted edits,
  the canonical JSON hash reflects the working-tree file while
  `source_commit` reflects `main`. That is acceptable for the local adapter
  smoke test but is not the final remote source-control pinning semantics.

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

Remaining ambiguity to settle during implementation review:

- **Project/workflow IDs:** deterministic source-derived IDs make upserts easy;
  UUIDv7 is the general epic preference. This design recommends deterministic
  IDs for source-document rows and UUIDv7/opaque generated IDs for runs.
- **Local adapter configuration:** first implementation can inject a
  `map[string]string` on the controller. A later startup/config slice should
  load that map from controller config.
- **Cross-repository submissions:** the client envelope allows project and
  workflow references to point at different repositories or commits. First
  implementation should support that structurally, but it may reject it if the
  resolver/project-variable semantics become unclear.
- **Project semantic model:** project JSON is provenance-only in 012f3. It does
  not yet contribute variables.
- **Run ID helper:** UUIDv7 is preferred, but the repo may need a tiny helper or
  dependency decision before implementing that precisely. The current 012f3-d
  implementation uses `run-<random hex>` as the temporary opaque run ID.
- **Atomicity:** ideal admission should persist project, workflow, run, stages,
  work items, and queue rows in one transaction. The current store methods are
  separate. If adding a transactional store method is too large for 012f3, note
  the partial-write risk and add a follow-up cleanup/transaction slice.

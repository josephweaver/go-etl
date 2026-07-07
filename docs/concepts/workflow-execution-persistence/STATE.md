# Workflow Execution Persistence State

Last updated: 2026-07-07

Parent Strategic Concept: [README.md](README.md)

This file preserves workflow-execution persistence sections moved out of the root PROJECT_STATE.md.

## Workflow Execution Persistence

`internal/persistence` contains the first SQLite-backed workflow execution
store for the `workflow-execution-persistence` epic. The schema currently
tracks projects, workflows, workflow runs, stage plans, work items, queued
work, attempts, running work, completed work, failed work, and worker records.
Schema version 2 stores project and workflow source revision identity as
nullable `source_revision_id` columns, represented in Go as `*string`
`SourceRevisionID` fields. GitHub-backed rows can store the resolved immutable
commit ID; local filesystem rows can leave revision identity null while still
recording repository identity, source path, canonical JSON SHA-256, and
created-at evidence.

The store can insert workflow/run/stage/work-item records, enqueue work,
derive per-stage queued/running/completed/failed counts, atomically claim the
oldest queued work into `running_work`, atomically terminate a running attempt
into either `completed_work` or `failed_work`, and atomically mark a stage
complete when persisted work rows prove every work item for the stage completed
successfully. Terminal rows preserve the copied `queued_at` and `started_at`
values from `running_work` plus the terminal timestamp. Completion records store
output JSON, output hash, pre-state hash, post-state hash, and optional
`skipped_parent_id`; failure records store the error and failure time. Repeated
identical terminal reports are idempotent; conflicting terminal reports fail.
Stage completion can publish caller-supplied newly ready work items and queue
rows in the same transaction, while dependency readiness and downstream
compilation remain out of scope. Restart-oriented read queries can list running
work, retrieve one running attempt, list terminal attempts for a run, and derive
run-level queued/running/completed/failed counts from placement and terminal
tables.

This persistence package is not yet wired into the live controller HTTP
assignment and report paths. The older `internal/ledger` attempt snapshot
ledger remains the currently wired local demo ledger.

Source-control resolution, GitHub retrieval, local cache layout, and
materialization have been split into the separate
`source-control-resolution-and-cache` epic. Workflow execution persistence keeps
the database-owned source locator fields but does not own the source-control
implementation.
Workflow-run `SubmissionContextJSON` now includes a structured
`goet/workflow-run-submission-context/v1` source-admission context with
repository identity, requested ref, nullable source revision identity, a
manifest reference, and admitted file roles/paths. Controller admission now
stores the concrete admitted source manifest path produced by the repository
cache layout. Local filesystem admissions store null source revision identity
and include the local provenance warning in run submission context.
Startup recovery uses that context as the authority for source-cache reload
verification and GitHub-only repair.

The source-control epic now defines the first local cache directory contract.
The intended cache shape is provider/repository/commit based:

```text
<cache-root>/repositories/<provider>/<repository-key>/commits/<commit-sha>/files/<repo-relative-path>
```

Each commit directory has a `manifest.json` for raw file-byte integrity and a
`pins/` directory for operational cache pins that can be reconstructed from
workflow execution database records. The cache uses immutable commit IDs for
execution lookup; mutable refs are only admission inputs.

The persistence epic now has a `012f4` cleanup slice for guard and demotion
work. The first implementation removed `Controller.pending`,
`Controller.assigned`, and `Controller.failed` entirely so the controller no
longer exposes a process-local queue authority.

Controller cutover has started by adding a workflow-execution store handle to
`Controller` and opening that store as the configured main database during live
startup. The older attempt ledger remains in code for legacy skip/reuse helpers
and tests, but live startup no longer opens it as the main database. When a
workflow-execution store is configured, `/status` now derives pending,
assigned, and failed-equivalent counts from persisted queued, running, and
failed work rows instead of the in-memory queue maps. Raw `POST /work`
submissions also persist into a synthetic raw-work run and queue row when the
workflow-execution store is configured; the old in-memory raw submission path
has been removed. `/work/next` claims persisted
queued work through the workflow-execution store and decodes the stored worker
payload back into the existing worker response shape when the store is
configured. Queue endpoints return service unavailable when no workflow store is
configured.

## Workflow Execution Persistence Cutover

Feature 012e now routes `/work/complete` and `/work/fail` through
`internal/persistence.Store` when `Controller.workflowStore` is configured.
Persisted `/work/next` returns the store-created `attempt_id`, and workers echo
that attempt ID in completion and failure reports. Failure reports also carry
optional `failed_at` so duplicate persisted failure reports can be idempotent
when they repeat the same durable payload.

Worker completions now carry three JSON evidence documents:

- `output_json`
- `pre_state_json`
- `post_state_json`

The controller validates and canonicalizes the completion evidence, stores
canonical `output_json`, and writes SHA-256 hashes for output, pre-state, and
post-state through `Store.CompleteAttempt`. Legacy in-memory completion and
failure behavior is unchanged when no workflow-execution store is configured.

The worker demo and summary operations now return `WorkEvidence` to the worker
loop so terminal reports can include discernible output, pre-state, and
post-state evidence. This changed the worker execution shape from:

```go
err := worker.Run(item)
```

to:

```go
evidence, err := worker.Run(item)
```

Feature 012e2 extends that contract with worker-observed skip evidence. Work
assignments can now carry `reuse_candidates`, and completion reports can carry:

- `skipped`
- `skipped_parent_id`
- `skip_reason`
- `input_sha256`
- `output_sha256`
- `pre_state_sha256`
- `post_state_sha256`

The worker uses `internal/fingerprint` for canonical JSON and SHA-256 hashing.
The demo worker can skip when its current pre-state and expected output match a
prior candidate. The summary worker includes input file path, size, and content
SHA-256 in its input observation before deciding whether reuse is safe.

Persisted `/work/next` currently selects reuse candidates from prior completed
attempts in the same run when `resolved_inputs_sha256` and
`worker_payload_json` match. This is a conservative temporary stand-in until
`controller_sha256` and `plugin_sha256` are precisely defined. The database
schema still stores worker-observed input/output hashes inside canonical
`output_json`; explicit columns are deferred to a later schema slice.

Feature 012f has started by blocking the remaining live persisted path that
could create in-memory queue authority. When `Controller.workflowStore` is
configured, `/workflow` now rejects the legacy inline JSON payload with `501 Not
Implemented` instead of compiling it into a process-local queue. Source-reference
workflow admission is now the controller/client boundary.

Feature 012f2 updates the Go client side of that boundary. `internal/client`
now has a `WorkflowRunSubmission` envelope with project and workflow
`SourceDocumentReference` values, and `cmd/demo-client` now submits
`demo-workflow-run.json` through `SubmitWorkflowRunFile`. The old inline
workflow submission helpers remain as legacy compatibility methods, but they
are no longer the demo client's normal path. Controller-side source-reference
admission is still pending.

Feature 012f3 was designed as the controller-side source-reference admission
slice. The target `/workflow` path loads project/workflow JSON through a source
provider, persists source identity and canonical hashes, creates a workflow run,
compiles initially ready work, and queues that work without using process-local
controller state.

Earlier 012f3 implementation atoms used a controller-local source adapter as a
bridge. That bridge has now been removed in favor of `internal/reposource`
providers.

The second 012f3 atom updates store-configured `/workflow` to decode the
source-reference `WorkflowRunSubmission` envelope and validate project/workflow
repository, ref, and path fields. Valid source-reference submissions currently
reach a not-yet-implemented admission helper; legacy inline workflow JSON is
rejected in persisted mode without mutating `pending`, `assigned`, or `failed`.

The third 012f3 atom wired provenance persistence into that helper using the
then-current source adapter. Current admission now resolves through
`internal/reposource`, decodes and canonicalizes JSON documents through
`internal/fingerprint`, computes canonical SHA-256 values, and upserts
`projects` and `workflows` rows with deterministic source-derived IDs.

The fourth 012f3 atom now decodes the resolved workflow source as the existing
`WorkflowSubmission` JSON shape, builds the resolver from workflow variables,
source-submission variables, and run-submission variables, compiles the
workflow, creates an opaque workflow run, stores bounded source-reference
submission context JSON, inserts ready stage rows, inserts run-scoped persisted
work item rows, and enqueues them. Persisted work item IDs use
`runID:generatedID` so repeated workflow submissions do not collide on the
global `work_items.work_item_id` primary key, while the worker payload still
contains the original `model.WorkItem` JSON. Store-configured `/workflow` can
now create queued persisted work that the existing persisted `/work/next` path
can claim.

The fifth 012f3 atom wires worker scaling for source-reference admission.
After persisted work is enqueued, the controller derives demand from
`ListQueuedWorkItems` and `ListRunningWork`, then uses the existing
`WorkerScaleState` and `startConfiguredWorkers` path.

Persisted source-reference admission can now also start local command-backed
workers when no configured `ExecutionEnvironment` is present. It uses the
existing `LocalWorkerStarter` path and worker configuration variables from the
resolved workflow source. This keeps the local demo path working while the
configured execution-environment model remains the preferred HPCC-facing path.

The sixth 012f3 atom adds an end-to-end controller test for the migrated sibling
demo project. The test loads
`../go-etl-demo-project/submissions/demo-workflow-run.json`, maps `local:demo`
to `../go-etl-demo-project`, submits the real source-reference body to
`/workflow`, verifies persisted project/workflow/run/stage/queued-work state,
checks that queued worker payload JSON decodes as `model.WorkItem`, claims one
item through persisted `/work/next`.

The local demo repository-source provider is now wired into live controller
startup. When the controller starts from the `go-etl` working directory,
`local:demo` maps to `../go-etl-demo-project`. This is a development/demo bridge
so the current demo-client source-reference submission has a provider during
live admission. Future source-control work should replace the hard-coded mapping
with controller configuration.

The local demo controller config now writes to
`.run/controller/workflow-execution.sqlite` instead of the old
`.run/controller/ledger.sqlite` path. The old file was created by an earlier
ledger shape and is not automatically replaced. The source-reference demo client
has been smoke-tested successfully with the new path:

```text
final status: pending=0 assigned=0 failed=0 pending_reuse_candidates=0 attempts=0 attempt_variables=0
```

Feature 012f4 is now being used as an epic-closure and boundary cleanup slice.
The controller no longer has `pending`, `assigned`, or `failed` queue fields;
the workflow-execution store is the only supported queue authority. The first
closure cleanup replaced a skipped legacy inline `/workflow` invalid-payload
test with source-reference validation coverage. Remaining 012f4 work is to
replace or explicitly retire the other skipped legacy inline-workflow tests and
to reconcile the epic README/status trail before marking the persistence epic
ready for review.

The next 012f4 cleanup pass replaced the legacy inline worker startup and
worker-scaling `/workflow` tests with source-reference fixtures backed by the
local repository-source provider. The converted coverage now exercises persisted
workflow admission before asserting configured Slurm worker submission, planned
worker count, submitted worker-scale configuration, and organic scale-up after a
worker claim.

The two skipped legacy ledger-handler tests were removed during 012f4 cleanup.
The controller completion handler no longer writes old ledger attempt-variable
rows, and status no longer reports ledger attempt or attempt-variable counts.
Active coverage for those surfaces is now the persisted terminal-attempt test
and the persisted status-count test.

The final 012f4 cleanup converted the remaining skipped inline `/workflow`
tests to source-reference coverage for general workflow admission, submitted
code version, Singularity runtime, invalid worker scale config, and duplicate
generated work-item IDs. `cmd/controller/main_test.go` now has no skipped tests,
and the persistence epic is ready for implementation review rather than further
feature expansion.

The controller startup path now has a small assembly helper in `cmd/controller/main.go` so tests can exercise the full startup sequence without launching a live listener. The new startup coverage verifies precedence, qualified database lookup protection, recovery-mode startup, and fail-closed behavior before bind.

The controller queue is now database-backed through the workflow-execution
store. The older SQLite ledger remains an attempt snapshot helper for legacy
skip/reuse code, but it is no longer the queue authority. Do not add retry rules
or broad workflow parsing until the workflow-execution store boundary remains
clear.

Operational observability slice 007 is now implemented: Python subprocess stdout
and stderr are replayed from the captured attempt logs into `internal/model`
`LogObservation` records via the worker logging client, with best-effort
delivery and fallback on failure.

For HPCC work, use the configured execution-environment path against the locally controlled Dockerized Slurm cluster as the next integration target. Keep the controller-worker ownership split intact: Slurm starts capacity, but workers still pull assignments from the controller. The four current roles are transport, dialect, scheduler, and runtime; future backends should add implementations behind those roles instead of reintroducing hard-coded worker target strings. SSH is now one concrete transport implementation for that boundary; it should remain transport-level plumbing, while setup/questionnaire behavior belongs in client setup code.

Operational slice 011 for data assets now has a validated fake-HPCC data-assets
smoke path. `scripts/fake-hpcc-data-assets-smoke.sh` starts the controller with
a configured execution environment that uses local transport, Bash dialect,
Slurm scheduling through `scripts/fake-hpcc/sbatch`, and `WorkerRuntime`
preparation. The controller writes a worker config containing `asset_cache_dir`
and `data_location_roots`, submits a generated worker Slurm script, and the
worker completes a source-reference Python workflow that references one named
fixture input, extracts one zip-selected archive member, promotes one CSV
artifact, publishes it to a named `published_data` root, and records artifact
plus published-asset evidence. The validated command was:

```bash
bash scripts/fake-hpcc-data-assets-smoke.sh
```

The runbook is
`docs/concepts/data-assets-and-materialized-outputs/fake-hpcc-data-assets-smoke.md`.
This smoke proves the local fake Slurm boundary only; real SSH, Dockerized
Slurm containers, and SingularityCE image execution remain separate future
fake-HPCC boundaries.
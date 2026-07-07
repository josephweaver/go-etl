# Source-Control Resolution And Cache State

Last updated: 2026-07-07

Completed Strategic Concept docs: [`../source-control-resolution-and-cache/README.md`](../source-control-resolution-and-cache/README.md)

This file preserves source-control and repository-cache current-state excerpts moved out of the root `PROJECT_STATE.md`.

## Current-State Excerpts
- Data assets and materialized outputs operational slice `012-cdl-yanroy-fixture-pipeline` is now implemented: the sibling `../go-etl-demo-project` has a tiny CDL/Yan/Roy field composition fixture with a ZIP-selected CDL CSV grid, a Yan/Roy-style field-ID CSV grid, a crop-code lookup CSV, a dominant-share policy JSON file, provider and publish-target declarations, a source-reference Python workflow, local/fake-HPCC submission references, and a standard-library `scripts/cdl_yanroy_fixture.py` script. The script reads resolved `${data.<alias>.local_path}` inputs, ignores declared background field IDs, computes field/year/crop composition rows, computes dominant crop assignments under `dominant_share_v1`, writes two artifacts under `GOET_ARTIFACT_DIR`, and declares both artifacts through `GOET_OUTPUT_JSON`. `scripts/cdl-yanroy-fixture-smoke.ps1` and `scripts/cdl-yanroy-fixture-smoke.sh` run local and fake-HPCC fixture smokes that verify source admission, data binding, worker materialization, ZIP extraction, Python execution, artifact promotion, publication, deterministic published CSV contents, and controller completion. Real CDL/Yan/Roy data, real Google Drive access, credentials, and geospatial dependencies remain out of scope.

The repository-source model, provider-read, cache-layout, cached-admission,
manifest-materialization, cache-pin, and source-declaration slices now exist in `internal/reposource`. That
package defines repository identity, resolved source references, admitted
source manifest records, file roles, slash-separated repository/path
validation, a narrow source `Provider` interface, GitHub and local filesystem
providers, raw file-byte SHA-256 evidence, admitted manifest construction for
caller-declared file sets, deterministic repository cache path derivation,
manifest-file lookup under controller-owned cache roots, cache publication,
verified cached reads, and local filesystem materialization from verified
cache reads. It also defines reconstructable workflow-run cache pin files under
local and GitHub cache entries. Workflow source documents can now include a
validated top-level `source_manifest` for supplemental Python entrypoint,
Python environment, and support files. Source-reference `/workflow` admission
now uses `internal/reposource` provider reads, admitted manifests, cache
publication, and verified cached reads before compiling workflow work.
Controller startup recovery now verifies active run source caches before opening
normal admission: it reloads the admitted manifest from the persisted run
context, reads cached project/workflow files through `reposource.CacheAccess`,
recomputes canonical JSON SHA-256, and compares those hashes with persisted
project/workflow rows. GitHub-backed cache misses or corruptions are repaired by
reading the recorded immutable revision and admitted source paths. Local-backed
cache misses or corruptions fail recovery with a provenance error and do not
reread local filesystem source files. The controller can now also serve a
read-only source bundle for an admitted workflow run at
`GET /workflow-runs/{run_id}/source-bundle.zip`. That endpoint reads the run's
persisted source-admission context, reloads the admitted manifest from the
controller-owned cache reference, and returns a zip containing only
worker-stageable `source_manifest` files (`python_entrypoint`,
`python_environment`, and `support_file`) using verified repository-cache reads.
It does not reread provider source files or expose controller cache filesystem
paths in the HTTP response. The bundle's `.goet/source-manifest.json` entry is
now a worker-facing sanitized manifest that omits `CachePath` and other
controller filesystem details.
`internal/model/work_item.go` now also carries `WorkItemTypePythonScript = "python_script"` and the optional `WorkItem.Source` / `WorkItemSource` locator used for admitted Python execution. `WorkItem.Validate()` remains strict and still requires a source locator for `python_script` items in raw/controller-submitted and queued worker payloads. `WorkItem.ValidateForWorkflowCompile()` is now the narrow internal compile-time boundary used by `internal/workflow` so workflow compilation can emit a source-less `python_script` item only as an intermediate result before controller admission attaches `Source` and reruns strict validation. The worker now has a source-bundle staging helper that downloads `GET /workflow-runs/{run_id}/source-bundle.zip` and extracts safe entries into attempt-local `source/`, `work/`, and `logs/` directories under `TmpDir`.
Source-reference workflow admission in `cmd/controller` now validates compiled `python_script` work items against admitted `source_manifest` facts before insertion. The controller requires `python_entrypoint`, optionally validates `python_environment`, checks both paths against admitted manifest roles instead of live provider state, replaces any would-be workflow-authored source locator with a controller-generated `WorkItem.Source`, and then reruns strict `WorkItem.Validate()` before persisting and queueing the work item payload.
`cmd/worker` now also dispatches `python_script` items through a subprocess runner that stages admitted source first, requires `python_entrypoint`, optionally accepts `python_environment` and `python_args`, writes `work/input.json`, captures stdout and stderr under the attempt log directory, and promotes the script's `work/output.json` into the worker data directory. The runner defaults to `python3` when `Config.PythonExecutable` is unset.
PW-005 is now complete: the Python runner decodes exactly one `GOET_OUTPUT_JSON` document, rejects invalid or trailing content, canonicalizes the logical output, promotes the canonical output into `DataDir/{output_filename}`, and returns wrapper evidence with top-level `input_sha256` / `output_sha256` plus optional stdout/stderr hashes.

Client-facing demo project artifacts now live in the sibling `../go-etl-demo-project`
repository. That repo owns source-control-style customer files such as
`project.json`, workflow documents under `workflows/`, demo run submissions under
`submissions/`, and demo input data under `data/`. The reusable Go ETL repo keeps
runtime code, tests, scripts, and low-level worker fixtures such as
`demo-item.json`.

The sibling demo repo now also includes a minimal `python-hello` fixture that
proves the source-admission-to-Python-execution vertical slice with local source
admission, a system-Python placeholder, and a small standard-library script.
The first Python WorkItem phase is implemented end to end: shared
`python_script` work-item validation, controller source-bundle delivery, worker
staging, subprocess execution, canonical output promotion, workflow admission
validation, the sibling demo fixture, and the repeatable local smoke path.
Later Python work remains intentionally deferred to separate concepts or later
phases for environment management, execution observability, submission CLI
status, dependency-aware workflows, resource constraints, and Python SDK/client
behavior.

```text
go.mod
demo-item.json
.gitignore
docs/
  fake-hpcc.md
  sqlite-ledger.md
scripts/
  fake-hpcc/
    run-demo
    sbatch
internal/
  ledger/
    sqlite.go
    sqlite_test.go
  client/
    local_controller.go
    local_controller_test.go
    workflow.go
    workflow_test.go
  clientsetup/
    ssh_setup.go
    ssh_setup_test.go
  model/
    work_item.go
    work_item_test.go
  reposource/
    cache_access.go
    cache_access_test.go
    cache_layout.go
    cache_layout_test.go
    cache_pin.go
    cache_pin_reconstruction.go
    cache_pin_reconstruction_test.go
    cache_pin_test.go
    cache_publish.go
    cache_publish_test.go
    cache_verify.go
    cache_verify_test.go
    github_provider.go
    github_provider_test.go
    local_provider.go
    local_provider_test.go
    manifest.go
    manifest_test.go
    materialize.go
    materialize_test.go
    model.go
    model_test.go
    path.go
    path_test.go
    provider.go
    provider_test.go
    source_declaration.go
    source_declaration_test.go
  workflow/
    fanout.go
    fanout_test.go
    step.go
    step_test.go
    workflow.go
    workflow_test.go
  variable/
    literal.go
    literal_test.go
    accessor.go
    accessor_test.go
    name.go
    name_test.go
    namespace.go
    namespace_test.go
    reference.go
    reference_test.go
    resolver.go
    resolver_test.go
    scope.go
    scope_test.go
    type.go
    type_test.go
    variable.go
    variable_test.go
cmd/
  demo-client/
    main.go
  controller/
    bash_shell_platform.go
    bash_shell_platform_test.go
    main.go
    main_test.go
    config.go
    config_test.go
    controller-default-config.json
    docker_slurm_submit.go
    docker_slurm_submit_test.go
    worker_launch_config.go
    worker_launch_config_test.go
    docker_transport.go
    docker_transport_test.go
    fake-hpcc-ssh-config.json
    execution_environment.go
    execution_environment_test.go
    local_worker.go
    local_worker_test.go
    preparer.go
    preparer_test.go
    runtime.go
    runtime_test.go
    scheduler.go
    shell_dialect.go
    ssh_transport.go
    ssh_transport_test.go
    ssh_transport_integration_test.go
    slurm_scheduler.go
    slurm_scheduler_test.go
    slurm_worker_script.go
    slurm_worker_script_test.go
    transport.go
    worker_scaler.go
    worker_scaler_test.go
    README.md
    demo-config.json
  worker/
    main.go
    main_test.go
    config.go
    config_test.go
    state.go
    state_test.go
    worker.go
    worker_test.go
    work_demo.go
    work_demo_test.go
    demo-config.json
    .run/
      logs/
      tmp/
      data/
```

1. Start the controller on `:8080`.
2. A client submits a source-reference workflow run to `POST /workflow`.
3. The controller resolves the referenced project/workflow JSON, persists
   provenance, compiles initially ready work, and stores queued work in the
   workflow-execution database.
4. Start the worker from `cmd/worker`.
5. The worker loads `demo-config.json`.
6. The worker validates required runtime directories.
7. The worker requests `GET /work/next`.
8. The controller claims one queued row into running work and returns the worker
   payload JSON.
9. The worker validates and dispatches the item by `Type`.
10. The demo handler writes temporary output under `TmpDir`.
11. The demo handler renames completed output into `DataDir`.
12. The worker reports success with `POST /work/complete`, or failure with
    `POST /work/fail`, including the assigned `attempt_id`.
13. The worker asks for more work.
14. The worker exits cleanly when `GET /work/next` returns `204 No Content`.

```text
GET  /work/next      assign the next pending item, or return 204
POST /work/complete  mark an assigned item complete
POST /work/fail      record failure for an assigned item
POST /work           submit one raw work item
POST /workflow       submit source references for project and workflow JSON; success returns 202 with submission acknowledgement JSON
GET  /workflow-runs/{run_id}/source-bundle.zip  return admitted staged source files as a zip bundle
GET  /submissions/{submission_id}/status  return per-submission execution status
POST /shutdown       ask the controller process to shut down
GET  /status         return queue counts
```

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

- Controller source-bundle endpoint behavior for admitted Python source files,

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

The final 012f4 cleanup converted the remaining skipped inline `/workflow`
tests to source-reference coverage for general workflow admission, submitted
code version, Singularity runtime, invalid worker scale config, and duplicate
generated work-item IDs. `cmd/controller/main_test.go` now has no skipped tests,
and the persistence epic is ready for implementation review rather than further
feature expansion.

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

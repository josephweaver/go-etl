# cmd/controller

This directory owns the local Go controller executable.

The controller is the current control plane for local workflow runs. It accepts workflow submissions, normalizes dependency stages, compiles dependency-ready stages into concrete work items, owns queue state, coordinates CareTaker-driven worker launch when configured, records completed attempts, and exposes HTTP endpoints used by clients and workers.

It is not the worker runtime, Python-facing API, reusable workflow language, variable system, or ledger implementation. It coordinates those pieces from the process boundary.

## Files

- `main.go` owns the controller process, HTTP API surface, durable queue lifecycle, workflow submission handling, submission acknowledgement, submission-scoped status reporting, dependency-aware status rendering, runtime metadata attachment, ledger write coordination, and shutdown hook.
- `config.go` owns loading controller startup configuration into typed variables.
- `local_worker.go` owns the local worker-starting adapter used by the controller.
- `worker_execution.go` owns the worker execution policy and in-memory inflight start reservations used by the CareTaker reconciler.
- `caretaker.go` owns the event-driven CareTaker reconciliation loop and controller adapters for worker recovery, capacity snapshots, and launch.
- `demo-config.json` is the local demo controller configuration.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Controller process boundary.
- HTTP API for work assignment, completion, failure, workflow submission, submission acknowledgement, submission-scoped status, and shutdown.
- Resource-aware claim behavior: dependency-ready queue candidates must also satisfy controller-evaluated resource predicates to be assigned.
- Durable queued, running, completed, and failed work state through the workflow-execution store.
- Workflow submission orchestration, including initial ready-stage queueing and just-in-time downstream stage activation.
- Controller-generated runtime metadata for work items.
- CareTaker-owned worker startup reconciliation.
- Controller-owned ledger write coordination.
- Local aggregate status summary for clients and demos.
- Submission-scoped status for `goet status <submission_id>`.
- Submission log read API `GET /submissions/{submission_id}/logs`.
- Dependency transition observations for `goet logs <submission_id>` when logging is enabled.

## Concepts Owned Elsewhere

- Workflow shape and fan-out compilation belong in `internal/workflow`.
- Work-item, completion, failure, and status transport shapes belong in `internal/model`.
- Typed variables, precedence, and resolution belong in `internal/variable`.
- SQLite schema and durable attempt storage belong in `internal/ledger`.
- Worker execution, output promotion, and reporting belong in `cmd/worker`.
- Client startup, submission, polling, and shutdown behavior belong in `internal/client`.

## Invariants

- The controller owns queue transitions; workers pull assignments and report outcomes.
- Direct SQLite access stays inside the controller process through `internal/ledger`.
- Workflow submission is the target boundary; raw work submission is local administrative/test support.
- Runtime configuration is resolved through typed variables rather than a separate hidden config authority.
- Workers should receive concrete work-item parameters and metadata, not unresolved workflow intent.
- Worker startup is bounded by the configured worker execution pattern, live worker sessions, inflight starts, and claimable queued work.
- Queue state is stored in the workflow-execution database; a controller without a workflow store rejects queue endpoints.
- Workflow submission queues only dependency-ready work. Sequential downstream stages are not assignable until their predecessor stage completes successfully.
- `parallel_with` groups only adjacent steps with the same label; non-contiguous label reuse is rejected before queue mutation.
- Successful workflow admission returns a submission acknowledgement with `submission_id`, `workflow_id`, and initial work-item count.
- `GET /submissions/{submission_id}/status` is the controller-owned status endpoint for one submission.
- `GET /submissions/{submission_id}/logs` is the controller-owned log-read endpoint.
- Queue visibility and status include resource-blocked queued work separately from generic pending work.

## Dependency Smoke

Run the dependency-aware workflow smoke path from the repository root:

```powershell
powershell -NoProfile -File scripts/dependency-aware-workflow-smoke.ps1
```

The smoke path uses the sibling `../go-etl-demo-project` local source provider, `POST /workflow`, `GET /work/next`, `POST /work/complete`, `goet status --json`, and `goet logs --json` to prove sequential readiness, contiguous `parallel_with` readiness, and invalid non-contiguous `parallel_with` rejection.

## Major Dependencies

- `net/http` for the controller API.
- `sync` for protecting controller admission and recovery flags.
- `database/sql` through the configured ledger handle.
- `internal/workflow` for workflow compilation.
- `internal/model` for shared HTTP payloads.
- `internal/variable` for runtime configuration and submission variables.
- `internal/ledger` for durable attempt recording and prior-attempt lookup.
- `os/exec` for local worker startup.

## Bootup Sequence

main() (main.go)
├── buildControllerServer
|   ├── parseControllerStartupOptions - get config path, override options
|   ├── controllerConfigPath - choose explict config, or default config
|   ├── loadControllerStartupSources - constructs config/default
|   |   ├── loadControllerConfig - reads config file, convert to json, validate envelope, validate config
|   |   |   ├── ControllerConfig.validateEnvelope()
|   |   |   ├── ControllerConfig.Validate()
|   |   |   └── return ControllerConfig()
|   |   ├── defaultsPathForControllerConfig() - resolve path for defaults.json
|   |   ├── loadDefaultsDocument() - reads config file, convert to json, validate envelope, validate config
|   |   |   ├── DefaultsDocument.validateEnvelope()
|   |   |   ├── DefaultsDocument.Validate()
|   |   |   └── return DefaultsDocument()
|   |   └── controllerStartupSources()
|   ├── parseControllerStartupOverrides - converts overrides to override scope.
|   ├── newStartupRuntimeScope - defines processID, instanceID, startedAt, buildVersion 
|   |   └── variable.Create()
|   ├── newControllerStartupResolver - creates a variable resolver
|   |   ├── variable.NewSet(defaultScope, controllerScope, overrideScope, runtimeScope)
|   |   ├── variable.ResolverConfig
|   |   └── return variable.NewResolver()
|   ├── resolveControllerHTTPSettings - convert config to HttpSettings
|   |   ├── resolveStringPolicy
|   |   ├── resolvePositiveIntPolicy
|   |   └── controllerHTTPSettings
|   ├── resolveControllerAuthSettings
|   |   ├── authValue <- Resolve("authentication")
|   |   ├── authConfig <- controllerauth.ConfigFromResolved(authValue)
|   |   ├── resolveBoolPolicy(resolver)
|   |   |   ├── 
|   |   ├── validateControllerAuthStartup()
|   |   ├── controllerauth.LoadCredentials(authConfig, sources)
|   |   └── return controllerAuthSettings
|   ├── initWorkflowExecutionStore
|   ├── acquireControllerDatabaseOwnershipForPath
|   ├── getwd()
|   ├── resolveControllerFilesystemPaths
|   ├── resolveControllerOperationalPolicy
|   ├── initConfiguredExecutionEnvironment
|   └── return newController()

...

    controller.completeStartupRecovery
    caretakerConfig<-controllerCareTakerConfig
    return server


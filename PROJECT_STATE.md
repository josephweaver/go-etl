# Project State

Last updated: 2026-06-30

## Current Focus

We now have a minimal local Go controller and worker runtime with the first SQLite-backed attempt ledger. The controller owns an in-memory work queue and owns all direct SQLite access. The worker loads local runtime config, repeatedly pulls assigned work over HTTP, dispatches supported work-item types, writes completed output through mounted-style local directories, and reports completion or failure.

The current HPCC-facing work has shifted from a command-backed `hpcc` worker target toward a configurable execution-environment model. The controller now requires serialized controller documents to declare `api_version: goet/v1alpha1` and `kind: Controller` before it validates variables or execution-environment settings. It can load `cmd/controller/controller-default-config.json`, build an `ExecutionEnvironment`, store it on `Controller.env`, prepare its components, and submit worker jobs through a scheduler. The default configured chain is:

```text
transport = DockerContainerTransport backed by DockerTransport
dialect   = BashShellPlatform
scheduler = SlurmScheduler
runtime   = WorkerRuntime
```

This represents the locally controlled Dockerized Slurm fake-HPCC backend. Docker provides the default transport into the Slurm control container, Bash describes the command/path/string dialect inside that environment, Slurm schedules worker jobs, and WorkerRuntime prepares the worker-side filesystem/config/artifact locations. The repository now also has an SSH transport path that can connect to a target host with key-based authentication, copy files, list remote directories, run commands, perform basic filesystem operations, and reconnect conservatively after session/client-open failures. `cmd/controller/fake-hpcc-ssh-config.json` is the first controller config for reaching Fake HPCC through SSH instead of Docker. The repository still has a tiny fake `sbatch` smoke-test command and fake-HPCC demo runner, but those should remain fallback fixtures rather than grow into the main scheduler path.

The first client-side SSH setup engine now lives in `internal/clientsetup`. It can run a questionnaire through an injected prompter, generate local Ed25519 key material under `.run/goetl/ssh`, and write a generated controller config under `.run/goetl/generated`. It does not yet install the public key into the target user's `authorized_keys`, write an OpenSSH `known_hosts` file, or expose the questionnaire through `cmd/demo-client`.

The target product still has a reusable Python interface that submits external pipeline/config files to a Go controller on backends such as HPCC. The current implementation is a local runtime foundation, not the intended user-facing API.

Project guidance is in `AGENTS.md`. The longer product and architecture direction is in `TARGET_STATE.md`.

## Development Governance

`EPI_CTL.md` now uses a three-category epistemic-control model:

```text
Strategic Understanding (SU) /20
Operational Control (OC) /10
Implementation Recall (IR) /5
Surprise Penalty -/5
Total EC /35
```

The protocol distinguishes architectural and causal understanding from practical codebase control and from short- or medium-term recall of implementation details. Low implementation recall is explicitly acceptable when Strategic Understanding and Operational Control remain strong.

`EPI_CTL.md` also now defines longitudinal retention reviews. Same-day audits are `T`; follow-up retention-chain reviews are `T+3`, `T+14`, and `T+180`, named with the original session date, such as:

```text
epi_ctl/20260624.md
epi_ctl/20260624_T3.md
epi_ctl/20260624_T14.md
epi_ctl/20260624_T180.md
```

Retention reviews are first-class audits and are treated as the primary evidence for durable ownership. The protocol also records Codex usage indicators and ActivityWatch distraction/context-switch metrics when available.

## Current Layout

```text
go.mod
demo-workflow.json
demo-fake-hpcc-workflow.json
demo-summary-workflow.json
demo-summary-input.txt
demo-summary-input-2.txt
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

The repository uses one root Go module:

```go
module goetl
```

Shared controller-worker JSON contracts live in `internal/model`.

## Runtime Flow

The local runtime flow is:

1. Start the controller on `:8080`.
2. The controller creates an in-memory queue with one demo work item.
3. Start the worker from `cmd/worker`.
4. The worker loads `demo-config.json`.
5. The worker validates required runtime directories.
6. The worker requests `GET /work/next`.
7. The controller moves one item from `pending` to `assigned`.
8. The worker validates and dispatches the item by `Type`.
9. The demo handler writes temporary output under `TmpDir`.
10. The demo handler renames completed output into `DataDir`.
11. The worker reports success with `POST /work/complete`, or failure with `POST /work/fail`.
12. The worker asks for more work.
13. The worker exits cleanly when `GET /work/next` returns `204 No Content`.

## Controller

The controller stores three in-memory collections:

```text
pending    work that can be assigned
assigned   work currently owned by a worker
failed     failed work and its error text
```

Access is protected by `sync.Mutex` so later concurrent workers can safely use the queue.

Current endpoints:

```text
GET  /work/next      assign the next pending item, or return 204
POST /work/complete  mark an assigned item complete
POST /work/fail      record failure for an assigned item
POST /work           submit one raw work item
POST /workflow       submit one tiny workflow and variables
POST /shutdown       ask the controller process to shut down
GET  /status         return queue counts
```

Completed items are removed from `assigned`. Failed items are removed from `assigned` and stored in `failed`. Queue state is currently process-local and is lost when the controller exits.

If started with `--config`, the controller loads that variable document and normalizes all variables into `controller_config`. A relative explicit path remains relative to the process working directory. If no config path is supplied, the controller loads:

```text
controller.json
```

from the directory containing the running executable. It does not search the process working directory or source tree. Repository development commands use `--config` because `go run` places its temporary executable outside the repository.

`cmd/controller/defaults.json` now contains the agreed canonical
`controller_config` defaults under a `goet/v1alpha1` / `Defaults` envelope. The
controller package can load and validate this document, including its allowed
configuration namespaces and per-namespace duplicate keys. It can also retain
the defaults and selected controller documents as distinct sources and produce
ordered `controller_config` scopes where explicit controller declarations win.
Controller documents now reject non-`controller_config` declarations instead
of silently rewriting their namespaces. `main` does not yet consume these
retained scopes for service construction.

The demo client starts the controller with:

```powershell
go run ./cmd/controller --config ./cmd/controller/demo-config.json
```

`cmd/controller/demo-config.json` currently defines:

```text
controller_config.controller_url
controller_config.ledger_db_path
```

`cmd/controller/controller-default-config.json` currently defines those same controller variables plus an `execution_environment` block for the Dockerized Slurm backend:

```text
name = dockerized-slurm
transport = docker container slurmctld
dialect = bash
scheduler = slurm
runtime = worker rooted at /data/goetl
worker controller_url = http://host.docker.internal:8080
```

When `controller_config.ledger_db_path` is present, the controller opens or creates the SQLite ledger, initializes the version 1 schema, and stores the DB handle on the `Controller`. The controller remains the only process that talks directly to SQLite. Clients and workers interact through HTTP APIs.

When `execution_environment` is present, the controller builds an `ExecutionEnvironment` and stores it on `Controller.env`. The current environment is assembled from four role interfaces:

- `Transport` copies files into and executes commands inside a target environment.
- `ShellDialect` localizes paths, newlines, and shell quoting for the target command dialect.
- `Scheduler` submits prepared jobs.
- `Runtime` prepares the worker process environment that the scheduler will start.

The current concrete implementations are `DockerContainerTransport`, `SSHTransport`, `BashShellPlatform`, `SlurmScheduler`, and `WorkerRuntime`.

`POST /work` is useful for internal testing and local administration, but it is not the intended customer-facing submission boundary. The target submission boundary is workflow submission. The controller will eventually compile submitted workflows into concrete work items.

`POST /workflow` currently accepts JSON containing a workflow and optional submitted variables. Workflow-scope variables live inside the workflow object. Top-level submitted variables are reserved for overrides and runtime/config variables. The controller builds variable scopes from workflow variables and submitted variables, compiles the workflow through `internal/workflow`, checks generated work-item IDs against the existing queue state, and appends generated work items to the pending queue.

The current workflow compiler eagerly compiles and queues every submitted step;
it does not retain per-workflow resolver context, track step dependencies, or
compile downstream steps after predecessor completion. The proposed
`dependency-aware-workflows` epic records this correctness gap and is a
prerequisite for the resource-constraint epic's workflow-eligibility gate.
The proposed `workflow-dependency-resolution` epic separately owns lookup of
dependent workflow definitions from a GitHub repository and cross-workflow
readiness after workflow-instance lifecycle and typed outputs exist.
The proposed `workflow-execution-persistence` epic owns database-backed run,
step, work-item, attempt, configuration-snapshot, output, and restart state.
The proposed `attempt-liveness-recovery` epic owns worker heartbeat leases, the
controller caretaker loop, fencing, and abandoned-attempt recovery. Both are
prerequisites consumed by dependency-aware workflow execution.

After workflow submission creates pending work, the controller uses worker-scaling state to decide how many workers to start. If `Controller.env` is configured, `submitWorkflowHandler` prepares the execution environment and asks `env.Scheduler` to submit worker jobs. The Slurm path generates a worker Slurm script using the configured shell dialect, copies the generated script through the transport, and submits it through `sbatch` inside the Dockerized Slurm control container.

The older `LocalWorkerStarter` remains in the repository for the local process path and tests, but the current target path is the configured execution-environment model rather than hard-coded worker target strings.

`GET /status` currently reports pending, assigned, failed, pending reuse-candidate, attempt, and attempt-variable counts. Attempt and reuse-candidate counts are zero when the controller has no configured ledger.

`POST /work/complete` still accepts legacy completion payloads containing only `id`. When a completion payload includes full attempt metadata, the controller converts it into a `ledger.Attempt` and records it in SQLite before removing the item from `assigned`. The stored attempt snapshot now includes runtime variables for workflow definition, workflow fingerprint, workflow instance, step definition, step fingerprint, step instance, work-item ID, work-item fingerprint, input fingerprint, output fingerprint, code version, attempt ID, started time, and completed time. Completion payload parameters are stored as `work_item` variables so the ledger records the resolved inputs used by the worker.

The controller has small read, comparison, decision, marker, and skipped-attempt helpers for idempotency groundwork. `priorCompletedAttempt` asks the ledger for the latest completed attempt matching a work-item fingerprint. `priorCompletedAttemptMatchesWorkItem` checks that the prior attempt was completed and that work-item, input, output, and code-version fingerprints still match the current assignment. `reusablePriorAttempt` composes those checks into a single controller question. `workReuseDecision` returns an observational decision with reason strings such as `no_prior_completed_attempt`, `prior_attempt_mismatch`, and `matched_prior_completed_attempt`. `workSkipForReuseDecision` can build a validated `WorkSkip` marker from a positive reuse decision. `skippedAttemptFromWorkSkip` can build a skipped `ledger.Attempt` snapshot with `runtime.prior_attempt_id` and `runtime.skip_reason`. `recordSkippedAttempt` can persist that skipped snapshot when called explicitly. `/status` reports how many pending work items are currently reuse candidates, derived from pending reuse-decision reason counts. `/work/next` now records and removes reusable pending items as skipped attempts before assigning the next non-reusable item.

`POST /shutdown` currently invokes a controller shutdown hook. In local client-started runs, the client should poll `GET /status` and call this endpoint when pending and assigned counts both reach zero.

## SQLite Ledger

`internal/ledger` contains the first SQLite-backed attempt ledger.

Current schema tables:

```text
schema_version
attempts
attempt_variables
```

The ledger supports:

- Opening SQLite databases through `OpenSQLite`.
- Creating missing parent directories for file-backed database paths.
- Initializing the version 1 schema through `InitSQLiteSchema`.
- Inserting one attempt and its variable snapshot transactionally through `InsertAttempt`.
- Finding the latest completed attempt for a work-item fingerprint through `FindLatestCompletedAttemptByWorkItemFingerprint`.
- Storing `completed`, `failed`, and `skipped` attempt statuses. Skipped attempts can link to the reused prior attempt through runtime variables such as `runtime.prior_attempt_id` and `runtime.skip_reason`.

The first local demo ledger is created at:

```text
.run/controller/ledger.sqlite
```

The current local demo ledger was re-verified on 2026-06-16. Starting from the earlier demo ledger count of four attempts and twenty-four variables, one new demo run added two attempts and twenty runtime variables and printed:

```text
final status: pending=0 assigned=0 failed=0 pending_reuse_candidates=0 attempts=6 attempt_variables=44
```

That corresponds to six total demo fan-out work items in the existing ledger at the time of that verification. New runs store fourteen runtime variables per completed attempt.

## Worker Config

The worker loads `cmd/worker/demo-config.json`:

```json
{
  "log_dir": ".run/logs",
  "tmp_dir": ".run/tmp",
  "data_dir": ".run/data",
  "controller_url": "http://localhost:8080"
}
```

The matching Go model is:

```go
type Config struct {
	LogDir        string `json:"log_dir"`
	TmpDir        string `json:"tmp_dir"`
	DataDir       string `json:"data_dir"`
	ControllerURL string `json:"controller_url"`
}
```

The local paths are relative to the directory where the worker is run.

## Shared Models

`internal/model/work_item.go` defines:

```go
type WorkItem struct {
	ID                   string       `json:"id"`
	Type                 WorkItemType `json:"type"`
	OutputFilename       string       `json:"output_filename"`
	Parameters           Parameters   `json:"parameters,omitempty"`
	WorkflowDefinitionID string       `json:"workflow_definition_id,omitempty"`
	WorkflowFingerprint  string       `json:"workflow_fingerprint,omitempty"`
	WorkflowInstanceID   string       `json:"workflow_instance_id,omitempty"`
	StepDefinitionID     string       `json:"step_definition_id,omitempty"`
	StepFingerprint      string       `json:"step_fingerprint,omitempty"`
	StepInstanceID       string       `json:"step_instance_id,omitempty"`
	WorkItemFingerprint  string       `json:"work_item_fingerprint,omitempty"`
	InputFingerprint     string       `json:"input_fingerprint,omitempty"`
	OutputFingerprint    string       `json:"output_fingerprint,omitempty"`
	CodeVersion          string       `json:"code_version,omitempty"`
}

type WorkCompletion struct {
	ID                   string     `json:"id"`
	AttemptID            string     `json:"attempt_id,omitempty"`
	WorkflowDefinitionID string     `json:"workflow_definition_id,omitempty"`
	WorkflowFingerprint  string     `json:"workflow_fingerprint,omitempty"`
	WorkflowInstanceID   string     `json:"workflow_instance_id,omitempty"`
	StepDefinitionID     string     `json:"step_definition_id,omitempty"`
	StepFingerprint      string     `json:"step_fingerprint,omitempty"`
	StepInstanceID       string     `json:"step_instance_id,omitempty"`
	WorkItemFingerprint  string     `json:"work_item_fingerprint,omitempty"`
	InputFingerprint     string     `json:"input_fingerprint,omitempty"`
	OutputFingerprint    string     `json:"output_fingerprint,omitempty"`
	CodeVersion          string     `json:"code_version,omitempty"`
	StartedAt            string     `json:"started_at,omitempty"`
	CompletedAt          string     `json:"completed_at,omitempty"`
	Parameters           Parameters `json:"parameters,omitempty"`
}

type WorkFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type WorkSkip struct {
	ID             string `json:"id"`
	PriorAttemptID string `json:"prior_attempt_id"`
	Reason         string `json:"reason"`
}
```

`Parameters` is a map of resolved work-item parameter names to typed JSON values. It is the first transport slot for concrete worker inputs such as input paths, output roots, tile IDs, and other already-resolved values. The worker should receive concrete parameters here rather than resolving workflow expressions locally.

Workflow-compiled work items now include optional controller-generated runtime identity metadata before they enter the pending queue:

```text
workflow_definition_id
workflow_fingerprint
workflow_instance_id
step_definition_id
step_fingerprint
step_instance_id
work_item_fingerprint
input_fingerprint
output_fingerprint
code_version
```

Workflow-generated workflow, step, work-item, input, and output fingerprints are deterministic SHA-256 labels over resolved assignment content. The workflow fingerprint currently hashes the workflow definition ID. The step fingerprint currently hashes the workflow fingerprint plus step definition ID. The work-item fingerprint includes ID, type, output filename, and parameters. The input fingerprint currently hashes the resolved parameter map. The output fingerprint currently hashes the resolved output filename. Raw work-item submissions may still omit these fields for local administration and tests. The worker echoes assignment metadata into `POST /work/complete` when present and falls back to demo values only for legacy/raw assignments.

Workflow-generated assignments set `code_version` from the resolved variable `code_version` when present, so launchers may submit values such as `override.code_version` or `controller_config.code_version`. If no variable is present, the controller falls back to Go build VCS metadata. If the Go toolchain did not embed a revision, the controller records `unknown`. A dirty working tree appends `-modified`.

`WorkItem.Validate()` checks structural validity:

- A non-empty ID.
- A non-empty type.
- A non-empty output filename.
- An output filename without directory components.
- Parameter names, types, and values when parameters are present.

Operation support is separate from structural validity. The worker dispatcher rejects unsupported operation types.

`WorkSkip` is a shared marker shape for skip behavior. It is currently used inside the controller to build skipped attempt snapshots when `/work/next` determines that a pending item can reuse a prior completed attempt.

## Variable Model

`internal/variable` contains the early typed-variable model and resolver foundation.

Current canonical variable namespaces, from lowest to highest precedence, are:

```text
global_config
client_env
controller_env
worker_env
client_config
controller_config
worker_config
project_config
workflow
override
step
work_item
runtime
```

The legacy namespaces `global`, `backend`, and `project` remain valid during migration but are no longer the target model.

Unqualified references use precedence lookup. Qualified references such as `worker_env.GDAL_DATA` explicitly select a namespace.

Current variable types include:

```text
string
int
bool
datetime
path
object
list
```

`list` is now a generic collection type. Each resolved item retains its own
type, so empty, heterogeneous, and nested lists are valid. Consumers that need
a narrower shape, such as a string list, validate every item at their boundary.

`TypedExpression` now defines the recursive language-neutral JSON node used by
the structured-variable target model. Every node serializes with compact
`type` and `expression` fields. Object expressions decode to named child nodes,
list expressions decode to ordered independently typed child nodes, and scalar
JSON values retain their serialized shape. `Variable` is now a name plus an
embedded root `TypedExpression`, serialized through flat lowercase `name`,
`type`, and `expression` fields. Repository-owned workflow and controller JSON
fixtures use this form; legacy raw-JSON structured expressions are rejected.

Typed expressions now support context-free definition validation. Validation
checks scalar literal shapes, datetime syntax, whole-value reference grammar,
string and path interpolation tokens, scalar accessors, and every recursive
object or list child. It does not look up variables or require controller,
project, workflow, or override scopes; those contextual checks remain resolver
work.

Current resolver behavior supports:

- Typed scalar literal parsing.
- Recursive conversion of explicitly typed object and list literal nodes into
  resolved values without nested type inference.
- Variable precedence merging.
- Qualified and unqualified references.
- Recursive resolution with a configurable maximum depth.
- Whole-value references at the variable root or inside any object field or
  list item, with declared-type checking after supported accessors.
- String and path interpolation of canonical scalar text at any structured
  depth, while preserving the enclosing expression type.
- Structured resolution diagnostics that identify the qualified root and
  escaped JSON Pointer node path, with distinct cycle-chain and maximum-depth
  failures.
- Escaped variable references such as `\${year}`.
- Scalar structured access in reference expressions, such as `${record.year}` and `${years[0]}`.
- Fan-out list access through `Resolver.ResolveFanOutExpression("${years[*]}")`.
- Typed convenience accessors for required and optional variables, including string, path-or-string, object, and string-list values.
- Optional object-field helpers for resolved object settings used by layer-specific worker launch config.
- Lazy string-only `controller_env` lookup through an injected function. Each
  bounded resolver caches present and missing keys without enumerating the
  process environment; resolver copies share that concurrency-safe cache.

Structured access remains intentionally small. Literal object fields and list items declare their own types and resolve into the existing `ResolvedValue` tree. Whole-value references resolve recursively at any structured node through normal namespace precedence while preserving the referencing node's declared type. Scalar access supports `.field` and `[index]`. Fan-out supports only `[*]` and returns a list of resolved values for later workflow compilation. Mixed-text interpolation resolves string, path, int, bool, and datetime values into string or path expressions; it rejects object and list values and does not reinterpret reference syntax produced by a resolved value. Resolution failures retain their underlying cause while reporting the qualified root variable and an escaped JSON Pointer node path. Active qualified reference chains distinguish cycles from long acyclic chains that exceed the configured depth.

The structured-variable-resolution epic is complete. A controller integration
test now proves that separately assembled project and worker scopes resolve
recursive typed expressions into the concrete transport, scheduler, runtime,
path, and string-list values consumed by `workerLaunchConfig`.

Runtime configuration must flow through the variable subsystem. Controller settings, worker settings, backend choices, command-line flags, API arguments, and client overrides should be represented as typed variables with clear namespaces and sources. Config structs and HTTP JSON fields are transport surfaces, not a separate configuration authority.

Important near-term runtime variables include:

```text
controller_config.controller_url
controller_config.controller_start_executable
controller_config.controller_start_args
controller_config.controller_start_lock_path
controller_config.ledger_db_path
controller_config.code_version
worker_config.worker_target_environment
worker_config.transport
worker_config.dialect
worker_config.scheduler
worker_config.runtime
worker_config.worker_min_count
worker_config.worker_max_count
worker_config.worker_count_per_start
worker_config.worker_min_elapsed_time_between_starts
worker_config.client_status_poll_interval
override.controller_url
override.controller_start_executable
override.controller_start_args
override.controller_start_lock_path
override.ledger_db_path
override.worker_target_environment
override.worker_start_executable
override.worker_start_args
override.worker_min_count
override.worker_max_count
override.worker_count_per_start
override.worker_min_elapsed_time_between_starts
override.client_status_poll_interval
override.code_version
```

The local Go client currently uses `controller_config` variables to start the local controller. Worker launch settings are moving toward structured layer-owned `worker_config` object variables. The current launch resolver prefers `worker_config.transport`, `worker_config.scheduler`, and `worker_config.runtime` object settings, while still accepting older flat variables such as `worker_config.worker_script_path`, `worker_config.worker_start_executable`, `worker_config.worker_config_path`, and `worker_config.worker_log_dir` as compatibility fallbacks. Future client/API arguments may still submit `override` variables when the caller intentionally overrides config.

Workflow identity, step identity, work-item identity, attempt identity, code version, and fingerprints must flow through the variable subsystem. Future durable storage, likely SQLite for local execution, should persist typed variable snapshots rather than create a separate identity/configuration model.

Important generated runtime variables for idempotency and traceability should include:

```text
runtime.workflow_definition_id
runtime.workflow_instance_id
runtime.workflow_fingerprint
runtime.step_definition_id
runtime.step_instance_id
runtime.step_fingerprint
runtime.work_item_id
runtime.work_item_fingerprint
runtime.attempt_id
runtime.code_version
runtime.input_fingerprint
runtime.output_fingerprint
runtime.completed_at
```

SQLite tables may expose common IDs and fingerprints as convenience columns for indexing, but those columns should mirror typed variables with namespace, type, value, source, and lifecycle. Verified skip decisions should compare the current resolved variables against a prior successful attempt's stored variables; an output filename alone is not enough.

The ledger now has the first read-side helper for this skip path: it can find the latest completed attempt matching a work-item fingerprint. The controller can call this through its own ledger adapter and compare the prior attempt against the current assignment through `reusablePriorAttempt`. The ledger can store skipped attempt snapshots, and `/work/next` creates them when a pending item is reusable.

The next controller scheduler should use a conservative organic worker-scaling model:

- Start at most `worker_count_per_start` workers in one decision.
- Keep started/active workers at or above `worker_min_count` while pending work exists.
- Never exceed `worker_max_count`.
- For organic scale-up above the minimum floor, wait at least `worker_min_elapsed_time_between_starts`.
- For organic scale-up above the minimum floor, wait until the previous worker start is confirmed by assigned work increasing.

Early local defaults should be `worker_min_count = 0`, `worker_max_count = 2`, `worker_count_per_start = 1`, and `worker_min_elapsed_time_between_starts = "30s"`. A workflow known to require fast parallel startup can set `worker_min_count = 10`, `worker_max_count = 10`, and `worker_count_per_start = 10` to request ten workers immediately, still bounded by pending work and the hard maximum.

`cmd/controller/worker_scaler.go` contains the first worker-scaling decision state. It plans how many workers to start from pending count, assigned count, started-worker count, elapsed time, and the scaling config. `submitWorkflowHandler` now uses that plan instead of starting exactly one worker. Current controller defaults are `worker_min_count = 0`, `worker_max_count = 2`, `worker_count_per_start = 1`, and `worker_min_elapsed_time_between_starts = "30s"`. Submitted variables can override these defaults.

## Workflow Compilation

`internal/workflow` contains the first local workflow-compilation helper. It does not expose HTTP workflow submission yet.

Current workflow model:

- A `Workflow` has an ID and an ordered list of steps.
- Workflow-scope variables live on `Workflow.Variables`.
- A `Step` has an ID and currently supports one compiler path: `FanOut`.
- A `FanOutStep` wraps the fan-out work-item template.
- A step ID becomes the default generated work-item ID prefix when the fan-out template does not provide one.
- `CompiledWorkItem` carries workflow ID and step ID metadata next to the generated `model.WorkItem`.
- `CompileResult` carries the workflow ID, step count, and compiled work items.
- Workflow compilation rejects duplicate step IDs.
- Workflow compilation rejects duplicate generated work-item IDs.

Current fan-out compilation behavior:

- Wraps fan-out work-item templates in a minimal `FanOutStep` with a required step ID.
- Resolves one fan-out expression with `Resolver.ResolveFanOutExpression`.
- Expands each fan-out value into one `model.WorkItem`.
- Builds stable item IDs and output filenames from a template plus the fan-out value.
- Reuses `WorkItem.Validate()` so generated work items obey the shared controller-worker contract.
- Supports scalar fan-out tokens for `string`, `path`, and `int`.
- Supports explicit token accessors for object fan-out values, such as `.year`.
- Supports separate token accessors for work-item IDs and output filenames.
- Copies static resolved template parameters into every generated work item.
- Can bind parameter values from the current fan-out object using `ParameterAccessors`.

Object fan-out values must use an explicit token accessor. The compiler does not guess which object field should become the work-item ID or output filename.

`CompileWorkflow` still returns plain `[]model.WorkItem` for compatibility. `CompileWorkflowItems` returns `[]CompiledWorkItem` when the caller needs workflow and step traceability. `CompileWorkflowResult` returns the richer compile result.

`demo-workflow.json` contains the first serialized workflow submission payload. It keeps workflow-scope variables inside the workflow object and defines a one-step fan-out workflow that produces `write_demo_output` work items for demo years.

`demo-summary-workflow.json` is a tiny parameterized workflow fixture. It fans out over object records, uses `.id` for the work-item/output token, and uses `ParameterAccessors.input_path = ".input_path"` so each generated `summarize_input_file` item receives a different `parameters.input_path`.

## Local Client

`internal/client` contains the first Go local workflow client helper.

Current client behavior:

- Resolves `controller_url` through the variable resolver.
- Checks controller reachability through `GET /status` before submission.
- Can call an injected `ControllerStarter` when the controller is not reachable, then retry the reachability check.
- Provides a `LocalControllerStarter` that resolves `controller_start_executable` plus `controller_start_args` and starts them as a background process.
- Waits for a newly started controller to become reachable through repeated `GET /status` checks.
- Sends workflow submissions to `POST /workflow`.
- Loads serialized workflow submission files from disk.
- Can submit a serialized workflow submission file directly.
- Can fetch controller status and call `POST /shutdown` when pending and assigned work are both zero.
- Returns the final idle controller status from `ShutdownWhenIdle`, so callers can inspect queue and ledger counts before shutdown.
- Uses `client_status_poll_interval` as the typed variable for delay between non-idle status checks.
- Uses JSON containing a workflow plus optional submitted override/runtime variables.
- Treats the controller URL as a typed variable, not a separate config path.

The local controller starter is intentionally minimal. It resolves structured executable and argument variables and starts them as a background process. If `controller_start_lock_path` is configured, it uses an atomic lock file to avoid multiple clients starting duplicate local controllers at the same time. A pre-existing lock is treated as "another client is starting the controller," so the client continues into readiness polling. The client waits for readiness with a bounded number of status checks. The current local client uses ten readiness checks so cold `go run` startup has time to compile and bind.

The demo client currently starts the controller with:

```text
controller_config.controller_start_args = ["run", "./cmd/controller", "--config", "./cmd/controller/demo-config.json"]
```

`internal/clientsetup` contains the first client-side SSH setup engine. `SSHSetup` is intentionally decoupled from terminal I/O through a `Prompter` interface and from filesystem writes through a `FileStore` interface. The current setup flow can ask for transport choice, SSH host, port, user, key creation or existing key path, and host public key confirmation. For SSH it can generate a project-local Ed25519 key pair and write a generated controller config that selects `transport.type = "ssh"`. This package is not yet wired into `cmd/demo-client`; remote public-key installation and durable `known_hosts` management remain future slices.

## Fake HPCC And Dockerized Slurm

The repository now has a first fake-HPCC bootstrap path documented in `docs/fake-hpcc.md`.

Current repository pieces:

- `scripts/fake-hpcc/sbatch` is a deliberately tiny fake `sbatch` command. It accepts a script path, records fake scheduler state under `.run/fake-slurm`, prints a Slurm-like submitted-job line, and runs the script in background or foreground test mode.
- `cmd/controller/slurm_worker_script.go` generates a small Slurm-style worker script.
- `cmd/controller/docker_transport.go` implements Docker CLI-backed copy and exec operations, with a `DockerContainerTransport` adapter for a specific container such as `slurmctld`.
- `cmd/controller/ssh_transport.go` implements SSH-backed connect, exec, copy, list, and filesystem helper behavior with key-based auth and host-key checking.
- `cmd/controller/slurm_scheduler.go` writes generated Slurm scripts to local temp files, copies them through a `Transport`, and submits them through `sbatch`.
- `cmd/controller/bash_shell_platform.go` implements the current Bash shell dialect for newline handling, argument quoting, path localization, and simple command builders.
- `cmd/controller/runtime.go` defines `WorkerRuntime`, which prepares remote worker directories, writes worker config, and can upload a local worker artifact when configured.
- `cmd/controller/execution_environment.go` builds a configured environment from transport, dialect, scheduler, and runtime component config.
- `cmd/controller/fake-hpcc-ssh-config.json` defines a first Fake HPCC controller config that uses SSH transport placeholders for host, port, user, identity file, and pinned host key.
- `WriteFakeHPCCWorkerScript` prepares `.run/fake-hpcc/worker.slurm` for the current fake-HPCC fixture.
- `demo-fake-hpcc-workflow.json` submits a one-year `write_demo_output` workflow with `worker_target_environment = "hpcc"`.
- The controller now treats a configured execution environment as the preferred worker-start path. For the Dockerized Slurm backend, workflow submission prepares the worker runtime, generates a Slurm worker script, copies it into the Slurm control container, and submits it with `sbatch`.
- `scripts/fake-hpcc/run-demo` builds and starts a Bash-side controller, submits the fake-HPCC fixture, and shuts the controller down when idle.

The repository fake `sbatch` is now a smoke-test fallback, not the preferred long-term fake scheduler. The preferred locally controlled fake-HPCC backend is a Dockerized Slurm cluster installed outside this repository:

```text
/home/the_amatuer/src/slurm-docker-cluster
```

Upstream project:

```text
https://github.com/giovtorres/slurm-docker-cluster
```

Current checked-out commit:

```text
978c3de
```

Install/start path used in WSL:

```bash
cp .env.example .env
docker pull giovtorres/slurm-docker-cluster:latest
docker tag giovtorres/slurm-docker-cluster:latest slurm-docker-cluster:25.11.4
make up
```

Verified containers:

```text
mysql
slurmdbd
slurmctld
slurmrestd
slurm-cpu-worker-1
slurm-cpu-worker-2
```

Verified Slurm behavior:

```text
sinfo -> cpu partition up, c[1-2] idle
sbatch --version -> slurm 25.11.4
sbatch --wrap="hostname" -> Submitted batch job 1
sacct -> job 1 COMPLETED 0:0
```

Future fake-HPCC work should adapt the controller/runtime boundary to submit generated worker scripts to this Dockerized Slurm stack. Avoid expanding the homegrown fake `sbatch` beyond the minimum smoke-test role unless the Dockerized Slurm stack is unavailable.

The repository-local Fake HPCC Slurm/Singularity container definition now installs OpenSSH client/server packages, creates a `goetl` user, prepares `/home/goetl/.ssh`, prepares `/data/goetl`, generates host keys at container startup, validates `sshd` configuration in its container test script, and exposes port 22. The image is ready for an SSH-accessible fake HPCC path, but caller-side key installation is not automated yet.

The current Docker transport assumes a Docker-compatible command-line executable is available on the controller host. `FUTURE.md` records the deferred idea of detecting the Docker environment on first use and, after a user prompt, installing or guiding installation when Docker is missing.

The current local Singularity path has also been verified in WSL. `cmd/controller/local-singularity-config.json` configures `LocalTransport`, `DirectProcessScheduler`, and `SingularityWorkerRuntime`. `demo-local-singularity-workflow.json` now submits structured `worker_config.scheduler` and `worker_config.runtime` objects, and `scripts/local-singularity/run-demo` exports the Docker worker image to `/tmp/goetl-worker-dev.tar`, starts the controller, submits one demo work item, and verifies the Singularity-started worker writes `completed write-demo-2024`.

## Demo Work

The worker currently supports one operation:

```go
WorkItemTypeWriteDemoOutput WorkItemType = "write_demo_output"
WorkItemTypeSummarizeInputFile WorkItemType = "summarize_input_file"
```

`cmd/worker/work_demo.go`:

- Logs that the item is starting.
- Writes a small file under `TmpDir`.
- Logs the temporary output path.
- Removes an existing completed output with the same filename.
- Uses `os.Rename` to promote the completed file into `DataDir`.
- Logs that the item completed.

This models the intended mounted-storage pattern: incomplete output stays temporary, while completed output appears in persistent data storage. The demo operation is idempotent by overwrite: rerunning the same work item writes the same deterministic content and replaces any existing completed output. Future skip behavior must be based on verifiable correctness, not just the presence of an output filename.

`cmd/worker/work_summary.go` adds the first parameter-consuming worker operation:

```text
summarize_input_file
```

It requires `parameters.input_path` with type `path` or `string`, checks that the input is a file, writes a small summary containing the input path and byte size under `TmpDir`, and promotes the completed summary into `DataDir`.

The worker completion reporter now includes a worker-generated attempt ID plus runtime start and completion timestamps in `POST /work/complete`. The worker captures `StartedAt` before executing the item and `CompletedAt` when building the completion payload. The completion payload echoes assigned work-item parameters so SQLite can record the concrete resolved inputs used by the worker. Workflow-generated assignments carry controller-provided workflow definition, workflow fingerprint, workflow instance, step definition, step fingerprint, step instance, work-item/input/output fingerprint, and code-version fields; raw or legacy assignments still receive demo fallback values until the runtime variable snapshot is fully generated by the controller/worker runtime.

## Tests

The project uses Go's standard `testing` package. Run all tests from the repository root:

```powershell
go test ./...
```

Current coverage includes:

- Shared work-item validation.
- Variable type validation, including scalar, object, and list types.
- Variable literal parsing for scalar, object, and list types.
- Variable object field, list index, and fan-out accessors.
- Variable precedence merging and reference lookup.
- Recursive variable resolution, scalar structured access, fan-out expression resolution, and max-depth failure behavior.
- Local workflow fan-out compilation into validated draft work items.
- Local client workflow submission HTTP behavior.
- JSON config loading and validation.
- Runtime directory validation.
- Demo temporary-output promotion, deterministic overwrite, and logging.
- Worker dispatch validation.
- Worker HTTP fetch, completion, and failure clients.
- Empty-queue handling.
- Worker looping across multiple items.
- Worker failure reporting.
- Controller assignment, completion, and failure endpoints.
- Controller raw work submission and status endpoint behavior.
- Controller workflow submission into the pending queue.
- Controller worker-start hook selection from submitted variables.
- Controller local worker command resolution.
- Controller worker-scaling decision state.
- Controller shutdown endpoint behavior.
- Controller rejection of invalid methods and payloads.
- Controller config loading and namespace normalization.
- Controller default config loading when no config path is supplied.
- Controller execution-environment config validation and construction.
- Docker transport command construction for `exec` and `cp` behavior.
- SSH transport config validation, key loading, host-key checking, connect/close behavior, command execution, copy/list behavior, filesystem helpers, reconnect behavior, and end-to-end in-process SSH/SFTP fixture coverage.
- Fake HPCC SSH controller config construction.
- Client SSH setup key generation, existing-key config generation, and required host-key confirmation behavior.
- Bash shell dialect newline, quoting, path localization, copy command, and remove command behavior.
- Slurm scheduler script writing, copy, and submit behavior.
- WorkerRuntime path derivation, remote directory preparation, worker config upload, and optional worker artifact upload.
- Optional `Preparer` helper behavior for components that need setup hooks.
- Controller workflow submission using `Controller.env` to prepare the runtime and submit scheduled worker jobs.
- Controller SQLite ledger initialization from `controller_config.ledger_db_path`.
- SQLite schema creation, parent-directory creation, and attempt snapshot insertion.
- Controller-owned attempt recording adapter.
- Controller completion handling that records full completion metadata when present and still accepts legacy `id`-only completions.

Norton antivirus may briefly lock Go's temporary test executables after tests finish. If that happens, assertions still report `PASS`, but Go may print a cleanup error. Re-running the command usually succeeds.

## How To Run

Run the local workflow demo from the repository root:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl"
go run ./cmd/demo-client
```

Run the parameterized summary workflow demo from the repository root:

```powershell
go run ./cmd/demo-client demo-summary-workflow.json
```

Run the repository fake-HPCC smoke demo from WSL/Bash:

```bash
scripts/fake-hpcc/run-demo
```

This uses the repository's tiny fake `sbatch` command and should remain a smoke test.

Validate the repository Fake HPCC Slurm/Singularity container, including SSH server setup, from WSL/Bash:

```bash
containers/fake-hpcc-slurm-singularity/test
```

This builds the image and checks Singularity, `sshd -t`, the `goetl` user, SSH directories, and selected `sshd -T` settings.

Start and inspect the preferred Dockerized Slurm fake-HPCC backend from WSL:

```bash
cd ~/src/slurm-docker-cluster
make up
docker compose ps
docker exec slurmctld sinfo
docker exec slurmctld sbatch --version
docker exec slurmctld sbatch --wrap="hostname"
docker exec slurmctld sacct --format=JobID,JobName,State,ExitCode --parsable2
```

The current verified summary demo prints:

```text
final status: pending=0 assigned=0 failed=0 pending_reuse_candidates=0 attempts=17 attempt_variables=164
```

The latest verified summary run added two attempts and twenty-two attempt variables under the previous ten-runtime-variable snapshot shape. New summary runs add fourteen generated `runtime` variables plus one `work_item.input_path` variable per item.
It also recorded two distinct `runtime.input_fingerprint` values with the `input:sha256:` prefix and two distinct `runtime.output_fingerprint` values with the `output:sha256:` prefix.
The latest run recorded `runtime.code_version = "unknown"` for both attempts because this local `go run` path did not submit a `code_version` variable and did not embed VCS revision metadata.

The first verified skip run after enabling `/work/next` skip behavior ran the summary workflow twice:

```powershell
go run ./cmd/demo-client demo-summary-workflow.json
go run ./cmd/demo-client demo-summary-workflow.json
```

The two runs printed:

```text
final status: pending=0 assigned=0 failed=0 pending_reuse_candidates=0 attempts=19 attempt_variables=194
final status: pending=0 assigned=0 failed=0 pending_reuse_candidates=0 attempts=21 attempt_variables=224
```

The ledger then reported:

```text
completed=17
skipped=4
skip_reason "matched_prior_completed_attempt" 4
```

The two summary items were reusable from existing completed attempts, so each run recorded two skipped attempts rather than assigning those items to a worker.

Expected completed summary output:

```text
cmd/worker/.run/data/summary-demo-fixture.txt
input_path=demo-summary-input.txt
size_bytes=22

cmd/worker/.run/data/summary-demo-fixture-2.txt
input_path=demo-summary-input-2.txt
size_bytes=29
```

The demo client:

- Starts a local controller if `http://localhost:8080` is not reachable.
- Passes `cmd/controller/demo-config.json` to the local controller.
- Submits `demo-workflow.json`.
- Lets the controller start local workers using variables from the submitted workflow file.
- Polls controller status.
- Prints the final idle status, including queue and ledger counts.
- Calls `POST /shutdown` when pending and assigned work reach zero.

The worker can still be run manually:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl"
go run ./cmd/worker ./cmd/worker/demo-config.json
```

Expected worker output after exhausting the queue:

```text
worker starting
log dir: .run/logs
no work available
```

Expected completed demo output:

```text
cmd/worker/.run/data/cdl-demo-2024.txt
cmd/worker/.run/data/cdl-demo-2025.txt
```

Expected local ledger output:

```text
.run/controller/ledger.sqlite
```

The current verified demo run records two attempt rows and four attempt-variable rows.

## Design Direction

The controller now owns queue semantics. The worker stays relatively dumb: pull, execute, report, repeat.

The current in-memory queue is intentionally small. The SQLite ledger is only an attempt snapshot ledger; it is not yet a durable queue, retry system, workflow state store, or skip engine. Do not add retry rules or broad workflow parsing until the local controller state and ledger boundary are clear.

For HPCC work, use the configured execution-environment path against the locally controlled Dockerized Slurm cluster as the next integration target. Keep the controller-worker ownership split intact: Slurm starts capacity, but workers still pull assignments from the controller. The four current roles are transport, dialect, scheduler, and runtime; future backends should add implementations behind those roles instead of reintroducing hard-coded worker target strings. SSH is now one concrete transport implementation for that boundary; it should remain transport-level plumbing, while setup/questionnaire behavior belongs in client setup code.

## Likely Next Step

Wire `internal/clientsetup.SSHSetup` into `cmd/demo-client` behind an explicit setup flag or subcommand so the questionnaire can create local key material and a generated controller config from the demo client. Keep remote `authorized_keys` installation and durable `known_hosts` file management as separate, explicit follow-up slices.

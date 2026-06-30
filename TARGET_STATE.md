# Target State

Last updated: 2026-06-26

## Application Target

The target application is an ETL workflow for deriving consistent agricultural field boundaries from the USDA Cropland Data Layer (CDL).

The intended processing idea is:

1. Download CDL raster data for each target year.
2. Run a 2D XOR-like convolution window over each yearly raster to identify class-transition edges.
3. Convert the yearly edge output into field boundary candidates.
4. Combine the yearly boundary candidates across years.
5. Derive a least-common-area set so downstream work can operate on stable spatial units that are consistent across the full time series.

The exact image-processing method is still experimental. The important product direction is that the ETL system should support chunked raster/vector processing where each work item can run independently inside a worker container.

## System Shape

The finished system should expose a reusable Python package interface, tentatively named `goetl`, while keeping the controller and worker runtime in Go.

Customer-facing workflows should live outside the reusable ETL package as pipeline/config files. The Python package should consume those files and submit work to the Go controller.

Example target API:

```python
import goetl

goetl.run("cdl.pipe", "hpcc")
```

This separation is important:

- Reusable ETL tool IP lives in the `goetl` package.
- Customer-facing workflow IP lives in pipeline/config files such as `cdl.pipe`.
- Deployment-specific behavior lives behind backend names such as `hpcc`.

The Python package should be an interface layer. It should know how to call the Go controller, submit workflow work, select a backend, and possibly start a local or remote instance of the Go controller when needed. It should not become the implementation home for controller scheduling, worker coordination, or work queue semantics.

A workflow file should describe what needs to happen, not how to bootstrap HPCC workers or manage worker coordination.

Workflows should be composable. A workflow step may invoke a sub-workflow, including a fan-out where one parent step starts many child workflow instances with different variable bindings. This allows reusable workflow definitions to be nested without flattening customer-facing workflow intent into low-level work-item submissions.

Internally, the system needs Go controller and worker roles:

- Go `worker`: runs assigned ETL work inside a container.
- Go `controller`: owns the work list, starts workers, and coordinates progress.

The worker should stay relatively dumb. It receives one work item, validates its local runtime environment, performs the assigned processing, writes output to mounted storage, reports status, and asks for more work.

The controller owns broader workflow state. It decides what work exists, which work is ready, which work is complete, and which work failed.

## Development Governance Target

The project should continue using explicit HCI/epistemic-control audits for AI-assisted development. The audit target is durable ownership, not immediate recall.

Audits should score:

- Strategic Understanding (SU): architectural and causal understanding.
- Operational Control (OC): ability to navigate, debug, and safely modify the codebase using ordinary references.
- Implementation Recall (IR): short- or medium-term memory of implementation details.

Retention reviews should be part of the normal process for significant slices. Immediate reviews measure comprehension, delayed reviews measure retention, and long-term reviews measure ownership. Low Implementation Recall should not automatically count against long-term ownership when Strategic Understanding and Operational Control remain strong.

## Runtime Target

The Go worker should be packaged as a Docker container.

The Python package should expose a backend abstraction at the API level, with `hpcc` as the first concrete target. Calling `goetl.run("cdl.pipe", "hpcc")` should contact, configure, or start a Go controller, then submit the workflow to it.

The client must know where the controller is expected to be available. For local execution this may be a configured URL such as:

```text
http://localhost:8080
```

Before submitting a workflow, the client should check whether the controller is reachable at the configured location. If the controller is not running where expected, the client may start a local controller process using backend configuration. A client-started local controller should expose a shutdown API so the client can terminate it when the submitted workflow is done.

The Go controller should be able to bootstrap worker jobs on the HPCC. After startup, those workers should pull work from the controller rather than receiving all work details through the HPCC job submission itself.

Before any institutional HPCC integration, the HPCC backend should be proven against a locally controlled Dockerized Slurm environment. This preserves the same boundary the real backend needs -- generated worker scripts submitted through `sbatch`, workers pulling from the Go controller, and shared mounted storage -- without relying on external institutional infrastructure or site-specific configuration.

Backend execution should be configured as an execution environment composed from a small set of roles:

- Transport: copies files into and executes commands inside a target environment.
- Dialect: describes the target shell/OS command dialect, including paths, newlines, and quoting.
- Scheduler: submits jobs and represents backend capacity acquisition.
- Runtime: prepares the process or system that actually runs work, initially the Go worker.

The current target chain for the Docker-backed local fake-HPCC backend is:

```text
transport = DockerTransport / DockerContainerTransport
dialect   = BashShellPlatform
scheduler = SlurmScheduler
runtime   = WorkerRuntime
```

Docker is one current transport into the Dockerized Slurm control container. SSH is also a target transport for fake HPCC and later institutional HPCC access. Slurm is the scheduler. Bash is the initial Linux shell dialect. WorkerRuntime prepares the worker-side directories, config, script location, and worker artifact. Later environments may use a chain of transports, such as SSH into an HPCC login node followed by scheduler submission.

SSH transport should remain controller-side execution plumbing. It should connect, copy/list files, perform basic remote filesystem operations, execute commands, and surface authentication or host-key failures clearly. It should not own user interaction, key enrollment policy, or backend selection. Those setup concerns belong in the client/backend setup layer.

The client/backend setup layer should reduce the operational pain of first-time SSH configuration. A setup command should be able to ask for transport, host, port, user, whether to generate a key pair, and whether the presented host identity is trusted. The setup output should make the three SSH trust artifacts explicit:

- a local private/public key pair or an existing private key path;
- target-side authorization, usually the public key in the target account's `authorized_keys`;
- host identity pinning, either in the generated controller config or in an OpenSSH-compatible `known_hosts` file.

Automatic SSH setup must not silently trust arbitrary hosts or mutate remote accounts without an explicit user decision. Fake HPCC may support a controlled local convenience path for installing the generated public key, but real HPCC setup should respect site policy and may require manual instructions or institution-approved enrollment instead of password-based automation.

For local execution, the same ownership boundary applies. The controller owns the compiled queue and decides when worker capacity is needed. When the controller detects pending work, it should attempt to start worker capacity through configured execution-environment components. Workers still pull work from the controller after startup.

Over time, the controller should scale worker startup one worker at a time based on queue pressure and configured limits. The controller should not blindly start unlimited workers. It should consider at least:

- Pending work count.
- Assigned work count.
- Number of workers already started or known to be active.
- Backend-specific worker limits.
- Recent worker startup failures.

This creates the desired split:

- HPCC launches compute capacity.
- The Go controller manages the queue of work to be done.
- Workers repeatedly pull, execute, and report work.
- Workflow authors provide pipeline/config files through the Python interface.

## Work Model

Work item types will grow over time. Early work items should stay concrete and local, but the design should leave room for a broader task list later.

Likely CDL-oriented work items include:

- Download one CDL year or tile.
- Extract boundary candidates for one year/tile.
- Merge boundary candidates for a spatial region.
- Build least-common-area units for a spatial region.
- Validate or summarize generated output.

Each work item should have:

- A stable ID.
- A type.
- Input references.
- Output references.
- Status.
- Enough parameters to run independently.

Workflows should be idempotent by default. Running the same workflow with the same inputs, variables, code version, and backend configuration should produce the same final outputs. Early workers may overwrite an existing output with the same deterministic result. Later workers may skip execution only when correctness is verifiable, such as matching workflow identity, work-item identity, input fingerprints, parameters, code version, and a recorded prior success. An existing output filename alone is not sufficient proof that work can be skipped.

## Storage Direction

The current local directory model is the right first step:

- `LogDir` for persistent worker logs.
- `TmpDir` for temporary output.
- `DataDir` for completed output.

For now, these are local filesystem paths, which maps well to container-mounted HPCC storage. Future versions may support non-local locations, such as FTP, HTTP, S3, or logging APIs, but the project should not introduce a generic storage abstraction until there is a real second implementation.

Completed output promotion should preserve idempotent reruns. A worker should write incomplete output under `TmpDir`, then replace or verify the completed output under `DataDir`. Verification-based skipping is a later feature; the initial local behavior can safely replace deterministic outputs.

## Configuration Direction

The Python interface should consume a pipeline/config file plus a runtime backend name. For example:

```python
goetl.run("cdl.pipe", "hpcc")
```

The pipeline/config file should define customer-facing workflow behavior: datasets, steps, parameters, inputs, outputs, and processing intent.

The backend should define reusable deployment behavior: local execution, HPCC execution, Docker image details, mount conventions, Go controller settings, and worker launch strategy.

Runtime configuration must not become a parallel system beside variables. Controller settings, worker settings, backend choices, CLI flags, API arguments, and client overrides should enter the system as typed variables in the variable subsystem. Config structs, JSON files, HTTP JSON fields, and command-line flags are transport or parsing surfaces; after ingestion they should be represented as variables with a clear namespace and source.

A serialized workflow submission should keep workflow-local variables inside the workflow object. Top-level submission variables are for runtime, backend, API, and override values supplied by the caller. This keeps customer workflow scope distinct from launch-time configuration while still using the same typed variable subsystem after ingestion.

Backend configuration should include the controller contact point and worker launch target. For local execution this should include:

- Controller URL or host/port.
- Whether the client may auto-start the controller if it is not reachable.
- Controller start executable and argument list for local startup.
- Controller start lock path for coordinating concurrent local clients.
- Client status polling interval.
- Worker target location, initially `localhost`.
- Whether the controller may auto-start local workers.
- Minimum local worker count while work is pending.
- Maximum local worker count.
- Worker count per start decision.
- Minimum elapsed time between worker start decisions.
- Worker runtime config path or values to pass when starting each worker.

Backend configuration should also include an `execution_environment` definition. This should be deserialized into concrete controller components rather than handled through backend-specific conditionals. A minimal configured environment should identify:

- One or more transports.
- One shell or platform dialect.
- One scheduler.
- One runtime.
- Component-specific settings such as container name, runtime root, controller URL as seen by workers, and local worker artifact path.

For example, the Dockerized Slurm fake-HPCC backend should be expressible as:

```text
execution_environment.name = dockerized-slurm
transport.type = docker
transport.container = slurmctld
dialect.type = bash
scheduler.type = slurm
runtime.type = worker
runtime.root = /data/goetl
runtime.controller_url = http://host.docker.internal:8080
```

For the SSH-accessible Fake HPCC backend, the same execution-environment shape should be expressible with a different transport:

```text
execution_environment.name = fake-hpcc-ssh
transport.type = ssh
transport.host = 127.0.0.1
transport.port = 2222
transport.user = goetl
transport.identity_file = .run/goetl/ssh/id_ed25519
transport.host_public_key = <pinned-host-public-key>
dialect.type = bash
scheduler.type = slurm
runtime.type = worker
runtime.root = /data/goetl
runtime.controller_url = http://host.docker.internal:8080
```

The config file is the controller's transport surface. It may contain a pinned host public key or a path to key material, but the interactive collection and generation of those values should live in client setup code.

The controller should load a default controller config when no explicit config path is supplied. Client APIs may still provide a different config path or override variables when a specific backend or run needs different settings.

The worker runtime configuration must still define:

- Local paths for logs, temp files, and data mounts.
- The controller API URL.

These worker runtime settings should also be available as typed variables, usually through `worker_env` for values injected into workers, `worker_config` for explicit worker configuration documents, and `override` for deployment choices made before worker startup.

Early startup priority for the worker is to load local configuration before doing any work. Be cautious about global state; prefer passing a clear config object until a manager or runtime context solves an actual problem.

## Future Runtime Model

Workflow state and variable management are central to the system design. Variable resolution is not a convenience feature; it is part of the workflow execution contract and should be designed before workflow compilation grows complex.

### Variable Sources And Precedence

Variable values may come from multiple immutable scopes. When the same variable name exists in more than one scope, the more specific scope overrides the less specific scope.

Use this precedence order, from lowest to highest:

1. Global configuration shared across installations or deployments.
2. Client environment variables captured by the submitting client.
3. Controller environment variables captured from the controller process.
4. Worker environment variables configured for worker containers.
5. Client configuration documents.
6. Controller configuration documents.
7. Worker configuration documents.
8. Project-level configuration.
9. Workflow-level variables.
10. Command-line, Python API, or HTTP-submission overrides.
11. Step-local bindings created by the controller.
12. Work-item-local bindings created by the controller.
13. Generated runtime variables.

Use explicit namespaces for these scopes:

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

For example:

```text
global_config.default_output_root
client_env.USERPROFILE
controller_env.TEMP
worker_env.GDAL_DATA
controller_config.ledger_db_path
worker_config.data_dir
project_config.crop
workflow.years
override.worker_max_count
step.current_year
work_item.output_filename
runtime.work_item_id
```

An unqualified reference such as `GDAL_DATA` uses precedence lookup. A qualified reference such as `worker_env.GDAL_DATA` explicitly selects one namespace and bypasses precedence lookup.

`worker_env` means the configured environment that the controller injects into worker containers. It is available to the controller control plane before worker creation and may participate in controller-side workflow compilation. It must not be inferred from incidental values observed inside an arbitrary running worker.

Treat all scopes as immutable snapshots once captured for a given resolution boundary. Later lifecycle scopes are added by constructing a new resolver or resolved snapshot, not by mutating earlier scopes. In practical terms:

```text
base snapshot = global_config + *_env + *_config + project_config
workflow snapshot = base snapshot + workflow + override + workflow runtime values
step snapshot = workflow snapshot + step + step runtime values
work-item snapshot = step snapshot + work_item + work-item runtime values
attempt snapshot = work-item snapshot + attempt runtime values
```

If a worker's actual environment differs from the configured `worker_env`, treat that as a runtime validation error.

Sub-workflow invocation adds a nested binding scope. Values passed by a parent workflow into a child workflow override the child's inherited values, while preserving the same typed-variable rules. The exact position of explicit child bindings relative to workflow-local defaults should be specified when the first workflow compiler is implemented; parent-provided bindings should generally behave as invocation overrides.

Variable resolution should preserve where each final value came from so errors, status output, and debugging tools can explain which scope won.

The `override` namespace covers command-line arguments, Python API arguments, and HTTP-submission overrides consistently.

Workflow-level variables belong to the workflow definition itself. In a serialized submission, they should appear under the workflow object, not beside it. A top-level submission variable may still override a workflow value through the normal precedence rules, but that top-level position means "caller-supplied override/runtime input," not "ordinary workflow-local default."

Runtime control variables should use the same precedence system. Examples include:

```text
controller_config.controller_url
controller_config.controller_start_executable
controller_config.controller_start_args
controller_config.controller_start_lock_path
controller_config.ledger_db_path
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
```

Layer-specific worker launch configuration should live in structured `worker_config` object variables. `worker_config.transport` describes how the controller reaches the execution environment, `worker_config.dialect` describes command/path rendering rules, `worker_config.scheduler` describes capacity acquisition and scheduler script settings, and `worker_config.runtime` describes how the worker process starts once capacity exists. `worker_env` should remain reserved for configured or captured system environment variables that the controller injects into or expects inside workers.

For example, a local Go client may submit `override.worker_target_environment = "local"` and `override.client_status_poll_interval = "5s"`. The controller should resolve worker-related variables through the variable subsystem before deciding how to launch workers. Avoid adding separate controller-specific or worker-specific flag/config paths that bypass variable resolution.

Worker scaling should begin with an organic scheduler rather than immediate full fan-out. The controller should start workers gradually, observe whether started workers actually claim work, and then start more workers only when queue pressure remains. Early local defaults should be:

```text
worker_min_count = 0
worker_max_count = 2
worker_count_per_start = 1
worker_min_elapsed_time_between_starts = "30s"
```

The controller should start no more than `worker_count_per_start` workers in one scaling decision and should never exceed `worker_max_count`. It should also avoid starting more workers than useful pending work unless a later warm-worker feature explicitly requires idle capacity.

`worker_min_count` is a floor while pending work exists. If pending work exists and the known started/active worker count is below this floor, the controller may start workers immediately up to the floor, still bounded by `worker_max_count` and pending work. This lets a known highly parallel workflow request fast startup explicitly.

`worker_count_per_start` is the batch size for one scale-up decision. For example, `worker_min_count = 10`, `worker_max_count = 10`, and `worker_count_per_start = 10` means a workflow can request ten local workers immediately, short-circuiting the normal one-at-a-time organic ramp while still using explicit variables and the hard max.

For organic scaling beyond the minimum floor, the controller should require both:

- At least `worker_min_elapsed_time_between_starts` since the previous worker start decision.
- Confirmation that the previous started worker has claimed work.

Before explicit worker registration or heartbeats exist, "claimed work" can be inferred from controller state: a worker is considered confirmed when assigned work increases after the start. Later, worker registration and heartbeat state should replace this inference.

Local controller auto-start should be coordinated with a lock variable such as `controller_start_lock_path`. On Unix-like systems this could eventually map to `flock`; the portable local implementation may use an atomic lock file. The key requirement is that concurrent clients do not each start their own controller when they all observe the configured URL as temporarily unavailable.

### Generated Runtime Variables

Some variables are generated by the controller during a workflow run rather than supplied by the user, environment, backend, or project configuration. These values should be typed, namespaced, and resolvable through the same variable system as submitted values.

Generated runtime variables should include stable identifiers and timestamps such as:

- Workflow definition ID.
- Workflow instance ID.
- Step definition ID.
- Step instance ID.
- Work-item ID.
- Attempt ID.
- Workflow, step, work-item, input, output, and code fingerprints.
- Workflow start, end, and run duration values.
- Step start, end, and run duration values.
- Work-item start, end, and run duration values.
- Current datetime at the point an expression is evaluated.

Use an explicit namespace for generated runtime values, such as:

```text
runtime
```

Example references:

```text
runtime.workflow_instance_id
runtime.step_instance_id
runtime.work_item_id
runtime.work_item_fingerprint
runtime.output_fingerprint
runtime.workflow_start
runtime.current_datetime
```

Generated runtime variables should be read-only from the workflow author's perspective. Their lifecycle matters: workflow-level runtime variables are available while compiling or running the workflow instance, step-level runtime variables are available while compiling or running a step instance, and work-item runtime variables are available when a concrete work item is created or executed.

The meaning of `runtime.current_datetime` must be explicit. It should represent the controller's current evaluation time unless a worker-local runtime expression is deliberately introduced later. Prefer stable captured timestamps such as `runtime.workflow_start` for reproducible paths and IDs; use current datetime only when the workflow intentionally needs evaluation time.

### Identity And Fingerprints

Workflow identity, step identity, work-item identity, attempt identity, code version, and fingerprints are variables. They must not become a separate metadata/configuration system beside the variable subsystem.

Use generated read-only runtime variables for identities and fingerprints created by the controller or worker:

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

Workflow-authored stable IDs, such as workflow IDs and step IDs declared in a workflow file, may originate as workflow-scope values or struct fields during parsing, but after ingestion they should be represented in the resolver as typed variables. Generated instance IDs and fingerprints should be added by the controller as runtime-scope variables at the correct lifecycle boundary.

Fingerprints should be deterministic typed values, initially strings. A fingerprint should describe the thing named by the variable:

- `runtime.workflow_fingerprint` identifies the workflow definition content relevant to execution.
- `runtime.step_fingerprint` identifies a step definition plus its resolved execution-relevant parameters.
- `runtime.work_item_fingerprint` identifies one concrete executable work item.
- `runtime.input_fingerprint` identifies resolved inputs used by the work item.
- `runtime.output_fingerprint` identifies produced output content or a verifiable output manifest.
- `runtime.code_version` identifies the worker/controller code version that produced or would produce the output.

Fingerprints should be computed by the controller from normalized definitions plus resolved variables that are execution-relevant at the current lifecycle boundary. A variable is execution-relevant when it is referenced by the workflow, step, work item, input, output, or parameter being fingerprinted. Merely existing in an available scope is not enough.

For example, `project_config.state = "Michigan"` and `project_config.crop = "corn"` should affect a fingerprint when a step uses those variables to choose inputs, outputs, or processing parameters. An available value such as `runtime.current_datetime` should not affect a fingerprint unless the workflow deliberately references it in an execution-relevant expression.

Use separate meanings for definition identity and concrete run identity:

```text
workflow definition fingerprint = hash(normalized workflow definition)
workflow instance fingerprint = hash(workflow definition fingerprint + resolved execution-relevant workflow variables)
step fingerprint = hash(workflow instance fingerprint + normalized step definition + resolved execution-relevant step variables)
work-item fingerprint = hash(step fingerprint + bound fan-out item + resolved execution-relevant work-item variables)
```

These generated fingerprints should be exposed as read-only `runtime.*` variables after the controller computes them. Workflow authors may define stable labels, output path templates, or user-facing IDs, but they should not directly author correctness fingerprints used for skip decisions.

SQLite is an appropriate local persistence backend for these values, but SQLite should store and index variable snapshots rather than define identity outside the variable model. Tables may have convenience columns for common variables such as `workflow_instance_id` or `work_item_id`, but those columns should mirror typed variables and preserve their namespace, type, value, source, and lifecycle.

Verified idempotent skip decisions should be expressed in terms of variables. A skip is valid only when the stored successful attempt variables match the current resolved variables required for correctness, including identity variables, fingerprints, code version, inputs, parameters, output fingerprint or manifest, and prior success status.

When a skipped step has downstream dependencies, the controller must also restore the skipped step's logical outputs from durable state. For fan-out work steps, the step output can initially be derived from the ordered list of successful work-item outputs. Persisted outputs are needed so downstream unresolved steps can resolve references such as `step.previous.items[*]` without rerunning the skipped step.

### Typed Variables

Variables should be typed rather than stored as unstructured strings. Initial supported types should include:

- `string`
- `int`
- `bool`
- `datetime`
- `path`
- `list`
- `object`

Additional types may be added when a concrete workflow need appears. Dataset-specific values should not be added until their behavior is clear.

Every variable and nested structured value should use the same recursive typed
expression node with language-neutral `type` and `expression` fields. Object
expressions map field names to child nodes, while list expressions contain
ordered child nodes. Nested value types must be declared rather than inferred
from raw JSON.

The `path` type is intentionally distinct from `string`. It represents filesystem files or directories and should support path-aware operations such as joining path segments, normalization, and validation. Path evaluation must account for where a path is used: controller-local paths and worker-container paths may refer to different filesystems even when they originate from the same workflow expression.

Lists are a first-class workflow type because they are the primary mechanism for fan-out. A list value can drive creation of many parallel work items or many sub-workflow invocations. A list does not declare one element type; each item is an independently typed expression. This permits heterogeneous values and nested lists while allowing consumers to validate narrower requirements such as string-only lists.

Objects represent JSON-like structured values with named fields. They are needed for step outputs and for fan-out records that carry more than one value per generated work item. For example, one fan-out item might need both a `year` and an `input_path`.

Nested lists are valid when the data naturally has recursive collection structure. Objects with named list fields remain preferable when field names make the workflow meaning clearer.

### Expression-Based Resolution

Variable values should support expressions rather than only literals. Expressions must be able to reference previously available variables and produce a typed result.

Resolution must be recursive. If one variable expression references another variable whose value is itself an expression, the resolver should continue evaluating until it produces a final typed value.

Recursive resolution must have a configurable maximum depth. This prevents unbounded evaluation caused by accidental cycles or excessively deep reference chains. Reaching the configured maximum depth should produce a clear error that includes the variable being resolved and, when available, the reference chain.

The resolver should also detect direct and indirect cycles when practical:

```text
a = b
b = a
```

Maximum-depth enforcement is still required even when cycle detection exists, because future expression forms may recurse in ways that are not simple variable-reference cycles.

Expected early expression behavior includes:

- Referencing another variable.
- Combining string values.
- Joining path values and path segments.
- Calling small built-in functions that transform typed values.
- Supplying typed literals.
- Reading fields or elements from structured values.
- Resolving values through the precedence scopes before workflow compilation creates concrete work items.

Example intent:

```text
project_root = path("/data/projects/cdl")
year = 2025
input_dir = join(project_root, "inputs", year)
```

The expression syntax is not selected yet. Choose and document a deliberately small syntax before implementation. Avoid embedding a general-purpose scripting language unless a concrete requirement justifies it.

Built-in functions should be small, typed, deterministic helpers for common workflow expressions. They are not an extension mechanism for arbitrary scripts. Function evaluation should validate argument types and return a typed value.

Expected early built-in functions include:

- `day(dt)` returns the day component from a `datetime`.
- `month(dt)` returns the month component from a `datetime`.
- `year(dt)` returns the year component from a `datetime`.
- `filename(path)` returns the final filename from a `path`.
- `dirname(path)` returns the parent directory from a `path`.
- `join(path, segment, ...)` returns a `path` built from path segments.

Example intent:

```text
run_day = day(runtime.workflow_start)
source_name = filename(input_path)
output_path = join(project_root, "outputs", year(runtime.workflow_start), source_name)
```

Function behavior must be controller-side by default so workflow compilation is reproducible and workers receive already-resolved parameters. Worker-local functions should be added only when a concrete runtime need requires them.

Structured value access should start with a small JSONPath-like subset rather than a full query language. Expected early access forms are:

```text
step_output.field
step_output.items[0]
step_output.items[*]
```

The intended initial operators are:

- `.field` to select an object field.
- `[index]` to select one list element.
- `[*]` to select every list element in a fan-out context.

Do not add filters, predicates, recursive descent, arbitrary scripts, or full JSONPath compatibility until a specific workflow requires them. The important early distinction is between a value expression that resolves to one typed value and a fan-out expression that resolves to a list of typed values.

### Resolution Responsibilities

The controller should own variable resolution needed to compile workflow definitions into concrete work items. Workers should receive already-resolved work-item parameters whenever practical. This keeps workers simple and prevents the same workflow expression from resolving differently across containers.

Worker-runtime variables such as mounted directories or container-specific paths should come from the configured `worker_env` namespace. The controller resolves them using the same immutable values that it later injects into worker containers.

Near-term fix: workers should not resolve workflow variables. The controller knows the selected worker target, configured worker environment, mounts, paths, and backend variables before it starts workers, so it should resolve work-item values into concrete worker-local parameters before assignment. A worker may still read its own runtime config to know where it is running, but it should not evaluate workflow expressions or make independent variable-precedence decisions.

For uneven worker environments, work-item resolution has two phases:

1. Compile-time: the controller resolves workflow intent as far as possible and records pending logical work.
2. Assignment-time: when a specific worker requests work, the controller finalizes worker-local values using that worker's configured environment, mounts, target, and runtime variables.

The worker receives a finalized work assignment with concrete parameters. For the local demo path, this means the controller should eventually compile enough resolved output information that the worker does not infer workflow-level output paths from unresolved variables. Until the work-item model carries richer resolved parameters, the worker may join its configured `DataDir` with an already-resolved output filename, but this is a transitional boundary.

A dedicated `Variable` model and resolver package should be introduced before building a broad workflow compiler. Start with typed literals, precedence merging, and a small path expression capability; add expression features incrementally with tests.

Resolver configuration should include the recursive-resolution maximum depth. Choose a conservative default and allow deployments or controller configuration to override it.

The resolver should expose typed required and optional lookup helpers so controller, client, scheduler, and runtime code do not duplicate missing-variable and wrong-type handling. Structured object-field access should likewise live with the variable package rather than in backend-specific launch code.

## Workflow Compilation Direction

The controller-facing submission boundary should accept workflows, not raw work items. A workflow contains ordered steps, and each step compiles into one or more concrete work items when its dependencies and variables are resolved.

The early JSON submission envelope should have two distinct variable positions: `workflow.Variables` for variables owned by the workflow definition, and top-level `variables` for caller-supplied runtime or override variables. The controller should merge both into resolver scopes instead of inventing a separate configuration path.

Expected step behavior includes:

- Sequential steps where downstream work waits for upstream completion.
- Fan-out where one step compiles into many parallel work items.
- Fan-out driven by a typed list variable or a list found in a previous step's structured output.
- Sub-workflow steps where one step invokes another reusable workflow definition.
- Sub-workflow fan-out where one step starts many child workflow instances with different typed variable bindings.

Step outputs should be represented as JSON-like typed values, not unstructured strings. Later steps may reference prior step outputs through the same expression/accessor system used for variables. This allows a step to produce a list of records, and a later step to fan out over that list.

Downstream step work items should not be enqueued until their dependency steps are complete. For example, if a workflow has step 1 and step 2, step 2 work items remain uncompiled or unqueued until step 1 is marked complete, because step 2 may depend on step 1 outputs. This keeps queue state aligned with workflow readiness instead of flattening the whole workflow into pending work at submission time.

Fan-out compilation should be explicit. A workflow step should identify the expression that produces the list to iterate. The controller evaluates that expression, then creates one work item or sub-workflow invocation per list element, binding the current element into the step's variable context.

Raw work-item submission may remain useful as an internal test or administrative capability, but it is not the primary customer-facing API.

## Local Bootstrap Direction

The first end-to-end local workflow path should be:

1. A client reads backend configuration.
2. The client checks whether the configured controller URL is reachable.
3. If the controller is not reachable and local auto-start is enabled, the client starts a local controller.
4. The controller loads controller config and constructs the configured execution environment.
5. The client submits a workflow to the controller.
6. The controller compiles the workflow into concrete work items and places them in the pending queue.
7. The controller observes pending work and uses scaling policy to decide whether to start worker capacity.
8. The controller prepares the configured execution environment.
9. The controller submits a worker job through the configured scheduler.
10. The worker pulls work from the controller, processes it, and reports completion or failure.
11. The controller may start additional workers one by one when pending work remains and configured worker limits allow it.
12. The client polls controller status every configured interval.
13. When the client observes no pending or assigned work, it calls the controller shutdown API if it started that controller.

This local path should be built before HPCC orchestration. The HPCC backend should reuse the same control-plane responsibilities, but replace local process startup with HPCC job submission.

All local bootstrap behavior should be driven by resolved variables and explicit execution-environment config. The local client may provide defaults and overrides, but runtime decisions should come from variables such as `controller_url`, `worker_target_environment`, `max_worker_count`, and `client_status_poll_interval`, plus the selected transport/dialect/scheduler/runtime components, not from a separate hidden configuration channel.

## Near-Term Build Direction

The near-term implementation can still build from the worker inward, but each step should keep the eventual Python package boundary in mind:

1. Keep the minimal local worker runnable.
2. Add a tiny hard-coded local-file work item.
3. Have the worker write temporary output under `TmpDir`.
4. Rename or move completed output into `DataDir`.
5. Log each step.
6. Add a small package-facing entry point later, after the worker behavior is concrete enough to wrap.
7. Add controller API polling.
8. Define the typed variable model and precedence resolver before expanding workflow compilation.
9. Add a minimal workflow submission model that compiles one fan-out step into concrete work items.
10. Add local client-to-controller workflow submission.
11. Add controller-side local worker startup when pending work exists.
12. Add client polling and controller shutdown API use.
13. Move remaining workflow-variable resolution out of workers by expanding work items to carry resolved worker-local parameters.
14. Continue replacing hard-coded worker startup with configured execution-environment components.
15. Prove the Dockerized Slurm backend end to end with a real worker artifact.
16. Add sequential dependency tracking and sub-workflow invocation incrementally.

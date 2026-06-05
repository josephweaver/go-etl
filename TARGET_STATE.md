# Target State

Last updated: 2026-06-02

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

## Runtime Target

The Go worker should be packaged as a Docker container.

The Python package should expose a backend abstraction at the API level, with `hpcc` as the first concrete target. Calling `goetl.run("cdl.pipe", "hpcc")` should contact, configure, or start a Go controller, then submit the workflow to it.

The client must know where the controller is expected to be available. For local execution this may be a configured URL such as:

```text
http://localhost:8080
```

Before submitting a workflow, the client should check whether the controller is reachable at the configured location. If the controller is not running where expected, the client may start a local controller process using backend configuration. A client-started local controller should expose a shutdown API so the client can terminate it when the submitted workflow is done.

The Go controller should be able to bootstrap worker jobs on the HPCC. After startup, those workers should pull work from the controller rather than receiving all work details through the HPCC job submission itself.

For local execution, the same ownership boundary applies. The controller owns the compiled queue and decides when worker capacity is needed. When the controller detects pending work, it should attempt to start a worker using the configured worker target location, such as `localhost`. Workers still pull work from the controller after startup.

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

## Storage Direction

The current local directory model is the right first step:

- `LogDir` for persistent worker logs.
- `TmpDir` for temporary output.
- `DataDir` for completed output.

For now, these are local filesystem paths, which maps well to container-mounted HPCC storage. Future versions may support non-local locations, such as FTP, HTTP, S3, or logging APIs, but the project should not introduce a generic storage abstraction until there is a real second implementation.

## Configuration Direction

The Python interface should consume a pipeline/config file plus a runtime backend name. For example:

```python
goetl.run("cdl.pipe", "hpcc")
```

The pipeline/config file should define customer-facing workflow behavior: datasets, steps, parameters, inputs, outputs, and processing intent.

The backend should define reusable deployment behavior: local execution, HPCC execution, Docker image details, mount conventions, Go controller settings, and worker launch strategy.

Runtime configuration must not become a parallel system beside variables. Controller settings, worker settings, backend choices, CLI flags, API arguments, and client overrides should enter the system as typed variables in the variable subsystem. Config structs, JSON fields, and command-line flags are transport or parsing surfaces; after ingestion they should be represented as variables with a clear namespace and source.

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

The worker runtime configuration must still define:

- Local paths for logs, temp files, and data mounts.
- The controller API URL.

These worker runtime settings should also be available as typed variables, usually through `worker_env` for values injected into workers and `backend` or `override` for deployment choices made before worker startup.

Early startup priority for the worker is to load local configuration before doing any work. Be cautious about global state; prefer passing a clear config object until a manager or runtime context solves an actual problem.

## Future Runtime Model

Workflow state and variable management are central to the system design. Variable resolution is not a convenience feature; it is part of the workflow execution contract and should be designed before workflow compilation grows complex.

### Variable Sources And Precedence

Variable values may come from multiple scopes. When the same variable name exists in more than one scope, the more specific scope overrides the less specific scope.

Use this precedence order, from lowest to highest:

1. Client environment variables captured by the submitting client.
2. Controller environment variables captured from the controller process.
3. Worker environment variables configured for worker containers.
4. Global variables shared across installations or deployments.
5. Backend-specific variables for a named execution backend.
6. Project-level variables.
7. Workflow-level variables.
8. Command-line or API-submission overrides.

Environment variables must use distinct namespaces:

```text
client_env
controller_env
worker_env
```

For example:

```text
client_env.USERPROFILE
controller_env.TEMP
worker_env.GDAL_DATA
```

An unqualified reference such as `GDAL_DATA` uses precedence lookup. A qualified reference such as `worker_env.GDAL_DATA` explicitly selects one namespace and bypasses precedence lookup.

`worker_env` means the configured environment that the controller injects into worker containers. It is available to the controller control plane before worker creation and may participate in controller-side workflow compilation. It must not be inferred from incidental values observed inside an arbitrary running worker.

Treat `worker_env` as immutable for the lifetime of a controller runtime or submitted workflow. Changing it requires a controller restart or a new workflow run. If a worker's actual environment differs from the configured `worker_env`, treat that as a runtime validation error.

Sub-workflow invocation adds a nested binding scope. Values passed by a parent workflow into a child workflow override the child's inherited values, while preserving the same typed-variable rules. The exact position of explicit child bindings relative to workflow-local defaults should be specified when the first workflow compiler is implemented; parent-provided bindings should generally behave as invocation overrides.

Variable resolution should preserve where each final value came from so errors, status output, and debugging tools can explain which scope won.

Use explicit namespaces for the remaining scopes:

```text
global
backend
project
workflow
override
```

The `override` namespace covers command-line arguments, Python API arguments, and HTTP-submission overrides consistently.

Workflow-level variables belong to the workflow definition itself. In a serialized submission, they should appear under the workflow object, not beside it. A top-level submission variable may still override a workflow value through the normal precedence rules, but that top-level position means "caller-supplied override/runtime input," not "ordinary workflow-local default."

Runtime control variables should use the same precedence system. Examples include:

```text
backend.controller_url
backend.controller_start_executable
backend.controller_start_args
backend.controller_start_lock_path
backend.worker_target_environment
backend.worker_start_executable
backend.worker_start_args
backend.worker_min_count
backend.worker_max_count
backend.worker_count_per_start
backend.worker_min_elapsed_time_between_starts
backend.max_worker_count
backend.client_status_poll_interval
override.controller_url
override.controller_start_executable
override.controller_start_args
override.controller_start_lock_path
override.worker_target_environment
override.worker_start_executable
override.worker_start_args
override.worker_min_count
override.worker_max_count
override.worker_count_per_start
override.worker_min_elapsed_time_between_starts
override.client_status_poll_interval
```

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
runtime.workflow_start
runtime.current_datetime
```

Generated runtime variables should be read-only from the workflow author's perspective. Their lifecycle matters: workflow-level runtime variables are available while compiling or running the workflow instance, step-level runtime variables are available while compiling or running a step instance, and work-item runtime variables are available when a concrete work item is created or executed.

The meaning of `runtime.current_datetime` must be explicit. It should represent the controller's current evaluation time unless a worker-local runtime expression is deliberately introduced later. Prefer stable captured timestamps such as `runtime.workflow_start` for reproducible paths and IDs; use current datetime only when the workflow intentionally needs evaluation time.

### Typed Variables

Variables should be typed rather than stored as unstructured strings. Initial supported types should include:

- `string`
- `int`
- `bool`
- `datetime`
- `path`
- `list[T]`
- `object`

Additional types may be added when a concrete workflow need appears. Dataset-specific values should not be added until their behavior is clear.

The `path` type is intentionally distinct from `string`. It represents filesystem files or directories and should support path-aware operations such as joining path segments, normalization, and validation. Path evaluation must account for where a path is used: controller-local paths and worker-container paths may refer to different filesystems even when they originate from the same workflow expression.

Lists are a first-class workflow type because they are the primary mechanism for fan-out. A list value can drive creation of many parallel work items or many sub-workflow invocations. Early list support should allow lists of scalar values and lists of objects.

Objects represent JSON-like structured values with named fields. They are needed for step outputs and for fan-out records that carry more than one value per generated work item. For example, one fan-out item might need both a `year` and an `input_path`.

List-of-list support is not an early requirement. If nested grouped data is needed, prefer an object that contains a named list field. That keeps access patterns explicit and avoids adding recursive list behavior before there is a concrete workflow need.

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

A dedicated `Variable` model and resolver package should be introduced before building a broad workflow compiler. Start with typed literals, precedence merging, and a small path expression capability; add expression features incrementally with tests.

Resolver configuration should include the recursive-resolution maximum depth. Choose a conservative default and allow deployments or controller configuration to override it.

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

Fan-out compilation should be explicit. A workflow step should identify the expression that produces the list to iterate. The controller evaluates that expression, then creates one work item or sub-workflow invocation per list element, binding the current element into the step's variable context.

Raw work-item submission may remain useful as an internal test or administrative capability, but it is not the primary customer-facing API.

## Local Bootstrap Direction

The first end-to-end local workflow path should be:

1. A client reads backend configuration.
2. The client checks whether the configured controller URL is reachable.
3. If the controller is not reachable and local auto-start is enabled, the client starts a local controller.
4. The client submits a workflow to the controller.
5. The controller compiles the workflow into concrete work items and places them in the pending queue.
6. The controller observes pending work and starts one local worker using the configured worker target.
7. The worker pulls work from the controller, processes it, and reports completion or failure.
8. The controller may start additional workers one by one when pending work remains and configured worker limits allow it.
9. The client polls controller status every configured interval.
10. When the client observes no pending or assigned work, it calls the controller shutdown API if it started that controller.

This local path should be built before HPCC orchestration. The HPCC backend should reuse the same control-plane responsibilities, but replace local process startup with HPCC job submission.

All local bootstrap behavior should be driven by resolved variables. The local client may provide defaults and overrides, but runtime decisions should come from variables such as `controller_url`, `worker_target_environment`, `max_worker_count`, and `client_status_poll_interval`, not from a separate hidden configuration channel.

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
13. Add sequential dependency tracking and sub-workflow invocation incrementally.

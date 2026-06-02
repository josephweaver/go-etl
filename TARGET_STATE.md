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

The Go controller should be able to bootstrap worker jobs on the HPCC. After startup, those workers should pull work from the controller rather than receiving all work details through the HPCC job submission itself.

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

The worker runtime configuration must still define:

- Local paths for logs, temp files, and data mounts.
- The controller API URL.

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

Expected step behavior includes:

- Sequential steps where downstream work waits for upstream completion.
- Fan-out where one step compiles into many parallel work items.
- Fan-out driven by a typed list variable or a list found in a previous step's structured output.
- Sub-workflow steps where one step invokes another reusable workflow definition.
- Sub-workflow fan-out where one step starts many child workflow instances with different typed variable bindings.

Step outputs should be represented as JSON-like typed values, not unstructured strings. Later steps may reference prior step outputs through the same expression/accessor system used for variables. This allows a step to produce a list of records, and a later step to fan out over that list.

Fan-out compilation should be explicit. A workflow step should identify the expression that produces the list to iterate. The controller evaluates that expression, then creates one work item or sub-workflow invocation per list element, binding the current element into the step's variable context.

Raw work-item submission may remain useful as an internal test or administrative capability, but it is not the primary customer-facing API.

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
10. Add sequential dependency tracking and sub-workflow invocation incrementally.

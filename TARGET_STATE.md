# Target State

Last updated: 2026-05-27

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

Long term, workflow state and variable management are central to the system design.

The system will likely need typed variables plus expression-based resolution. Expected variable categories include:

- `string`
- `int`
- `datetime`
- `path`
- possibly dataset-specific values

A dedicated `Variable` model is likely, but expression evaluation should be introduced in small steps after the core runtime data model is clear.

## Near-Term Build Direction

The near-term implementation can still build from the worker inward, but each step should keep the eventual Python package boundary in mind:

1. Keep the minimal local worker runnable.
2. Add a tiny hard-coded local-file work item.
3. Have the worker write temporary output under `TmpDir`.
4. Rename or move completed output into `DataDir`.
5. Log each step.
6. Add a small package-facing entry point later, after the worker behavior is concrete enough to wrap.
7. Only then add controller API polling.

# Fake HPCC Development Environment

## Purpose

The fake HPCC environment is a required development target for `goetl`.

Its purpose is to prove the reusable Go ETL controller and worker runtime without relying on any real institutional HPCC system. The project should be able to demonstrate SSH-based worker startup, Slurm-style job submission, shared storage, and worker-controller coordination inside a locally controlled environment.

This environment must stay generic. Do not put real institutional hostnames, usernames, queues, accounts, partitions, module names, filesystem paths, or launch scripts in this repository.

The preferred fake-HPCC backend is now a locally controlled Dockerized Slurm cluster, currently installed outside this repository from `https://github.com/giovtorres/slurm-docker-cluster`. The repository's `scripts/fake-hpcc/sbatch` command remains useful as a minimal smoke-test fallback, but it should not grow into a full scheduler replacement while Dockerized Slurm is available.

## Current Backend Choice

The current fake-HPCC backend is Dockerized real Slurm, not a hand-rolled fake scheduler.

Current local install:

```text
/home/the_amatuer/src/slurm-docker-cluster
```

Upstream:

```text
https://github.com/giovtorres/slurm-docker-cluster
```

The repository's `scripts/fake-hpcc/sbatch` remains a minimal smoke-test fallback for testing the command boundary without the Dockerized Slurm stack.

The first Go helper for the Dockerized Slurm boundary is:

```text
cmd/controller/docker_slurm_submit.go
```

It builds and executes the command shape:

```bash
docker exec slurmctld sbatch <script>
```

and parses the submitted Slurm job ID from `sbatch` output. It is not wired into workflow submission yet.

## Boundary

The fake HPCC should behave like an HPCC system at the boundary the Go controller needs:

- An SSH-accessible login node.
- A shared filesystem visible to the login node and worker jobs.
- An `sbatch` command that accepts a generated Slurm script.
- Worker jobs that can reach the Go controller over HTTP.
- Worker jobs that can read runtime config and write logs, temporary output, and completed data.

The fake environment does not need to reproduce a real scheduler internally at first. It only needs to preserve the external contract well enough for local development and tests.

## Current Topology

The preferred fake HPCC runs on a local machine with Docker Compose:

```text
host or containerized controller
  |
  | sbatch worker.slurm
  v
Dockerized Slurm controller
  |
  v
Dockerized Slurm compute node
  |
  v
goetl worker process
```

The controller may still run on the host during early development. Later, it can also run in a container if that better matches test needs.

## Components

### Slurm Controller

The Slurm controller container provides the scheduler boundary.

It should include:

- An `sbatch` executable on `PATH`.
- Access to the generated worker script.
- Access to storage visible to worker jobs.

The Slurm controller should not contain real institutional HPCC-specific configuration.

### Shared Filesystem

The shared filesystem is a Docker volume or mounted local directory.

It should contain separate areas for:

- Uploaded worker artifacts.
- Generated worker runtime config.
- Generated Slurm scripts.
- Worker logs.
- Temporary output.
- Completed output.

The exact paths should be fake-environment paths, not copied from any real cluster.

### Smoke-Test `sbatch`

The repository fallback `sbatch` implementation is a small script.

Minimum behavior:

- Accept a Slurm script path.
- Return a fake job ID in a stable Slurm-like format.
- Run the submitted script in the background or hand it to a simple fake job runner.
- Write enough log output to debug failed submissions.

The smoke-test `sbatch` should intentionally support only the Slurm options needed by the current generated worker script. Unsupported options should fail clearly so the contract stays small.

The initial repository script lives at:

```text
scripts/fake-hpcc/sbatch
```

Run it by placing `scripts/fake-hpcc` before the real system paths:

```bash
PATH="$PWD/scripts/fake-hpcc:$PATH" sbatch worker.slurm
```

By default, it writes fake scheduler state and job logs under:

```text
.run/fake-slurm/
```

Set `FAKE_SLURM_RUN_ROOT` to choose another run directory.
Set `FAKE_SLURM_FOREGROUND=1` to run the submitted script synchronously during tests.

### Worker Job

The submitted job should start a `goetl` worker with a concrete runtime config.

The worker must receive:

- Controller URL.
- Log directory.
- Temporary directory.
- Completed data directory.

These values should come from typed runtime variables and generated worker config, not from hard-coded cluster assumptions.

## Controller Contract

From the Go controller's point of view, fake HPCC worker startup should look like remote Slurm startup:

1. Open an SSH connection to the login node.
2. Ensure the remote working directories exist.
3. Upload or reference the worker artifact.
4. Write worker runtime config.
5. Generate and write a Slurm worker script.
6. Run `sbatch <script>`.
7. Capture the submitted job ID.

The controller should not need to know whether the target is the fake HPCC or a real Slurm-backed environment, except through resolved backend variables.

The current controller starter treats `local` and `hpcc` as command-backed targets. For fake HPCC, those command variables can point at the fake `sbatch` path until SSH-backed submission is introduced.

The repository includes a first fake-HPCC submission fixture:

```text
demo-fake-hpcc-workflow.json
```

It resolves `worker_target_environment` to `hpcc` and points `worker_start_executable` plus `worker_start_args` at the fake `sbatch` command. The local fixture runs fake `sbatch` with `FAKE_SLURM_FOREGROUND=1` so end-to-end tests are deterministic on the current Windows/Git Bash development path. It is a variable-contract fixture: a generated `.run/fake-hpcc/worker.slurm` script must exist before it can be used to launch a worker.

## Slurm Script Contract

The generated worker script should be small and boring.

It should:

- Set strict shell behavior where practical.
- Create needed runtime directories.
- Start exactly one worker process.
- Pass the generated worker config path to the worker.
- Send stdout and stderr to known log locations.

The first script does not need advanced Slurm behavior such as arrays, dependencies, reservations, modules, GPU resources, or site-specific accounting.

The initial Go helper for this contract is:

```text
cmd/controller/slurm_worker_script.go
```

It generates one script for one worker process. The script creates the log directory, then runs the configured worker executable with the generated worker config path.

The companion writer helper creates parent directories and writes the generated script to a path such as:

```text
.run/fake-hpcc/worker.slurm
```

`WriteFakeHPCCWorkerScript` prepares that path for the current local fake-HPCC fixture. It writes a script that starts the Go worker with `cmd/worker/demo-config.json`.

When a submitted workflow resolves `worker_target_environment` to `hpcc`, the controller prepares this fake-HPCC worker script before it calls the command-backed worker starter.

## Current Local End-To-End Run

On the current Windows plus Git Bash development path, run the fake-HPCC demo from the Bash/Linux side. The generated worker script starts the worker through Bash, and that worker must be able to reach the controller at `http://localhost:8080`.

Use the helper script for the normal local check:

```bash
scripts/fake-hpcc/run-demo
```

The helper builds a controller binary, starts it from Bash, waits for `/status`, submits `demo-fake-hpcc-workflow.json`, and asks the controller to shut down when the demo client finishes.

Build the controller binary from Bash first:

```bash
go build -o .run/fake-hpcc/controller ./cmd/controller
```

Start the controller from Bash:

```bash
.run/fake-hpcc/controller ./cmd/controller/demo-config.json
```

In another Bash shell, submit the fake-HPCC workflow:

```bash
go run ./cmd/demo-client demo-fake-hpcc-workflow.json
```

The expected successful final status has no pending, assigned, or failed work:

```text
final status: pending=0 assigned=0 failed=0
```

If the controller is started from Windows while the fake worker runs under Bash, the worker may fail to reach `http://localhost:8080`. That is a local development namespace issue, not an HPCC scheduler issue. Keep both controller and fake worker on the same side of the boundary until the SSH-backed path gives the controller an explicit network address.

## Generated Run Artifacts

Fake-HPCC runs create local artifacts under `.run/`.

The fake HPCC helper uses:

```text
.run/fake-hpcc/
  controller
  controller.out
  controller.err
  worker.slurm
  logs/
```

The fake `sbatch` command uses:

```text
.run/fake-slurm/
  job-counter
  submissions.log
  job-<id>.out
  job-<id>.err
```

These files are generated runtime state. They are useful for debugging a failed local run, but they are not reusable workflow or backend configuration.

To reset the fake HPCC runtime state:

```bash
rm -rf .run/fake-hpcc .run/fake-slurm
```

## Later Home-Cluster Option

A home multi-machine cluster may still be useful later for testing real-network behavior. It is not the current fake-HPCC path. The current path is Dockerized Slurm first, repository fake `sbatch` second as a smoke-test fallback.

## Provenance Rules

To keep the development history clean:

- Build and verify fake HPCC support before adding any real HPCC deployment config.
- Keep real cluster credentials and configuration outside the repository.
- Use generic names such as `fake-hpcc`, `ssh`, `slurm`, `login`, `worker`, and `shared`.
- Do not encode real institutional details in tests, examples, docs, or commit messages.
- Treat any later real HPCC configuration as private deployment data.

## First Useful Milestones

1. Keep the fake HPCC contract documented.
2. Keep the repository fake `sbatch` as a minimal smoke-test fallback.
3. Submit generated worker scripts to real `sbatch` inside Dockerized Slurm.
4. Run an existing demo workflow through Dockerized Slurm.
5. Add SSH or remote submission only after the Dockerized Slurm boundary is clear.

Each milestone should be small enough to review independently under the HCI slice budget.

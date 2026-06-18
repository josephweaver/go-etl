# Fake HPCC Development Environment

## Purpose

The fake HPCC environment is a required development target for `goetl`.

Its purpose is to prove the reusable Go ETL controller and worker runtime without relying on any real institutional HPCC system. The project should be able to demonstrate SSH-based worker startup, Slurm-style job submission, shared storage, and worker-controller coordination inside a locally controlled environment.

This environment must stay generic. Do not put real institutional hostnames, usernames, queues, accounts, partitions, module names, filesystem paths, or launch scripts in this repository.

## Boundary

The fake HPCC should behave like an HPCC system at the boundary the Go controller needs:

- An SSH-accessible login node.
- A shared filesystem visible to the login node and worker jobs.
- An `sbatch` command that accepts a generated Slurm script.
- Worker jobs that can reach the Go controller over HTTP.
- Worker jobs that can read runtime config and write logs, temporary output, and completed data.

The fake environment does not need to reproduce a real scheduler internally at first. It only needs to preserve the external contract well enough for local development and tests.

## Initial Topology

The first fake HPCC can run on a local machine with Docker Compose:

```text
host controller
  |
  | SSH
  v
fake login container
  |
  | sbatch worker.slurm
  v
fake job runner
  |
  v
goetl worker process or container
```

The controller may run on the host during early development. Later, it can also run in a container if that better matches test needs.

## Components

### Login Node

The login node container provides the SSH boundary.

It should include:

- An SSH server.
- A configured test user.
- A writable working directory.
- Access to the shared filesystem volume.
- An `sbatch` executable on `PATH`.

The login node should not contain real HPCC-specific configuration.

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

### Fake `sbatch`

The first `sbatch` implementation can be a small script.

Minimum behavior:

- Accept a Slurm script path.
- Return a fake job ID in a stable Slurm-like format.
- Run the submitted script in the background or hand it to a simple fake job runner.
- Write enough log output to debug failed submissions.

The fake `sbatch` should intentionally support only the Slurm options needed by the current generated worker script. Unsupported options should fail clearly so the contract stays small.

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

## Provenance Rules

To keep the development history clean:

- Build and verify fake HPCC support before adding any real HPCC deployment config.
- Keep real cluster credentials and configuration outside the repository.
- Use generic names such as `fake-hpcc`, `ssh`, `slurm`, `login`, `worker`, and `shared`.
- Do not encode real institutional details in tests, examples, docs, or commit messages.
- Treat any later real HPCC configuration as private deployment data.

## First Useful Milestones

1. Document the fake HPCC contract.
2. Add a minimal local fake `sbatch` script and tests for its behavior.
3. Add a generated Slurm worker script function.
4. Add an SSH connection interface with a local or fake implementation for tests.
5. Add an HPCC worker starter that composes SSH, script generation, and `sbatch`.
6. Run an existing demo workflow through the fake HPCC path.

Each milestone should be small enough to review independently under the HCI slice budget.

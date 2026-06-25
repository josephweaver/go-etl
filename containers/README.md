# Containers

This directory holds container build assets used to prove the local fake-HPCC
runtime before adding real institutional HPCC configuration.

The near-term target is:

```text
Dockerized Slurm cluster
  -> Slurm job script
  -> SingularityCE worker runtime
  -> goetl worker pulls work from the controller
```

Keep these assets generic. Do not add real HPCC hostnames, accounts, queues,
partitions, module names, or private filesystem paths here.

## Go ETL Worker

`goetl-worker/` builds the worker runtime image. For now it contains only the
compiled Go worker and the minimal OS packages needed to make HTTPS requests.
Python, R, and ETL libraries should be added in later slices when the worker has
a script-execution work item to exercise them.

Run the narrow verification from WSL or another shell with Docker available:

```bash
containers/goetl-worker/test
```

The expected production entrypoint is:

```text
/goetl/goetl-worker
```

The expected HPCC/Singularity command shape is:

```bash
singularity exec goetl-worker.sif /goetl/goetl-worker /data/goetl/config/worker.json
```

## Fake HPCC Slurm plus SingularityCE

`fake-hpcc-slurm-singularity/` builds a local Slurm-derived image with
SingularityCE 4.1.2 installed.

Run the narrow verification from WSL or another shell with Docker available:

```bash
containers/fake-hpcc-slurm-singularity/test
```

The current local Slurm base is Rocky Linux 9, so this image installs the
SingularityCE 4.1.2 EL9 RPM. The verified institutional target is
SingularityCE 4.1.2 on Ubuntu Jammy; matching the Jammy package exactly would
require a later Ubuntu 22.04 Slurm base image.

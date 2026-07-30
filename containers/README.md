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

`goetl-worker/` builds the worker runtime image. It contains the compiled Go
worker, Python 3, and the minimal OS packages needed for the implemented worker
operations.

The DMTCP feasibility branch also pins DMTCP `v4.2.0` at commit
`f8009ce7b4ad211311ca2f72a929b975e4aa1155`, verifies source SHA-256
`3b240c78804bbf1e9354ee3da5c8760c3c952045f71773e9ed490846b15adce0`,
installs its runtime `bin` and `lib` trees, and builds the Go worker with
external dynamic linking.

That image change does not establish checkpoint support. The feasibility smoke
proved that DMTCP can checkpoint a control process but cannot checkpoint the
Go worker, and a Python process launched through Go does not join the DMTCP
computation.

The follow-up CRIU feasibility gate pins CRIU `v4.2.1` at commit
`f3e4ef5389601ed0893820d5eef1a769a5eee901` and verifies source SHA-256
`fbe32da7dec8d8443f162b81ff28dae1e75195fd78ca502d94c478504798e5fe`.
`criu-smoke` records host and container security facts, then checkpoints a
control process before attempting the actual `goetl-worker execute` plus
Python process tree.

Institutional Slurm job `13875979` on Ubuntu Jammy node `skl-010` stopped at
the control dump with `blocked_host_capability`. Normal-account
SingularityCE 4.1.2 exposed zero capability masks with `NoNewPrivs` enabled,
and CRIU reported that both `CAP_CHECKPOINT_RESTORE` and `CAP_SYS_ADMIN` were
missing. The GOET probe did not run. The current HPCC target therefore does
not support this CRIU path without an explicit institutional policy change.

## R/BRMS DMTCP Feasibility Image

`dmtcp-r-brms-feasibility/` is a standalone experimental image for the narrow
direct-R checkpoint boundary. It is not the GOET worker image. It pins R 4.4.3,
tidyverse 2.0.0, BRMS 2.23.0, RStan 2.32.7, CmdStanR 0.9.0, CmdStan 2.39.0,
and DMTCP 4.2.0. Its fixture compiles models before the checkpoint proof and
runs one chain on one core.

Run the image build, version assertions, and both Docker backend smokes from a
Linux shell:

```bash
containers/dmtcp-r-brms-feasibility/test
```

Run one already-built backend directly with:

```bash
GOETL_BRMS_RUNTIME=docker \
GOETL_BRMS_IMAGE=goetl/dmtcp-r-brms:os003 \
GOETL_BRMS_BACKEND=cmdstanr \
  containers/dmtcp-r-brms-feasibility/dmtcp-smoke
```

The smoke creates one owner-only evidence directory containing environment
facts, a complete sorted R package manifest, model and baseline artifacts,
DMTCP coordinator and client evidence, finalized checkpoint images, restart
logs, canonical draws, and `summary.json`. It explicitly limits common native
thread pools to one; removing those limits is outside the proven boundary.

SingularityCE 4.1.2 may fail to import a local Docker daemon image. The verified
conversion path uses a Docker archive:

```bash
docker save \
  --output /tmp/goetl-dmtcp-r-brms-os003.tar \
  goetl/dmtcp-r-brms:os003
singularity build \
  /tmp/goetl-dmtcp-r-brms-os003.sif \
  docker-archive:/tmp/goetl-dmtcp-r-brms-os003.tar
```

On a memory-constrained WSL installation, conversion can require temporary
additional swap because Singularity unpacks the complete image while building
the SIF. Do not make that temporary swap part of the production runtime.

After staging the SIF and smoke script on shared storage, use the following
shape inside an ordinary Slurm allocation:

```bash
GOETL_BRMS_RUNTIME=singularity \
GOETL_BRMS_SIF=/shared/path/goetl-dmtcp-r-brms-os003.sif \
GOETL_BRMS_BACKEND=rstan \
GOETL_BRMS_EVIDENCE_DIR=/shared/temp/goetl-os003-rstan-${SLURM_JOB_ID} \
  bash containers/dmtcp-r-brms-feasibility/dmtcp-smoke
```

Do not add `sudo`, `--fakeroot`, CRIU capabilities, or a site security
override. OS-003 passed this normal-account path for both `rstan` and
`cmdstanr`. That result authorizes R-specific supervisor design only; it does
not make the Go worker or arbitrary work-item types DMTCP-compatible.

## Python DMTCP Feasibility Image

`dmtcp-python-feasibility/` is the standalone OS-004 image for the direct
CPython boundary. It pins Python 3.11.15, NumPy 2.4.6 from a hash-verified
amd64 wheel, and DMTCP 4.2.0. It is not the GOET worker image and contains no
Go executable.

Run the build, exact-version assertions, pure-Python control, and full
NumPy-plus-child Docker proof from a Linux shell:

```bash
containers/dmtcp-python-feasibility/test
```

Run an already-built image directly with:

```bash
GOETL_PYTHON_RUNTIME=docker \
GOETL_PYTHON_IMAGE=goetl/dmtcp-python:os004 \
  containers/dmtcp-python-feasibility/dmtcp-smoke
```

The smoke limits OpenBLAS, OpenMP, MKL, and NumExpr to one thread. It runs an
uninterrupted baseline, checkpoints a directly launched Python parent during
NumPy native work together with one Python-created child, terminates the
original computation, restores both images in a fresh container invocation,
and requires exact parent and child output equality.

Use a Docker archive to make the Singularity image, then run the same smoke
inside an ordinary allocation:

```bash
docker save \
  --output /tmp/goetl-dmtcp-python-os004.tar \
  goetl/dmtcp-python:os004
singularity build \
  /tmp/goetl-dmtcp-python-os004.sif \
  docker-archive:/tmp/goetl-dmtcp-python-os004.tar

GOETL_PYTHON_RUNTIME=singularity \
GOETL_PYTHON_SIF=/shared/path/goetl-dmtcp-python-os004.sif \
GOETL_PYTHON_EVIDENCE_DIR=/shared/temp/goetl-os004-python-${SLURM_JOB_ID} \
  containers/dmtcp-python-feasibility/dmtcp-smoke
```

Normal-account Slurm job `13966757` passed this complete path under
SingularityCE 4.1.2. The result approves only the tested CPython 3.11,
NumPy 2.4.6, one-native-thread, one-Python-descendant shape. Additional Python
packages, native extensions, descendants, and thread shapes require their own
compatibility evidence.

A GDAL-enabled sibling image is available at `goetl-worker-gdal/` for worker
operations that require native GDAL dependencies and command-line tools.

Run the narrow verification from WSL or another shell with Docker available:

```bash
containers/goetl-worker/test
```

The test verifies the missing-config assertion and exact CRIU version, then
runs both CRIU process-tree probes in privileged local Docker. This proves the
smoke orchestration only; it does not prove normal-user HPCC compatibility.

Run the focused local Docker smoke directly with:

```bash
GOETL_WORKER_IMAGE=goetl/worker:dev \
GOETL_CRIU_DOCKER_PRIVILEGED=1 \
  bash containers/goetl-worker/criu-smoke
```

The explicit Docker privilege, host PID visibility, and unconfined seccomp
profile are test-only. Omit `GOETL_CRIU_DOCKER_PRIVILEGED=1` to verify blocker
classification without a test-only capability grant.

To reproduce the institutional gate after placing a SIF on shared storage, run
the probe inside a real Slurm allocation as the normal account:

```bash
GOETL_CRIU_RUNTIME=singularity \
GOETL_WORKER_SIF=/shared/path/goetl-worker.sif \
GOETL_CRIU_EVIDENCE_DIR=/shared/temp/goetl-criu-${SLURM_JOB_ID} \
  bash containers/goetl-worker/criu-smoke
```

Do not add `sudo`, `--fakeroot`, a capability grant, or a test-only security
override to the HPCC command. The smoke labels a Singularity run
`institutional_hpcc` only when `SLURM_JOB_ID` is present. It stops after a
failed control dump, emits `summary.json`, and preserves complete logs in the
evidence directory. A nonzero, classified blocker is the expected result under
the current target policy. The known-blocked DMTCP reproducer remains available
with:

```bash
GOETL_RUN_DMTCP_BLOCKED_SMOKE=1 containers/goetl-worker/test
```

The expected production entrypoint is:

```text
/goetl/goetl-worker
```

The expected HPCC/Singularity command shape is:

```bash
singularity exec \
  --bind /data/goetl:/data/goetl \
  goetl-worker.sif \
  /goetl/goetl-worker \
  /data/goetl/config/worker.json
```

The controller's `singularity_worker` runtime binds the runtime root to the same
absolute path inside the container when `runtime.settings.bind` is omitted. For
example, a runtime root of `/data/goetl` produces `--bind /data/goetl:/data/goetl`.
Use an explicit `bind` value only when the host and container paths intentionally
differ.

For local WSL testing with SingularityCE installed, export the Docker image to a
Docker archive:

```bash
docker tag goetl/worker:dev goetl-worker:dev
docker save -o /tmp/goetl-worker-dev.tar goetl-worker:dev
```

The local Singularity controller fixture uses that archive through:

```text
docker-archive:/tmp/goetl-worker-dev.tar
```

To build a SIF from the local Docker image:

```bash
docker save -o /tmp/goetl-worker-dev.tar goetl/worker:dev
singularity build /tmp/goetl-worker.sif docker-archive:/tmp/goetl-worker-dev.tar
```

Run the local controller-to-Singularity worker demo from WSL:

```bash
scripts/local-singularity/run-demo
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

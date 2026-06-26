# 012 Fake-HPCC Add SSH Server

Status: implemented

Slice:
fake-HPCC container / SSH server / exposes the Slurm plus Singularity fake backend through SSH

Objective:
Update the fake-HPCC Slurm plus Singularity container path so local development can reach the fake login boundary through SSH instead of Docker exec.

Allowed Production Files:
- none by default

Allowed Infrastructure Files:
- container build files for the fake-HPCC Slurm plus Singularity image
- docker compose override or local fake-HPCC launch files, if needed
- scripts that build, start, or verify the fake SSH login boundary

Tests:
- opt-in container or script verification, to be decided by the implementation slice

Documentation:
- docs/fake-hpcc.md

Out Of Scope:
- real HPCC credentials or hostnames
- production SSH hardening
- automatic host-key trust mutation
- controller config wiring beyond documenting the expected settings
- changing scheduler or runtime semantics
- broad Dockerized Slurm refactors

Acceptance:
- The fake-HPCC Slurm plus Singularity image includes an SSH server suitable for local development.
- The SSH server exposes a generic fake login user.
- The fake login environment can reach the same shared fake-HPCC filesystem used by Slurm worker jobs.
- The fake login environment can execute `sbatch`.
- The fake login environment can execute `singularity --version` or clearly document where Singularity is available.
- The local startup path exposes the SSH port in a predictable, documented way.
- The verification path proves SSH login, remote command execution, and access to the shared `/data/goetl` root.
- No real institutional hostnames, users, keys, queues, accounts, partitions, or paths are committed.

## Intended Topology

This feature makes the local fake-HPCC backend look more like a real login-node boundary:

```text
host controller
  |
  | SSH
  v
fake-hpcc login service
  |
  | sbatch
  v
Dockerized Slurm controller
  |
  v
Dockerized Slurm worker node
```

The fake login service may be the Slurm controller container itself or a small companion container, as long as it can:

- accept SSH connections from the host
- see the shared `/data/goetl` runtime root
- run `sbatch` against the Dockerized Slurm scheduler
- use generic local-only credentials

## SSH Server Requirements

The first implementation should keep the SSH server intentionally local and boring.

Expected behavior:

- listen only on a locally published development port
- authenticate with a generated or repository-ignored development key
- use a generic user such as `goetl`
- expose a stable host key for local testing, or generate one with a documented command
- never require real HPCC credentials

The repository should not commit private keys. If a local key is generated, it should live under an ignored run or development directory such as:

```text
.run/fake-hpcc-ssh/
```

## Shared Filesystem Requirements

The fake SSH login boundary must see the same fake runtime paths the controller already prepares:

```text
/data/goetl/artifacts/goetl-worker
/data/goetl/config/worker.json
/data/goetl/scripts/worker.slurm
/data/goetl/logs
/data/goetl/tmp
/data/goetl/data
```

This matters because `SSHTransport.Copy` writes worker artifacts, worker config, and generated scripts through SSH, while Slurm worker jobs must later read those same files.

## Verification

The implementation slice should provide one clear verification path. A script is acceptable if a Go test would require too much Docker orchestration.

Minimum verification:

```text
ssh connects to the fake login boundary
remote `true` succeeds
remote `sbatch --version` succeeds or returns a clear fake-compatible response
remote `singularity --version` succeeds when Singularity is part of the image
remote mkdir/write/list/remove under /data/goetl succeeds
```

The verification may be opt-in if it requires Docker:

```text
GOETL_FAKE_HPCC_SSH=1
```

## Later Features Enabled

This feature enables:

- controller configs that use `transport.type = "ssh"` against fake-HPCC
- Slurm submission through SSH instead of Docker exec
- end-to-end fake-HPCC runs that better match real login-node behavior
- safer documentation before any real HPCC deployment config exists

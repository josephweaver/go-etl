# Home Fake HPCC Cluster

## Purpose

The home fake HPCC cluster is a locally owned, multi-machine development environment for proving `goetl` remote worker startup without using any institutional HPCC system.

This environment is separate from the Docker fake HPCC. The Docker version should stay the most reproducible developer test. The home cluster is for proving the same controller-worker boundary across real machines on a private network.

## Initial Machine Roles

Use generic role names instead of personal machine names in project docs and configs:

```text
controller-host
login-node
worker-node-1
worker-node-2
worker-node-3
```

One physical computer may play more than one role at first. A practical starting layout is:

```text
machine A: controller-host and login-node
machine B: worker-node-1
machine C: worker-node-2
machine D: worker-node-3
```

The important boundary is that workers are started through the login-node path, not by the controller directly managing each worker machine.

## Network Requirements

Each machine should have:

- A stable LAN IP address or stable local hostname.
- SSH enabled where remote commands need to run.
- Firewall rules that allow workers to reach the controller HTTP URL.
- A shared understanding of the controller URL visible from worker nodes.

Avoid using `localhost` in worker config unless the worker runs on the same machine as the controller. For multi-machine runs, workers need a LAN-reachable controller URL such as:

```text
http://controller-host:8080
```

## SSH Requirements

The first home cluster path should use SSH keys:

- Controller host can SSH to the login node.
- Login node can SSH to each worker node.
- Worker nodes do not need to SSH back to the controller.

Use a dedicated test user if practical. Keeping the same username across machines will make early scripts simpler.

Do not commit private keys, real usernames, or machine-specific SSH config to this repository.

## Shared Storage

The cluster needs shared or shared-like storage for worker runtime files.

Early options:

- A network share mounted at the same path on all machines.
- A directory on the login node copied or synced to worker nodes by the fake scheduler.
- A single shared machine path used only for logs, temporary files, and completed data.

The cleanest HPCC-like model is a mounted shared directory visible to the login node and worker nodes:

```text
/fake-hpcc/shared/goetl/
  artifacts/
  configs/
  scripts/
  logs/
  tmp/
  data/
```

The path above is an example fake path. Do not copy real cluster paths into this project.

## Fake Scheduler Shape

The login node should provide a fake `sbatch` command.

For the home cluster, fake `sbatch` can:

1. Accept a generated Slurm script path.
2. Pick a worker node from a small configured list.
3. Start the script on that worker node over SSH.
4. Print a fake Slurm-style job ID.
5. Record submission details in a local log.

This keeps the controller contract close to real Slurm while avoiding real scheduler setup at the beginning.

## Worker Startup Contract

The submitted worker script should start exactly one `goetl` worker.

The worker needs:

- A worker binary or a way to run the worker source.
- A generated worker config file.
- A controller URL reachable from the worker node.
- Log, temporary, and data directories.

Early development can use a copied worker binary. That is simpler and more reproducible than depending on Go being installed on every worker node.

## First Bring-Up Checklist

1. Choose which machine is `controller-host`.
2. Choose which machine is `login-node`.
3. Choose one machine as `worker-node-1`.
4. Confirm `controller-host` can SSH to `login-node`.
5. Confirm `login-node` can SSH to `worker-node-1`.
6. Confirm `worker-node-1` can reach `http://controller-host:8080/status`.
7. Create or mount the fake shared storage path.
8. Run one generated worker command manually over SSH.
9. Replace the manual command with fake `sbatch`.
10. Add more worker nodes only after one remote worker completes a demo item.

## Provenance Rules

- Keep the home cluster generic in committed docs, tests, and examples.
- Keep real machine names, LAN IP addresses, usernames, and secrets out of the repository.
- Do not use institutional HPCC details as templates.
- Prefer fake role names in all reusable configuration examples.
- Treat any machine-specific setup as private local configuration.

## Relationship To Future Real HPCC

The home fake HPCC should exercise the same reusable layers needed later:

- SSH transport.
- Remote file writing or upload.
- Slurm-style script generation.
- `sbatch` submission.
- Worker startup.
- Worker-controller HTTP communication.
- Shared storage behavior.

Real HPCC support should be a deployment configuration over these same layers, not a separate controller design.

# Fake HPCC Data Assets Smoke

This runbook proves the fake-HPCC worker launch boundary with tiny data assets.
It does not use real HPCC configuration, CDL data, Yan/Roy tiles, Google Drive
credentials, or private paths.

## What It Proves

The smoke starts a controller with an execution environment using:

- `transport.type = local`
- `scheduler.type = slurm`
- `runtime.type = worker`
- `scripts/fake-hpcc/sbatch` as the local fake Slurm command

The controller prepares the worker runtime, writes the worker config, generates
and submits a Slurm worker script, and the worker then:

- resolves a named local fixture data root;
- references a plain text input data asset;
- copies a zip asset into the worker cache;
- extracts one selected archive member;
- renders `${data.<alias>.local_path}` Python arguments;
- writes one CSV artifact under `${artifact_dir}`;
- promotes the artifact into the worker data root;
- publishes the promoted artifact to a named `published_data` root;
- reports artifact-manifest evidence containing `artifacts` and
  `published_assets`.

## Run

PowerShell:

```powershell
pwsh -NoProfile -File scripts/fake-hpcc-data-assets-smoke.ps1
```

Bash:

```bash
bash scripts/fake-hpcc-data-assets-smoke.sh
```

The scripts generate temporary source-reference workflow files under:

```text
../go-etl-demo-project/.goetl-smoke/fake-hpcc-data-assets
```

They generate runtime state under:

```text
.run/fake-hpcc-data-assets
```

The scripts print the submission ID, manifest path, promoted artifact path,
published artifact path, controller logs, worker logs, and fake Slurm logs.

## Prerequisites

Required:

- Go toolchain on `PATH`;
- sibling `../go-etl-demo-project`;
- controller port `localhost:8080` available;
- `scripts/fake-hpcc/sbatch`;
- PowerShell 7 plus `bash` for the PowerShell script;
- `bash`, `curl`, and `python3` for the Bash script.

The PowerShell script creates an `sbatch.cmd` shim under `.run` so
`LocalTransport.Exec("sbatch", ...)` can find the fake Slurm command from a
Windows controller process.

## Expected Outputs

Worker logical output:

```text
.run/fake-hpcc-data-assets/worker-data/fake-hpcc-data-assets-smoke.json
```

Promoted artifact:

```text
.run/fake-hpcc-data-assets/worker-data/artifacts/raw/fake-hpcc-data-assets-smoke/reports/summary.csv
```

Published artifact:

```text
.run/fake-hpcc-data-assets/published-data/reports/summary.csv
```

Worker config written by `WorkerRuntime.Prepare`:

```text
.run/fake-hpcc-data-assets/runtime/config/worker.json
```

Generated Slurm worker script:

```text
.run/fake-hpcc-data-assets/runtime/scripts/worker.slurm
```

## Unsupported Cases

This smoke does not prove:

- real SSH transport to a login node;
- real Dockerized Slurm containers;
- SingularityCE image execution;
- real institutional HPCC queues, partitions, accounts, modules, or paths;
- large data transfer behavior;
- Google Drive or rclone data providers.

Use the container and SSH fake-HPCC runbooks for those later boundaries.

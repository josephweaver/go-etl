# Runtime Runbook

Last updated: 2026-07-07

This file preserves the moved runtime command and expected-output section from the pre-split root state file.

## How To Run

Run the local workflow demo from the repository root:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl"
go run ./cmd/demo-client
```

Run the dependency-aware workflow smoke path from the repository root:

```powershell
powershell -NoProfile -File scripts/dependency-aware-workflow-smoke.ps1
```

This starts a local controller, writes temporary sibling demo-project workflow fixtures, and verifies sequential stage readiness, contiguous `parallel_with` readiness, invalid non-contiguous `parallel_with` rejection, `goet status --json`, and `goet logs --json`.

Run the parameterized summary workflow demo from the repository root:

```powershell
go run ./cmd/demo-client demo-summary-workflow.json
```

Run the repository fake-HPCC smoke demo from WSL/Bash:

```bash
scripts/fake-hpcc/run-demo
```

This uses the repository's tiny fake `sbatch` command and should remain a smoke test.

Validate the repository Fake HPCC Slurm/Singularity container, including SSH server setup, from WSL/Bash:

```bash
containers/fake-hpcc-slurm-singularity/test
```

This builds the image and checks Singularity, `sshd -t`, the `goetl` user, SSH directories, and selected `sshd -T` settings.

Start and inspect the preferred Dockerized Slurm fake-HPCC backend from WSL:

```bash
cd ~/src/slurm-docker-cluster
make up
docker compose ps
docker exec slurmctld sinfo
docker exec slurmctld sbatch --version
docker exec slurmctld sbatch --wrap="hostname"
docker exec slurmctld sacct --format=JobID,JobName,State,ExitCode --parsable2
```

The current verified summary demo prints:

```text
final status: pending=0 assigned=0 failed=0 pending_reuse_candidates=0 attempts=17 attempt_variables=164
```

The latest verified summary run added two attempts and twenty-two attempt variables under the previous ten-runtime-variable snapshot shape. New summary runs add fourteen generated `runtime` variables plus one `work_item.input_path` variable per item.
It also recorded two distinct `runtime.input_fingerprint` values with the `input:sha256:` prefix and two distinct `runtime.output_fingerprint` values with the `output:sha256:` prefix.
The latest run recorded `runtime.code_version = "unknown"` for both attempts because this local `go run` path did not submit a `code_version` variable and did not embed VCS revision metadata.

The first verified skip run after enabling `/work/next` skip behavior ran the summary workflow twice:

```powershell
go run ./cmd/demo-client demo-summary-workflow.json
go run ./cmd/demo-client demo-summary-workflow.json
```

The two runs printed:

```text
final status: pending=0 assigned=0 failed=0 pending_reuse_candidates=0 attempts=19 attempt_variables=194
final status: pending=0 assigned=0 failed=0 pending_reuse_candidates=0 attempts=21 attempt_variables=224
```

The ledger then reported:

```text
completed=17
skipped=4
skip_reason "matched_prior_completed_attempt" 4
```

The two summary items were reusable from existing completed attempts, so each run recorded two skipped attempts rather than assigning those items to a worker.

Expected completed summary output:

```text
cmd/worker/.run/data/summary-demo-fixture.txt
input_path=demo-summary-input.txt
size_bytes=22

cmd/worker/.run/data/summary-demo-fixture-2.txt
input_path=demo-summary-input-2.txt
size_bytes=29
```

The demo client:

- Starts a local controller if `http://localhost:8080` is not reachable.
- Passes `cmd/controller/demo-config.json` to the local controller.
- Submits `demo-workflow.json`.
- Lets the controller start local workers using variables from the submitted workflow file.
- Polls controller status.
- Prints the final idle status, including queue and ledger counts.
- Calls `POST /shutdown` when pending and assigned work reach zero.

The worker can still be run manually:

```powershell
cd "c:\Joe Local Only\College\Research\go-etl"
go run ./cmd/worker ./cmd/worker/demo-config.json
```

Expected worker output after exhausting the queue:

```text
worker starting
log dir: .run/logs
no work available
```

Expected completed demo output:

```text
cmd/worker/.run/data/cdl-demo-2024.txt
cmd/worker/.run/data/cdl-demo-2025.txt
```

Expected local ledger output:

```text
.run/controller/workflow-execution.sqlite
```

The current verified demo run records two attempt rows and four attempt-variable rows.
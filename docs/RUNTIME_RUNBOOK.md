# Runtime Runbook

Last updated: 2026-07-11

This file preserves the moved runtime command and expected-output section from the pre-split root state file.

## How To Run

## Direct one-shot worker execution

`worker execute` is a development and worker/plugin diagnostic harness. It is
not a production execution path. Use development-only data and credentials; the
mode intentionally does not provide additional production credential or
leakage guarantees.

The command executes one already-resolved work item through the real
`Worker.Run` dispatcher. It does not compile a workflow, execute dependencies,
request work, retrieve source from a controller, report completion/failure,
deliver log observations, or send heartbeats.

Inputs:

```text
worker config JSON
resolved model.WorkItem JSON
optional local source-bundle ZIP
```

The config may omit `controller_url`. Its log, temporary, and data directories
must exist before execution. Python work requires `--source-bundle`; other
operations accept an extra bundle without reading it.

### Local process

From the repository root, prepare the checked-in Python fixture in PowerShell:

```powershell
New-Item -ItemType Directory -Force `
  .run/direct/logs, .run/direct/tmp, .run/direct/data | Out-Null

@'
{
  "log_dir": "logs",
  "tmp_dir": "tmp",
  "data_dir": "data",
  "python_executable": "python"
}
'@ | Set-Content -Encoding ascii .run/direct/worker.json

Copy-Item cmd/worker/testdata/direct-python/work-item.json `
  .run/direct/work-item.json
Compress-Archive -Force `
  -Path cmd/worker/testdata/direct-python/source/* `
  -DestinationPath .run/direct/source-bundle.zip

go run ./cmd/worker execute `
  --config .run/direct/worker.json `
  --work-item .run/direct/work-item.json `
  --source-bundle .run/direct/source-bundle.zip `
  --result .run/direct/worker-result.json

$LASTEXITCODE
Get-Content .run/direct/worker-result.json
```

On Linux, use `python3` in `worker.json` and create the ZIP with its files at the
archive root:

```bash
mkdir -p .run/direct/{logs,tmp,data}
(cd cmd/worker/testdata/direct-python/source && zip -r "$OLDPWD/.run/direct/source-bundle.zip" .)

go run ./cmd/worker execute \
  --config .run/direct/worker.json \
  --work-item cmd/worker/testdata/direct-python/work-item.json \
  --source-bundle .run/direct/source-bundle.zip \
  --result .run/direct/worker-result.json
```

Exit status is `0` for completed work and `1` for any invocation, validation,
execution, or result-writing failure. The result path is removed after options
parse, before config/work-item loading, so a stale completed result cannot appear
current.

For the fixture, inspect:

```text
.run/direct/worker-result.json
.run/direct/data/direct-python-result.json
.run/direct/data/artifacts/raw/direct-python-001/reports/fixture.txt
.run/direct/tmp/attempts/<result.attempt_id>/source/main.py
.run/direct/tmp/attempts/<result.attempt_id>/work/input.json
.run/direct/tmp/attempts/<result.attempt_id>/work/output.json
.run/direct/tmp/attempts/<result.attempt_id>/logs/stdout.log
.run/direct/tmp/attempts/<result.attempt_id>/logs/stderr.log
```

### Container

Mount a directory containing config, work item, source ZIP, and the configured
runtime directories. Paths inside `worker.json` must be container paths:

```bash
docker run --rm \
  -v "$PWD/.run/direct:/direct" \
  <worker-image> \
  /opt/gorc/worker execute \
    --config /direct/worker.json \
    --work-item /direct/work-item.json \
    --source-bundle /direct/source-bundle.zip \
    --result /direct/worker-result.json
```

### Apptainer/Singularity

Bind the direct fixture directory at the paths used by the worker config:

```bash
apptainer exec \
  --bind "$PWD/.run/direct:/direct" \
  worker.sif \
  /opt/gorc/worker execute \
    --config /direct/worker.json \
    --work-item /direct/work-item.json \
    --source-bundle /direct/source-bundle.zip \
    --result /direct/worker-result.json
```

### HPCC interactive allocation

Obtain the allocation first; direct mode does not request Slurm resources. Run
inside the actual target node/container environment:

```bash
salloc <site-specific-options>
srun --pty bash

apptainer exec --nv \
  --bind /path/to/direct:/direct \
  worker.sif \
  /opt/gorc/worker execute \
    --config /direct/worker.json \
    --work-item /direct/work-item.json \
    --source-bundle /direct/source-bundle.zip \
    --result /direct/worker-result.json
```

Scheduler flags, GPU flags, binds, image paths, and Python executable paths are
site-specific. Direct mode proves worker behavior inside the allocation; it does
not prove controller compilation, scheduling, queueing, claim, reporting, or
ledger behavior.

## Controller API Exposure Profiles

Local loopback development may run without controller API authentication only
when both the listener and advertised URL are loopback, for example
`http://127.0.0.1:8080`. The controller rejects disabled authentication for
non-loopback listeners or advertised URLs.

Authenticated local development uses the same loopback listener with bearer
credentials enabled. Clients should pass a token file rather than a raw token
argument:

```powershell
go run ./cmd/demo-client submit `
  --controller-url http://127.0.0.1:8080 `
  --controller-token-file .run\secrets\controller-client-token `
  --project project.json `
  --workflow workflow.json
```

Laptop-hosted external testing is documented in
[`deployment/laptop-test-controller-ingress.md`](deployment/laptop-test-controller-ingress.md).
It is test-only. Capture the temporary HTTPS URL before generating worker config,
and restart workers with regenerated config if that URL changes.

Server mode is not required for every user. A developer may run the controller on
their laptop and use SSH as the execution transport for HPCC command execution,
file copy, and Slurm submission. In that mode, the local client talks to the
local controller, and the local controller talks to HPCC over SSH. The worker
callback still needs a URL reachable from HPCC compute nodes; do not use
`localhost`, `127.0.0.1`, or a laptop LAN address as the worker-facing
`controller_url`.

For users without a domain, VM, or managed HTTPS tunnel, the `ssh_reverse`
callback tunnel remains a supported compatibility path. It exposes an
operator-configured HTTP callback URL on the SSH side and forwards that traffic
back to the laptop controller over SSH. If the SSH server forces reverse-forward
listeners to loopback, configure the callback tunnel relay fields so the dev or
login node runs a worker-visible relay that forwards to the loopback reverse
listener:

```json
{
  "callback_tunnel": {
    "type": "ssh_reverse",
    "remote_bind_host": "127.0.0.1",
    "remote_bind_port": 38281,
    "relay_bind_host": "0.0.0.0",
    "relay_bind_port": 39281,
    "worker_controller_url": "http://<hpcc-dev-node>:39281"
  },
  "runtime": {
    "settings": {
      "controller_url": "http://<hpcc-dev-node>:39281",
      "controller_token_file": "<hpcc-runtime-root>/secrets/controller-worker-token",
      "controller_insecure_external_http_allowed": true
    }
  }
}
```

The `controller_insecure_external_http_allowed` setting is intentionally
explicit. Use it only when the plain-HTTP worker URL is protected by the
controller-owned SSH reverse tunnel plus dev-node relay path and the worker also
uses a bearer token file. Prefer HTTPS when a stable domain or managed tunnel is
available. Run the callback preflight before admitting real work.

For short local/no-domain smoke tests, `scheduler.type = "remote_process"` can
start a worker process on the SSH target instead of submitting to Slurm. This
works with loopback-only `ssh_reverse` binds because the worker runs on the same
dev/login node as the reverse listener. Treat that as a smoke-test path, not the
normal production scheduler.

Dedicated-server deployment is documented in
[`deployment/dedicated-controller-server.md`](deployment/dedicated-controller-server.md).
The verified production-like shape is public HTTPS ingress on ports `80`/`443`
proxying to a controller bound only to `127.0.0.1:8080`. SSH remains the
execution transport for HPCC command execution, file copy, and Slurm submission;
workers call back to the controller over HTTPS.

For remote controllers, clients may use:

```powershell
go run ./cmd/demo-client submit `
  --controller-url https://controller.example.org `
  --controller-token-file .run\secrets\controller-client-token `
  --project project.json `
  --workflow workflow.json
```

Credential rotation in phase 1 requires a controlled controller restart and
worker/client token-file replacement. A controller session has one advertised
`controller_url`; migrating from one URL to another happens between runs by
updating controller config, client config, and generated worker config.

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

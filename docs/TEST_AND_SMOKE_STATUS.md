# Test And Smoke Status

Last updated: 2026-07-11

This file preserves the moved test coverage and smoke-test status section from the pre-split root state file.

## Tests

The project uses Go's standard `testing` package. Run all tests from the repository root:

```powershell
go test ./...
```

Current coverage includes:

- Shared work-item validation.
- Variable type validation, including scalar, object, and list types.
- Variable literal parsing for scalar, object, and list types.
- Variable object field, list index, and fan-out accessors.
- Variable precedence merging and reference lookup.
- Recursive variable resolution, scalar structured access, fan-out expression resolution, and max-depth failure behavior.
- Local workflow fan-out compilation into validated draft work items.
- Local client workflow submission HTTP behavior.
- JSON config loading and validation.
- Runtime directory validation.
- Demo temporary-output promotion, deterministic overwrite, and logging.
- Worker dispatch validation.
- Worker HTTP fetch, completion, and failure clients.
- Empty-queue handling.
- Worker looping across multiple items.
- Worker failure reporting.
- Controller assignment, completion, and failure endpoints.
- Controller raw work submission and status endpoint behavior.
- Controller submission status endpoint behavior.
- Controller source-bundle endpoint behavior for admitted Python source files,
  including missing-run, missing-source-context, unsafe-path, and cache
  miss/corruption errors.
- Controller workflow submission into the pending queue.
- Controller worker-start hook selection from submitted variables.
- Controller local worker command resolution.
- Controller worker-scaling decision state.
- Controller shutdown endpoint behavior.
- Controller rejection of invalid methods and payloads.
- Controller config loading and namespace normalization.
- Controller default config loading when no config path is supplied.
- Controller execution-environment config validation and construction.
- Controller startup assembly coverage for precedence, recovery mode, qualified lookup protection, and fail-closed startup.
- Docker transport command construction for `exec` and `cp` behavior.
- SSH transport config validation, key loading, host-key checking, connect/close behavior, command execution, copy/list behavior, filesystem helpers, reconnect behavior, and end-to-end in-process SSH/SFTP fixture coverage.
- Fake HPCC SSH controller config construction.
- Client SSH setup key generation, existing-key config generation, and required host-key confirmation behavior.
- Bash shell dialect newline, quoting, path localization, copy command, and remove command behavior.
- Slurm scheduler script writing, copy, and submit behavior.
- WorkerRuntime path derivation, remote directory preparation, worker config upload, and optional worker artifact upload.
- Optional `Preparer` helper behavior for components that need setup hooks.
- Controller workflow submission using `Controller.env` to prepare the runtime and submit scheduled worker jobs.
- Required controller SQLite initialization from the qualified main-database driver and connection-string variables.
- SQLite schema creation, strict version-1 validation, parent-directory creation, and attempt snapshot insertion.
- Controller-owned attempt recording adapter.
- Controller completion handling that records full completion metadata when present and still accepts legacy `id`-only completions.
- Explicit data-operator fixture smoke coverage for `asset.materialize -> compute -> commit_data`, including materialized input manifest hydration into compute, terminal records for all three operator families, source-transfer resource serialization, and publish-location write serialization.
- Worker use of controller-provided `materialized_data_assets` manifests without reacquiring provider data.
- Direct worker development execution for source-free and Python work, including
  runtime-only config, local source ZIP staging, generated bookkeeping,
  subprocess environment, logical output, artifact promotion, retained
  stdout/stderr, failure results, and zero controller HTTP requests.

Norton antivirus may briefly lock Go's temporary test executables after tests finish. If that happens, assertions still report `PASS`, but Go may print a cleanup error. Re-running the command usually succeeds.

## Direct Worker Development Execution Evidence

Recorded on 2026-07-11 on branch
`concept/gorc-worker-direct-execution`.

Focused command:

```powershell
go test ./cmd/worker -run 'TestRunDirectPythonTargetFixture' -count=1 -v
```

Observed result on Windows with Python 3.10.9:

```text
PASS
TestRunDirectPythonTargetFixture/sentinel_controller
TestRunDirectPythonTargetFixture/no_controller_URL
TestRunDirectPythonTargetFixtureFailure
```

The fixture builds a source ZIP at test time and invokes `runDirectCommand`.
Assertions cover generated attempt/source bookkeeping, source extraction,
required `GOET_*` environment variables, input/output JSON, a promoted file
artifact, result evidence, stdout/stderr retention, and failed-process
diagnostics. The sentinel server counts every path and observed zero total HTTP
requests during successful and failed direct Python execution.

Fixture sources:

```text
cmd/worker/testdata/direct-python/source/main.py
cmd/worker/testdata/direct-python/work-item.json
```

## Secure Network Exposure OS-009 Evidence

Recorded on 2026-07-10 against branch
`concept/secure-network-exposure-gorc-controller-api` at commit
`bf67915`.

### Automated security tests

Command:

```powershell
go test ./internal/controllerauth ./internal/controllerhttp ./cmd/controller ./cmd/worker
```

Result: pass.

Evidence added in this slice:

- `internal/controllerauth` has an explicit route-role matrix for every phase-1
  route and role.
- `internal/controllerhttp` has HTTPS fixture coverage, untrusted certificate
  rejection, same-origin redirect handling, and cross-origin credential
  forwarding rejection.

### Production-like VM HTTPS smoke

Target:

```text
Dedicated Linux VM with temporary wildcard DNS
Temporary DNS: <temporary-controller-host> -> <dedicated-vm-public-ip>
Ingress: Caddy v2.11.4
Controller listener: 127.0.0.1:8080
```

The VM controller was rebuilt from the concept branch and installed at:

```text
<controller-install-root>/bin/gorc-controller
<controller-install-root>/bin/gorc-worker
```

The controller config used bearer credentials from restrictive service-owned
token files and isolated OS-009 state under service-owned controller data and
log roots:

```text
<controller-state-root>
<controller-log-root>
```

External endpoint smoke command:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\network\smoke-controller-endpoint.ps1 `
  -ControllerUrl https://<temporary-controller-host> `
  -HttpUrl http://<temporary-controller-host> `
  -TokenFile .run\os009-secrets\controller-client-token `
  -SkipLocalLoopbackCheck
```

Result:

```text
GET /healthz over HTTPS -> HTTP 204
GET /status without token over HTTPS -> HTTP 401
GET /status with token over HTTPS -> HTTP 200
GET /healthz over HTTP -> HTTP 308 redirect to HTTPS
Controller endpoint smoke passed.
```

External TCP reachability from the development machine:

```text
<dedicated-vm-public-ip>:80    open
<dedicated-vm-public-ip>:443   open
<dedicated-vm-public-ip>:8080  closed
```

VM loopback verification:

```text
127.0.0.1:8080 listened by gorc-controller
GET http://127.0.0.1:8080/healthz -> HTTP 204
GET http://127.0.0.1:8080/status without token -> HTTP 401
```

### External worker callback smoke

The development machine acted as the external worker host. The worker read its
token from `.run/os009-secrets/controller-worker-token` and used:

```text
https://<temporary-controller-host>
```

Work item submitted over HTTPS:

```json
{
  "id": "os009-external-worker-001",
  "type": "write_demo_output",
  "output_filename": "os009-external-worker-001.txt"
}
```

Worker command:

```powershell
go run ./cmd/worker .run\os009-worker\worker.json
```

Result:

```text
worker starting
log dir: C:\Joe Local Only\College\Research\go-etl\.run\os009-worker\logs
no work available
```

Output evidence:

```text
.run/os009-worker/data/os009-external-worker-001.txt
completed os009-external-worker-001
```

Controller status after the worker run:

```json
{"pending":0,"assigned":0,"failed":0,"pending_reuse_candidates":0,"attempts":0,"attempt_variables":0}
```

### Singularity worker image build and staging

The worker container image was built from `containers/goetl-worker/Dockerfile`
in WSL with Docker and SingularityCE 4.1.2 available.

Build and verification commands:

```bash
containers/goetl-worker/test
docker save -o .run/os009-bin/goetl-worker-dev.tar goetl/worker:dev
singularity build .run/os009-bin/goetl-worker.sif docker-archive:.run/os009-bin/goetl-worker-dev.tar
singularity exec .run/os009-bin/goetl-worker.sif /goetl/goetl-worker /missing-worker-config.json
```

Result:

```text
invalid config: read config file /missing-worker-config.json: open /missing-worker-config.json: no such file or directory
```

Artifact:

```text
.run/os009-bin/goetl-worker.sif
size: 48 MiB
sha256: 5f32cbe58ca7ed11981a4efdacc17c8d216001d465d4fba6894ede6fe1898e29
```

The SIF was staged on the dedicated controller VM for later transfer to the
execution host:

```text
<controller-install-root>/images/goetl-worker.sif
sha256: 5f32cbe58ca7ed11981a4efdacc17c8d216001d465d4fba6894ede6fe1898e29
```

The controller `singularity_worker` runtime now defaults an omitted `bind`
setting to `<runtime root>:<runtime root>`, so the generated worker config,
token file, logs, temp directory, data directory, and cache roots can live under
one HPCC runtime root mounted at the same absolute path inside the container.

### Actual HPCC Slurm worker scheduling smoke

The dedicated VM controller was reconfigured to schedule workers on an HPCC
through SSH transport, Slurm, and the staged Singularity worker image. The
controller service uses service-owned SSH material outside user home
directories.

Runtime paths used by the smoke:

```text
HPCC runtime root: <hpcc-runtime-root>
Worker image: <hpcc-runtime-root>/images/goetl-worker.sif
Worker token file: <hpcc-runtime-root>/secrets/controller-worker-token
Generated worker config: <hpcc-runtime-root>/config/worker.json
Generated Slurm script: <hpcc-runtime-root>/scripts/worker.slurm
```

Submitted work item:

```json
{
  "id": "os009-hpcc-worker-001",
  "type": "write_demo_output",
  "output_filename": "os009-hpcc-worker-001.txt"
}
```

Controller evidence:

```text
worker_start_requested start_count=1 reason=active_capacity_below_claimable_work
worker_start_confirmed_by_claim reservation_id=worker-start-1
persisted work item completed: os009-hpcc-worker-001 attempt-47ab8d033630ef101bb7303b8e9379f6
```

HPCC Slurm evidence:

```text
Slurm output: <hpcc-runtime-root>/logs/goetl-worker-<job-id>.out
Slurm error: <hpcc-runtime-root>/logs/goetl-worker-<job-id>.err
Worker log: <hpcc-runtime-root>/logs/worker.log
```

The generated Slurm script ran:

```bash
/usr/bin/singularity exec \
  --bind <hpcc-runtime-root>:<hpcc-runtime-root> \
  <hpcc-runtime-root>/images/goetl-worker.sif \
  /goetl/goetl-worker \
  <hpcc-runtime-root>/config/worker.json
```

HPCC output evidence:

```text
<hpcc-runtime-root>/data/os009-hpcc-worker-001.txt
completed os009-hpcc-worker-001
```

Controller status after the HPCC worker run:

```json
{"pending":0,"assigned":0,"failed":0,"pending_reuse_candidates":0,"attempts":0,"attempt_variables":0}
```

### Sentinel scan

Sentinel:

```text
goet-controller-auth-sentinel-009-do-not-persist
```

The exact sentinel was absent from:

- `.run/os009-worker`;
- `.run/os009-deploy`;
- service-owned controller logs;
- service-owned controller OS-009 state;
- service-owned controller config;
- `journalctl -u gorc-controller -u caddy`;
- `<hpcc-runtime-root>` on HPCC, excluding the intentional worker token file.

The exact sentinel is intentionally present only in explicitly provisioned
credential fixture files.

### Remaining OS-009 evidence gap

The production-like HTTPS VM smoke, external worker callback, and actual
HPCC/Slurm worker scheduling smoke are complete. No OS-009 external smoke
evidence gap remains open.

## Local Controller SSH Reverse Callback Evidence

Recorded on 2026-07-10 against branch
`concept/secure-network-exposure-gorc-controller-api`.

This smoke verified the local/no-domain controller mode that uses SSH reverse
callback transport instead of public HTTPS. The local controller listened only on
`127.0.0.1:8080`; an SSH reverse callback listener on an HPCC dev node forwarded
worker HTTP callbacks back to the laptop controller.

Execution shape:

```text
local client -> local controller -> SSH transport -> HPCC dev-node process
HPCC dev-node worker -> ssh_reverse loopback callback -> local controller
```

The worker was launched through the `remote_process` scheduler, not Slurm. This
uses an HPCC dev-node process and is therefore suitable only for short smoke
tests or sites that explicitly permit that process model.

Evidence:

```text
controller /healthz: 200 OK
submission: completed
initial work items: 2
completed work items: 2
remote output files: cdl-demo-2024.txt, cdl-demo-2025.txt
remote process stderr: empty
```

Output contents:

```text
completed write-demo-2024
completed write-demo-2025
```

The direct non-loopback SSH reverse bind remained unavailable: requesting a
non-loopback reverse bind on the HPCC dev node still produced a loopback-only
listener. A dev-node relay was added for the Slurm path below.

### Local Controller SSH Reverse Relay Slurm Evidence

Recorded on 2026-07-10 against branch
`concept/secure-network-exposure-gorc-controller-api`.

This smoke verified local-controller HPCC orchestration without DNS, a public VM,
or managed HTTPS ingress. The local controller listened on `127.0.0.1:8080`.
The controller opened an SSH reverse listener on the HPCC dev node, then started
a dev-node relay bound to a worker-visible interface. Slurm compute workers used
the relay URL and the relay forwarded callbacks through the SSH reverse tunnel
to the laptop controller.

Execution shape:

```text
local client -> local controller -> SSH transport -> HPCC Slurm
HPCC compute worker -> dev-node relay -> ssh_reverse loopback listener -> local controller
```

The worker config generated for this smoke included the explicit opt-in flag:

```json
{
  "controller_url": "http://<hpcc-dev-node>:<relay-port>",
  "controller_token_file": "<hpcc-runtime-root>/secrets/controller-worker-token",
  "controller_insecure_external_http_allowed": true
}
```

The worker SIF was rebuilt from the current source and staged to the HPCC image
path used by the smoke:

```text
sha256: f76788783e0d0ea0355cc10f714989ed63744f7e20d6196f4166eace7bda5f72
```

CLI result:

```text
Submission: run-c13a4d12909e63a80081fcaeda6df94c
Workflow: cdl-demo
Initial work items: 2
Status: completed
Known work items: 2
Queued: 0
Running: 0
Completed: 2
Failed: 0
Skipped: 0
Stage 0: completed steps=1 assignable_pending=0 blocked_future=0 active=0 completed=2 failed=0 skipped=0
```

Controller evidence:

```text
worker_start_requested start_count=1 reason=active_capacity_below_claimable_work
worker_start_confirmed_by_claim reservation_id=worker-start-1
persisted work item completed: write-demo-2024 attempt-29c686cd2d5c0bef303086e2534efb2d
worker_start_confirmed_by_claim reservation_id=worker-start-2
persisted work item completed: write-demo-2025 attempt-9581947532efcebcb12b2bbe9010ab11
```

HPCC evidence:

```text
Slurm jobs: goetl-worker-12222949, goetl-worker-12222950
Compute nodes: skl-035, skl-083
Output files:
<hpcc-runtime-root>/data/cdl-demo-2024.txt
<hpcc-runtime-root>/data/cdl-demo-2025.txt
```

Post-shutdown check: neither the reverse-listener port nor the relay port
remained listening on the HPCC dev node.

Residual issue: one late extra worker printed a `connection refused` fetch error
after the workflow had already completed and the controller had shut down. Slurm
still marked that job `COMPLETED` because the current worker main logs errors
and returns without a non-zero process exit. Track that as a worker process exit
semantics follow-up; it did not prevent the workflow from completing.

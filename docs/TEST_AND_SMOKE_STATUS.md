# Test And Smoke Status

Last updated: 2026-07-10

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
- Explicit data-operator fixture smoke coverage for `cache_data -> compute -> commit_data`, including materialized input manifest hydration into compute, terminal records for all three operator families, source-transfer resource serialization, and publish-location write serialization.
- Worker use of controller-provided `materialized_data_assets` manifests without reacquiring provider data.

Norton antivirus may briefly lock Go's temporary test executables after tests finish. If that happens, assertions still report `PASS`, but Go may print a cleanup error. Re-running the command usually succeeds.

## Secure Network Exposure OS-009 Evidence

Recorded on 2026-07-10 against branch
`concept/secure-network-exposure-gorc-controller-api` at commit
`d3f7fe16165fefe9166efe73e56efc19feea7c86`.

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
VM: instance-20260710-150616, project gorc-2026-07, zone us-central1-a
Temporary DNS: 34-10-225-164.sslip.io -> 34.10.225.164
Ingress: Caddy v2.11.4
Controller listener: 127.0.0.1:8080
```

The VM controller was rebuilt from the concept branch and installed at:

```text
/opt/gorc/bin/gorc-controller
/opt/gorc/bin/gorc-worker
```

The controller config used bearer credentials from restrictive token files under
`/etc/gorc/secrets` and isolated OS-009 state under:

```text
/var/lib/gorc/os009/controller
/var/log/gorc/os009
```

External endpoint smoke command:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\network\smoke-controller-endpoint.ps1 `
  -ControllerUrl https://34-10-225-164.sslip.io `
  -HttpUrl http://34-10-225-164.sslip.io `
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
34.10.225.164:80    open
34.10.225.164:443   open
34.10.225.164:8080  closed
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
https://34-10-225-164.sslip.io
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

The SIF was staged on the dedicated controller VM for later transfer to HPCC:

```text
/opt/gorc/images/goetl-worker.sif
sha256: 5f32cbe58ca7ed11981a4efdacc17c8d216001d465d4fba6894ede6fe1898e29
```

The controller `singularity_worker` runtime now defaults an omitted `bind`
setting to `<runtime root>:<runtime root>`, so the generated worker config,
token file, logs, temp directory, data directory, and cache roots can live under
one HPCC runtime root mounted at the same absolute path inside the container.

### Actual HPCC Slurm worker scheduling smoke

The dedicated VM controller was reconfigured to schedule workers on MSU HPCC
through SSH transport, Slurm, and the staged Singularity worker image. The
controller service runs as `gorc` and uses service-owned SSH material under
`/etc/gorc/ssh`.

Runtime paths used by the smoke:

```text
HPCC runtime root: /mnt/scratch/weave151/etl/runtime
Worker image: /mnt/scratch/weave151/etl/runtime/images/goetl-worker.sif
Worker token file: /mnt/scratch/weave151/etl/runtime/secrets/controller-worker-token
Generated worker config: /mnt/scratch/weave151/etl/runtime/config/worker.json
Generated Slurm script: /mnt/scratch/weave151/etl/runtime/scripts/worker.slurm
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
Slurm output: /mnt/scratch/weave151/etl/runtime/logs/goetl-worker-12217152.out
Slurm error: /mnt/scratch/weave151/etl/runtime/logs/goetl-worker-12217152.err
Worker log: /mnt/scratch/weave151/etl/runtime/logs/worker.log
```

The generated Slurm script ran:

```bash
/usr/bin/singularity exec \
  --bind /mnt/scratch/weave151/etl/runtime:/mnt/scratch/weave151/etl/runtime \
  /mnt/scratch/weave151/etl/runtime/images/goetl-worker.sif \
  /goetl/goetl-worker \
  /mnt/scratch/weave151/etl/runtime/config/worker.json
```

HPCC output evidence:

```text
/mnt/scratch/weave151/etl/runtime/data/os009-hpcc-worker-001.txt
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
- `/var/log/gorc`;
- `/var/lib/gorc/os009`;
- `/etc/gorc/controller.json`;
- `journalctl -u gorc-controller -u caddy`.
- `/mnt/scratch/weave151/etl/runtime` on HPCC, excluding the intentional worker
  token file.

The exact sentinel is intentionally present only in the local and VM worker token
fixture files.

### Remaining OS-009 evidence gap

The production-like HTTPS VM smoke, external worker callback, and actual
HPCC/Slurm worker scheduling smoke are complete. No OS-009 external smoke
evidence gap remains open.

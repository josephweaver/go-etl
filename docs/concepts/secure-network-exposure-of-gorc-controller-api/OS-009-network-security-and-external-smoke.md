# OS-009: Network Security and External Smoke

Status: Implemented
Minimum recommended model: GPT-5.5 with high reasoning
Reference: EC-4 / operational slice / tests+fixtures+runbook

## Objective

Prove the secure network boundary end to end, including negative authorization,
credential non-disclosure, HTTPS ingress, and a real worker callback outside the
controller host.

## Test Layers

### Layer 1: Pure authorization

Cover every registered route and method:

```text
public
missing credential
invalid credential
client
worker
admin
unknown route
wrong method
```

Assertions include status code and whether the underlying handler ran.

### Layer 2: Controller HTTP integration

Start an in-process controller with test credentials and verify:

- public health;
- authenticated status;
- workflow admission;
- worker claim;
- completion;
- failure;
- logs/source-bundle policy;
- admin shutdown;
- recovery-mode interaction;
- request-size behavior remains intact.

### Layer 3: TLS client integration

Use `httptest.NewTLSServer` or an equivalent local TLS fixture to verify:

- standard trusted test client succeeds;
- untrusted certificate fails safely;
- HTTPS base URL handling;
- redirect rejection;
- authorization header does not cross origins.

### Layer 4: Fake HPCC HTTPS smoke

Run:

```text
controller -> HTTPS ingress fixture -> fake HPCC worker
```

The worker must:

- read its token from a file;
- claim work;
- run a fixture;
- report completion;
- leave no token sentinel in generated config, logs, output, or persistence.

### Layer 5: Laptop external smoke

Manual or guarded integration run:

```text
laptop controller
  -> managed HTTPS test ingress
  -> request from unrelated network or HPCC compute allocation
```

Prove:

- `/healthz` externally;
- unauthorized protected route returns `401`;
- authenticated worker reaches controller;
- no SSH reverse callback tunnel is active;
- one tiny assignment completes.

### Layer 6: Production-like server smoke

On a Linux VM/container-host fixture:

```text
public/staged HTTPS
  -> Caddy or equivalent
  -> loopback controller
```

Verify only 80/443 are exposed for API ingress and 8080 is unavailable remotely.

## Credential Sentinel

Use a distinctive test value such as:

```text
goet-controller-auth-sentinel-009-do-not-persist
```

After every controlled smoke, inspect:

- controller stdout/stderr;
- controller filesystem logs;
- worker stdout/stderr;
- worker logs;
- generated worker JSON;
- Slurm script;
- status/submission JSON;
- structured outputs;
- SQLite text/blob fields;
- ingress access logs;
- test artifacts under GORC control.

The exact sentinel must be absent except in the intentionally created restrictive
credential fixture file.

## Failure Cases

Test or document:

- missing credential;
- wrong role;
- expired/rotated token represented by an invalid old token;
- unreadable worker token file;
- controller unavailable;
- ingress unavailable;
- laptop sleeps or ingress exits;
- DNS name resolves but endpoint is unreachable;
- TLS certificate not trusted;
- public URL changed after worker launch;
- non-loopback startup with authentication disabled;
- plain external HTTP URL;
- reverse proxy strips `Authorization`;
- cross-origin redirect;
- oversized error response;
- controller shutdown requested by non-admin.

## Required Context

Read first:

- all implemented slices in this concept;
- fake HPCC runbook;
- test and smoke status;
- SSH callback preflight;
- sensitive-variable sentinel tests;
- controller persistence inspection helpers.

## Allowed Production/Test Files

- `internal/controllerauth/*_test.go`
- `internal/controllerhttp/*_test.go`
- `cmd/controller/*_test.go`
- `cmd/worker/*_test.go`
- fake-HPCC smoke fixtures/scripts
- `scripts/network/smoke-controller-endpoint`
- `docs/deployment/laptop-test-controller-ingress.md`
- `docs/deployment/dedicated-controller-server.md`
- `docs/TEST_AND_SMOKE_STATUS.md` only after evidence exists
- this concept README only for tracker/status updates

Do not weaken production code solely to simplify a test.

## Acceptance Criteria

- Complete route-role matrix passes.
- Token sentinels are absent from controlled surfaces.
- Cross-origin credential forwarding is impossible.
- Worker token-file failure is safe and actionable.
- Fake HPCC completes through authenticated HTTPS.
- Laptop external test completes without SSH reverse callback tunneling.
- A request from the actual Slurm compute context reaches `/healthz` and performs
  an authenticated worker operation where site policy permits.
- Production-like ingress keeps 8080 private.
- Existing SSH execution and Slurm submission tests remain green.
- Evidence and exact commands are recorded in the smoke-status document.

## Implementation State

Partial OS-009 evidence exists in `docs/TEST_AND_SMOKE_STATUS.md`:

- focused automated route-role and controller HTTP client tests pass;
- a Google Compute Engine VM at `34.10.225.164` serves the controller through
  trusted HTTPS at `https://34-10-225-164.sslip.io`;
- public TCP `80` and `443` are reachable while `8080` is not reachable
  externally;
- the controller process listens on `127.0.0.1:8080`;
- public `/healthz`, unauthenticated protected-route rejection, authenticated
  status, and HTTP-to-HTTPS redirect smoke checks pass;
- an external worker process on the development machine claimed and completed a
  `write_demo_output` item through the VM HTTPS endpoint;
- a current `goetl-worker.sif` was built from `containers/goetl-worker`, verified
  with SingularityCE, and staged on the dedicated controller VM at
  `/opt/gorc/images/goetl-worker.sif`;
- the `singularity_worker` runtime now defaults an omitted `bind` setting to
  `<runtime root>:<runtime root>`, keeping generated worker config, token file,
  logs, temp, data, and cache paths under one root mounted into the container;
- the dedicated VM scheduled an actual HPCC Slurm worker through SSH transport;
- the HPCC Slurm job ran the staged Singularity image, claimed
  `os009-hpcc-worker-001` through the VM HTTPS endpoint, wrote
  `os009-hpcc-worker-001.txt`, and reported completion;
- the OS-009 sentinel is absent from controlled logs, local worker artifacts, VM
  OS-009 state files, VM journal output, HPCC runtime files outside the worker
  token file, and SQLite state outside credential fixture files.

Remaining gap:

- no OS-009 external smoke evidence gap remains open.

## Stop Conditions

Stop and append to `issues.md` if:

- the token appears outside the intended credential file/in-memory boundary;
- external ingress strips authorization;
- an endpoint is missing from the route matrix;
- a state-changing request must be automatically retried;
- the HPCC compute network blocks the selected public HTTPS endpoint;
- laptop ingress is being treated as production evidence;
- port 8080 is publicly reachable in the production-like profile.

# Secure Network Exposure of the GORC Controller API

Status: Proposed  
Cadence: CSxIx  
Revision: 2026-07-10 laptop-test and dedicated-server deployment split

> Repository naming note: the product is referred to here as **GORC**, while current
> executable names, module paths, environment variables, and JSON API versions may
> still use the existing `goet` / `GOET_` names. This concept does not perform the
> product rename.

## Purpose

Replace SSH reverse tunneling as the normal worker-to-controller callback path with
a secure HTTP(S) controller endpoint.

The controller already serves an HTTP API and already separates its listen address
from its advertised `controller_url`. The work in this concept is therefore not a
protocol rewrite. It is a control-plane hardening and deployment project:

```text
client  ───────┐
               ├── HTTPS controller URL ── ingress ── HTTP 127.0.0.1:8080
workers ───────┘                         Caddy / managed test ingress
```

The final product should normally run the controller on a dedicated server with a
stable DNS name. During development and HPCC testing, the controller may continue
to run on the developer laptop and be published temporarily through a test-only
HTTPS ingress.

## Immediate Motivation

The current SSH refinement supports a controller-owned reverse callback tunnel.
That proved that HPCC workers can call a laptop-hosted controller, but it couples
worker callback reachability to the controller's SSH execution topology.

The target direction is cleaner:

```text
Controller -> execution transport -> starts workers
Workers    -> HTTPS              -> claim and report work
Clients    -> HTTPS              -> submit and observe workflows
```

SSH remains a valid execution transport for command execution, SFTP, jump hosts,
and Slurm submission. It no longer needs to be the normal carrier for the
controller API.

## Current State

The repository already has most of the transport-neutral structure needed:

- the controller uses Go `net/http`;
- listen host, listen port, advertised URL, timeouts, request size, and header size
  are startup settings;
- the default controller binds to `localhost:8080`;
- clients accept a controller URL and use HTTP;
- workers receive `controller_url` in generated worker configuration;
- the controller can generate worker configuration for local, Docker, SSH, Slurm,
  and Singularity-backed execution environments;
- the current HTTP routes include workflow admission, work claiming, work
  completion/failure, status, logs, source bundles, health, and shutdown;
- SSH reverse callback tunneling is implemented and can remain available during
  migration.

Important gaps:

- controller routes do not yet have a complete authentication and authorization
  boundary;
- `/shutdown` and other control-plane routes must not be exposed merely because
  they validate an HTTP method;
- worker callback code uses package-level `http.Get` and `http.Post`, so there is
  no shared place to add credentials, redirect policy, or safe HTTP behavior;
- worker configuration contains a URL but no control-plane credential source;
- the controller has no startup interlock preventing an unauthenticated non-loopback
  exposure;
- deployment documentation does not yet separate internal listen address,
  externally advertised URL, TLS ingress, and temporary laptop exposure.

## Strategic Decisions

### 1. Keep the controller's application listener unprivileged and private

The GORC controller should normally listen on:

```text
127.0.0.1:8080
```

The controller does not need to bind directly to privileged ports `80` or `443`.
It also does not need to own public certificate files.

A deployment ingress owns public networking:

```text
public :80/:443 -> TLS ingress -> 127.0.0.1:8080
```

For a dedicated server, the first documented ingress should be Caddy because a
small configuration can provide reverse proxying, automatic HTTPS, renewal, and
HTTP-to-HTTPS redirect. The controller implementation must remain independent of
Caddy and must also work behind Nginx, Traefik, a cloud load balancer, or another
equivalent ingress.

### 2. Enforce authentication inside GORC

TLS and reverse-proxy access controls are defense layers, not substitutes for
controller authorization.

The controller itself must verify every protected request. This preserves the
security boundary when:

- the ingress is misconfigured;
- a local process bypasses the ingress;
- deployment moves between laptop, VM, server, container, or load balancer;
- a future ingress product is replaced.

### 3. Use a small phase-1 role model

The first secure network boundary should use role-scoped bearer credentials:

| Role | Intended caller | Capabilities |
|---|---|---|
| `client` | CLI or trusted automation | Submit workflows/raw work; read status, logs, and permitted source artifacts |
| `worker` | GORC worker runtime | Claim work; report completion/failure; send worker observations; obtain worker-required controller artifacts |
| `admin` | Operator | Administrative operations, especially shutdown; may be treated as a superset where explicitly documented |

`/healthz` may remain unauthenticated, but it must disclose only minimal liveness
information.

Every registered route and HTTP method must have an explicit policy. There must be
no implicit "authenticated means allowed" fallback.

### 4. Keep credentials out of ordinary configuration and persistence

Raw bearer tokens must not appear in:

- committed controller JSON;
- workflow/project JSON;
- command-line arguments;
- generated worker JSON;
- controller status or submission status;
- logs or error messages;
- fingerprints or provenance;
- SQLite persistence.

Controller credential declarations identify a protected source such as an
environment variable or restrictive file. The controller loads credentials only
at startup and keeps comparison material in memory.

The worker receives a path to a pre-provisioned restrictive token file. The first
implementation should not place the token literal in worker JSON or Slurm command
arguments.

This control-plane credential is distinct from workflow execution secrets covered
by the Sensitive Variable Propagation concept. Later keystore integration may
supply controller and worker credentials without changing the HTTP authentication
contract.

### 5. Treat `controller_url` as an external session contract

The externally advertised URL is the URL clients and workers use:

```text
https://controller.example.org
```

It may differ from the internal listener:

```text
http://127.0.0.1:8080
```

The advertised URL must be resolved before workers are launched. GORC should not
periodically discover a public IP and silently mutate the callback URL during a
run.

A controller session has one advertised URL. If that URL becomes unreachable,
workers fail with a network error and retry only according to an explicit retry
policy. They do not discover an alternate controller.

### 6. Keep laptop exposure test-only

The laptop test profile exists to prove real external connectivity before paying
for or configuring a dedicated server.

Preferred test path:

```text
GORC controller on laptop
  -> local 127.0.0.1:8080
  -> temporary managed HTTPS ingress
  -> stable-for-the-test HTTPS URL
  -> HPCC worker callbacks
```

A Tailscale Funnel or Cloudflare Tunnel-style ingress avoids needing:

- a stable home IP;
- router port forwarding;
- public inbound firewall rules;
- knowledge of whether the ISP uses carrier-grade NAT;
- direct exposure of the laptop's public address.

A direct dynamic-DNS profile may also be documented, but it is secondary. Merely
discovering the current public IP is insufficient: the address must be publicly
routable, inbound ports must reach the laptop, local and router firewalls must
permit the traffic, and DNS/TLS must agree with the endpoint.

No dynamic-DNS updater or public-IP discovery service belongs in controller core.

### 7. Use a dedicated server for production

The production target is:

```text
stable DNS
  -> public HTTPS ingress on dedicated server
  -> loopback controller listener
  -> persistent controller state
```

The dedicated server may be a Google Compute Engine VM, another cloud VM, a hosted
server, an institutional server, or a machine managed by the operator. Provider
selection is deployment configuration, not a controller API concern.

## Deployment Profiles

### Profile A: Local Development

```text
client -> http://127.0.0.1:8080
worker -> http://127.0.0.1:8080
```

Rules:

- loopback only;
- authentication may be explicitly disabled for narrow local tests;
- local unauthenticated mode must fail startup if the controller binds to a
  non-loopback address;
- no claim of production security.

### Profile B: Laptop-Hosted External Test

```text
external client/HPCC worker
  -> temporary public HTTPS URL
  -> managed test ingress on laptop
  -> http://127.0.0.1:8080
```

Rules:

- authentication required;
- controller remains loopback-bound;
- the ingress URL is captured before worker configuration is generated;
- the URL is considered stable only for that test session;
- no production availability claim;
- laptop sleep, restart, network change, and ingress termination are expected
  failure modes;
- tests use small, restartable work.

### Profile C: Dedicated-Server Production

```text
client/worker
  -> https://controller.<domain>
  -> Caddy or equivalent
  -> http://127.0.0.1:8080
```

Rules:

- stable DNS;
- authentication required;
- controller runs as an unprivileged service user;
- persistent controller data and ingress certificate state;
- firewall exposes only required public ports;
- secrets are delivered by restrictive files, service environment, or future
  keystore integration;
- operational restart, backup, monitoring, and recovery procedures are documented.

## Candidate Authentication Configuration

The exact JSON placement should follow the repository's canonical configuration
direction. A top-level object is shown because `execution_environment` already
uses a structured top-level contract and credential sources are not ordinary
workflow variables.

```json
{
  "api_version": "goet/v1alpha1",
  "kind": "Controller",
  "variables": [
    {
      "name": {
        "namespace": "controller_config",
        "key": "controller_url"
      },
      "type": "string",
      "expression": "https://controller.example.org"
    }
  ],
  "authentication": {
    "mode": "bearer",
    "credentials": [
      {
        "id": "primary-client",
        "role": "client",
        "token_env": "GOET_CONTROLLER_CLIENT_TOKEN"
      },
      {
        "id": "hpcc-workers",
        "role": "worker",
        "token_env": "GOET_CONTROLLER_WORKER_TOKEN"
      },
      {
        "id": "operator",
        "role": "admin",
        "token_file": "/etc/goet/secrets/controller-admin-token"
      }
    ]
  }
}
```

Requirements:

- exactly one protected source per credential;
- duplicate credential IDs are rejected;
- empty tokens are rejected;
- duplicate token material across roles is rejected;
- raw token values are never rendered;
- configured tokens are transformed into constant-time comparison material in
  memory;
- authentication-disabled mode is rejected for non-loopback listeners;
- external advertised HTTP URLs require an explicit test-only insecure override;
- HTTPS is required for laptop external testing and production.

## Candidate Worker Configuration

Generated worker JSON references a token file but does not contain the token:

```json
{
  "log_dir": "/data/goetl/logs",
  "tmp_dir": "/data/goetl/tmp",
  "data_dir": "/data/goetl/data",
  "controller_url": "https://controller.example.org",
  "controller_token_file": "/data/goetl/secrets/controller-worker-token"
}
```

The token file is pre-provisioned on the worker side with restrictive permissions.
For fake HPCC or container tests it may be mounted as a secret. For a real HPCC it
may be installed in an operator-controlled path readable only by the account that
runs GORC workers.

The controller may later support a keystore-backed bootstrap provider, but that is
not required to establish the HTTP contract.

## Route Authorization Policy

The implementation must inventory actual route callers before finalizing the
table. The initial policy is:

| Route family | Initial role |
|---|---|
| `GET /healthz` | public |
| workflow/raw-work admission | `client` or `admin` |
| submission status and logs | `client` or `admin` |
| controller status | `client` or `admin` |
| work claim/complete/fail | `worker` or `admin` |
| worker observations | `worker` or `admin` |
| worker-required source bundles | `worker` or `admin`; add `client` only if current CLI behavior requires it |
| shutdown | `admin` only |

Existing callback preflight should use the minimal public `/healthz` endpoint rather
than relying on authenticated `/status`.

## HTTP Client Policy

All controller callers should share safe request behavior:

- injected `*http.Client`;
- explicit timeout;
- strict base-URL parsing;
- `Authorization: Bearer ...` added in one place;
- no raw credential in errors;
- redirects disabled or restricted to the same origin;
- bounded error-body reads;
- content type and accepted response status checked;
- HTTPS required except for explicit loopback development;
- user-agent identifying GORC caller type and version;
- optional retry only for operations proven safe to retry.

Work claim and state-changing reports must not gain automatic retry semantics
without deciding idempotency and attempt behavior.

## Relationship to SSH Refinement

This concept does not delete SSH support.

After completion:

```text
SSH transport:
  - gateway and jump-host traversal
  - remote command execution
  - SFTP
  - Slurm submission
  - runtime preparation

HTTPS controller API:
  - work claims
  - completion/failure reports
  - status and logs
  - workflow admission
```

The reverse callback tunnel remains available as a compatibility fallback until
the HTTPS path is proven. It should eventually be documented as optional rather
than the preferred laptop callback topology.

## Goals

- Add an explicit controller API authentication and authorization contract.
- Prevent unauthenticated non-loopback controller startup.
- Protect shutdown and all other control-plane routes.
- Consolidate client and worker HTTP request behavior.
- Load client and worker credentials from protected local sources.
- Keep token literals out of ordinary JSON, arguments, logs, status, and
  persistence.
- Preserve the existing controller API payload contracts where possible.
- Support HTTPS URLs without embedding certificate management in the controller.
- Prove a laptop-hosted external test without SSH reverse callback tunneling.
- Provide a provider-neutral dedicated-server deployment baseline.
- Preserve SSH as an execution transport.
- Add negative security tests and external connectivity smoke tests.

## Non-Goals

- Renaming GOET packages, commands, API versions, or environment prefixes.
- Building a general identity provider.
- OAuth/OIDC login flows.
- Multi-tenant customer authorization.
- Browser session/cookie authentication.
- A web administration UI.
- Direct TLS certificate issuance inside controller core.
- Binding the controller process directly to privileged ports.
- Implementing a full dynamic-DNS client in GORC.
- Guaranteeing laptop availability.
- Automatically migrating a running controller between laptop and VM.
- Replacing SSH, Slurm, Singularity, Docker, or execution-environment transports.
- Implementing the final external keystore integration.
- Claiming bearer tokens are the final long-term identity architecture.
- Adding automatic retries to non-idempotent controller operations without a
  separate correctness decision.

## Implementation Tracker

| Slice | Status | Minimum recommended model | Reason |
|---|---|---|---|
| `OS-001-controller-api-auth-contract.md` | Proposed | GPT-5.5 high reasoning | Security contract, startup interlocks, and route-role semantics are long-lived. |
| `OS-002-controller-route-authorization-middleware.md` | Proposed | GPT-5.5 high reasoning | A route omission would expose the control plane. |
| `OS-003-shared-controller-http-client.md` | Proposed | GPT-5.4-mini | Narrow reusable HTTP abstraction after policy is fixed. |
| `OS-004-cli-controller-credential-loading.md` | Proposed | GPT-5.4-mini | Mostly client configuration, safe token loading, and request migration. |
| `OS-005-worker-control-plane-credential-bootstrap.md` | Proposed | GPT-5.5 high reasoning | Crosses controller/runtime/HPCC secret boundaries. |
| `OS-006-worker-controller-http-client-migration.md` | Proposed | GPT-5.4-mini | Mechanical migration once shared client and bootstrap contract exist. |
| `OS-007-laptop-test-https-ingress.md` | Proposed | GPT-5.4-mini | Deployment scripts/runbook with a narrow test-only boundary. |
| `OS-008-dedicated-server-https-deployment.md` | Proposed | GPT-5.4-mini | Provider-neutral service and reverse-proxy baseline. |
| `OS-009-network-security-and-external-smoke.md` | Proposed | GPT-5.5 high reasoning | End-to-end negative authorization and remote callback evidence. |
| `OS-010-concept-closure-and-doc-sync.md` | Proposed | GPT-5.3-Codex-Spark | Tracker, state, runbook, and compatibility documentation. |

## Suggested Implementation Order

Core security and callers:

```text
OS-001 auth contract
OS-002 controller middleware
OS-003 shared HTTP client
OS-004 CLI credential loading
OS-005 worker credential bootstrap
OS-006 worker HTTP migration
```

Laptop proof:

```text
OS-007 laptop test ingress
OS-009 external smoke, laptop profile
```

Production baseline:

```text
OS-008 dedicated server deployment
OS-009 production-like smoke
OS-010 closure and documentation
```

The reverse SSH callback tunnel should remain available until the OS-009 external
smoke proves a worker can complete a real assignment through HTTPS.

## Completion Criteria

- The controller cannot start unauthenticated on a non-loopback listener.
- Every registered route/method has an explicit public or role policy.
- Missing credentials return `401`.
- Valid credentials with the wrong role return `403`.
- Only an administrator can invoke shutdown.
- The public health endpoint reveals no queue, workflow, path, version, or secret
  information beyond minimal liveness.
- CLI requests can authenticate without a raw token command-line argument.
- Worker requests authenticate through a token loaded from a restrictive worker
  file.
- Generated worker JSON contains a token-file reference, not token material.
- No controlled log, status payload, error, fingerprint, or SQLite row contains
  test sentinel credentials.
- The worker no longer uses package-level `http.Get` or `http.Post` for controller
  communication.
- An HPCC/fake-HPCC worker can claim and complete work through a public HTTPS URL
  while the controller remains loopback-bound on the laptop.
- The laptop test requires no SSH reverse callback tunnel.
- A dedicated-server runbook exposes only HTTPS publicly and keeps the controller
  listener private.
- SSH execution transport and Slurm submission continue to work.
- State and runtime documentation distinguish local, laptop-test, and production
  profiles.

## Issues Policy

If implementation reveals a security ambiguity, route caller uncertainty, unsafe
credential propagation, retry/idempotency issue, or deployment assumption that
cannot be proven, stop and append it to:

```text
docs/concepts/secure-network-exposure-of-controller-api/issues.md
```

Do not silently:

- make a route public;
- grant a role broader access;
- store a token in JSON or SQLite;
- allow an insecure external HTTP URL;
- copy a token through command-line arguments;
- add automatic retries to a state-changing request;
- treat laptop networking as production-ready.

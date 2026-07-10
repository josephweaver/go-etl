# OS-008: Dedicated Server HTTPS Deployment

Status: Proposed  
Minimum recommended model: GPT-5.4-mini  
Reference: EC-3 / operational slice / deploy(3)+smoke+doc

## Objective

Provide a provider-neutral production baseline for running the controller on a
dedicated server with stable HTTPS ingress.

A Google Compute Engine VM may implement this profile, but the core deployment
must not require Google-specific APIs.

## Target Topology

```text
Internet
  -> DNS controller.example.org
  -> server ports 80/443
  -> Caddy or equivalent
  -> http://127.0.0.1:8080
  -> GORC controller
  -> persistent SQLite/cache/log/artifact paths
```

## Requirements

### Controller service

- Run as an unprivileged dedicated service account.
- Bind only to `127.0.0.1:8080`.
- Advertise the public HTTPS URL.
- Require bearer authentication.
- Load controller credentials from restrictive service environment or secret
  files.
- Persist the controller root directory outside temporary filesystem space.
- Define restart behavior.
- Use graceful shutdown.
- Set explicit working directory.
- Ensure only one controller owns the database.
- Keep logs and persistence paths writable by the service account.

### HTTPS ingress

- Document Caddy as the first supported example.
- Redirect HTTP to HTTPS.
- Reverse proxy only the controller API.
- Preserve request methods and bodies.
- Set conservative request/header limits compatible with controller limits.
- Do not log authorization headers.
- Use a persistent writable certificate/data directory.
- Expose `/healthz` for liveness.
- Do not publish the controller's internal port.

### Host networking

- Public firewall permits only required ingress, normally TCP 80 and 443.
- SSH administration policy is operator/provider-specific and separate.
- Port 8080 is loopback-only.
- Database and artifact paths are not network shares by default.
- Document outbound requirements for source downloads and execution transports.

### Credentials

- Service credentials are not committed.
- File permissions are restrictive.
- Client and worker tokens are distinct.
- Administrator token is distinct.
- Token rotation requires a controlled controller restart in phase 1.
- Old worker credentials are invalid after rotation unless the operator
  intentionally overlaps credentials during a documented transition.

### Operations

Document:

- install;
- configuration;
- first start;
- health verification;
- authenticated status verification;
- log location;
- restart;
- backup;
- restore prerequisites;
- credential rotation;
- certificate/data persistence;
- rollback to prior binary/config;
- migration from laptop test URL to server URL.

## Candidate Files

- `deploy/caddy/Caddyfile.example`
- `deploy/systemd/goet-controller.service.example`
- `deploy/systemd/goet-controller.env.example`
- `docs/deployment/dedicated-controller-server.md`
- `scripts/network/smoke-controller-endpoint`
- this concept README only for tracker/status updates

Do not place a real domain, token, private path, or cloud project ID in committed
examples.

## Example Caddy Shape

The implementation may provide a template equivalent to:

```text
controller.example.org {
    reverse_proxy 127.0.0.1:8080
}
```

Add only reviewed operational settings. Avoid a large proxy configuration that
duplicates application authorization.

## Google VM Appendix

A short appendix may describe Google Compute Engine as one realization:

- small Linux VM;
- reserved external IP or stable load-balancer address;
- DNS record;
- firewall for 80/443;
- persistent disk;
- systemd services.

Do not require a Google VM for local or laptop testing. Do not embed Google billing,
project, region, or credential assumptions into GORC.

## Required Context

Read first:

- `docs/RUNTIME_RUNBOOK.md`
- `docs/ARCHITECTURE_STATE.md`
- current controller startup configuration;
- OS-001 through OS-007;
- database ownership/recovery documentation;
- log and artifact path documentation.

## Out of Scope

- High availability.
- Multi-controller consensus.
- Managed database migration.
- Kubernetes.
- Automatic horizontal scaling.
- Cloud provider procurement.
- Terraform.
- A provider-specific production module.
- OIDC.
- Browser UI.
- Worker compute provisioning unrelated to controller hosting.

## Acceptance Criteria

- A production-like Linux host runs controller and ingress as separate services.
- Only ports 80/443 are publicly reachable for the controller API.
- Port 8080 is loopback-only.
- HTTP redirects to HTTPS.
- Public HTTPS uses a trusted certificate.
- `/healthz` works.
- Protected routes require GORC credentials even when reached locally behind the
  proxy.
- Authorization headers are absent from ingress access logs.
- Controller restart preserves database and required operational state.
- Token sentinels are absent from service logs.
- The runbook is provider-neutral.
- A Google VM is documented only as an optional implementation.

## Stop Conditions

Stop and append to `issues.md` if:

- the controller must run as root;
- public port 8080 is required;
- ingress configuration logs bearer credentials;
- certificate state is ephemeral;
- controller data paths are not persistent;
- graceful restart can corrupt or duplicate database ownership.

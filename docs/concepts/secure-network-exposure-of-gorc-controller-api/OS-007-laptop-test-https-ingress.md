# OS-007: Laptop Test HTTPS Ingress

Status: Implemented
Minimum recommended model: GPT-5.4-mini  
Reference: EC-2 / operational slice / scripts(2)+config+doc

## Objective

Provide a repeatable, explicitly non-production way to expose a laptop-hosted
controller to external clients and HPCC workers through HTTPS without an SSH
reverse callback tunnel.

## Current State

A laptop-hosted controller can reach HPCC execution hosts through SSH and can expose
callbacks through an SSH reverse tunnel.

The controller can already advertise a URL different from its listener, but there
is no runbook for obtaining a temporary HTTPS URL and binding it into controller
and worker configuration before launch.

## Target State

Preferred test topology:

```text
GORC controller
  listens:    http://127.0.0.1:8080
  advertised: https://temporary-test-name
                     |
                     v
             managed HTTPS ingress
```

The first runbook should support one concrete managed ingress. Prefer a tool that:

- initiates outbound connections from the laptop;
- supplies a publicly trusted HTTPS URL;
- does not require a stable public IP;
- does not require router port forwarding;
- can be stopped after the test;
- can expose only the controller listener.

Tailscale Funnel is a suitable first documented option where available. A
Cloudflare Tunnel-style connector is an acceptable alternative.

## Implementation State

Implemented a test-only laptop HTTPS ingress runbook and helper artifacts:

- `docs/deployment/laptop-test-controller-ingress.md` documents the loopback
  controller listener, Tailscale Funnel managed-ingress path, validation steps,
  worker configuration timing, expected laptop availability failures, cleanup, and
  direct dynamic-DNS fallback checklist.
- `scripts/network/validate-controller-ingress.ps1` validates the public HTTPS
  endpoint, public `/healthz`, unauthenticated protected-route rejection, client
  token access to `/status`, client rejection from `/work/next`, and optional
  worker-token behavior.
- `scripts/network/render-controller-url-override.ps1` validates an HTTPS
  controller URL and writes or prints an uncommitted local controller JSON
  fragment for `controller_config.controller_url`.
- `docs/deployment/laptop-test-controller.example.json` provides a placeholder
  bearer-authenticated laptop-test controller config shape with matching worker
  runtime `controller_url` and `controller_token_file` settings.

## Dynamic IP Decision

Yes, an operator can discover the current public IP just in time and update a DNS
record or render a controller URL.

That is not the preferred test implementation because public-IP discovery does not
prove inbound reachability. Direct exposure also requires:

- a publicly routable address rather than carrier-grade NAT;
- router forwarding for `80` and `443`;
- host firewall rules;
- an ingress/TLS process;
- DNS propagation;
- stable-enough connectivity for the duration of the run.

Therefore:

```text
managed HTTPS ingress = preferred laptop test path
dynamic DNS/direct IP = documented optional fallback
```

No public-IP discovery code belongs in controller core.

## Requirements

- Add a laptop-test runbook.
- Keep the controller bound to loopback.
- Require controller bearer authentication.
- Start the selected managed ingress against `127.0.0.1:8080`.
- Capture or accept the resulting HTTPS URL.
- Validate:
  - URL scheme is HTTPS;
  - `/healthz` is reachable externally;
  - protected routes return `401` without credentials;
  - the expected role credential succeeds.
- Render or override `controller_url` before execution-environment preparation.
- Ensure generated worker configuration contains the same URL.
- Clearly stop if the ingress URL changes after workers have launched.
- Document laptop sleep, restart, Wi-Fi change, and process termination as
  expected availability failures.
- Provide cleanup instructions.
- Keep the SSH reverse callback tunnel available as a fallback during migration.
- Do not expose SQLite, worker paths, or controller internal ports separately.

## Candidate Files

- `docs/deployment/laptop-test-controller-ingress.md`
- `scripts/network/validate-controller-ingress`
- `scripts/network/render-controller-url-override`
- optional PowerShell equivalents where the repository supports Windows-first
  operator scripts
- a placeholder config showing authentication and external `controller_url`
- this concept README only for tracker/status updates

## Script Contract

`validate-controller-ingress` should accept:

```text
--controller-url
--client-token-file or --worker-token-file
```

It should test:

```text
GET /healthz                 -> success without token
GET /status                  -> 401 without token
GET /status with client      -> success
GET /work/next with client   -> 403
```

Do not print the token.

`render-controller-url-override` should validate and emit a canonical override or
local generated controller JSON. It must not edit committed configuration.

## Direct Dynamic-DNS Appendix

The runbook may include a secondary direct-exposure checklist:

```text
discover public IPv4/IPv6
verify not behind CGNAT
update short-TTL DNS record
forward 80/443 to laptop ingress
allow host firewall
run Caddy or equivalent
verify from an unrelated network
```

The appendix must state that DNS caches and address changes can interrupt active
workers. It is a diagnostic/testing option, not production architecture.

## Out of Scope

- GORC-managed dynamic DNS.
- Router configuration automation.
- ISP-specific NAT traversal.
- Production availability.
- Automatic controller migration to a VM.
- Permanent public laptop service.
- Replacing controller authentication with ingress authentication.
- Removing the SSH callback tunnel before external smoke passes.

## Acceptance Criteria

- A laptop controller remains bound to `127.0.0.1`.
- An external HTTPS URL reaches `/healthz`.
- Protected routes reject unauthenticated traffic.
- A role-scoped token succeeds through the ingress.
- A fake-HPCC or real small HPCC worker can claim and report work through the
  HTTPS URL.
- The run uses no SSH reverse callback tunnel.
- The ingress URL is captured before worker config generation.
- Shutdown/cleanup removes the temporary public ingress.
- Documentation labels the profile as test-only.
- No GORC production code depends on the chosen tunnel vendor.

## Stop Conditions

Stop and append to `issues.md` if:

- the selected ingress cannot preserve normal HTTP methods or response bodies;
- the ingress rewrites paths unexpectedly;
- bearer headers are stripped;
- the public URL is not usable from the HPCC compute network;
- the controller must bind publicly on the laptop;
- the only working solution requires disabling GORC authentication.

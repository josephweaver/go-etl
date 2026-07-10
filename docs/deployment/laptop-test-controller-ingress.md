# Laptop Test Controller HTTPS Ingress

Status: test-only

This runbook exposes a laptop-hosted GORC controller through a temporary managed
HTTPS ingress for external client and HPCC worker testing. It is not a production
deployment pattern.

As of 2026-07-10, this profile is documented but has not been separately recorded
as a successful smoke run. The verified external callback evidence for the secure
network exposure concept is the dedicated-server HTTPS profile plus an actual
HPCC Slurm/Singularity worker callback.

## Topology

```text
external client or HPCC worker
  -> temporary public HTTPS URL
  -> managed ingress on laptop
  -> http://127.0.0.1:8080
  -> GORC controller
```

The controller must stay bound to `127.0.0.1:8080`. The public URL is only the
advertised `controller_url` used by clients and generated worker configs.

## Prerequisites

- A controller config with bearer authentication enabled.
- A restrictive client token file for validation.
- A restrictive worker token file for workers.
- A managed HTTPS ingress tool. The first documented path is Tailscale Funnel.
- PowerShell for the helper scripts.

Do not put token values in command lines, generated worker JSON, committed config,
logs, or status output.

## Prepare Token Files

Create local token files outside committed source. Example paths used by the
placeholder config:

```text
.run/secrets/controller-client-token
.run/secrets/controller-worker-token
```

On Unix-like systems, use owner-only permissions:

```bash
chmod 600 .run/secrets/controller-client-token .run/secrets/controller-worker-token
```

On Windows, store these files under a user-private workspace path.

## Start The Controller

Start the controller so it listens only on loopback:

```powershell
go run ./cmd/controller --config .run/laptop-test-controller.json
```

The effective controller settings must include:

```text
controller_listen_host = 127.0.0.1
controller_listen_port = 8080
authentication.mode = bearer
controller_url = https://<temporary-ingress-host>
```

## Start Tailscale Funnel

Use Tailscale Funnel or an equivalent managed HTTPS ingress to forward the public
HTTPS endpoint to the loopback controller listener:

```powershell
tailscale funnel --bg 8080
```

Capture the HTTPS URL printed by the ingress tool before launching workers. If the
URL changes after worker config generation, stop the run and regenerate controller
and worker configuration.

Equivalent managed ingress tools are acceptable only when they preserve HTTP
methods, response bodies, paths, and `Authorization` headers.

## Render A Local Controller URL Override

Use the helper to generate an uncommitted local controller override:

```powershell
pwsh -NoProfile -File scripts/network/render-controller-url-override.ps1 `
  -ControllerUrl https://example-name.ts.net `
  -OutputPath .run/laptop-test-controller-url.json
```

The helper validates that the URL is HTTPS and writes a local JSON file. It does
not edit committed configuration.

The worker execution-environment runtime must use the same URL in
`runtime.settings.controller_url`, and generated worker JSON must contain that URL
plus `controller_token_file`.

## Validate The Public Ingress

Run the validator from an environment that reaches the public URL:

```powershell
pwsh -NoProfile -File scripts/network/validate-controller-ingress.ps1 `
  -ControllerUrl https://example-name.ts.net `
  -ClientTokenFile .run/secrets/controller-client-token `
  -WorkerTokenFile .run/secrets/controller-worker-token
```

The validator checks:

- `GET /healthz` succeeds without credentials.
- `GET /status` returns `401` without credentials.
- A client token can call `GET /status`.
- A client token receives `403` from `GET /work/next`.
- A worker token can call `GET /work/next` and receives either work or no work.
- A worker token receives `403` from `GET /status`.

The script never prints token contents.

## Worker Test

Before starting fake-HPCC or real HPCC workers, confirm the generated worker
configuration contains:

```json
{
  "controller_url": "https://example-name.ts.net",
  "controller_token_file": "/path/readable/by/worker/controller-worker-token"
}
```

Do not use the SSH reverse callback tunnel for this test profile. SSH may still be
used as an execution transport for copying files or submitting Slurm work.

## Expected Failures

Laptop sleep, restart, Wi-Fi changes, VPN changes, ingress process exit, and
temporary URL rotation can interrupt workers. Treat those as expected test-profile
availability failures, not production evidence.

If the temporary URL changes after workers launch, stop the test, shut down the
workers, render a fresh controller URL override, regenerate worker config, and
start a new run.

## Cleanup

Stop workers first, then stop the ingress and the controller:

```powershell
tailscale funnel reset
```

Then call controller shutdown with an admin credential or stop the local controller
process directly if the test is already isolated.

Remove temporary generated files under `.run/` when the test is complete.

## Direct Dynamic-DNS Appendix

Direct public-IP or dynamic-DNS exposure is a secondary diagnostic path, not the
preferred laptop test path. Before using it, verify all of the following:

- The laptop has a publicly routable IPv4 or IPv6 address and is not behind CGNAT.
- DNS points at the current address with a short TTL.
- Router forwarding sends only public ingress ports, normally `80` and `443`, to
  the local ingress.
- Host firewall rules permit the ingress and do not expose the controller's
  internal `8080` listener.
- A Caddy or equivalent ingress terminates HTTPS and proxies to
  `127.0.0.1:8080`.
- Verification is performed from an unrelated network.

DNS caches and address changes can interrupt active workers. Do not treat this as
production architecture.

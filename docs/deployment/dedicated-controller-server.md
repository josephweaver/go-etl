# Dedicated Controller Server HTTPS Deployment

Status: provider-neutral production baseline

This runbook describes a single-controller deployment on a dedicated Linux server
with stable DNS, public HTTPS ingress, and a private loopback controller listener.
It is intentionally provider-neutral. A Google Compute Engine VM can implement
this shape, but GORC does not require Google-specific APIs.

## Topology

```text
Internet
  -> DNS controller.example.org
  -> server TCP 80/443
  -> Caddy or equivalent
  -> http://127.0.0.1:8080
  -> GORC controller
  -> persistent SQLite/cache/log/artifact paths
```

The controller process must not bind public interfaces. Public networking belongs
to the ingress.

## Host Baseline

- Create an unprivileged service account, for example `goet`.
- Install the controller binary at `/usr/local/bin/goet-controller`.
- Use `/opt/goet` as the working directory for release files and static
  configuration.
- Use `/etc/goet` for controller config and secret file references.
- Use `/var/lib/goet` for persistent database, repo cache, temp, artifact cache,
  and Caddy certificate/data dependencies owned by the relevant service account.
- Use `/var/log/goet` for controller filesystem logs.
- Permit public TCP `80` and `443` only for controller API ingress.
- Keep TCP `8080` loopback-only. Do not open it in host or cloud firewalls.
- Treat SSH administration policy as operator/provider-specific and separate from
  GORC.
- Permit outbound traffic required for configured source downloads, package or
  container pulls, execution transports, DNS, and certificate issuance/renewal.
  Avoid granting broad inbound access for those outbound workflows.

The controller currently uses SQLite for the main store. Run only one controller
service against one SQLite database path.

## Controller Configuration

Use a controller config equivalent to this shape, replacing only placeholders:

```json
{
  "api_version": "goet/v1alpha1",
  "kind": "Controller",
  "variables": [
    {"name": {"namespace": "controller_config", "key": "controller_listen_host"}, "type": "string", "expression": "127.0.0.1"},
    {"name": {"namespace": "controller_config", "key": "controller_listen_port"}, "type": "int", "expression": 8080},
    {"name": {"namespace": "controller_config", "key": "controller_url"}, "type": "string", "expression": "https://controller.example.org"},
    {"name": {"namespace": "controller_config", "key": "controller_root_dir"}, "type": "path", "expression": "/var/lib/goet/controller"},
    {"name": {"namespace": "controller_config", "key": "controller_log_root_path"}, "type": "path", "expression": "/var/log/goet/controller"},
    {"name": {"namespace": "controller_config", "key": "main_database_driver"}, "type": "string", "expression": "sqlite"},
    {"name": {"namespace": "controller_config", "key": "main_database_connection_string"}, "type": "string", "expression": "/var/lib/goet/controller/workflow-execution.sqlite"},
    {
      "name": {"namespace": "controller_config", "key": "authentication"},
      "type": "object",
      "expression": {
        "mode": {"type": "string", "expression": "bearer"},
        "credentials": {"type": "list", "expression": [
          {"type": "object", "expression": {
            "id": {"type": "string", "expression": "client-primary"},
            "role": {"type": "string", "expression": "client"},
            "token_file": {"type": "path", "expression": "/etc/goet/secrets/controller-client-token"}
          }},
          {"type": "object", "expression": {
            "id": {"type": "string", "expression": "workers-primary"},
            "role": {"type": "string", "expression": "worker"},
            "token_file": {"type": "path", "expression": "/etc/goet/secrets/controller-worker-token"}
          }},
          {"type": "object", "expression": {
            "id": {"type": "string", "expression": "admin-primary"},
            "role": {"type": "string", "expression": "admin"},
            "token_file": {"type": "path", "expression": "/etc/goet/secrets/controller-admin-token"}
          }}
        ]}
      }
    }
  ]
}
```

Secret files must not be committed. Use distinct client, worker, and admin
tokens. On Unix-like systems:

```bash
chown -R root:goet /etc/goet
chmod 0750 /etc/goet /etc/goet/secrets
chmod 0640 /etc/goet/secrets/controller-*-token
```

The controller rejects group/other-readable token files where permission bits are
available, so adjust ownership and mode to match the actual service account.

## systemd Service

Use `deploy/systemd/goet-controller.service.example` and
`deploy/systemd/goet-controller.env.example` as starting points.

Install:

```bash
sudo install -d -o goet -g goet /opt/goet /var/lib/goet/controller /var/log/goet/controller
sudo install -d -o root -g goet -m 0750 /etc/goet /etc/goet/secrets
sudo install -m 0644 deploy/systemd/goet-controller.service.example /etc/systemd/system/goet-controller.service
sudo install -m 0640 deploy/systemd/goet-controller.env.example /etc/goet/goet-controller.env
sudo systemctl daemon-reload
```

First start:

```bash
sudo systemctl enable --now goet-controller
sudo systemctl status goet-controller
```

The example `ExecStop` reads the admin token file and sends `POST /shutdown` to
the loopback listener without putting the token in the curl command line. If that
stop path fails, inspect logs before forcing termination to avoid corrupting or
duplicating database ownership assumptions.

## Caddy HTTPS Ingress

Use `deploy/caddy/Caddyfile.example` as the starting point and replace
`controller.example.org`.

The example:

- lets Caddy redirect HTTP to HTTPS;
- proxies only to `127.0.0.1:8080`;
- sets a 16 MiB request body limit to match the controller default;
- sets a 1 MiB request header limit to match the controller default;
- does not enable access logs, avoiding accidental Authorization header logging;
- relies on persistent Caddy storage for certificates.

Install and validate Caddy according to the operating system package. Keep Caddy
state on persistent disk, not ephemeral temp storage.

## Health And Smoke Verification

From the server:

```bash
ss -ltnp | grep ':8080'
```

The listener must show `127.0.0.1:8080` or `[::1]:8080`, not `0.0.0.0:8080`.

From any host with PowerShell:

```powershell
pwsh -NoProfile -File scripts/network/smoke-controller-endpoint.ps1 `
  -ControllerUrl https://controller.example.org `
  -HttpUrl http://controller.example.org `
  -TokenFile /etc/goet/secrets/controller-client-token
```

The smoke checks:

- public HTTPS `/healthz`;
- unauthenticated `/status` returns `401`;
- authenticated `/status` succeeds;
- optional HTTP URL redirects to HTTPS;
- optional loopback `/healthz` and protected-route checks when run on the server.

## Operations

### Logs

- Controller service logs: `journalctl -u goet-controller`.
- Controller filesystem logs: configured `controller_log_root_path`, for example
  `/var/log/goet/controller`.
- Ingress logs: disabled in the example. If enabled, never include request
  headers.

### Backup

Back up before upgrades and credential rotations:

- `/etc/goet/controller.json`;
- `/etc/goet/goet-controller.env`;
- `/var/lib/goet/controller/workflow-execution.sqlite`;
- controller repo/artifact cache only if the recovery objective requires it;
- Caddy data/cert storage according to Caddy packaging.

Stop the controller or otherwise ensure SQLite consistency before copying the
database.

### Restore Prerequisites

Restore config, database, cache policy, and secret files before starting the
service. The service account must be able to read config/token files and write
controller root and log paths.

### Restart

Use:

```bash
sudo systemctl restart goet-controller
```

Then run the smoke script. Restart should preserve the database and required
operational state because the controller root is persistent.

### Credential Rotation

Phase 1 rotation requires a controlled controller restart:

1. Write the new token file with restrictive permissions.
2. Update `controller_config.authentication` to reference it or overlap old and
   new credential declarations intentionally.
3. Restart the controller.
4. Update client and worker token files.
5. Remove old credential declarations after all old workers are drained.

Old worker credentials are invalid after rotation unless overlap is intentionally
configured.

### Rollback

Keep the prior binary and controller config available. To roll back:

1. Stop the controller through systemd.
2. Restore the prior binary/config.
3. Restore the database only if the newer binary performed an incompatible
   migration. At present this runbook does not define automatic schema migration.
4. Start the service and run the smoke script.

### Migrating From Laptop Test URL

Replace the laptop ingress URL with the stable server URL in:

- `controller_config.controller_url`;
- execution-environment `runtime.settings.controller_url`;
- client controller configs;
- worker token-file provisioning instructions.

Workers launched with the laptop URL must be stopped and relaunched with generated
worker config that names the server URL.

## Google Compute Engine Appendix

One possible implementation is:

- a small Linux VM;
- reserved external IP or stable load-balancer address;
- DNS `A`/`AAAA` record for the controller hostname;
- firewall allowing TCP `80` and `443`;
- persistent disk mounted for `/var/lib/goet`;
- systemd services for controller and Caddy.

Do not embed Google project, billing, region, service account, or credential
assumptions into GORC configuration. The same runbook applies to any equivalent
dedicated server.

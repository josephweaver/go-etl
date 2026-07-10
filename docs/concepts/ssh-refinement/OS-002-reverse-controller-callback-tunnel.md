# OS-002: Reverse Controller Callback Tunnel

Status: Complete
Scope: GOET controller execution environment and SSH transport

## Purpose

Allow a controller running on a user's laptop to expose a worker-reachable
callback URL through an SSH reverse tunnel.

This supports HPCC runs where the controller is local, but Slurm workers need to
POST work claims, completions, failures, and logs back to the controller API.

## Requirements

- Add a controller-managed reverse tunnel over an SSH transport.
- Use one shared controller-owned tunnel per configured callback tunnel, not one
  tunnel per worker or one tunnel per worker callback.
- Forward a remote bind address and port to the local controller host and port.
- Bind the remote listener on the configured SSH hop. For a gateway-based HPCC
  target this may be the configured gateway host.
- Produce or validate the `controller_url` workers should use. Workers should
  receive only an HTTP callback URL; they should not own SSH credentials,
  gateway topology, or tunnel setup.
- Keep tunnel lifecycle tied to the controller process or execution environment.
- Fail preflight if the remote tunnel cannot be established.
- Fail preflight if the remote callback URL is not reachable from the HPCC side.
- Do not require worker access to the controller database.

## Candidate Config Shape

```json
{
  "execution_environment": {
    "callback_tunnel": {
      "type": "ssh_reverse",
      "transport": "login",
      "bind_hop": "jump_hosts[0]",
      "remote_bind_host": "0.0.0.0",
      "remote_bind_port": 18080,
      "relay_bind_host": "0.0.0.0",
      "relay_bind_port": 19080,
      "local_host": "127.0.0.1",
      "local_port": 8080,
      "worker_controller_url": "http://hpcc.example.edu:19080"
    }
  }
}
```

If site SSH policy forces the reverse-forward listener to loopback, the optional
relay fields let the controller start a small dev/login-node relay that binds a
worker-visible address and forwards to the loopback reverse listener. The worker
runtime must then use the relay URL as `controller_url` and explicitly enable
`controller_insecure_external_http_allowed`.

## Implementation Notes

- Use SSH reverse forwarding primitives rather than invoking `ssh -R`.
- `bind_hop` selects which SSH client owns the reverse listener. For an
  `SSHTransport` with `jump_hosts`, `jump_hosts[0]` means the first gateway
  client. If omitted, the final target client owns the bind.
- The tunnel needs to accept remote connections and proxy bytes to the local
  controller.
- When relay settings are present, the controller copies and starts the relay on
  the SSH target, validates the relay process ID, and stops that process during
  callback tunnel close.
- The controller should close the listener when shutting down.
- Preflight should check the tunnel from the remote side with a small HTTP GET
  to public `/healthz`.
- For Slurm-backed environments, preflight should submit a tiny `sbatch --wait`
  job that uses `curl` to request the worker-facing `/healthz` URL, because
  login/dev-node reachability may not prove compute-node reachability.
- The Slurm compute-node check may report a missing `curl` command as an
  actionable environment preflight failure.

## Validation

- Unit test reverse tunnel using in-process SSH server and local HTTP test
  server.
- Integration-style test proves remote-side HTTP request reaches local
  controller handler.
- Preflight failure is explicit when:
  - remote bind is denied;
  - remote port is already in use;
  - local controller is not listening;
  - callback URL is not reachable from a remote command.

## Stop Conditions

- The tunnel requires privileged ports or gateway execution.
- The tunnel only works from the dev node but not from Slurm worker nodes, and
  no site-approved bind address is available.
- The controller URL written into worker config still points at worker-local
  `localhost` incorrectly.
- The implementation requires workers to establish SSH sessions or hold SSH
  credentials.

## Completion Criteria

- A laptop-hosted controller can expose a remote callback URL through SSH.
- Workers can be configured with the tunnel URL instead of laptop-local
  `localhost`.
- Preflight can detect callback reachability before admitting real work.

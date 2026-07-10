# OS-002: Reverse Controller Callback Tunnel

Status: Approved
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
- Bind the remote listener on the configured SSH hop. For the LandCore HPCC
  target this is expected to be the gateway reachable as `hpcc.msu.edu`.
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
      "local_host": "127.0.0.1",
      "local_port": 8080,
      "worker_controller_url": "http://hpcc.msu.edu:18080"
    }
  }
}
```

If compute nodes cannot reach the remote bind host, the rendered config must use
a site-approved bind address or the run must move the controller to the HPCC dev
side.

## Implementation Notes

- Use SSH reverse forwarding primitives rather than invoking `ssh -R`.
- `bind_hop` selects which SSH client owns the reverse listener. For an
  `SSHTransport` with `jump_hosts`, `jump_hosts[0]` means the first gateway
  client. If omitted, the final target client owns the bind.
- The tunnel needs to accept remote connections and proxy bytes to the local
  controller.
- The controller should close the listener when shutting down.
- Preflight should check the tunnel from the remote side with a small HTTP GET
  to `/status`.
- A stronger HPCC preflight should submit a tiny Slurm job that curls the
  worker-facing controller URL, because login/dev-node reachability may not
  prove compute-node reachability.
- The first implementation may validate gateway-side reachability and leave
  Slurm compute-node reachability as a follow-up if it would exceed the active
  HCI budget.

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

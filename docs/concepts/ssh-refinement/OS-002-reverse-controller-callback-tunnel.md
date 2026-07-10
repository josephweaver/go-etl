# OS-002: Reverse Controller Callback Tunnel

Status: Proposed
Scope: GOET controller execution environment and SSH transport

## Purpose

Allow a controller running on a user's laptop to expose a worker-reachable
callback URL through an SSH reverse tunnel.

This supports HPCC runs where the controller is local, but Slurm workers need to
POST work claims, completions, failures, and logs back to the controller API.

## Requirements

- Add a controller-managed reverse tunnel over an SSH transport.
- Forward a remote bind address and port to the local controller host and port.
- Produce or validate the `controller_url` workers should use.
- Keep tunnel lifecycle tied to the controller process or execution environment.
- Fail preflight if the remote tunnel cannot be established.
- Fail preflight if the remote callback URL is not reachable from the HPCC side.
- Do not require worker access to the controller database.

## Candidate Config Shape

```json
{
  "callback_tunnel": {
    "type": "ssh_reverse",
    "transport": "login",
    "remote_bind_host": "127.0.0.1",
    "remote_bind_port": 18080,
    "local_host": "127.0.0.1",
    "local_port": 8080,
    "worker_controller_url": "http://127.0.0.1:18080"
  }
}
```

If compute nodes cannot reach the remote bind host, the rendered config must use
a site-approved bind address or the run must move the controller to the HPCC dev
side.

## Implementation Notes

- Use SSH reverse forwarding primitives rather than invoking `ssh -R`.
- The tunnel needs to accept remote connections and proxy bytes to the local
  controller.
- The controller should close the listener when shutting down.
- Preflight should check the tunnel from the remote side with a small HTTP GET
  to `/status`.
- A stronger HPCC preflight should submit a tiny Slurm job that curls the
  worker-facing controller URL, because login/dev-node reachability may not
  prove compute-node reachability.

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

## Completion Criteria

- A laptop-hosted controller can expose a remote callback URL through SSH.
- Workers can be configured with the tunnel URL instead of laptop-local
  `localhost`.
- Preflight can detect callback reachability before admitting real work.

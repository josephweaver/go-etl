# SSH Refinement

Status: Implemented

## Purpose

Refine GOET's SSH transport so real HPCC deployments can use the same connection
topologies that operators already use with OpenSSH.

The immediate driver is the LandCore OS-007 one-tile HPCC preflight:

- the login path uses a gateway and a development node;
- the user's working OpenSSH alias uses `ProxyJump`;
- workers launched through Slurm need a reachable controller callback URL when
  the controller runs on a laptop;
- host-key verification should not require copying opaque values by hand for
  every local rendered config.

## Goals

- Support SSH jump-host chains without relying on the external `ssh` binary.
- Support controller-managed reverse tunnels for worker-to-controller callback
  traffic.
- Preserve explicit host-key verification for every SSH hop.
- Keep gateway nodes as transit only; do not run GOET controller or worker
  payloads on the gateway.
- Make rendered controller configs practical for local use without committing
  private keys, host keys, or site-specific paths.
- Add preflight checks that fail with actionable messages before a workflow is
  submitted.

## Non-Goals

- Replacing Slurm scheduling.
- Implementing a general VPN or long-lived network daemon.
- Storing private SSH keys in the repository.
- Running work payloads on gateway nodes.
- Requiring Google Drive or rclone for the first HPCC path.

## Current State

`cmd/controller/ssh_transport.go` can connect directly to one SSH host or to a
final SSH target through explicit `jump_hosts`, execute commands, transfer files
with SFTP, list remote paths, run basic filesystem commands, reconnect after
session failures, and verify each hop with either a pinned key or a configured
`known_hosts_file`. Local SSH credential paths can use `~`, `$VAR`, or
`${VAR}` expansion. The execution environment can also establish a
controller-owned SSH reverse callback tunnel on the final target or a selected
jump host, then proxy worker HTTP callbacks to the local controller.

The secure network exposure concept proved the preferred production callback
shape: SSH remains the execution transport for remote command execution, file
copy, runtime preparation, and Slurm submission, while workers use the advertised
HTTPS `controller_url` to claim and report work. Reverse callback tunneling
remains supported as optional compatibility behavior for sites where a direct
HTTPS controller endpoint is not available or not yet approved. Callback tunnel
preflight now checks public `/healthz` rather than protected `/status`.

The local/no-domain `ssh_reverse` path has also been smoke-tested with an HPCC
dev-node worker process launched by `RemoteProcessScheduler`. A second smoke
used real HPCC Slurm/Singularity workers through a controller-managed dev-node
relay that forwarded from a worker-visible port to the loopback reverse
listener. Together these prove local client -> local controller -> SSH execution
-> HPCC worker -> reverse callback -> local controller for both dev-node and
relay-backed Slurm execution.

Current gaps:

- Slurm compute-node callback preflight currently depends on `curl` being
  available in the Slurm job environment.
- Site SSH policy may force reverse-forward listeners to loopback even when a
  non-loopback bind is requested. In that case use the callback relay fields to
  expose a worker-visible dev/login-node port, or use another callback path.
- The laptop-hosted temporary HTTPS profile has not been separately smoke-tested;
  the verified external callback evidence is the dedicated-server HTTPS plus real
  HPCC Slurm/Singularity worker path.

## Target State

The controller can prepare and submit Slurm workers on a dev/login node reached
through one or more SSH jump hosts, while preserving host-key checks for each
hop. The preferred worker-safe `controller_url` is an HTTPS URL served by a
laptop-test ingress or dedicated server ingress. If that is not available, a
controller-owned reverse tunnel can still expose a compatibility callback URL.

Rendered local configs can use normal operator-friendly SSH paths and either a
pinned key or local `known_hosts` file for host-key verification. Committed
templates continue to use placeholders.

## Proposed Slices

- [OS-001 SSH ProxyJump Transport](OS-001-ssh-proxyjump-transport.md)
- [OS-002 Reverse Controller Callback Tunnel](OS-002-reverse-controller-callback-tunnel.md)
- [OS-003 SSH Config Ergonomics and Host-Key Verification](OS-003-ssh-config-ergonomics-and-host-key-verification.md)

## Open Design Questions

- How should the controller prove callback reachability from an actual Slurm
  worker node rather than only from the login/dev node?

## Completion Criteria

- A controller config can connect through a gateway to a target dev/login node
  without invoking the external `ssh` command.
- A laptop-hosted controller can expose a worker callback URL through an
  explicitly configured reverse tunnel when the site permits it.
- A dedicated-server controller can use SSH to schedule Slurm/Singularity workers
  while those workers call back over HTTPS.
- SSH identity paths and host-key verification behave predictably in rendered
  local configs.
- Preflight diagnostics identify unsupported gateway/tunnel topologies before a
  workflow run is admitted.

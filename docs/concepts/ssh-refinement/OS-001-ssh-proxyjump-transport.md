# OS-001: SSH ProxyJump Transport

Status: Complete
Scope: GOET controller SSH transport

## Purpose

Allow GOET to reach a target SSH host through one or more jump hosts, matching
the topology operators commonly express with OpenSSH `ProxyJump`.

This is required for HPCC environments where users first enter a gateway and
then reach a development or login node where Slurm commands and shared
filesystems are available.

## Requirements

- Add explicit jump-host configuration to `SSHTransportConfig`.
- Preserve direct SSH behavior when no jump host is configured.
- Support at least one jump host for the first slice; allow the public config
  shape to represent a chain.
- Verify host keys independently for each hop.
- Authenticate each hop with either the inherited target identity or a
  hop-specific identity.
- Run GOET commands, file copies, and Slurm submission only on the final target
  host, not on gateway hosts.
- Keep command timeout and connect timeout behavior understandable across hops.

## Candidate Config Shape

```json
{
  "host": "dev-amd20",
  "port": "22",
  "user": "weave151",
  "identity_file": "~/.ssh/id_weave151_rsa",
  "host_key_policy": "pinned",
  "pinned_host_key": "ssh-rsa <dev-node-public-host-key>",
  "jump_hosts": [
    {
      "host": "<gateway-host>",
      "port": "22",
      "user": "weave151",
      "identity_file": "~/.ssh/id_weave151_rsa",
      "host_key_policy": "pinned",
      "pinned_host_key": "ssh-rsa <gateway-public-host-key>"
    }
  ]
}
```

## Implementation Notes

- Build the first SSH client to the jump host normally.
- Dial the target through the jump client with `jumpClient.Dial("tcp",
  targetHostPort)`.
- Run `ssh.NewClientConn` for the final target over the tunneled net.Conn.
- Keep SFTP and Exec bound to the final target client.
- Avoid parsing OpenSSH config in this first slice unless it is clearly smaller
  than explicit JSON support.

## Validation

- Unit test direct SSH behavior remains unchanged.
- Test fixture with two in-process SSH servers proves:
  - gateway receives only tunnel traffic;
  - final host receives Exec/SFTP operations;
  - wrong gateway host key fails;
  - wrong final host key fails;
  - final connection failure reports both hop and target context.
- Controller config parsing accepts `jump_hosts`.

## Stop Conditions

- The implementation requires shelling out to `ssh`.
- Host-key verification is skipped for either hop.
- GOET filesystem or Slurm commands can accidentally run on a gateway.

## Completion Criteria

- `SSHTransport` can execute and copy files to a target host through a jump host.
- Tests cover direct and jump-host modes.
- Public config docs include the explicit jump-host JSON shape.

# 010 SSHTransport Config Wiring

Status: proposed

Slice:
cmd/controller / ExecutionEnvironment config / SSHTransport factory / builds SSHTransport from worker_config.transport.settings

Objective:
Wire `SSHTransport` into controller execution-environment construction so backend config can select SSH as the transport implementation.

Allowed Production Files:
- cmd/controller/execution_environment.go
- cmd/controller/config.go
- cmd/controller/ssh_transport.go

Tests:
- cmd/controller/execution_environment_test.go
- cmd/controller/config_test.go
- cmd/controller/ssh_transport_test.go

Out Of Scope:
- real HPCC connection
- Slurm submission over SSH
- Singularity runtime changes
- new scheduler or runtime implementations
- preflight checks beyond constructing components that can later preflight

Acceptance:
- Allows execution-environment config to select an SSH transport type.
- Reads SSH settings from structured `worker_config.transport.settings` or the existing execution-environment config shape, consistent with the current config model.
- Builds an `SSHTransport` using validated `SSHTransportConfig`.
- Preserves existing Docker and Local transport behavior.
- Adds tests showing SSH transport can be selected without affecting existing transport selection.
- Does not require real network access in unit tests.

## Config Intent

The target config shape should keep layer ownership clear:

```text
worker_config.transport.type = "ssh"
worker_config.transport.settings.host
worker_config.transport.settings.user
worker_config.transport.settings.port
worker_config.transport.settings.identity_file
worker_config.transport.settings.host_key_policy
```

The transport config should not contain scheduler settings such as Slurm partition or runtime settings such as Singularity image path.

## Dependency

SSH implementation should use:

```text
golang.org/x/crypto/ssh
```

This is a Go module dependency, not an external system install. The config wiring tests should not require OpenSSH.

## Test Cases

Add tests covering:

- builds SSH transport from valid config
- rejects SSH transport config with missing required settings
- preserves Docker transport config behavior
- preserves Local transport config behavior
- rejects unknown transport type

## Later Features Enabled

This feature enables real backend configs to choose SSH transport while keeping scheduler and runtime choices independent.

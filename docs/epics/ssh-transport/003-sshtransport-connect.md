# 003 SSHTransport Connect

Status: implemented

EC Mode:
EC-4 / file(1)+test+doc

Slice:
cmd/controller / SSHTransport / Connect / establishes and closes an SSH client connection

Objective:
Add the production behavior that turns a validated `SSHTransportConfig` into an SSH client connection and closes it cleanly.

Allowed Production Files:
- cmd/controller/ssh_transport.go

Tests:
- cmd/controller/ssh_transport_test.go

Out Of Scope:
- Execute behavior
- CopyInto behavior
- Preflight issue reporting
- retry policy
- persistent connection pooling
- controller config factory wiring
- real HPCC connection
- Slurm or Singularity execution

Acceptance:
- Adds an `SSHTransport` type or equivalent owner for SSH connection state.
- Creates an SSH client connection from `SSHTransportConfig`.
- Authenticates using the configured noninteractive identity source from feature 002.
- Verifies host identity according to the configured host-key policy.
- Supports context-aware connection timeout behavior.
- Provides a close/shutdown method for the connection.
- Adds tests using the in-process SSH server fixture from feature 001.
- Does not execute remote commands or copy files.

## Artifact

This feature should produce or modify:

```text
cmd/controller/ssh_transport.go
cmd/controller/ssh_transport_test.go
```

The implementation may add minimal private helpers for loading identity material and constructing an SSH client config, but only as needed for connection setup.

## Connection Behavior

The connection behavior should:

- Resolve the target address from `Host` and `Port`.
- Default `Port` to `22` when omitted.
- Use `ConnectTimeout` when present.
- Use a bounded default timeout when `ConnectTimeout` is omitted.
- Return errors that distinguish connection failure, authentication failure, and host-key rejection where practical.
- Close the underlying SSH client without leaking resources.

## Host-Key Behavior

Connection tests should cover:

- pinned host key accepted
- pinned host key mismatch rejected
- explicit `insecure_ignore` accepted for development tests

`known_hosts` file parsing can be deferred if it would expand the slice too much, but the production behavior must not silently disable host-key verification.

## Auth Behavior

Connection tests should cover:

- valid generated client key authenticates to the in-process server
- wrong client key is rejected
- missing configured identity material is rejected before or during connection setup

Password and SSH agent authentication remain deferred.

## Context And Timeout Tests

Tests should cover at least one bounded failure path:

- connection to an unavailable local port returns within the configured timeout, or
- canceled context stops the connection attempt.

## Later Features Enabled

This feature enables later slices to use an established SSH connection for:

- remote execute
- remote copy
- preflight probes
- retry and reconnect behavior

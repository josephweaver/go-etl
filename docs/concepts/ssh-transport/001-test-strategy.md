# 001 SSHTransport Test Fixture

Status: implemented

EC Mode:
EC-4 / file(1)+test+doc

Slice:
cmd/controller / SSH transport test fixture / creates in-process SSH server helpers

Objective:
Create a reusable Go test fixture for SSH transport work before adding production SSH transport code.

Allowed Production Files:
- none

Tests:
- cmd/controller/ssh_transport_test.go

Out Of Scope:
- SSHTransport implementation
- SSHTransportConfig implementation
- controller config wiring
- real HPCC connection
- OpenSSH server dependency
- retry logic
- copy implementation
- Singularity or Slurm execution

Acceptance:
- Adds a reusable in-process SSH server fixture in `cmd/controller/ssh_transport_test.go`.
- Adds helpers to create test host keys and client keys in memory.
- Adds a helper to start a temporary SSH server without OpenSSH, Docker, HPCC credentials, or local key generation.
- Adds a helper to create a matching test SSH client config.
- Adds one self-test proving the fixture can accept a connection and handle a minimal command/session path.
- Does not add production SSH transport code.

## Artifact

This feature should produce:

```text
cmd/controller/ssh_transport_test.go
```

The file should contain test-only helpers. Because it is a `_test.go` file, these helpers do not become production API.

## Required Helpers

The exact helper names may change during implementation, but the fixture should provide these capabilities:

- Generate a host private key in memory.
- Generate a client private key in memory.
- Start a temporary in-process SSH server bound to localhost on an ephemeral port.
- Accept authentication from the generated client key.
- Return the server address to the test.
- Build a client config that pins or trusts the generated host key for the test server.
- Shut down cleanly at test cleanup.

## Self-Test

The feature should include one test that verifies the fixture itself.

Expected behavior:

- Start the temporary SSH server.
- Connect with the generated client config.
- Open one session or minimal command path.
- Verify the server responds in a predictable way.
- Close the connection and server without leaking goroutines where practical.

This self-test is not an SSHTransport test. It proves the test harness is usable for later features.

## Test Dependency

Prefer:

```text
golang.org/x/crypto/ssh
```

This is acceptable because it is a Go module dependency, not an external system install. The test fixture should not require `ssh`, `scp`, `ssh-keygen`, Docker, or a running remote service.

## Later Features Enabled

This fixture should support later feature tests for:

- SSHTransport config validation.
- SSHTransport connection setup.
- Remote execute behavior.
- stdout, stderr, and exit status semantics.
- Host-key rejection and pinned host-key acceptance.
- Authentication failure.
- Connection timeout and context cancellation.
- Connection loss before command execution.
- Copy-to-temp-then-promote behavior.

## Deferred Behaviors

These are not first-slice requirements:

- Persistent SSH connection pooling.
- SSH agent support.
- Password authentication.
- Full known-hosts file management.
- Real HPCC submission.
- Slurm or Singularity execution over SSH.

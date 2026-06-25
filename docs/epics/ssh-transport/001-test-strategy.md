# 001 SSHTransport Test Strategy

Status: proposed

EC Mode:
EC-4 / file(0)+test+doc

Slice:
docs/epics/ssh-transport / test strategy / define test layers before implementation

Objective:
Define the test strategy for SSH transport support before adding production SSH code.

Allowed Production Files:
- none

Tests:
- none in this slice

Out Of Scope:
- SSHTransport implementation
- SSHTransportConfig implementation
- controller config wiring
- real HPCC connection
- OpenSSH server dependency
- Singularity or Slurm execution

Acceptance:
- Separates unit tests from integration tests.
- Prefers Go-native test fixtures over external system installs.
- Defines how host-key, auth, execute, copy, timeout, and retry behavior should be tested.
- Records which behaviors are not part of the first implementation slice.

## Test Layers

### Unit Tests

Unit tests should not require OpenSSH, Docker, HPCC credentials, local key generation, or network access outside the test process.

Use these tests for:

- SSH transport config validation.
- Command timeout and context cancellation behavior.
- Error classification for connection failure, authentication failure, host-key failure, remote nonzero exit, and missing remote command.
- Retry policy decisions.
- Copy-to-temp-then-promote planning.

### In-Process SSH Server Tests

Use a Go-native in-process SSH server, likely based on `golang.org/x/crypto/ssh`, for the first real transport behavior tests.

Use these tests for:

- Establishing a client connection.
- Authenticating with test-generated keys.
- Verifying pinned host-key behavior.
- Executing simple remote commands.
- Returning stdout, stderr, and exit status.
- Simulating connection loss before command execution.

The in-process server should generate keys inside the test and should not require a user's `~/.ssh` directory.

### Copy Tests

Prefer a Go-native server-side fixture for copy behavior if practical.

Copy behavior should preserve this invariant:

```text
CopyInto must not expose a partial final destination file.
```

The expected shape is:

1. Transfer bytes to a unique remote temp path.
2. Promote the temp path to the final destination.
3. Remove the temp path on failure when possible.

The shell dialect should own remote command syntax for parent-directory creation, promotion, and cleanup.

### Preflight Tests

Preflight should report structured issues while the client is still waiting for controller startup or workflow submission.

Preflight should be able to report:

- unknown host key
- authentication failure
- connection timeout
- remote runtime root not writable
- missing `singularity`
- missing `sbatch`

The controller should not ask interactive SSH questions. It should return actionable issues to the client.

### Integration Tests

Integration tests may use an external SSH server only after the unit and in-process server tests define the behavior.

External integration tests should be opt-in and skipped by default unless required environment variables are present.

## Deferred Behaviors

These are not first-slice requirements:

- Persistent SSH connection pooling.
- SSH agent support.
- Password authentication.
- Full known-hosts file management.
- Real HPCC submission.
- Slurm or Singularity execution over SSH.

## First Implementation Candidate

Recommended next slice:

```text
EC-4 / file(1)+test+doc
Slice: cmd/controller / SSHTransportConfig / Validate / rejects incomplete SSH transport config
```

Rationale:

Config validation is the smallest production slice that can be tested without choosing the full SSH client implementation shape.

# 004 SSHTransport Execute

Status: proposed

Slice:
cmd/controller / SSHTransport / Exec / runs one remote command over an established SSH connection

Objective:
Implement `Transport.Exec` behavior for `SSHTransport`, returning remote command output and distinguishing remote command failure from SSH connection failure.

Allowed Production Files:
- cmd/controller/ssh_transport.go

Tests:
- cmd/controller/ssh_transport_test.go

Out Of Scope:
- Copy behavior
- list/filesystem helper behavior
- retry/reconnect policy
- preflight issue reporting
- controller config factory wiring
- Slurm or Singularity execution

Acceptance:
- Runs one remote command over the SSH connection from feature 003.
- Returns stdout for successful commands.
- Captures stderr for error reporting when practical.
- Treats remote nonzero exit as a command failure, not as a connection failure.
- Treats SSH session creation, connection loss, and authentication problems as transport failures.
- Applies context cancellation or configured command timeout.
- Adds focused tests using the in-process SSH fixture from feature 001.
- Does not retry commands in this slice.

## Artifact

This feature should produce or modify:

```text
cmd/controller/ssh_transport.go
cmd/controller/ssh_transport_test.go
```

## Execute Semantics

`Transport.Exec` currently returns:

```go
([]byte, error)
```

For SSH transport, the returned bytes should be stdout for successful commands.

When the remote command exits nonzero, the error should include enough information to debug:

- command or argv
- exit status when available
- stderr when available

The first implementation does not need a public rich error type unless tests or callers need to distinguish error classes directly.

## Command Construction

`SSHTransport.Exec(ctx, args...)` receives already-separated command arguments from callers.

The implementation must preserve argument boundaries and avoid unsafe string joining where possible. If SSH session execution requires a shell command string, command construction should use the configured shell dialect for quoting.

Do not add scheduler-specific command behavior here. Slurm and Singularity remain scheduler/runtime responsibilities.

## Test Cases

Add tests covering:

- successful command returns stdout
- command that writes stderr and exits zero still succeeds
- command that exits nonzero returns an error containing exit status or stderr
- session setup failure returns a transport error
- canceled context or command timeout stops the command where practical
- arguments containing spaces are preserved or safely quoted

## Later Features Enabled

This feature enables:

- filesystem command helpers
- preflight probes
- Slurm command submission over SSH
- remote diagnostics

# 007 SSHTransport Filesystem Commands

Status: proposed

Slice:
cmd/controller / SSHTransport / filesystem commands / supports mkdir, move, remove, chmod, and chown helpers

Objective:
Add narrow remote filesystem command helpers needed by runtime preparation, copy promotion, cleanup, and diagnostics.

Allowed Production Files:
- cmd/controller/ssh_transport.go
- cmd/controller/bash_shell_platform.go

Tests:
- cmd/controller/ssh_transport_test.go
- cmd/controller/bash_shell_platform_test.go

Out Of Scope:
- controller config factory wiring
- broad remote filesystem abstraction
- recursive synchronization
- retry policy
- Slurm or Singularity execution
- user/group discovery
- cross-platform remote shell support beyond the configured shell dialect

Acceptance:
- Adds focused helpers for remote directory creation, move, remove, chmod, and chown behavior.
- Uses the configured shell dialect for command construction and quoting.
- Keeps the public `Transport` interface unchanged unless implementation proves a shared interface is necessary.
- Adds focused tests using the in-process SSH fixture from feature 001.
- Does not require OpenSSH, Docker, HPCC credentials, or local key generation as system installs.

## Artifact

This feature should produce or modify:

```text
cmd/controller/ssh_transport.go
cmd/controller/ssh_transport_test.go
cmd/controller/bash_shell_platform.go
cmd/controller/bash_shell_platform_test.go
```

`BashShellPlatform` may be updated with the smallest required command builders for mkdir, move, remove, chmod, and chown. Do not redesign the dialect interface in this slice.

## Commands

The intended helper set is:

```text
mkdir -p <path>
mv <source> <destination>
rm -f <path>
rm -rf <path>
chmod <mode> <path>
chown <owner[:group]> <path>
```

Use conservative method names during implementation. Avoid exposing a general "run arbitrary filesystem command" API when a named helper communicates intent better.

## Command Ownership

`SSHTransport` owns:

- sending the command over the SSH connection
- returning stdout, stderr, and command errors according to the execute semantics
- applying context/timeouts

`ShellDialect` owns:

- command string construction
- path quoting
- argument quoting
- shell-specific syntax

Do not hard-code Bash quoting inside SSH transport if the existing dialect can provide the command safely.

## Test Cases

Add tests covering:

- creates a nested directory
- moves a file from temp path to final path
- removes one file
- removes one directory tree only when explicitly using recursive remove
- applies chmod to a file when supported by the test filesystem
- rejects or clearly reports chown failure when the test user lacks permission
- quotes paths containing spaces
- returns a clear error for missing source path on move
- respects canceled context or configured timeout where practical

## Safety Notes

Remote remove commands are risky. Recursive remove must be explicit and should reject empty paths, root paths, and obviously unsafe paths.

`chown` may not be usable on shared HPCC systems for ordinary users. Treat chown as optional or best-effort unless a concrete runtime requirement appears.

## Later Features Enabled

This feature enables:

- safe copy promotion and cleanup
- runtime directory preparation over SSH
- preflight checks that can create and remove probe files
- remote permission setup when allowed by the target environment

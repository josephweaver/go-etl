# 006 SSHTransport List

Status: implemented

Slice:
cmd/controller / SSHTransport / List / lists one remote directory or path for preflight and diagnostics

Objective:
Add a focused remote listing capability for `SSHTransport` so later preflight and diagnostics can inspect remote paths without implementing broad filesystem management.

Allowed Production Files:
- cmd/controller/ssh_transport.go
- cmd/controller/bash_shell_platform.go

Tests:
- cmd/controller/ssh_transport_test.go
- cmd/controller/bash_shell_platform_test.go

Out Of Scope:
- controller config factory wiring
- recursive directory walking
- glob expansion
- file metadata beyond the minimal fields needed by callers
- delete, move, chmod, or mkdir APIs
- Slurm or Singularity execution
- retry policy

Acceptance:
- Lists entries for one configured remote directory or path over the established SSH connection.
- Returns enough structured information to distinguish files from directories when the transfer mechanism supports it.
- Returns clear errors for missing paths and permission-denied paths.
- Adds focused tests using the in-process SSH fixture from feature 001.
- Does not require OpenSSH, `ls`, Docker, HPCC credentials, or local key generation as system installs.
- Does not broaden the public `Transport` interface unless the implementation slice explicitly decides that listing belongs there.

## Artifact

This feature should produce or modify:

```text
cmd/controller/ssh_transport.go
cmd/controller/ssh_transport_test.go
cmd/controller/bash_shell_platform.go
cmd/controller/bash_shell_platform_test.go
```

If listing is implemented as an SSHTransport-specific helper rather than part of `Transport`, keep the method narrow and document why it is not yet a shared interface method.

`BashShellPlatform` changes are allowed only if implementation uses remote shell commands for listing or stat-like behavior. Prefer SFTP listing first.

## Proposed Output Shape

The implementation should return structured data rather than parsing human-formatted `ls` output.

Possible shape:

```go
type RemoteFileInfo struct {
    Path  string
    Name  string
    IsDir bool
    Size  int64
}
```

Keep this shape minimal. Add timestamps, modes, owner, group, or symlink behavior only if a later feature requires them.

## Listing Mechanism

Prefer a Go-native mechanism over executing remote `ls`.

Preferred option:

- SFTP `ReadDir` or equivalent over the existing SSH connection.

Fallback option:

- remote shell command built through the shell dialect, only if SFTP is rejected during implementation.

Avoid parsing ordinary `ls` text output unless there is no better option.

## Test Cases

Add tests covering:

- lists entries in an existing remote directory
- distinguishes file and directory entries
- returns an empty list for an empty directory
- returns a clear error for a missing path
- returns a clear error for a regular file when directory listing requires a directory, unless file-stat behavior is intentionally supported
- respects canceled context or configured timeout where practical

The first implementation may use the test server's local temporary directory as the remote filesystem backing store.

## Later Features Enabled

This feature enables:

- preflight checks for remote runtime roots
- diagnostics when copy or runtime preparation fails
- optional validation that worker config, Slurm scripts, or worker artifacts exist after upload

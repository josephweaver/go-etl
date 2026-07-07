# 005 SSHTransport Copy

Status: implemented

Slice:
cmd/controller / SSHTransport / Copy / transfers one local file to a remote path without exposing partial output

Objective:
Implement the `Transport.Copy` behavior for `SSHTransport` using an established SSH connection.

Allowed Production Files:
- cmd/controller/ssh_transport.go
- cmd/controller/bash_shell_platform.go

Tests:
- cmd/controller/ssh_transport_test.go
- cmd/controller/bash_shell_platform_test.go

Out Of Scope:
- Execute behavior
- controller config factory wiring
- preflight issue reporting
- retry policy beyond cleanup of the current copy attempt
- directory synchronization
- recursive copy
- remote-to-local copy
- Slurm or Singularity execution

Acceptance:
- Copies a local file to the requested remote path through the SSH transport.
- Creates the remote parent directory when needed, or returns a clear error if this is intentionally deferred.
- Writes to a unique remote temporary path before promoting to the final remote path.
- Does not expose a partial final destination file when transfer fails before promotion.
- Removes the remote temporary path on failure when practical.
- Replaces or promotes to the final path atomically where the remote filesystem supports it.
- Adds focused tests using the in-process SSH fixture from feature 001.
- Does not require OpenSSH, `scp`, `sftp`, Docker, HPCC credentials, or local key generation as system installs.

## Artifact

This feature should produce or modify:

```text
cmd/controller/ssh_transport.go
cmd/controller/ssh_transport_test.go
cmd/controller/bash_shell_platform.go
cmd/controller/bash_shell_platform_test.go
```

If a Go module dependency is needed for SFTP support, that dependency should be added in this slice and documented in the slice report.

`BashShellPlatform` changes are allowed only for the smallest command/path helpers needed for remote parent-directory creation, temp-file promotion, or cleanup.

## Copy Invariant

The core correctness invariant is:

```text
SSHTransport.Copy must not expose a partially written final destination file.
```

Expected flow:

1. Open or establish the SSH connection from feature 003.
2. Create the remote parent directory if supported in this slice.
3. Choose a unique temp path in the same remote directory as the final path.
4. Transfer the local file bytes to the temp path.
5. Close and flush the remote temp file.
6. Promote the temp path to the final path.
7. Remove the temp path on any failure after temp creation when practical.

## Transfer Mechanism

Prefer a Go-native transfer mechanism.

Options:

- SFTP over the existing SSH connection.
- A minimal SCP implementation over an SSH session.

Prefer SFTP if it keeps the implementation smaller and safer. It avoids depending on remote shell quoting for the file transfer itself.

If the implementation uses remote shell commands for parent-directory creation, promotion, or cleanup, those command strings must be isolated and tested in `BashShellPlatform`. Do not broaden `Transport` or `ShellDialect` interfaces in this slice unless the existing boundary blocks the copy feature.

## Test Cases

Add tests covering:

- copies file content to a new remote path
- creates nested remote parent directory, if supported
- replaces an existing remote file with complete new content
- does not leave final destination changed when transfer fails before promotion
- removes remote temp file on failure when practical
- returns an error for missing local source file
- respects canceled context or configured command/copy timeout where practical

The first implementation may use the test server's local temporary directory as the remote filesystem backing store.

## Later Features Enabled

This feature enables:

- Slurm script upload over SSH
- worker config upload over SSH
- worker artifact upload over SSH
- preflight checks that verify remote write permissions

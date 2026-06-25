# 009 SSHTransport Retry And Reconnect

Status: proposed

Slice:
cmd/controller / SSHTransport / retry and reconnect / retries connection-level failures without repeating known side effects

Objective:
Add conservative retry and reconnect behavior for SSH transport operations after the basic connect, execute, copy, list, and filesystem helpers exist.

Allowed Production Files:
- cmd/controller/ssh_transport.go

Tests:
- cmd/controller/ssh_transport_test.go

Out Of Scope:
- broad job retry policy
- retrying Slurm jobs
- retrying worker execution
- automatic recovery from remote command nonzero exits
- persistent connection pooling beyond one reconnectable client
- controller config factory wiring unless already implemented

Acceptance:
- Distinguishes connection-level failures from remote command failures.
- Reconnects when the SSH connection is closed before an operation starts.
- Does not retry a remote command after the remote side has started executing unless the caller explicitly marks the operation idempotent.
- Allows copy retry only when the final destination has not been promoted.
- Cleans up temp copy paths after failed attempts when practical.
- Adds tests that simulate connection closure before execution.
- Adds tests that confirm nonzero remote command exits are not retried.

## Retry Rules

Default behavior should be conservative:

- connection failed before operation starts: retry may be allowed
- authentication failed: do not retry
- host-key verification failed: do not retry
- remote command exited nonzero: do not retry
- connection lost during copy before promotion: retry may be allowed
- connection lost after copy promotion: do not blindly retry

## Config Shape

If retry settings are added to `SSHTransportConfig`, keep them small:

```go
RetryAttempts int    `json:"retry_attempts,omitempty"`
RetryDelay    string `json:"retry_delay,omitempty"`
```

Defaults should be safe. Zero retry attempts should be valid.

## Test Cases

Add tests covering:

- reconnects after a closed idle connection
- retries connection failure before command execution
- does not retry authentication failure
- does not retry host-key failure
- does not retry remote nonzero command exit
- retries copy failure before final promotion when safe
- does not expose a partial final copy destination during retry

## Later Features Enabled

This feature makes SSH transport more reliable for HPCC login nodes without hiding correctness failures behind repeated side effects.

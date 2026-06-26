# 008 Execution Environment Preflight And Preparation

Status: implemented

Slice:
cmd/controller / ExecutionEnvironment / Preflight and Prepare / reports readiness issues and prepares components before worker startup

Objective:
Introduce a shared preflight concept so transports, schedulers, runtimes, and possibly dialects can validate external requirements while the client is still waiting for controller startup or workflow submission. Align that with the existing preparation concept so components can also establish required resources before worker startup.

Allowed Production Files:
- cmd/controller/execution_environment.go
- cmd/controller/transport.go
- cmd/controller/scheduler.go
- cmd/controller/runtime.go
- cmd/controller/preparer.go

Tests:
- cmd/controller/execution_environment_test.go
- focused component tests as needed

Out Of Scope:
- interactive prompts inside the controller
- automatic host-key trust changes
- automatic dependency installation
- real HPCC execution
- retry/reconnect policy
- broad health monitoring or worker heartbeat behavior
- Slurm-specific, Singularity-specific, or SSH-specific deep checks beyond what is needed to prove the shared contract

Acceptance:
- Defines a small preflight interface or equivalent optional component contract.
- Defines a structured `PreflightIssue` shape with component, severity, code, message, and remediation fields.
- Allows execution environment components to report zero or more issues.
- Aggregates preflight issues from configured transport, scheduler, and runtime components.
- Clarifies how preflight differs from preparation.
- Keeps or adapts the existing `Preparer` pattern so setup errors can fail before scheduler/runtime work starts.
- Adds tests showing aggregation across multiple components.
- Does not require every existing component to implement preflight immediately.

## Why This Is Cross-Cutting

SSH known-host and credential failures motivated this feature, but the concept is broader.

Examples:

- SSH transport can report unknown host key, auth failure, connect timeout, or missing writable remote root.
- Docker transport can report missing Docker CLI or missing target container.
- Slurm scheduler can report missing `sbatch` or unavailable partition.
- Singularity runtime can report missing `singularity`, missing image path, or invalid bind path.
- Worker runtime can report remote directory creation or permission failures.

The controller should surface these issues before accepting work that cannot start workers.

## Preflight Versus Preparation

Preflight and preparation are related but not identical.

Preflight:

- reports structured readiness issues
- should be safe to run while the client is waiting
- should not ask interactive questions
- should not silently mutate trust or credentials
- should avoid creating durable remote state unless explicitly documented

Preparation:

- makes a component ready to operate
- may create or hold resources, such as an SSH client connection
- may create runtime directories or copy required configuration
- returns ordinary errors when setup cannot complete

For SSH, preflight might report an unknown host key with remediation. Preparation might establish or verify the SSH client connection before the first copy or exec operation. These should align, but one should not silently replace the other.

## Proposed Shape

Possible interface:

```go
type PreflightComponent interface {
    Preflight(ctx context.Context) []PreflightIssue
}
```

Possible issue shape:

```go
type PreflightIssue struct {
    Component   string `json:"component"`
    Type        string `json:"type"`
    Severity    string `json:"severity"`
    Code        string `json:"code"`
    Message     string `json:"message"`
    Remediation string `json:"remediation,omitempty"`
}
```

Severity should start small:

```text
error
warning
```

Errors block workflow submission or worker startup. Warnings are reported but do not block.

## Controller Behavior

The controller should not ask interactive questions.

Instead, it should return actionable issues to the client, such as:

```json
{
  "component": "transport",
  "type": "ssh",
  "severity": "error",
  "code": "ssh_unknown_host_key",
  "message": "Host hpcc.example.edu is not trusted by the configured host-key policy.",
  "remediation": "Add the host key to known_hosts or configure a pinned host key."
}
```

The client may decide how to present remediation to the user.

## Test Cases

Add tests covering:

- environment with no preflight components returns no issues
- one component returns one blocking issue
- multiple components return aggregated issues
- warning issues do not block when the caller asks only for blocking errors
- issue JSON shape is stable enough for client display
- preparation still runs for components that implement `Preparer`
- preparation errors remain distinct from structured preflight issues unless explicitly converted at a controller boundary

## SSH-Specific Follow-Up

A later SSH-specific slice should implement preflight checks for:

- host key policy
- authentication
- remote runtime root writable
- `singularity` availability
- `sbatch` availability when paired with Slurm

Keep those checks out of the shared preflight-interface slice unless they are needed to prove the interface.

The SSH-specific implementation should also decide whether `SSHTransport` implements `Preparer` by establishing or verifying an SSH connection. If so, tests should cover:

- prepare succeeds against the in-process SSH server
- prepare fails with wrong client key
- prepare fails with wrong pinned host key
- prepare respects canceled context or connect timeout
- repeated prepare either reuses the existing connection or returns a clear error according to the implementation decision

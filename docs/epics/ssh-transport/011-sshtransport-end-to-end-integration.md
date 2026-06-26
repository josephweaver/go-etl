# 011 SSHTransport End-To-End Integration

Status: implemented

Slice:
cmd/controller / SSHTransport / integration / runs a controller worker-start path through SSH transport

Objective:
Verify an end-to-end controller execution path that uses SSH transport to prepare remote files and execute remote commands for a worker-start flow.

Allowed Production Files:
- none by default

Tests:
- opt-in integration test or script, to be decided by the implementation slice

Documentation:
- docs/fake-hpcc.md or a dedicated SSH transport runbook

Out Of Scope:
- broad HPCC production deployment
- credential management UI
- automatic SSH host trust mutation
- large scheduler/runtime refactors
- retry policy changes

Acceptance:
- Uses an SSH transport config to connect to a target environment.
- Copies at least one controller-generated artifact through SSH.
- Executes at least one remote command through SSH.
- Verifies the controller can reach the point where scheduler/runtime work would start, or completes a minimal worker flow if the target environment supports it.
- Reports actionable failure output for auth, host-key, missing command, and permission failures.
- Is opt-in when it needs external resources or credentials.

## Integration Options

Preferred early option:

- Go integration test using the in-process SSH fixture if it can realistically simulate remote copy and exec.

External-resource option:

- script or opt-in Go test controlled by environment variables such as:

```text
GOETL_SSH_INTEGRATION=1
GOETL_SSH_HOST
GOETL_SSH_USER
GOETL_SSH_IDENTITY_FILE
GOETL_SSH_HOST_KEY_POLICY
```

External integration tests must skip by default when required variables are absent.

## Expected Verification

At minimum, verify:

- SSH config loads
- SSH transport prepares
- remote copy succeeds
- remote exec succeeds
- cleanup succeeds when possible

If paired with Slurm and Singularity in the target environment, a later integration may verify:

- generated Slurm script upload
- `sbatch` invocation over SSH
- Singularity worker command shape
- worker pulls one `write_demo_output` item from the controller

That full HPCC-like worker execution is allowed only if prior slices have already implemented the required pieces.

## Later Features Enabled

This feature marks the transition from SSH transport construction to backend-level confidence.

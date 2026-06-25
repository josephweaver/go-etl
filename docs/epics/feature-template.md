# 003 SSHTransport Execute

Status: proposed

EC Mode:
EC-4 / file(1)+test+doc

Slice:
cmd/controller / SSHTransport / Execute / runs remote command through ssh argv

Objective:
Add Execute behavior for SSHTransport using the existing Transport interface.

Allowed Production Files:
- cmd/controller/ssh_transport.go

Tests:
- cmd/controller/ssh_transport_test.go

Out Of Scope:
- CopyInto
- Preparer
- real SSH server
- config factory wiring
- Slurm
- Singularity
- HPCC credentials

Acceptance:
- Builds expected ssh command argv.
- Propagates command failure.
- Does not change Scheduler or Runtime interfaces.

Notes:
- Start with per-command ssh invocation, not persistent sessions.
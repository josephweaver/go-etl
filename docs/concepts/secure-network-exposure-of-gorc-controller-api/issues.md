# Secure Network Exposure Issues

Status: Open issue log

Use this file for blockers, unsafe ambiguities, and implementation discoveries for
the Secure Network Exposure of the GORC Controller API concept.

## Open Issues

### 2026-07-10 - Laptop-hosted external smoke not separately run

Slice: OS-010 concept closure and documentation sync
Observed state: OS-009 verified a production-like dedicated server HTTPS
endpoint and a real HPCC Slurm/Singularity worker callback through that endpoint.
The runbook and helper scripts for a laptop-hosted temporary HTTPS ingress exist,
but a laptop-hosted managed-ingress smoke was not separately executed.
Why this is unsafe or blocking: The concept originally separated laptop-test
evidence from dedicated-server evidence. Closing the concept as if both profiles
were tested would overstate what was verified.
Evidence: `docs/TEST_AND_SMOKE_STATUS.md` records the dedicated-server HTTPS
smoke and HPCC Slurm worker smoke. It does not record a Tailscale Funnel,
Cloudflare Tunnel, or equivalent laptop-hosted ingress run.
Decision needed: Decide whether the dedicated-server plus HPCC smoke supersedes
the laptop-hosted test profile for this concept, or run the laptop-hosted profile
before final closure.
Safe temporary behavior: Treat the laptop profile as documented test-only
guidance, not verified smoke evidence. Treat the dedicated server profile as the
verified external callback path.

### 2026-07-10 - Worker process errors are not reflected in process exit status

Slice: OS-010 concept closure and documentation sync
Observed state: A local-controller `ssh_reverse` plus dev-node relay smoke
completed a two-item workflow through real HPCC Slurm/Singularity workers. During
shutdown, one late extra worker printed a controller fetch `connection refused`
error after the workflow had already completed. Slurm still marked the job
`COMPLETED` because the worker main logs startup and runtime errors and returns
without setting a non-zero process exit status.
Why this is unsafe or blocking: Scheduler-level job state can look successful
even when the worker process encountered an unrecoverable controller
communication error. That can hide operational failures during Slurm accounting
or runbook review.
Evidence: `docs/TEST_AND_SMOKE_STATUS.md` records Slurm jobs `12222949` and
`12222950` for the relay smoke. The submission completed with two completed work
items and zero failed work items, but the later worker stdout contained
`connection refused` while Slurm accounting still reported `COMPLETED|0:0`.
Decision needed: In a worker-runtime follow-up, decide whether worker startup,
configuration, controller-client, and run-loop errors should call `os.Exit(1)`
or return errors through a testable `run` function that `main` exits on.
Safe temporary behavior: Treat controller submission status and persisted
attempt state as authoritative for workflow success. Treat Slurm job state as
process-launch evidence only until worker process exit semantics are fixed.

## Resolved Issues

### 2026-07-10 - Slurm compute-node ssh_reverse callback reachability

Slice: OS-010 concept closure and documentation sync
Resolution: A local-controller `ssh_reverse` plus dev-node relay smoke completed
a two-item workflow through real HPCC Slurm/Singularity workers. The direct
non-loopback SSH reverse bind remained unavailable because the SSH server exposed
only a loopback listener, but the controller-managed relay bound a worker-visible
dev-node port and forwarded traffic to the loopback reverse listener.
Evidence: `docs/TEST_AND_SMOKE_STATUS.md` records the 2026-07-10 relay smoke,
including the generated worker config with
`controller_insecure_external_http_allowed = true`, completed controller status,
HPCC output files, and post-shutdown relay-port cleanup.

## Required Issue Shape

```markdown
### YYYY-MM-DD - Short issue title

Slice:
Observed state:
Why this is unsafe or blocking:
Evidence:
Decision needed:
Safe temporary behavior:
```

Do not resolve a security ambiguity silently inside implementation code.

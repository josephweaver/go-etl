# OS-010: Concept Closure and Documentation Sync

Status: In Progress
Minimum recommended model: GPT-5.3-Codex-Spark  
Reference: EC-2 / operational slice / docs only

## Objective

Close the concept after implementation and smoke evidence, update living project
state, and clearly reposition SSH reverse callback tunneling as an optional
compatibility path.

## Required Updates

### Concept tracker

- Mark each implemented OS with actual status.
- Record deviations from the proposed configuration shape.
- Record the final route-role table.
- Record the first successful laptop external test.
- Record the first successful production-like dedicated-server test.
- Link unresolved limitations to `issues.md`.

### Project state

Update:

- `PROJECT_STATE.md`
- `docs/STATE_INDEX.md`
- `docs/CURRENT_FOCUS.md`
- `docs/IMPLEMENTED_CAPABILITIES.md`
- `docs/ARCHITECTURE_STATE.md`
- `docs/RUNTIME_RUNBOOK.md`
- `docs/TEST_AND_SMOKE_STATUS.md`

Only state evidence that has actually been verified.

### SSH refinement

Update `docs/concepts/ssh-refinement/README.md`:

- SSH remains the execution transport.
- Reverse callback tunneling remains supported where useful.
- HTTPS is the preferred worker callback path.
- The old tunnel is not deleted until compatibility policy says so.
- Preflight uses `/healthz` or authenticated requests as appropriate.

### CLI and deployment docs

Document:

- local unauthenticated loopback profile;
- authenticated local profile;
- laptop temporary HTTPS profile;
- dedicated-server production profile;
- token-file/environment behavior;
- URL stability rule;
- credential rotation;
- migration from one controller URL to another between runs.

## Required Context

Read:

- all implemented concept slices;
- implementation diffs;
- smoke evidence;
- current state documents;
- SSH refinement documents;
- root README.

## Allowed Files

Documentation only:

- this concept directory;
- root `README.md`;
- current project state/index/runbook/smoke documents;
- SSH refinement README/state;
- deployment documents created by OS-007 and OS-008.

No production-code changes.

## Acceptance Criteria

- All tracker statuses match repository reality.
- Current state distinguishes API transport from execution transport.
- Documentation does not imply the controller itself terminates TLS.
- Laptop exposure is clearly test-only.
- Dedicated server is clearly the production target.
- Dynamic public-IP discovery is described as an operator/deployment concern, not
  controller behavior.
- Reverse SSH callbacks are described as optional compatibility behavior.
- Security limitations and bearer-token phase-1 status are explicit.
- No document includes real credentials, private domains, private IPs, cloud
  project IDs, or customer paths.
- Root CLI examples include authenticated HTTPS usage without a raw token argument.
- Smoke-status claims include dates and reproducible evidence.

## Stop Conditions

Stop and append to `issues.md` if:

- implementation evidence is incomplete;
- route policy still has unresolved callers;
- laptop external smoke did not actually run;
- production-like ingress did not keep the internal listener private;
- documentation would need to claim a security property that was not tested.

## Implementation State

OS-010 documentation sync is partially applied. Closure is not complete because
the laptop-hosted temporary HTTPS smoke profile was documented but not separately
run. The dedicated-server HTTPS profile and real HPCC Slurm/Singularity callback
smoke are verified in `docs/TEST_AND_SMOKE_STATUS.md`; the laptop-profile
evidence decision is recorded in `issues.md`.

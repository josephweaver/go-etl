# OS-003: Direct Python Target-Environment Smoke

## Status

Ready for implementation after OS-002 is accepted.

## Minimum capable model

Use **GPT-5.4-mini** with normal reasoning for the implementation and integration test.

A **GPT-5.3-spark**-class model is sufficient only for final documentation polish or adding another fixture after the direct command and provider contracts are already implemented and passing.

## Goal

Prove that a resolved `python_script` work item can execute through the
development-only direct command from a local source ZIP, with zero controller
HTTP requests, while preserving source staging, Python environment variables,
logs, output JSON, artifact promotion, evidence, and failure diagnostics.

## Current state

After OS-002, the direct command and local source provider exist, and Python can
retain subprocess logs without controller observation delivery. There is not yet
one target-environment fixture proving the entire direct Python path, source
bookkeeping defaults, manifest/artifact behavior, and zero-request invariant.

## Target state

A compact fixture and integration-style test exercise the real source staging
and Python runner. The test configures a sentinel controller URL, succeeds with
zero observed requests, verifies local output/log/artifact evidence, and proves
failure remains locally diagnosable. The runbook labels the command
development-only.

## Concept decision

This slice adds test and documentation evidence, not a second execution path.
The fixture has its own testdata directory because source, work-item input, and
fixture instructions form an independently reusable target-environment smoke.

## Required context

Read these files first:

- `docs/concepts/gorc-worker-direct-execution/README.md`
- `docs/concepts/gorc-worker-direct-execution/OS-002-source-bundle-provider-boundary.md`
- `cmd/worker/direct.go`
- `cmd/worker/direct_test.go`
- `cmd/worker/work_python.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/README.md`
- `docs/RUNTIME_RUNBOOK.md`

## User story

A developer stages the direct-execution inputs on the target environment:

```text
worker executable or container image
worker.json
python-work-item.json
source-bundle.zip
```

Then runs one command and receives:

```text
normal worker data output
attempt-local stdout/stderr/logs
promoted artifacts when declared
worker-result.json
process exit status
```

## In scope

- Add a compact direct Python fixture.
- Add an integration-style Go test around `runDirectCommand`.
- Prove no controller HTTP requests occur.
- Prove source ZIP extraction and entrypoint execution.
- Prove `GOET_*` Python runner environment variables are present.
- Prove `GOET_OUTPUT_JSON` is consumed through the normal worker path.
- Prove stdout/stderr files are retained locally.
- Prove success result/evidence and output file.
- Prove Python process failure produces failed result and nonzero direct exit.
- Prove direct config can omit controller URL.
- Prove a configured sentinel controller receives zero total requests, including
  log observations and source retrieval.
- Prove missing attempt and source bookkeeping receives the documented direct
  defaults.
- Add local/container/Singularity/HPCC runbook examples.
- Update project state and worker ownership documentation.

## Out of scope

- Slurm job submission automation.
- SSH copying or remote command orchestration.
- GPU/MPI-specific fixture behavior.
- Workflow compilation.
- Data asset provider coverage beyond what the fixture needs.
- Controller ledger comparison.
- Performance benchmarking.
- Long-running cancellation.
- A general remote-test client command.

## Allowed production files

None expected. If the smoke exposes a direct-mode defect, stop and obtain an
approved production-file boundary for the focused fix.

## Allowed test files

- `cmd/worker/direct_integration_test.go` (new)
- `cmd/worker/testdata/direct-python/` (new compact fixture tree)

## Allowed documentation files

- `cmd/worker/README.md`
- `docs/RUNTIME_RUNBOOK.md`
- `PROJECT_STATE.md`
- `docs/TEST_AND_SMOKE_STATUS.md`

Touch production files only when the smoke exposes a direct-mode bug. Any such change must stay within the SC boundaries and receive a focused regression test.

## Fixture layout

Suggested repository fixture:

```text
cmd/worker/testdata/direct-python/
├── source/
│   └── main.py
├── work-item.json
└── README.md
```

Build `source-bundle.zip` inside the test rather than committing binary ZIP content, unless the repository already has a standard fixture-bundle helper.

## Fixture work item

Use a resolved item that deliberately omits the direct bookkeeping defaults:

```json
{
  "id": "direct-python-001",
  "type": "python_script",
  "output_filename": "direct-python-result.json",
  "parameters": {
    "python_entrypoint": {
      "type": "path",
      "value": "main.py"
    },
    "python_args": {
      "type": "list",
      "value": ["fixture-value"]
    }
  }
}
```

Direct normalization supplies `direct-attempt-<short-random-id>`,
`direct-run-dummy`, and `source-manifest.json` before normal work-item
validation. Add a separate focused case preserving explicitly supplied source
and attempt identifiers if OS-002 does not already cover preservation.

## Python fixture behavior

The script should be small and deterministic. It should:

1. Read `GOET_INPUT_JSON`.
2. Confirm `GOET_WORK_ITEM_ID`, `GOET_ATTEMPT_ID`, `GOET_SOURCE_DIR`, `GOET_WORK_DIR`, `GOET_ARTIFACT_DIR`, `GOET_DATA_DIR`, `GOET_TMP_DIR`, and `GOET_LOG_DIR` are nonempty.
3. Write one line to stdout and one line to stderr.
4. Optionally create one small artifact in `GOET_ARTIFACT_DIR` if artifact declarations are already easy to express.
5. Write a valid JSON document to `GOET_OUTPUT_JSON`.
6. Include the received command argument and selected non-secret environment facts in the logical output.

Do not include timestamps in logical output unless the test normalizes them.

## Integration test design

Call the testable direct command function rather than spawning the compiled binary unless an existing command-test pattern strongly favors subprocess execution.

Test setup:

```text
TempDir/
├── config.json
├── source-bundle.zip
├── work-item.json
├── worker-result.json
├── logs/
├── tmp/
└── data/
```

The primary zero-request case sets `controller_url` to a sentinel HTTP server and
sets the temporary paths plus the test Python executable. A second focused case
omits `controller_url` to prove it is optional.

Invoke:

```text
execute
--config <config>
--work-item <item>
--source-bundle <zip>
--result <result>
```

## No-controller proof

Configure a sentinel HTTP server as the controller URL, count every request
regardless of path, execute direct Python, and assert the total request count is
zero. This catches current and future controller coupling. In particular, no
request may reach:

```text
/work/next
/work/complete
/work/fail
/workflow-runs/*/source-bundle.zip
/observations/logs
any future heartbeat endpoint
```

Direct mode should use local logs, not expected-failure controller log delivery.

## Success assertions

```text
exit code == 0
result.status == completed
result.schema == gorc/worker-direct-result/v1
result.work_item_id == direct-python-001
result.attempt_id starts with direct-attempt-
result.evidence.output_json is nonempty
DataDir/direct-python-result.json exists
TmpDir/attempts/<result.attempt_id>/source/main.py exists
TmpDir/attempts/<result.attempt_id>/work/input.json exists
TmpDir/attempts/<result.attempt_id>/work/output.json exists
TmpDir/attempts/<result.attempt_id>/logs/stdout.log exists
TmpDir/attempts/<result.attempt_id>/logs/stderr.log exists
stdout contains expected fixture line
stderr contains expected fixture line
```

If artifacts are included:

```text
promoted artifact exists under normal DataDir artifact layout
logical output contains promoted manifest
```

## Failure fixture

Use either a second script or an argument that makes the same script exit nonzero.

Assert:

```text
exit code == 1
result.status == failed
result.error mentions python process exit
stdout/stderr remain available
no completion output is falsely reported
```

Do not delete the attempt directory on failure; it is the main debugging artifact.

## Development credential boundary

Do not add a direct-mode secrets test or new leakage guarantee. Existing worker
redaction behavior remains unchanged, but the runbook must instruct developers
to use development-only data and credentials and must not describe direct mode
as safe for production credentials.

## Runbook documentation

Add a section to `docs/RUNTIME_RUNBOOK.md` titled:

```text
Direct one-shot worker execution
```

Begin the section with a warning that this mode is a local development and
worker/plugin diagnostic harness, not a production execution path.

### Local process

```bash
mkdir -p /tmp/gorc-direct/{logs,tmp,data}

./worker execute \
  --config /tmp/gorc-direct/worker.json \
  --work-item /tmp/gorc-direct/work-item.json \
  --source-bundle /tmp/gorc-direct/source-bundle.zip \
  --result /tmp/gorc-direct/worker-result.json

echo $?
cat /tmp/gorc-direct/worker-result.json
```

### Container

Use paths mounted exactly where the worker config expects them:

```bash
docker run --rm \
  -v "$PWD/direct:/direct" \
  <worker-image> \
  /opt/gorc/worker execute \
    --config /direct/worker.json \
    --work-item /direct/work-item.json \
    --source-bundle /direct/source-bundle.zip \
    --result /direct/worker-result.json
```

### Singularity/Apptainer

```bash
apptainer exec \
  --bind "$PWD/direct:/direct" \
  worker.sif \
  /opt/gorc/worker execute \
    --config /direct/worker.json \
    --work-item /direct/work-item.json \
    --source-bundle /direct/source-bundle.zip \
    --result /direct/worker-result.json
```

### HPCC interactive allocation

Document the command as something run after the user has obtained the allocation and entered the actual target node/container environment. Do not imply that direct mode requests Slurm resources.

Example:

```bash
salloc <site-specific-options>
srun --pty bash

apptainer exec --nv \
  --bind /path/to/direct:/direct \
  worker.sif \
  /opt/gorc/worker execute \
    --config /direct/worker.json \
    --work-item /direct/work-item.json \
    --source-bundle /direct/source-bundle.zip \
    --result /direct/worker-result.json
```

Mark scheduler and GPU flags as site-specific.

## Test commands

Narrow:

```bash
go test ./cmd/worker -run 'TestRunDirect.*Python|TestDirectPython'
```

Required package check:

```bash
go test ./cmd/worker
```

Recommended repository regression when affordable:

```bash
go test ./...
```

## Documentation state changes

After tests pass:

- `cmd/worker/README.md` lists direct execution as an owned worker process mode.
- The old invariant “workers pull work” becomes qualified: controller mode pulls; direct mode consumes exactly one explicit resolved item.
- Completion/failure reporting invariant becomes qualified: controller mode reports; direct mode writes a local result artifact.
- `PROJECT_STATE.md` records direct worker execution as implemented.
- `docs/TEST_AND_SMOKE_STATUS.md` records the fixture and exact command/test evidence.

## Implementation sequence

1. Add deterministic Python fixture source and work item.
2. Add ZIP-building test helper or reuse existing helper.
3. Add direct Python success integration test.
4. Add mandatory sentinel zero-total-request assertion and no-controller-URL
   case.
5. Add Python failure integration test.
6. Add a compact artifact/manifest assertion when supported by the fixture.
7. Run narrow worker tests.
8. Run full worker package tests.
9. Update worker README, runbook, project state, and smoke status.

## Acceptance criteria

OS-003 is complete when:

1. A resolved Python work item executes from local JSON plus source ZIP, with
   missing bookkeeping supplied by direct mode.
2. The normal source extraction and Python runner paths are exercised.
3. Direct mode works with no controller URL, and a configured sentinel
   controller observes zero total HTTP requests.
4. Success produces normal data output, attempt logs, evidence, and completed result JSON.
5. Python process failure produces a failed result JSON and nonzero direct exit while retaining logs.
6. Existing controller-driven worker tests remain green.
7. Local, container, Singularity/Apptainer, and allocated-HPCC usage are documented accurately.
8. `go test ./cmd/worker` passes.

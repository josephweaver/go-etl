# GORC Worker Direct Development Execution

## Status

Ready for implementation.

Approved design decisions were incorporated on 2026-07-11. Implement the three
approved Operational Slices in numeric order.

## Delivery cadence

Use grouped planning followed by implementation:

```text
CSxIx
```

The complete Operational Slice set is approved. Implement one slice at a time,
with one review and commit boundary per slice.

## Purpose

Add a development-only command that executes one resolved work item through the
real worker operation path in the current laptop, VM, container, Apptainer image,
Slurm allocation, or HPCC software environment.

The command answers:

> Can this resolved work item execute correctly in this worker environment?

It is not a production execution mode and does not test workflow compilation,
queueing, claiming, scheduling, controller persistence, or reporting.

## Goals

- Invoke the same `Worker.Run(model.WorkItem)` boundary used by controller mode.
- Accept every work-item type accepted by `Worker.Run`; direct mode must not
  maintain a separate operation allow-list.
- Support current operations including `write_demo_output`,
  `summarize_input_file`, `cache_data`, `commit_data`, and `python_script`, plus
  future operations added to `Worker.Run`.
- Supply Python and other source-staged work from a local source bundle.
- Preserve normal worker data, cache, manifest, artifact, staging, subprocess,
  and local log behavior.
- Produce one overwriteable local result document for developer inspection.
- Make all controller communication impossible in direct mode and prove that
  invariant with a zero-request test.

## Non-goals

- Production use or production security hardening.
- Starting a client, controller, queue, polling loop, or HTTP receive server.
- Compiling workflows or resolving unresolved workflow expressions.
- Automatically executing dependency work items.
- Supplying missing provider locations, operation parameters, artifact
  declarations, dependency outputs, or credentials.
- Slurm allocation, SSH copying, or remote launch automation.
- A generic assignment-source or result-sink framework.
- A second worker implementation or executable.
- GOET-to-GORC identifier renaming.

Development configurations must use development data and credentials. Existing
worker redaction and validation behavior remains in place, but this concept adds
no direct-mode leakage guarantees and must not be presented as safe for
production credentials.

## Architectural context

`cmd/worker/main.go` owns process-mode selection and the controller
pull-execute-report loop. `cmd/worker/worker.go` owns `Worker.Run`, the shared
single-item execution boundary. `internal/model.WorkItem` remains the resolved
assignment contract.

Direct mode changes only how one item and any source bundle enter the worker and
where terminal status is written. It does not introduce a second dispatcher.

## Current state

- `main()` loads controller-mode configuration and constructs a
  `WorkerControllerClient` before running any work.
- `runWorkerLoop` fetches work, calls `Worker.Run`, and reports completion or
  failure.
- `stageWorkItemSourceBundle` retrieves source bytes from the controller.
- Python subprocess log delivery constructs a controller client and posts log
  observations after local stdout/stderr files are written.
- The worker has no implemented heartbeat sender today.
- Running one worker operation therefore cannot yet be proven controller-free.

Repository review basis: `main` commit
`2d7c46ed79af0d5bd966edc7ec9e6860318b29de` (`Fix: Enabled rclone`) on
2026-07-11.

## Target state

The production invocation remains unchanged:

```bash
worker [worker-config.json]
```

Development-only direct invocation is:

```bash
worker execute \
  --config ./worker.json \
  --work-item ./work-item.json \
  [--source-bundle ./source-bundle.zip] \
  [--result ./worker-result.json]
```

Direct mode:

1. parses flags and determines the result path;
2. removes any previous result at that path;
3. loads one resolved `model.WorkItem` JSON document;
4. fills only missing bookkeeping identifiers required by the worker;
5. loads a local source bundle when the operation requires source staging;
6. calls `Worker.Run` once without a direct-mode operation allow-list;
7. retains the worker's normal local outputs and diagnostics;
8. overwrites the result path with a completed or failed JSON document; and
9. exits without constructing or using controller-backed work, source,
   observation, report, or heartbeat behavior.

Operations retain their normal prerequisites. For example, `cache_data` still
needs a resolved provider/location payload, and `commit_data` still needs its
referenced artifact in the expected worker data layout. Direct mode does not
create those behavioral inputs.

## Controller-independence invariant

Direct mode must send zero controller HTTP requests. It must not:

- fetch or poll for work;
- report work completion or failure;
- retrieve a source bundle from a controller;
- deliver log observations;
- send a heartbeat if heartbeat behavior is added later; or
- update controller queue, ledger, or attempt state.

The acceptance test configures a sentinel HTTP server as the controller URL,
executes representative direct work including Python, and asserts that the
server observed zero total requests. Successful execution with no controller URL
is also required.

## Work-item and bookkeeping contract

The input is the exact JSON shape represented by `model.WorkItem`. Direct mode
does not accept a wrapper object and does not resolve workflow expressions.

Behavior-affecting fields remain required and are validated normally. Direct
mode may fill these missing bookkeeping fields before final validation:

```text
attempt_id            direct-attempt-<short-random-id>
source.run_id         direct-run-dummy
source.manifest_path  source-manifest.json
```

The source fields are supplied only for an operation that needs source staging,
such as `python_script`. An absent `source` object may be created for that
purpose. Explicitly supplied nonempty values are preserved.

Every effective attempt ID must be a portable directory component matching
`[A-Za-z0-9._-]+` and must not be `.` or `..`. Reject an explicitly supplied
attempt ID that does not meet that rule. A generated attempt ID must also be
unique enough that repeated runs do not reuse an earlier attempt directory. Do
not use colon-containing identifiers such as `run:dummy` for filesystem path
components.

Direct mode does not invent work-item IDs, operation types, output filenames,
parameters, resolved data-provider payloads, publish targets, artifact
declarations, or dependency outputs.

## Source-bundle contract

Source-staged operations receive bytes from a local regular file supplied by
`--source-bundle`. Production continues to receive the same bytes from
`WorkerControllerClient` through a narrow provider boundary.

The existing source staging code continues to own ZIP validation, traversal and
collision rejection, extraction, and attempt-directory creation. A local source
path is not added to `model.WorkItem`.

## Result and exit contract

Default result path:

```text
worker-result.json
```

After flags parse and determine the result path, direct mode removes an existing
result at that path before loading config or the work item. This prevents an
invalid new config or work item from leaving a stale completed result that
appears current. If flag parsing itself fails, no reliable custom result path is
available; the command reports the error to stderr and does not alter a result.

After work execution starts, direct mode creates the result parent directory if
needed and writes a human-readable completed or failed JSON result directly to
the requested path with overwrite/truncation semantics. Atomic publication is
not required. The result uses an explicit direct evidence shape with stable
snake_case JSON field names; it must not depend on Go's default serialization of
`WorkEvidence` field names.

`data_output_path` is present only when the current run completed and the
operation produced that path. `attempt_dir` is present only when the current
operation created an attempt directory. Failed work must not advertise an old
output as newly produced.

The schema and status mapping are stable; timestamps and generated attempt IDs
make individual result documents intentionally non-byte-deterministic.

Exit status is conventional rather than a detailed API:

```text
0  work completed and the result was written
1  invocation, validation, work execution, or result writing failed
```

Diagnostics are also printed to stderr. Callers should inspect the result and
local worker files rather than depend on a multi-code exit taxonomy.

## Proposed Operational Slices

- `OS-001-direct-worker-command.md`: add process-mode selection, runtime-only
  config validation, resolved-item loading and bookkeeping normalization, direct
  result overwrite behavior, all-operation pass-through, and source-free tests.
- `OS-002-source-bundle-provider-boundary.md`: separate source acquisition,
  provide local source bytes, and explicitly bypass Python controller log
  observations in direct mode.
- `OS-003-direct-python-target-smoke.md`: prove Python staging, subprocess,
  artifacts, manifests, local diagnostics, and zero controller requests in a
  representative target environment.

## Completion criteria

The concept is complete when:

1. Production controller-driven invocation and tests remain valid.
2. Direct mode passes every input work-item type to `Worker.Run` without a
   separate operation allow-list.
3. Current source-free operations can be invoked directly with their normal
   operation prerequisites.
4. Python executes from a local source ZIP through the normal staging and runner
   paths.
5. Missing bookkeeping identifiers receive the documented direct defaults.
6. Repeated runs cannot leave a stale result that appears current.
7. Direct execution sends zero HTTP requests, including work, source, log,
   terminal-report, and any future heartbeat request.
8. Normal local data, cache, manifest, artifact, staging, and log behavior is
   preserved.
9. Direct success and failure produce the documented local result and simple
   exit status.
10. Worker documentation clearly labels this mode development-only.
11. `go test ./cmd/worker` passes.

## Documents

- `OS-001-direct-worker-command.md` - direct command and local result.
- `OS-002-source-bundle-provider-boundary.md` - production/local source
  providers and controller-observation bypass.
- `OS-003-direct-python-target-smoke.md` - target-environment proof.
- `MODEL_IMPLEMENTATION_PLAN.md` - implementation order and HCI recommendation.

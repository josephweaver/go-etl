# OS-002: Source-Bundle Provider Boundary

## Status

Ready for implementation after OS-001 is accepted.

## Minimum capable model

Use **GPT-5.4-mini** with normal reasoning.

This is a narrow dependency inversion around known code. The primary risk is accidentally changing ZIP validation/staging or leaving production source retrieval unwired.

## Goal

Allow the existing worker source-staging code to obtain a source ZIP either from
the controller in production mode or from a local file in direct development
mode, without changing `model.WorkItem`, and prevent Python subprocess log
delivery from contacting a controller in direct mode.

## Current state

`Worker.stageWorkItemSourceBundle` calls `worker.controllerClient()` and then
`WorkerControllerClient.SourceBundle`. Python subprocess log processing also
calls `worker.controllerClient()` before posting `/observations/logs`. Local ZIP
validation, extraction, attempt directories, stdout, and stderr already belong
to the worker and should remain unchanged.

## Target state

Production explicitly supplies a controller-backed source provider. Direct mode
supplies a local-file provider and sets the worker's local-only execution
capability. Source staging uses the configured provider, and Python retains local
logs without constructing a controller log client when local-only is true.

## Concept decision

This slice adds a narrow `SourceBundleProvider` concept with its own production
file and test surface. It also adds one explicit local-only worker capability to
bypass the existing Python controller-observation side effect. It does not add a
generic transport framework or second source-staging implementation.

## Required context

Read these files first:

- `docs/concepts/gorc-worker-direct-execution/README.md`
- `docs/concepts/gorc-worker-direct-execution/OS-001-direct-worker-command.md`
- `cmd/worker/worker.go`
- `cmd/worker/main.go`
- `cmd/worker/source_bundle.go`
- `cmd/worker/source_bundle_test.go`
- `cmd/worker/work_python.go`
- `cmd/worker/work_python_test.go`

## User story

A developer can provide the same resolved Python work item JSON that production uses plus a local source ZIP:

```bash
worker execute \
  --config ./worker.json \
  --work-item ./python-item.json \
  --source-bundle ./source-bundle.zip \
  --result ./worker-result.json
```

The worker stages and validates that ZIP through the same production extraction code.

## In scope

- Add one source-bundle provider interface.
- Add production controller-backed provider.
- Add direct local-file provider.
- Add `SourceBundles` dependency to `Worker` or an equally narrow execution dependency container.
- Refactor `stageWorkItemSourceBundle` to use the provider.
- Wire production main/worker loop to the controller provider.
- Add `--source-bundle` direct option.
- Preflight source-required work in direct mode.
- Supply missing direct source bookkeeping fields.
- Bypass Python controller log-observation delivery in local-only direct mode
  while retaining local stdout/stderr files.
- Add provider and staging tests.
- Preserve all existing ZIP safety validation.

## Out of scope

- A generic assignment source.
- A generic result sink.
- Multiple source bundles.
- Source directory mode.
- Automatic ZIP creation.
- Workflow compiler integration.
- HTTP upload/download in direct mode.
- Changes to `WorkItemSource`.
- Remote SSH transport.

## Allowed production files

- `cmd/worker/worker.go`
- `cmd/worker/main.go`
- `cmd/worker/direct.go`
- `cmd/worker/source_bundle.go`
- `cmd/worker/source_bundle_provider.go` (new)
- `cmd/worker/work_python.go`

## Allowed test files

- `cmd/worker/direct_test.go`
- `cmd/worker/source_bundle_test.go`
- `cmd/worker/source_bundle_provider_test.go` (new)
- `cmd/worker/work_python_test.go`

## Allowed documentation files

- `cmd/worker/README.md`
- `PROJECT_STATE.md`

Do not modify `internal/model/work_item.go`.

## Provider contract

Add a small interface:

```go
type SourceBundleProvider interface {
    SourceBundle(item model.WorkItem) ([]byte, error)
}
```

The provider receives the complete item so future acquisition decisions can use source metadata without changing the interface. It must not execute, extract, or interpret the bundle.

## Production provider

```go
type ControllerSourceBundleProvider struct {
    Controller WorkerControllerClient
}

func (p ControllerSourceBundleProvider) SourceBundle(item model.WorkItem) ([]byte, error) {
    if item.Source == nil { ... }
    return p.Controller.SourceBundle(item.Source.RunID)
}
```

Rules:

- Reuse current `WorkerControllerClient.SourceBundle`.
- Keep controller URL/token/TLS behavior in `WorkerControllerClient`.
- Return clear errors for missing source/run ID.
- Do not duplicate HTTP logic.

Production wiring after controller creation:

```go
worker := Worker{
    Config:        cfg,
    Controller:    controller,
    SourceBundles: ControllerSourceBundleProvider{Controller: controller},
}
```

## Direct provider

```go
type FileSourceBundleProvider struct {
    Path string
}
```

Rules:

- Require a nonempty path.
- Stat the path and require a regular file.
- Read the ZIP bytes.
- Wrap errors with the path.
- Do not extract the ZIP in the provider.
- Do not interpret `RunID` as a filesystem path.

The existing controller implementation already buffers the whole ZIP body, so `os.ReadFile` preserves current memory semantics for this slice.

## Worker dependency

Preferred explicit field:

```go
type Worker struct {
    Config        Config
    Controller    WorkerControllerClient
    SourceBundles SourceBundleProvider
    LocalOnly     bool
}
```

Avoid a global provider.

Add a helper only if it improves errors:

```go
func (w Worker) sourceBundleProvider() (SourceBundleProvider, error)
```

Do not silently construct a controller provider in direct mode. Production wiring should be explicit.

`LocalOnly` is true only for the development direct command. The zero value is
false so existing production construction retains controller-mode behavior.
Controller-mode wiring should remain explicit where this slice already touches
construction.

Existing source-free worker tests can leave the field unset because they never stage source.

## Source staging refactor

In `stageWorkItemSourceBundle` replace:

```text
worker.controllerClient
controller.SourceBundle(runID)
```

with:

```text
provider := worker.SourceBundles
provider.SourceBundle(item)
```

Everything after bytes are returned remains owned by `source_bundle.go` and should remain behaviorally unchanged:

- ZIP reader construction;
- attempt/source/work/artifact/log directory layout;
- duplicate path rejection;
- parent traversal rejection;
- absolute and drive-qualified path rejection;
- symlink-like entry rejection;
- file/directory collision checks;
- extraction modes.

This OS must be a retrieval refactor, not a ZIP subsystem rewrite.

## Direct option

Extend direct options:

```text
--source-bundle PATH
```

Behavior:

- Optional for source-free work-item types.
- Required when execution will call `stageWorkItemSourceBundle`.
- For the current code, `python_script` is the known required type.
- Accept and ignore an extra source bundle when the selected operation does not
  request source staging. This keeps automation independent of a second
  direct-mode operation allow-list.

Do not modify `item.Source` to contain the local path. It remains role-neutral
source metadata and provides run identity for artifact paths and evidence.

## Preflight

Before final work-item validation, direct mode fills missing source bookkeeping
for an operation known to require source staging:

```text
source.run_id         direct-run-dummy
source.manifest_path  source-manifest.json
```

If `item.Source` is absent, direct mode may create it for these bookkeeping
values. Preserve explicitly supplied nonempty values. Do not synthesize
entrypoints, parameters, artifact declarations, provider locations, or other
behavioral input.

Before `Worker.Run`, direct mode detects:

```text
item.Type == python_script && source bundle flag absent
```

and returns a clear invalid-input error. This is source-provider preflight, not
an allow-list of accepted work-item types. Every item type still passes through
to `Worker.Run`; future source-staged operations use the same provider boundary.

## Controller independence

Direct wiring:

```go
worker := Worker{
    Config:        cfg,
    SourceBundles: FileSourceBundleProvider{Path: options.SourceBundlePath},
    LocalOnly:     true,
}
```

Do not initialize `Worker.Controller`.

Python log observation code currently calls `worker.controllerClient()` after
the subprocess has written local logs. When `Worker.LocalOnly` is true, bypass
controller observation construction and delivery and return after retaining the
local stdout/stderr files. Do not attempt delivery and then swallow the expected
error.

Do not introduce a broad logging framework in this OS.

## Tests

Provider tests:

```text
TestFileSourceBundleProviderReadsRegularFile
TestFileSourceBundleProviderRejectsMissingPath
TestFileSourceBundleProviderRejectsDirectory
TestControllerSourceBundleProviderUsesRunID
TestControllerSourceBundleProviderRequiresSource
```

Staging tests:

```text
TestStageWorkItemSourceBundleUsesConfiguredProvider
TestStageWorkItemSourceBundlePreservesSafeExtraction
TestStageWorkItemSourceBundleStillRejectsTraversal
TestStageWorkItemSourceBundleRequiresProvider
```

Direct tests:

```text
TestDirectPythonRequiresSourceBundle
TestDirectOptionsAcceptSourceBundle
TestDirectSourceFreeWorkDoesNotReadSourceBundle
TestNormalizeDirectPythonSuppliesMissingSourceBookkeeping
TestNormalizeDirectPythonPreservesSourceBookkeeping
TestDirectPythonRetainsLogsWithoutControllerObservationDelivery
```

Production regression:

```text
TestRunWorkerLoopUsesControllerSourceBundleProvider
```

If existing Python tests rely on implicit controller construction, convert their fixtures to explicit fake providers rather than restoring hidden coupling.

## Expected state transitions

### Production Python work

```text
controller assignment
    -> controller source provider
    -> source ZIP bytes
    -> unchanged staging/execution
```

### Direct Python work

```text
work-item JSON + --source-bundle path
    -> file source provider
    -> source ZIP bytes
    -> unchanged staging/execution
```

## Failure behavior

- Missing local ZIP: invalid direct invocation; no execution.
- Unreadable ZIP: direct invalid-input or execution error with clear path.
- Malformed/unsafe ZIP: existing staging error; failed result JSON.
- Missing provider in a source-required worker: explicit error, not panic.
- Controller provider HTTP error: preserve existing wrapped production failure.

## Implementation sequence

1. Add provider interface and fake provider test.
2. Add controller provider and tests.
3. Add file provider and tests.
4. Refactor `stageWorkItemSourceBundle` without changing extraction logic.
5. Explicitly wire production worker.
6. Add direct `--source-bundle` option and provider wiring.
7. Add source bookkeeping defaults and source-required preflight.
8. Add the explicit local-only Python log-observation bypass.
9. Run `go test ./cmd/worker`.
10. Update README/project state.

## Acceptance criteria

OS-002 is complete when:

1. Production Python source retrieval still uses the controller client.
2. Direct mode can supply source bytes from a local regular ZIP file.
3. Existing ZIP safety tests still pass unchanged or with only fixture wiring changes.
4. `model.WorkItem` and `WorkItemSource` are unchanged.
5. Source-free direct work remains controllerless and provider-independent.
6. A missing source provider produces a clear error.
7. Missing direct source bookkeeping receives `direct-run-dummy` and
   `source-manifest.json` without changing behavioral inputs.
8. Direct Python retains local subprocess logs without constructing or using a
   controller log client.
9. An extra source bundle is accepted and ignored by work that does not stage
   source.
10. `go test ./cmd/worker` passes.

# 004 Controller Artifact Output Recording

Status: proposed

## Objective

Make the controller accept, persist, and surface compact artifact manifests returned as work-item logical outputs.

This slice should not make the controller read artifact bytes. It records the manifest evidence already produced by the worker and makes status/log surfaces useful enough to find produced artifacts.

## Current State

Workers report completion through `POST /work/complete`. Completion payloads can include `output_json` and output hashes. Workflow execution persistence stores compact output JSON for completed work attempts.

Dependency-aware workflows need completed step outputs to become downstream resolver inputs. Artifact manifests should use that same typed logical-output path.

## Target State

When a work item completes with `output_json` that is an artifact manifest, the controller treats it as valid compact logical output and persists it through the existing completion path.

The controller does not open worker artifact paths. It validates only the manifest shape and path safety that can be checked structurally.

Status surfaces should expose at least compact evidence such as:

```text
artifact_count
artifact_names
storage_scope
```

Full status JSON may include artifact descriptor summaries, but it should not dump huge nested directory manifests by default.

## Concept Decision

Do not create an artifact-byte service in this slice. The controller records compact manifest facts and lets downstream workflow resolution consume them.

If the existing persistence store already stores `completed_work.output_json`, prefer reusing that field rather than adding artifact-specific tables in this slice. Add artifact-specific tables only if the existing field cannot support status and downstream resolution requirements.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/workflow-execution-persistence/README.md`
- `internal/model/artifact_manifest.go`
- `internal/model/work_item.go`
- `cmd/controller/main.go`
- `cmd/controller/README.md`
- `internal/persistence` files related to completed work and output JSON

Avoid scheduler, transport, container, and source-control files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/status`-related files if status logic has already been split out
- `internal/persistence` files that already own completed-work output storage
- `internal/model/artifact_manifest.go` only for narrow validation adjustments

## Allowed Test Files

- `cmd/controller/main_test.go`
- controller status tests if split out
- `internal/persistence` completed-work/output tests
- `internal/model/artifact_manifest_test.go` only for narrow validation adjustments

## Out Of Scope

- Downloading or serving artifact bytes.
- Artifact retention cleanup.
- Object storage.
- Worker promotion changes.
- Python runner changes.
- Data asset declarations or materialization.
- HPCC smoke automation.

## Acceptance Criteria

- A completion payload containing a valid artifact manifest in `output_json` is accepted.
- A completion payload containing an invalid artifact manifest shape fails only if the controller can safely distinguish it from ordinary non-artifact logical output, or the design clearly treats manifests as opt-in by schema.
- The completed output JSON is persisted unchanged or in an equivalent canonical compact form.
- Submission/status JSON exposes artifact count or equivalent summary for completed work items when artifact manifests are present.
- The controller does not attempt to open worker artifact paths.
- Downstream output capture in dependency-aware workflows can treat the artifact manifest as the step output.
- Existing non-artifact completion tests still pass.
- Relevant controller and persistence tests pass.

## Notes

- Keep artifact manifests compact. Directory file-entry manifests may be referenced by hash rather than embedded in status output.
- If output JSON is arbitrary user output, use the top-level `schema` value to identify artifact manifests.

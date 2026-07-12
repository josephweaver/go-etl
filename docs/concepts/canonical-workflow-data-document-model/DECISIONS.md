# Accepted Decisions

Status: Proposed with decisions agreed in design discussion

## D-001 One SC, many slices

Canonical loading, workflow cleanup, YAML, data-tree ownership, explicit data operators, and expression authoring belong to one strategic boundary but must be implemented through small ordered slices.

## D-002 Workflow overrides project

Project provides reusable defaults and general provider bindings. Workflow may override any named data-tree element for a run-specific or scientifically narrower requirement. Submission overrides are higher still.

## D-003 Recursive overlay is document composition

The canonical document layer deep-overlays named data trees. This is not assumed to exist in the current root-variable resolver.

## D-004 Lists replace

A higher-precedence list replaces the lower-precedence list. This enables `select: [header]` to narrow a project default of `[raster, header]`.

## D-005 Selection is explicit

Workflows narrow available files through `select`; they do not delete provider file definitions.

## D-006 Asset definitions and instances are distinct

Asset definitions may be parameterized. Asset instances exist only after parameter values and selection resolve.

## D-007 Yan-Roy year is fixed

The Yan-Roy field-boundary asset is parameterized by tile only. The file templates contain fixed year `2010`.

## D-008 Step data alias is explicit

A step maps an asset to a local alias, such as `field_segments`. Computation accesses it through the `data` namespace.

## D-009 Bare data aliases are invalid

`${field_segments}` is not a data binding reference. Use `${data.field_segments...}`.

## D-010 `path` is an ordered list

`data.<alias>.path` is a list, even for a one-file selection. Named file roles remain available under `files`.

## D-011 No hidden work items

`asset.materialize` and `commit_data` are authored workflow steps. Legacy
automatic planning has been removed.

## D-012 Shared and worker scopes are stable vocabulary

Both values parse and pass definition validation. Only `shared` passes supported-capability validation in phase one. `worker` returns a sentinel-compatible not-implemented error.

## D-013 Shared materialization uses explicit work-item constraints

Provider/cache resource constraints belong to the explicit `asset.materialize`
work item and end when that work completes. Compute does not hold the
acquisition constraint.

## D-014 Worker scope is deferred

Worker-local preparation, cache locks, worker affinity, and nonshared-home-cluster behavior remain future implementation.

## D-015 Worker may finalize internal resolution

Assignment-time worker resolution may finalize paths and runtime values. Workers do not reload public project/workflow documents.

## D-016 JSON remains the semantic normalization basis

YAML normalizes to a JSON-compatible tree. Semantic fingerprints use normalized content, not YAML formatting.

## D-017 `$expr` is rejected as an internal-variable concern

Structured public directives normalize to semantic calls before `internal/variable` evaluation.

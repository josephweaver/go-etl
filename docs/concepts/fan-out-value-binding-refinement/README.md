# Fan-Out Value Binding Refinement

Status: Proposed  
Cadence: Strategic Concept with ordered Operational Slices  
Target repository: `josephweaver/go-etl`  
Reviewed against repository `main`: 2026-07-11  
Recommended design model: GPT-5.6-Sol, High reasoning

## Purpose

Refine the canonical `fan_out.over` / `fan_out.as` workflow contract so fan-out behaves as an ordered, type-preserving map over a resolved list.

For every element in the list resolved by `over`, the compiler must create one fan-out item and bind that element unchanged as the current item. The element may be any supported resolved value:

- string;
- path;
- integer;
- boolean;
- list, including a list produced by `list.crossproduct`;
- object, including nested lists and objects.

Fan-out must not require the list elements to have a particular shape. It must not flatten, wrap, stringify, or otherwise transform elements merely because they are used for fan-out.

## Motivating Case

The workflow has two lists:

```yaml
variables:
  years:
    - 2008
    - 2009
    - 2010
  tiles:
    - h18v07
    - h18v08

  year_tile_pairs:
    $type: list
    $call: list.crossproduct
    $args:
      - $ref: years
      - $ref: tiles
```

`list.crossproduct` produces a list whose elements are two-item lists:

```text
[
  [2008, h18v07],
  [2008, h18v08],
  [2009, h18v07],
  [2009, h18v08],
  ...
]
```

The desired fan-out is:

```yaml
fan_out:
  over: ${workflow.year_tile_pairs[*]}
  as: pair
  id: ${pair[0]}-${pair[1]}
```

Each pair is one item. It is not flattened into separate years and tiles, and it is not rejected because it is a list.

## Current Repository State

The repository already has most of the required primitives:

- canonical workflows expose `fan_out.over`, `fan_out.as`, and `fan_out.id`;
- the variable accessor layer supports chained field and list-index accessors;
- list functions can produce fan-out lists, including `list.crossproduct`, `list.zip`, and `list.flatten`;
- parameter-accessor binding can copy scalar, list, or object values into work-item parameters without losing their structure;
- empty fan-out steps already participate in dependency auto-advance;
- downstream step output ordering follows fan-out generation order.

The current mismatch is at the canonical adapter and fan-out token boundary:

1. `fan_out.as` is parsed but is not yet an authoritative current-item binding.
2. Canonical `fan_out.id` is adapted as a single accessor rather than a rendered per-item template.
3. The adapter accepts `${fanout}` and `${fanout.<field>}` but not list indexing such as `${fanout[0]}`.
4. Low-level ID/output token generation accepts only scalar string/path/integer values.
5. Canonical work parameters do not yet share one consistent per-item resolution contract with IDs and data-asset bindings.

## Strategic Decision

### Fan-out is an ordered map

The semantic operation is:

```text
resolved list L
  -> for each element L[i], in source order
  -> bind L[i] unchanged as the current fan-out item
  -> compile exactly one work item
```

This is not a flatten operation and not a Cartesian-product operation. Those transformations must happen explicitly before fan-out through functions such as `list.flatten` or `list.crossproduct`.

### `over` must resolve to a list

`fan_out.over` must resolve to one typed list.

- A non-list result is an error.
- An empty list creates zero work items and completes through the existing empty-fan-out behavior.
- Item order is preserved exactly.
- Items may have heterogeneous supported types.
- Canonical null remains unsupported.

### `as` is a real binding

`fan_out.as` is required and must be a valid non-reserved identifier.

For:

```yaml
fan_out:
  over: ${workflow.year_tile_pairs[*]}
  as: pair
```

the current element is available through both forms:

```text
${pair}         preferred author-facing alias
${fanout}       generic current-item alias
```

Accessors may be chained from either form:

```text
${pair[0]}
${pair[1]}
${fanout[0]}
${fanout[1]}
${pair.field}
${pair.rows[0].crop_id}
${fanout.field[2]}
```

The two names refer to the same immutable resolved value. `as` does not copy or reshape the value.

The alias is bound only while compiling one fan-out item. It has fan-out precedence and cannot leak into another item, step, workflow, or submission.

### Item types are preserved

Examples:

```yaml
# over resolves [2008, 2009]
as: year
# ${year} is an int
```

```yaml
# over resolves ["h18v07", "h18v08"]
as: tile
# ${tile} is a string
```

```yaml
# over resolves [[2008, "h18v07"], [2008, "h18v08"]]
as: pair
# ${pair} is a list; ${pair[0]} is an int; ${pair[1]} is a string
```

```yaml
# over resolves [{year: 2008, tile: h18v07}]
as: job
# ${job} is an object; ${job.year} is an int
```

No implicit object requirement is permitted.

### Whole-value resolution and string rendering are different operations

A whole-value reference preserves its resolved type:

```yaml
work:
  parameters:
    pair: ${pair}
    year: ${pair[0]}
    tile: ${pair[1]}
```

The compiled values remain list, integer, and string respectively.

A mixed string template renders scalar placeholders into a string:

```yaml
fan_out:
  id: year-${pair[0]}-tile-${pair[1]}
```

String-template placeholders may resolve only to scalar string, path, integer, or boolean values. A list or object placeholder must be accessed to a scalar component; fan-out must not silently JSON-encode composite values.

### IDs and output tokens are rendered per item

`fan_out.id` is a required string template evaluated once for each current item.

An optional `fan_out.output` uses the same template semantics and controls the per-item output token. When omitted, it defaults to the rendered `id` token.

```yaml
fan_out:
  over: ${workflow.year_tile_pairs[*]}
  as: pair
  id: ${pair[0]}-${pair[1]}
  output: tile-${pair[1]}-year-${pair[0]}
```

The final work-item ID remains step-scoped:

```text
<step-id>-<rendered-id-token>
```

The final output filename remains:

```text
<work.output_prefix>-<rendered-output-token><work.output_extension>
```

### No silent sanitization

Rendered ID and output tokens must be deterministic and safe. The compiler must reject, not rewrite:

- empty tokens;
- leading or trailing whitespace;
- path separators;
- control characters;
- `.` or `..` path-like tokens;
- values that would make an invalid output filename;
- duplicate final work-item IDs within the step;
- duplicate final output filenames within the step when outputs would collide.

### Sensitive values must not leak into identity

Sensitive fan-out values may be bound to protected execution parameters according to existing sensitive-value rules. They must not be rendered into:

- work-item IDs;
- output filenames;
- cache keys authored from fan-out templates unless the owning data contract explicitly permits protected identity, which phase one does not;
- logs or diagnostics containing plaintext values.

Attempting to render a sensitive value into an identity-bearing string must fail with a redacted diagnostic.

## Canonical Target Example

```yaml
api_version: goet/v1alpha1
kind: Workflow
id: cdl-by-yanroy

variables:
  years:
    - 2008
    - 2009
    - 2010
  tiles:
    - h18v07
    - h18v08
  year_tile_pairs:
    $type: list
    $call: list.crossproduct
    $args:
      - $ref: years
      - $ref: tiles

steps:
  - id: count-field-crops
    fan_out:
      over: ${workflow.year_tile_pairs[*]}
      as: pair
      id: ${pair[0]}-${pair[1]}
      output: ${pair[1]}-${pair[0]}

    work:
      type: python_script
      output_prefix: field-crop-counts
      output_extension: .json
      parameters:
        python_entrypoint: scripts/count_field_crops.py
        target_environment_id: hpcc
        year: ${pair[0]}
        tile: ${pair[1]}
        original_pair: ${pair}
        python_args:
          - --year
          - ${pair[0]}
          - --tile
          - ${pair[1]}
```

For 16 years and 88 tiles, the crossproduct list contains 1,408 pair elements and the step compiles 1,408 deterministic work items.

## Resolution Lifecycle

```text
1. Decode canonical workflow.
2. Resolve fan_out.over through the normal workflow resolver.
3. Require a list and preserve its item order.
4. For item i, create an immutable fan-out item context.
5. Bind the item as both `${fanout}` and `${<as>}`.
6. Resolve typed work/data/resource values against that context.
7. Render fan_out.id and fan_out.output string templates.
8. Validate identity and output safety.
9. Compile one work item.
10. Detect collisions across all generated items before returning the step result.
```

## Ownership Boundary

### Canonical document layer owns

- public `over`, `as`, `id`, and optional `output` shape;
- identifier validation for `as`;
- retaining raw template/value expressions for per-item compilation;
- JSON/YAML semantic equivalence.

### Variable layer owns

- typed values;
- field and list-index accessor semantics;
- reference parsing where generally reusable;
- sensitivity and provenance metadata.

### Workflow compiler owns

- resolving `over` to an ordered list;
- constructing one current-item context per element;
- alias and generic-current-item binding;
- whole-value versus string-template resolution;
- per-item work/data/resource compilation;
- ID/output rendering and collision checks.

### Worker owns

- executing already-compiled work;
- receiving concrete typed parameters and materialized data projections;
- no fan-out expression evaluation.

## Goals

- Make `fan_out.as` semantically meaningful.
- Permit fan-out over lists containing any supported resolved item type.
- Support list indexing such as `${fanout[0]}` and `${pair[0]}`.
- Support chained list/object accessors.
- Support composite ID and output templates.
- Preserve types for whole-value work and data bindings.
- Use one current-item resolution model across compute, cache, commit, and resource declarations.
- Preserve deterministic ordering and empty-fan-out behavior.
- Reject collisions and sensitive identity leaks before queue mutation.
- Maintain JSON/YAML semantic equivalence.

## Non-Goals

- Implicit flattening of nested lists.
- Implicit Cartesian products.
- Filtering, grouping, reducing, or conditional fan-out.
- Dynamic fan-out after a work item starts.
- Worker-side workflow expression evaluation.
- Arbitrary code execution in templates.
- JSON-stringifying lists or objects for IDs.
- Automatically sanitizing unsafe IDs or filenames.
- Changing stage dependency semantics.
- Adding floating-point or null canonical values.

## Operational Slice Order

| Slice | Objective | Recommended model |
|---|---|---|
| `001-canonical-over-as-contract.md` | Make the public `over`/`as`/`id`/`output` document contract authoritative. | GPT-5.4, Medium |
| `002-typed-current-item-binding.md` | Bind every list element unchanged as generic and named current-item values. | GPT-5.6-Sol, High |
| `003-fanout-template-resolution.md` | Resolve chained accessors and render composite scalar templates for ID/output. | GPT-5.6-Sol, High |
| `004-per-item-work-parameter-resolution.md` | Resolve work parameters per item while preserving whole-value types. | GPT-5.4, High |
| `005-data-operator-and-resource-binding.md` | Reuse the same item context for cache, commit, and resource declarations. | GPT-5.4, High |
| `006-identity-collision-and-sensitivity-safety.md` | Reject unsafe, duplicate, or sensitive identity-bearing results. | GPT-5.4, Medium |
| `007-migration-and-crossproduct-smoke.md` | Migrate fixtures and prove scalar/list/object fan-out plus the year-tile crossproduct. | GPT-5.3-codex-spark, Medium |

## Completion Criteria

- `fan_out.over` accepts any resolved list regardless of element shape.
- Each element produces exactly one current-item context and one compiled item unless the list is empty.
- Strings, paths, integers, booleans, lists, and objects retain their types.
- `${fanout[0]}` and `${pair[0]}` work for list-valued items.
- `${fanout.field}` and `${pair.field}` work for object-valued items.
- Chained accessors work across nested lists and objects.
- Composite IDs such as `${pair[0]}-${pair[1]}` render deterministically.
- Whole-value parameter references preserve list/object/scalar structure.
- Data-asset and output-target parameter bindings use the same semantics.
- Empty fan-out remains successful and produces ordered output `[]`.
- Unsafe, duplicate, or sensitive rendered identities fail before queue mutation.
- JSON and YAML forms compile equivalently.
- A smoke workflow proves `list.crossproduct(years, tiles)` can drive all year-tile work items without pre-authoring pair objects.

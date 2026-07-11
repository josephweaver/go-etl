# 006 Identity, Collision, and Sensitivity Safety

Status: Proposed  
Recommended model: GPT-5.4, Medium reasoning  
Reference: EC-3 / operational slice / files(3)+test

## Objective

Make rendered fan-out identity and output naming safe, deterministic, collision-free, and incapable of exposing sensitive values.

## Current State

The compiler already rejects duplicate generated work-item IDs at workflow/stage boundaries and validates output filenames. Composite per-item templates introduce new failure modes that should be rejected as close to fan-out compilation as possible with item-index diagnostics.

## Target State

Before returning a compiled fan-out step, validate all rendered values as one set.

### ID token rules

A rendered ID token must:

- be non-empty;
- have no leading or trailing whitespace;
- contain no `/` or `\\`;
- contain no NUL or control characters;
- not equal `.` or `..`;
- produce a valid final work-item ID under the existing step prefix;
- be unique within the fan-out step.

### Output token rules

A rendered output token must:

- satisfy the ID-token path-safety rules;
- produce a filename-only final output under prefix and extension;
- be unique within the fan-out step unless the runtime explicitly proves isolated output namespaces, which phase one must not assume.

### Sensitive-value rules

A sensitive value must not participate in:

- `fan_out.id`;
- `fan_out.output`;
- resource keys;
- authored cache-key or publish-path identity rendering.

The failure diagnostic must show the expression path and redaction label, not plaintext.

### Failure atomicity

All fan-out items must be compiled and validated before queue mutation. A collision or unsafe token in item 1,407 must prevent every item in that step from being queued.

## Required Context

Read first:

- `internal/workflow/fanout.go`
- `internal/workflow/compile_stage.go`
- `internal/model/work_item.go`
- current artifact/output path validators
- sensitive-variable propagation and redaction owners
- OS 003 through OS 005 implementations

## Allowed Production Files

- `internal/workflow/fanout.go`
- one focused fan-out token validation owner
- a shared existing filename/identifier validator only when reuse is semantically correct

## Allowed Test Files

- `internal/workflow/fanout_test.go`
- `internal/workflow/compile_stage_test.go`
- one focused token-safety test file

## Required Tests

Cover:

- duplicate rendered IDs;
- different ID templates producing the same final ID;
- duplicate rendered output filenames;
- empty token;
- leading/trailing whitespace;
- slash and backslash;
- `.` and `..`;
- control character;
- sensitive direct scalar;
- sensitive nested field/index;
- safe non-ASCII token if existing identifier policy permits it;
- late-item failure proving no partial compile result is returned.

## Data-State Transition

```text
all rendered per-item identities and outputs
  -> validated step-local set
  -> complete compiled step OR one atomic error
```

## Acceptance Criteria

- Unsafe tokens fail with item index and template field.
- Duplicate IDs fail before stage queue mutation.
- Duplicate output filenames fail before stage queue mutation.
- Sensitive identity use fails without plaintext leakage.
- Ordinary safe scalar IDs continue to work.
- Errors are deterministic regardless of map iteration order.
- Existing outer duplicate-ID guards remain as defense in depth.

## Out of Scope

- Automatic token sanitization.
- Hashing sensitive values into public IDs.
- Cross-step global output collision detection beyond existing storage ownership.
- Runtime cleanup after partial queueing, because partial queueing must not occur.

# Structured Variable Resolution Epic

Status: Complete

## Purpose

Upgrade GOET's variable model and resolver so object and list expressions are
hierarchical collections of explicitly typed expression nodes rather than raw
JSON whose nested types are inferred after parsing.

The resolver should recursively resolve references and supported string
expressions at any structured layer while preserving the declared type of every
node.

## Goals

- Require every scalar, object, and list layer in a structured variable to have
  an explicit declared type.
- Represent object fields as named, typed expressions rather than untyped raw
  JSON fields.
- Treat `list` as a generic collection type whose individual items each
  declare their own type and expression.
- Resolve variable references recursively inside object fields and list
  elements.
- Support interpolation where a typed string or path expression combines
  references with literal text.
- Preserve namespace precedence when structured variables reference other
  variables.
- Apply the existing maximum-resolution-depth protection throughout recursive
  structured resolution.
- Return errors that identify the variable and nested field or element path
  that failed.
- Produce the existing typed `ResolvedValue` tree after successful resolution.

## Required Type Model

Type information must be declared at each layer before resolution. Nested JSON
value inference is not the target model.

Every expression node has the same recursive shape:

```text
type: <declared type>
expression: <scalar, object-field map, or list of expression nodes>
```

An object expression is a map keyed by field name. Each map value is another
typed expression node. Object field order has no semantic meaning.

The following illustrates the agreed shape:

```text
name: python-env-torch
type: object
expression:
  kind:
    type: string
    expression: resource_constraint
  key:
    type: string
    expression: ${project_config.name}/python-env/torch
  capacity:
    type: int
    expression: 1
```

Here the outer variable is explicitly an object. Each object field is also a
typed expression. A nested object repeats the same pattern.

A list declares only that it is a collection. It does not declare one element
type for the entire collection. Each item is independently typed:

```text
name: example-values
type: list
expression:
  - type: string
    expression: alpha
  - type: int
    expression: 2
  - type: object
    expression:
      enabled:
        type: bool
        expression: true
  - type: list
    expression:
      - type: path
        expression: /data/input.txt
```

Lists may therefore contain heterogeneous values and nested lists. A consumer
that requires a homogeneous list, such as a string-list accessor, validates
that narrower requirement when it consumes the resolved list.

The final serialized representation must remain language-neutral and usable by
the CLI, REST API, and future Python and R adapters.

## Expression Grammar

A whole-value reference uses the existing `${reference}` form and preserves
the referenced value's type. The referenced type must match the expression
node's declared type.

String and path expressions may contain one or more reference tokens mixed
with literal text:

```text
${project_config.name}/python-env/torch
run-${year}-${region}
${project.data_root}/inputs/${year}
```

Qualified and unqualified references, namespace precedence, and supported
field or index accessors retain their existing meanings. Interpolation accepts
the supported scalar `string`, `path`, `int`, `bool`, and `datetime` values and
uses their canonical serialized text. Object and list values cannot be
interpolated into text.

The sequence `\${` escapes a literal `${`. In JSON source, the backslash is
itself escaped, for example `"\\${year}"`.

Path interpolation produces a resolved value whose declared type remains
`path`. Variable resolution does not normalize path separators or translate a
path for an operating system, container, mount, transport, or execution
environment. Those transformations require target context and belong to the
runtime, shell-dialect, or transport boundary.

## Non-Goals

- Adding resource-constraint scheduling behavior to the controller.
- Creating a general-purpose programming or templating language.
- Adding arithmetic, conditionals, functions, or arbitrary code execution to
  variable expressions.
- Changing namespace precedence.
- Allowing duplicate variable keys within one scope.
- Defining customer-specific structured object schemas in GOET Core.
- Treating Go structs or Go package names as the public configuration model.
- Normalizing or translating paths for a target operating system, mount,
  container, transport, or execution environment during variable resolution.

## Architectural Context

The capability belongs to `internal/variable`, which owns typed variables,
namespaces, references, accessors, and resolution. Consumers such as resource
constraints should receive a fully resolved typed object and validate its
domain-specific shape at their own boundary.

This preserves the separation described by:

- `docs/ARCHITECTURE_OVERVIEW.md`
- `docs/CUSTOMER_API.md`
- `internal/variable/README.md`

The resource-constraint epic is the first concrete consumer. It needs a key
expression such as:

```text
${project_config.name}/python-env/torch
```

inside an object field. The current resolver parses object expressions as raw
JSON and infers nested types. It does not recursively resolve that embedded
reference.

The current `TypeList(element)` model and `ResolvedList` constructor require a
single homogeneous element type and currently reject nested lists. The target
generic-list model replaces those restrictions. Existing consumers that expose
typed helpers such as string-list access must continue to validate their own
required item types.

## Compatibility and Migration

GOET has not yet produced a production version, so this epic does not preserve
backward compatibility with the current experimental structured-variable
forms. Existing raw-JSON object expressions and homogeneous
`list[element-type]` declarations will be replaced by the recursive typed form.

After the transition, the resolver must reject legacy structured forms with a
clear error. It must not silently reinterpret them or permanently support both
models. Repository-owned tests, fixtures, and demo JSON may be updated as
cleanup during the implementation slices that make the old forms invalid.

## Proposed Slices

The agreed implementation sequence is:

1. `001-generic-list-type.md` replaces parameterized homogeneous list types
   with one generic list type and consumer-owned item validation.
2. `002-typed-expression-json-model.md` adds the recursive language-neutral
   typed-expression representation.
3. `003-expression-definition-validation.md` validates reusable expression
   definitions without requiring assembled variable scopes.
4. `004-variable-typed-expression-integration.md` makes a variable a named
   root typed expression, migrates repository-owned usages, and resolves fully
   literal typed trees.
5. `005-recursive-whole-value-references.md` resolves whole-value references at
   any structured node with normal namespace precedence and bounded depth.
6. `006-string-path-interpolation.md` resolves mixed literal and referenced
   scalar text in string and path expressions.
7. `007-resolution-diagnostics.md` adds nested JSON Pointer diagnostics and
   distinguishes reference cycles from maximum-depth failures.
8. `008-worker-launch-config-integration.md` proves the complete behavior at an
   existing controller structured-value consumer.

## Agreed Design Decisions

- Every value is represented by a recursive node containing `type` and
  `expression`.
- An object expression is an unordered map from field names to recursive typed
  expression nodes.
- A list expression is an ordered array of independently typed expression
  nodes and may be heterogeneous or contain nested lists.
- A whole-value `${...}` reference preserves type; embedded `${...}` tokens
  interpolate canonical scalar text into string and path expressions.
- `\${` escapes a literal `${` sequence.
- Objects and lists cannot be interpolated into text.
- The new structured form replaces the experimental legacy forms without a
  backward-compatibility period.
- A resolved path retains its `path` type, but variable resolution does not
  perform target-specific path normalization or translation.

## Completion Criteria

- Structured variables have an agreed language-neutral recursive schema.
- Every structured layer has explicit type information before resolution.
- Object field types are no longer inferred from raw JSON in the target form.
- A list has no single element type; every list item declares its own type and
  expression.
- Heterogeneous and nested lists resolve without losing item type information.
- Nested whole-value references resolve through normal namespace precedence.
- Supported string and path interpolation resolves scalar references mixed
  with literal text.
- Nested objects and lists resolve recursively into typed `ResolvedValue`
  trees.
- Cycles, excessive depth, type mismatches, and missing references produce
  field-specific errors.
- Duplicate keys within one scope remain errors.
- Legacy structured forms are rejected, and repository-owned usages are
  migrated to the recursive typed form.
- The agreed implementation slices are complete and relevant tests pass.

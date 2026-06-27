# Structured Variable Resolution Epic

Status: Proposed

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
- Define an explicit type contract for list values and their elements.
- Resolve variable references recursively inside object fields and list
  elements.
- Support string interpolation where a structured string expression combines
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

The following is illustrative rather than an agreed public JSON schema:

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
typed expression. A nested object would repeat the same pattern, and a list
would explicitly declare its list and element types.

The final serialized representation must remain language-neutral and usable by
the CLI, REST API, and future Python and R adapters.

## Non-Goals

- Adding resource-constraint scheduling behavior to the controller.
- Creating a general-purpose programming or templating language.
- Adding arithmetic, conditionals, functions, or arbitrary code execution to
  variable expressions.
- Changing namespace precedence.
- Allowing duplicate variable keys within one scope.
- Defining customer-specific structured object schemas in GOET Core.
- Treating Go structs or Go package names as the public configuration model.

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

## Compatibility and Migration

Existing object variables store a JSON string in `Variable.Expression` and
infer nested field types during `ParseLiteral`. The target representation will
change that contract. Before implementation, the epic must decide whether to:

- reject the old inferred representation after a schema transition,
- support both representations during a migration period, or
- provide an explicit legacy form.

Silent reinterpretation of existing object expressions is not acceptable.

## Proposed Slices

The slice sequence is not yet agreed. Candidate implementation areas are:

1. Define the recursive typed-expression data model and serialized shape.
2. Parse and validate explicitly typed object fields.
3. Parse and validate explicitly typed list elements.
4. Recursively resolve whole-value references in structured expressions.
5. Add bounded string interpolation in typed string expressions.
6. Preserve nested error paths and recursion-depth protection.
7. Define and implement compatibility behavior for existing raw-JSON objects.
8. Integrate one structured-variable consumer as an end-to-end proof.

No numbered slice files should be created until the data model, expression
grammar, and compatibility policy are agreed and this epic is explicitly
marked Ready.

## Open Questions

- What exact JSON shape represents a recursive typed expression?
- Does an object expression use a map keyed by field name, a list of named
  expression nodes, or another language-neutral form?
- Must every list element repeat its type, or does the list's declared element
  type govern all elements?
- What interpolation syntax is supported beyond whole-value `${...}`
  references?
- How are literal `${` sequences escaped inside interpolated strings?
- What compatibility policy applies to current raw-JSON object expressions?
- Should structured interpolation be implemented for path expressions as well
  as string expressions?

## Completion Criteria

- Structured variables have an agreed language-neutral recursive schema.
- Every structured layer has explicit type information before resolution.
- Object field types are no longer inferred from raw JSON in the target form.
- Nested whole-value references resolve through normal namespace precedence.
- Supported string interpolation resolves references mixed with literal text.
- Nested objects and lists resolve recursively into typed `ResolvedValue`
  trees.
- Cycles, excessive depth, type mismatches, and missing references produce
  field-specific errors.
- Duplicate keys within one scope remain errors.
- Existing object variables follow the agreed compatibility or migration
  policy.
- The agreed implementation slices are complete and relevant tests pass.

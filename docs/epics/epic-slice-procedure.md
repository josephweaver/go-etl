# Epic Slice Procedure

Last updated: 2026-06-26

## Purpose

This document defines the procedure for creating implementation slices for GOET.

A slice is a small, bounded implementation task intended for surgical AI-assisted coding. A slice should provide enough context for Codex or another coding agent to make a focused change without reading or internalizing the entire repository.

## Core Rule

The human must be able to state the desired action concretely.

Good:

```text
Add CopyInto behavior to SSHTransport.
```

Bad:

```text
Build an adapter for talking to HPCC.
```

If the request is vague, first create or update the epic README. Do not create implementation slices until the human and AI can state a concrete action.

## Directory Layout

Epics live under:

```text
docs/epics/<epic-name>/
```

Each epic folder should contain:

```text
README.md
001-<slice-name>.md
002-<slice-name>.md
003-<slice-name>.md
```

Example:

```text
docs/epics/ssh-transport/
  README.md
  001-sshtransport-config.md
  002-sshtransport-connect.md
  003-sshtransport-copyinto.md
```

## Epic README

Each epic must have a `README.md` that explains the broader concept.

The epic README answers:

* What larger capability is being built?
* Why does GOET need it?
* What architectural boundary does it belong to?
* What is explicitly not part of the epic?
* What sequence of slices is expected?

Example epic concept:

```text
Adapter for talking to HPCC through SSH-backed execution.
```

The epic README is broad. Individual slices are narrow.

## Slice Creation Process

Slice creation is interactive.

The AI must not immediately commit a slice from a vague request. Instead, the human and AI should work through the scope until both agree on:

* Objective
* Allowed files
* Tests
* Out of scope
* Acceptance criteria
* Notes

Only after agreement should the slice be committed to the repository.

## Slice Template

```markdown
# <NNN> <Slice Title>

Status: proposed

## Objective

<One or two sentences describing the concrete behavior to add or change.>

## Required Context

Read these files first:

- <architecture or epic README>
- <relevant production file>
- <relevant test file>

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

- <path/to/file.go>

## Allowed Test Files

- <path/to/file_test.go>

## Out Of Scope

- <explicit non-goal>
- <explicit non-goal>
- <explicit non-goal>

## Acceptance Criteria

- <observable behavior>
- <observable behavior>
- <observable behavior>

## Notes

- <implementation hint, architectural constraint, or sequencing note>
```

## Required Fields

### Objective

The objective is the most important part of the slice.

It must describe a concrete action.

Good:

```text
Add CopyInto behavior to SSHTransport so the transport can copy a local file into a remote path over SFTP.
```

Bad:

```text
Improve SSH support.
```

### Required Context

This section controls context size.

It tells the coding agent what to read before implementing. Keep this list short.

Prefer:

```text
- docs/epics/ssh-transport/README.md
- docs/PLUGIN_CONTRACT.md
- cmd/controller/ssh_transport.go
- cmd/controller/ssh_transport_test.go
```

Avoid telling the agent to read the whole repository.

### Allowed Production Files

This is the change budget.

The coding agent may modify only these production files unless it reports that the slice cannot be completed within the budget.

### Allowed Test Files

The coding agent may modify only these test files unless it reports that the slice cannot be tested within the budget.

### Out Of Scope

This prevents scope creep.

Include anything tempting but not part of the current slice.

Examples:

```text
- Retry behavior
- Slurm integration
- Controller config factory wiring
- New public interfaces
- Real HPCC credentials
```

### Acceptance Criteria

Acceptance criteria define what must be true when the slice is complete.

They should describe observable behavior, not implementation preference.

Good:

```text
- Copies one local file to one remote path.
- Creates parent directories only if explicitly required by the transport contract.
- Returns a useful error when the remote copy fails.
```

Bad:

```text
- Make the code clean.
```

### Notes

Notes may include architectural constraints, sequencing details, or implementation hints.

Use notes to prevent known mistakes without over-specifying implementation.

## Slice Rules

1. Build slices one by one.
2. Do not create broad slices from vague goals.
3. Every slice belongs to an epic folder.
4. Every epic folder has a README.
5. Every slice has a concrete objective.
6. Every slice has allowed production files and test files.
7. Every slice has explicit out-of-scope boundaries.
8. Every slice has acceptance criteria.
9. Do not include EC mode in slice files. Creating the slice is the user-specified planning action.
10. Do not commit the slice until the human and AI agree on the final scope.

## Agent Behavior

When asked to create a slice, the agent should:

1. Ask the human for the concrete action if the request is vague.
2. Identify or propose the epic folder.
3. Check whether the epic README exists.
4. Draft the slice.
5. Review the objective, allowed files, tests, out of scope, and acceptance criteria with the human.
6. Revise until agreed.
7. Commit the slice only after agreement.

## Summary

Epic READMEs describe broad capability.

Slices describe bounded implementation actions.

A good slice lets Codex perform a surgical change without loading the whole project into context.

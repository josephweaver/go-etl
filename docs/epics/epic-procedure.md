# Epic Procedure

Last updated: 2026-06-26

## Purpose

This document defines the procedure for creating a new implementation epic.

An epic describes a significant architectural capability that will be implemented through a sequence of small implementation slices.

The purpose of an epic is **planning**, not implementation.

Implementation is performed through individual slices created according to `epic-slice-procedure.md`.

---

# Directory Structure

Each epic lives in its own directory.

```text
docs/
└── epics/
    └── <epic-name>/
        ├── README.md
        ├── 001-<slice>.md
        ├── 002-<slice>.md
        └── ...
```

The README defines the epic.

Each numbered document defines one implementation slice.

---

# Epic Creation Philosophy

An epic should be completed during a single collaborative planning session between the human and AI.

The objective is to fully understand the architectural capability before implementation begins.

The planning session may require many conversational turns.

The resulting README should capture the agreed design.

---

## Epic Lifecycle

Every implementation capability follows the same lifecycle.

```text
Idea
    ↓
Epic (Proposed)
    ↓
Collaborative Planning
    ↓
Epic (Ready)
    ↓
Slice Creation
    ↓
Implementation
    ↓
Epic Review
    ↓
Epic (Implemented)
```

The AI should help move an epic through these stages but should never advance an epic to **Ready** or **Implemented** without explicit agreement from the human.

---

# Epic Status

Every epic has one of three states.

```text
Proposed
```

The capability is still being explored.

Goals and implementation strategy may change.

```text
Ready
```

The human and AI agree the epic is sufficiently decomposed into implementation slices.

Implementation may begin.

```text
Implemented
```

All agreed slices have been completed and accepted.

---

# Required Sections

## Purpose

Describe the architectural capability being added.

The purpose should answer:

> What capability is GOET gaining?

---

## Goals

Goals describe what the completed epic should accomplish.

Goals should be architectural rather than implementation-specific.

Example:

* Execute commands through SSH.
* Transfer files to remote systems.
* Reuse existing transport abstractions.

---

## Non-Goals

Explicitly list things that are **not** part of the epic.

Examples:

* Slurm scheduling
* Runtime management
* Authentication redesign

This section prevents scope creep during planning.

---

## Architectural Context

Briefly explain where this epic belongs.

Reference relevant architecture documents.

Examples:

* PLUGIN_CONTRACT.md
* ARCHITECTURE_OVERVIEW.md
* CUSTOMER_API.md

Avoid duplicating architectural documentation.

---

## Proposed Slices

This section is a planning aid.

It represents the current understanding of the work required.

Example:

```text
001 Connection

002 Execute

003 CopyInto

004 CopyFrom

005 Directory Helpers
```

This list is **not** authoritative.

The actual implementation slices may differ as planning evolves.

Slices may be:

* added
* removed
* reordered
* merged
* split

during discussion with the human.

---

## Completion Criteria

Describe what must be true before the epic is considered complete.

Example:

* All proposed capabilities are implemented.
* All agreed slices are complete.
* Tests pass.
* Public interfaces remain consistent with architecture.

---

# Planning Procedure

The AI should guide the human through the planning process.

Typical workflow:

1. Identify the architectural capability.
2. Draft goals.
3. Draft non-goals.
4. Discuss architecture.
5. Brainstorm possible slices.
6. Refine or reorganize slices.
7. Determine whether additional slices are required.
8. Mark the epic as **Ready**.

Only after the epic is **Ready** should implementation slices be written.

---

# Relationship to Slice Procedure

Implementation slices are created using:

```text
docs/epics/epic-slice-procedure.md
```

Each slice should represent one focused implementation task.

---

# Agent Responsibilities

When assisting with epic planning, the AI should:

* Keep the discussion focused on architecture.
* Encourage small, concrete implementation slices.
* Suggest additional slices if important work appears to be missing.
* Compare completed slices with the proposed slice list.
* Notify the human when the agreed scope appears complete.
* Avoid discussing implementation details prematurely.

---

# Guiding Principle

The epic exists to answer:

> **What capability are we building?**

Each slice exists to answer:

> **What is the next concrete implementation step?**

The epic is complete when both the human and AI agree that the capability has been decomposed into a coherent set of implementation slices ready for execution.

## Epic Review

When implementation of all agreed slices is complete, the AI should:

1. Compare implemented slices against the Proposed Slices.
2. Identify missing or incomplete work.
3. Recommend additional slices if implementation revealed new requirements.
4. Recommend changing the epic status to **Implemented** if both the human and AI agree the goals have been satisfied.
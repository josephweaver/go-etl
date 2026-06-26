# GOET Architecture Overview

Last updated: 2026-06-26

## Purpose

This document describes the enduring architecture of GOET. It explains what the system is, the major architectural components, and the responsibilities of each layer. It intentionally avoids implementation details and current development status.

## Architectural Philosophy

GOET is a distributed orchestration platform.

GOET does not attempt to replace domain-specific computation. Instead, it manages where, when, and how computation executes.

A user already knows how to train a model, rasterize imagery, execute SQL, or run an analysis. GOET provides the infrastructure to execute those workflows reproducibly across local machines, containers, HPC clusters, and future execution environments.

## Layered Architecture

```text
Customer
│
├── CLI
├── Python SDK
├── R SDK
└── REST

        │

Controller
│
├── Submission API
├── Workflow Compiler
├── Variable Resolver
├── Scheduler
├── Worker Manager
├── Artifact Manager
└── Attempt Ledger

        │

Execution Environment
│
├── Transport
├── Scheduler
├── Runtime
└── Shell Dialect

        │

Workers
```

## Core Concepts

The architecture revolves around a small set of stable concepts:

- Controller
- Project
- Workflow
- Submission
- Worker
- Work Item
- Attempt
- Artifact
- Execution Environment

## Controller

The Controller is the operating system of GOET. It owns orchestration state, receives submissions, compiles workflows, schedules work, records execution attempts, manages artifacts, and coordinates workers. Workers are intentionally disposable; the Controller is authoritative.

## Project

A Project provides customer or research context, including configuration, defaults, plugins, data locations, and policies. Projects are independent of workflows so that workflows can be reused across multiple projects.

## Workflow

A Workflow defines reusable work. It specifies tasks, dependencies, variables, and expected artifacts without embedding deployment-specific details.

## Execution Environment

Execution environments isolate backend-specific behavior behind stable interfaces. Current architectural roles include:

- Transport
- Scheduler
- Runtime
- Shell Dialect

This allows the same workflow to execute on different infrastructures with minimal change.

## Worker

Workers obtain work from the Controller, execute assigned work items, report status, and return artifacts. Workers should remain lightweight and stateless wherever practical.

## Execution Pipeline

```text
Customer
    ↓
Submission
    ↓
Workflow Compilation
    ↓
Work Items
    ↓
Queue
    ↓
Workers
    ↓
Artifacts
    ↓
Attempt Ledger
    ↓
Status
```

## Architectural Principles

GOET follows these principles:

- Controller owns orchestration state.
- Workers own computation.
- Workflows are portable.
- Projects provide execution context.
- Execution environments are replaceable.
- Configuration is explicit.
- Public APIs remain stable.
- Customer-specific logic stays outside GOET core.
- Everything should be resumable where practical.
- Everything should be designed for reproducibility.

## Relationship to Customer APIs

The canonical customer interaction model is:

```text
controller.submit(project, workflow)
```

Near-term customers interact primarily through the CLI using canonical JSON files (`controller.json`, `project.json`, and `workflow.json`). Future Python and R SDKs are expected to be thin adapters over the same public model.

## Relationship to Internal Implementation

Internal packages may evolve over time. The architectural concepts described here are intended to remain stable even as implementations change. Public APIs, configuration schemas, and extension contracts should reflect these architectural concepts rather than incidental implementation details.

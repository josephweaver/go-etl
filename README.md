# GOET

GOET is a distributed orchestration runtime for executing workflows across local machines, HPC clusters, cloud infrastructure, containers, and future execution environments.

The project is under active research and development. The long-term objective is a stable orchestration platform that can execute customer workflows while keeping the orchestration engine reusable, extensible, and independently owned.

---

## Start Here

This repository is organized around a small number of living design documents.

| Document | Purpose |
|----------|---------|
| `PROJECT_STATE.md` | Current implementation status and verified capabilities. |
| `TARGET_STATE.md` | Long-term architectural direction. |
| `AGENTS.md` | Guidance for AI coding agents working on the project. |
| `EPI_CTL.md` | Epistemic Control methodology used during development. |
| `OWNERSHIP_BOUNDARY.md` | Architectural ownership boundaries between GOET core, plugins, workflows, and customer assets. |

Additional architecture documents will be added over time, including:

- `CUSTOMER_API.md`
- `PLUGIN_CONTRACT.md`
- `ARCHITECTURE.md`
- `LICENSING.md`

---

## Project Vision

GOET is evolving from an ETL-oriented prototype into a general orchestration platform.

The core architecture separates:

- Controller
- Worker
- Workflow Compiler
- Variable System
- Attempt Ledger
- Execution Environment
    - Transport
    - Shell Dialect
    - Scheduler
    - Runtime

The controller owns orchestration decisions while workers remain relatively simple: obtain work, execute work, report results, and repeat.

---

## Design Principles

- Stable public interfaces.
- Customer workflows instead of customer forks.
- Backend extensibility through plugins.
- Strong separation between orchestration infrastructure and customer business logic.
- Long-term maintainability suitable for research and commercial deployments.

## CLI Submission

The current user-facing submission path is the command-shaped CLI implemented in `cmd/demo-client`.

Current supported examples:

```bash
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json
```

```bash
goet submit \
  --controller-url http://localhost:8080 \
  --project project.json \
  --workflow workflow.json
```

```bash
goet status <submission_id>
```

```bash
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json \
  --wait
```

```bash
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json \
  --json
```

```bash
goet status <submission_id> --json
```

For repeated display, use operating-system tooling such as:

```bash
watch -n 5 goet status <submission_id>
```

GOET does not provide a built-in `--watch` option in this concept.

When `--wait` is used, completed submissions exit with status code `0`; failed or otherwise unrecognized terminal states exit non-zero.

---

## Repository Status

This repository is the primary development repository for GOET.

Interfaces should be considered experimental unless explicitly documented as stable.

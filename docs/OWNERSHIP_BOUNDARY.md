# GOET Ownership Boundary

Last updated: 2026-06-26

## Purpose

GOET is intended to become long-lived orchestration infrastructure for Joseph Weaver's research lab and customer workflow execution. This document defines the ownership boundary between GOET core, public interfaces, backend adapters, customer configuration, customer workflow logic, and customer data.

The central rule is simple:

```text
Customers use GOET.
Customers do not become co-owners of GOET core merely because GOET runs their work.
```

This document is not legal advice. It is an architectural and project-governance statement that should later be reflected in licenses, contracts, statements of work, repository permissions, and customer onboarding materials.

## Ownership Layers

```text
Layer 1: GOET Core Runtime
Layer 2: GOET Public API and SDKs
Layer 3: GOET Plugin Contracts
Layer 4: Backend Implementations and Worker Images
Layer 5: Customer Workflows and Configurations
Layer 6: Customer Data and Results
```

The lower layers are platform infrastructure. The upper layers are use-specific material.

## Layer 1: GOET Core Runtime

GOET Core Runtime is owned by Joseph Weaver unless explicitly transferred by a separate written agreement.

Core includes:

- Controller queue semantics.
- Controller HTTP API implementation.
- Workflow submission and compilation machinery.
- Work-item assignment semantics.
- Worker pull/execute/report loop.
- Attempt ledger semantics.
- Variable namespace, type, and resolver model.
- Runtime identity and fingerprint model.
- Idempotency and skip-decision logic.
- Internal Go packages that implement the orchestration engine.
- Security-sensitive controller-worker coordination logic.

Customer projects should not modify this layer. Customer needs should be expressed through public APIs, configuration, plugins, or worker-specific code.

## Layer 2: GOET Public API and SDKs

The public API and SDKs are owned by Joseph Weaver unless explicitly transferred by a separate written agreement.

This layer includes:

- CLI commands.
- Python package interface.
- R package interface.
- HTTP workflow-submission API.
- JSON schema for controller, project, and workflow submission.
- Status, artifact, and ledger-query APIs intended for users.

This layer is the primary customer-facing boundary. It should be stable enough that customer workflows can survive internal changes to GOET Core.

The public API should expose concepts such as:

```text
Controller
Project
Workflow
Submission
Status
Attempt
Artifact
```

It should not expose incidental internal package structure unless that structure is deliberately promoted into the public contract.

## Layer 3: GOET Plugin Contracts

Plugin contracts are owned by Joseph Weaver unless explicitly transferred by a separate written agreement.

This layer defines how external behavior attaches to GOET without changing the core runtime. Current architectural roles include:

```text
Transport
ShellDialect
Scheduler
Runtime
Worker operation
```

Plugins may implement these roles for systems such as:

- Local process execution.
- Docker.
- SSH.
- Slurm.
- Singularity/Apptainer.
- Spark.
- Python.
- R.
- GDAL.
- PyTorch.
- Cloud execution backends.

The plugin contract is part of the platform. Specific plugin implementations may have separate ownership depending on how they are created.

## Layer 4: Backend Implementations and Worker Images

Backend implementations and worker images should be classified explicitly.

Default rule:

```text
Generic reusable backend implementation: GOET-owned.
Customer-specific backend implementation: negotiate ownership explicitly.
Customer-authored worker image or model code: customer-owned unless agreed otherwise.
```

Examples of GOET-owned reusable backend implementations:

- Generic SSH transport.
- Generic Slurm scheduler.
- Generic Docker transport.
- Generic Singularity runtime.
- Generic worker-runtime preparation.
- Generic Python worker operation harness.
- Generic GDAL worker operation harness.

Examples of potentially customer-owned or separately negotiated implementations:

- A proprietary worker image containing customer business logic.
- A customer-specific connector to an internal database.
- A customer-specific scheduler adapter for a private platform.
- A customer-funded plugin whose statement of work grants the customer ownership.

If a feature might be reusable across customers, prefer implementing it generically and keeping it in GOET-owned infrastructure. If a feature exists only for one customer, keep it outside core unless there is a deliberate reason to generalize it.

## Layer 5: Customer Workflows and Configurations

Customer workflows and configurations are generally customer-owned unless the customer is using GOET-owned examples, templates, or reusable workflow libraries.

This layer includes:

- `project.json` or equivalent project configuration.
- `controller.json` or equivalent execution-environment configuration.
- `workflow.json` or equivalent workflow definition.
- Customer-specific variables.
- Customer-specific paths and resource choices.
- Customer-specific task graphs.
- Customer-specific workflow templates.

GOET should treat these as inputs to the platform, not as modifications to the platform.

A customer repository should look like:

```text
customer-project/
  project.json
  controller.json
  workflows/
    workflow-a.json
    workflow-b.json
  plugins/
  worker-images/
  data/
  README.md
```

The customer repository should depend on GOET. It should not fork GOET as the normal mode of use.

## Layer 6: Customer Data and Results

Customer data and customer results are customer-controlled unless a written agreement says otherwise.

This includes:

- Input datasets.
- Intermediate data derived from customer inputs.
- Output artifacts.
- Logs containing customer-sensitive information.
- Secrets, credentials, tokens, and keys.
- Customer-specific execution records.

GOET may store metadata about attempts, fingerprints, statuses, and artifacts. For customer deployments, the storage location, retention period, access policy, and deletion process should be explicit.

## Boundary Rules

### Rule 1: Customers submit workflows, not source patches

The intended customer-facing boundary is workflow submission, not raw work-item submission and not direct edits to GOET source.

Raw work-item submission may remain useful for testing and local administration, but it should not become the main customer API.

### Rule 2: Core changes require platform justification

A customer request should modify GOET Core only when the change is broadly useful, generic, and compatible with the long-term platform model.

Otherwise, the request should be implemented as:

- A workflow.
- A project configuration.
- A controller configuration.
- A plugin.
- A worker image.
- A customer repository change.

### Rule 3: Plugins attach behind contracts

New transports, schedulers, dialects, runtimes, and worker operations should attach behind explicit contracts. Avoid hard-coded customer or backend branches in the controller.

Preferred shape:

```text
GOET Core -> Role Interface -> Plugin Implementation -> Customer/Backend System
```

Avoid:

```text
GOET Core -> Customer-specific branch -> Customer/Backend System
```

### Rule 4: Secrets never enter core ownership

Customer secrets, keys, credentials, tokens, and private paths are not GOET source material. They should live in customer-controlled configuration, environment variables, secret stores, or deployment infrastructure.

### Rule 5: Generated artifacts require explicit classification

Generated artifacts may include customer data, GOET logs, attempt metadata, or reusable computed products. Each deployment should classify generated artifacts before customer use.

## Customer Engagement Modes

### Mode A: Platform Use

Customer uses GOET as a tool.

- GOET Core: Joseph Weaver.
- SDK/API: Joseph Weaver.
- Customer workflow/config/data: customer.
- Custom worker code: customer or separately negotiated.

This is the preferred default.

### Mode B: Plugin Development

A plugin is developed for a customer or backend.

Ownership should be explicit before development begins.

Recommended default:

- Generic plugin framework: Joseph Weaver.
- Reusable backend plugin: Joseph Weaver.
- Customer-specific secret/configuration/business logic: customer.

### Mode C: Custom Workflow Development

A customer pays for workflow creation.

Recommended default:

- GOET Core remains Joseph Weaver's property.
- Workflow files belong to the customer unless the contract says otherwise.
- Generic improvements discovered during the work may be incorporated into GOET Core if separated from customer confidential information.

### Mode D: Research Lab Internal Use

GOET runs internal research workflows.

- Core, SDKs, plugins, and workflows are Joseph Weaver's unless a collaborator agreement says otherwise.
- Research data ownership follows the applicable research agreement.

## Repository Rules

1. Keep GOET Core in the private `go-etl` repository unless and until a deliberate licensing strategy exists.
2. Keep customer workflows in separate repositories.
3. Do not commit customer data, secrets, or credentials to GOET Core.
4. Do not copy customer-specific business logic into GOET Core unless it has been generalized and cleared.
5. Document every new public API surface.
6. Keep examples synthetic unless customer permission exists.

## Design Implications

The current controller-worker split is correct:

```text
Controller: owns queue, workflow compilation, ledger, scheduling decisions.
Worker: pulls concrete assignments, executes supported operations, reports results.
```

The current execution-environment split is also correct:

```text
Transport + ShellDialect + Scheduler + Runtime
```

Future design should preserve these boundaries. GOET's long-term value is the stable orchestration model, not any one backend.

## Non-Goals

GOET Core should not become:

- A customer-specific application repository.
- A dumping ground for customer workflow files.
- A secrets store.
- A general-purpose data warehouse.
- A direct clone of Airflow, Spark, Slurm, or Kubernetes.
- A project where customer work casually contaminates platform ownership.

## Summary

GOET should be treated as a platform. Customers bring workflows, configurations, plugins, worker images, and data. GOET provides the orchestration runtime, public API, execution-environment model, worker protocol, and attempt ledger.

The safest long-term rule is:

```text
Keep reusable orchestration machinery in GOET.
Keep customer-specific work outside GOET.
Connect them through stable APIs and plugin contracts.
```

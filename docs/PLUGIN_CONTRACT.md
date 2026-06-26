# PLUGIN_CONTRACT

Last updated: 2026-06-26

# Purpose

GOET is intended to be a long-lived orchestration platform. New execution environments, runtimes, schedulers, languages, and storage systems should be added through plugins rather than by modifying GOET Core.

This document defines the architectural contract between GOET Core and plugin implementations.

---

# Design Goals

The plugin system should:

* Keep GOET Core backend-independent.
* Support multiple execution environments.
* Allow customer extensions without modifying GOET Core.
* Enable community-developed plugins.
* Maintain backward compatibility through stable contracts.
* Allow new capabilities to be added without changing workflow definitions.

---

# Architectural Principle

GOET Core owns orchestration.

Plugins own backend-specific behavior.

```text
                GOET Core
                     │
        ┌────────────┼────────────┐
        │            │            │
     Transport    Scheduler    Runtime
        │            │            │
        └────────────┼────────────┘
                     │
              Backend Platform
```

Customer-specific behavior should almost always be implemented as plugins, workflow definitions, or worker code—not by modifying GOET Core.

---

# Plugin Categories

## Execution Plugins

Execution plugins define where work executes.

### Transport

Responsible for communication.

Examples:

* Local
* SSH
* HTTP
* Docker API
* Kubernetes API

Responsibilities:

* Connect
* Authenticate
* Copy files
* Execute commands
* Stream output

---

### Scheduler

Responsible for requesting compute.

Examples:

* Local
* Slurm
* PBS
* LSF
* Kubernetes
* Spark

Responsibilities:

* Submit jobs
* Cancel jobs
* Query status
* Allocate resources

---

### Runtime

Responsible for preparing the execution environment.

Examples:

* Native Process
* Docker
* Apptainer
* Singularity
* Conda
* Python Virtual Environment

Responsibilities:

* Prepare runtime
* Install dependencies
* Launch worker
* Cleanup

---

### Shell Dialect

Responsible for shell syntax.

Examples:

* Bash
* PowerShell
* CMD

Responsibilities:

* Quote arguments
* Environment variables
* File paths
* Command generation

---

# Worker Plugins

Worker plugins implement executable work.

Examples:

* Python
* R
* GDAL
* Spark
* Shell
* PyTorch
* TensorFlow

Responsibilities:

* Deserialize work item
* Execute work
* Produce artifacts
* Report status

Workers should not perform orchestration.

---

# Future Plugin Types

Potential future plugin categories include:

* Artifact Store
* Secret Provider
* Authentication Provider
* Variable Provider
* Monitoring Provider
* Metrics Exporter
* Notification Provider

The plugin architecture should support these additions without changing GOET Core.

---

# Plugin Lifecycle

Every plugin participates in the same lifecycle.

```text
Load

↓

Validate Configuration

↓

Initialize

↓

Execute

↓

Shutdown
```

Plugins should release resources during shutdown.

---

# Configuration

Plugins receive typed configuration.

Plugins should never depend on global variables.

Preferred:

```text
TransportConfig

SchedulerConfig

RuntimeConfig
```

Avoid hidden configuration.

---

# Capability Discovery

Plugins should report capabilities.

Example:

```text
supportsFileCopy

supportsStreamingLogs

supportsJobCancellation

supportsContainers

supportsGPU

supportsResume
```

GOET Core should make scheduling decisions using capabilities rather than plugin type names.

---

# Versioning

Every plugin contract should have an API version.

Example:

```text
goet.plugin.transport.v1

goet.plugin.scheduler.v1

goet.plugin.runtime.v1
```

Breaking changes require a new version.

---

# Error Handling

Plugins should return structured errors.

Errors should distinguish between:

* Configuration error
* Authentication failure
* Communication failure
* Backend failure
* User error
* Internal plugin bug

---

# Ownership Rules

GOET Core owns plugin interfaces.

Individual plugin implementations may have different ownership depending on how they were developed.

Preferred default:

* Generic reusable plugin → GOET
* Customer-specific plugin → negotiated
* Customer workflow → customer

---

# Design Rules

1. Plugins should be stateless whenever practical.

2. Plugins should not own orchestration state.

3. Plugins should not communicate directly with other plugins.

4. Plugins should not depend on GOET internal packages beyond the published contract.

5. Plugins should expose explicit capabilities.

6. Plugins should prefer typed configuration.

7. Plugins should avoid customer-specific branching.

---

# Non-Goals

The plugin system is not intended to:

* Replace workflow definitions.
* Replace worker implementations.
* Expose internal controller state.
* Become a scripting language.
* Become a dependency injection framework.

---

# Summary

GOET grows by adding plugins, not by increasing complexity inside GOET Core.

Every new backend should first answer:

> Can this be implemented as a plugin?

If the answer is yes, the plugin approach should be preferred.

This keeps the orchestration engine stable while allowing execution environments, languages, and infrastructure to evolve independently.

# SC: Controller Containerization

Status: placeholder  
Owner: TBD  
Created: 2026-07-08  
Repository area: `docs/concepts/controller-containerization/`  
Implementation target: GOET / go-etl controller runtime

## Purpose

Define the strategic concept for running the GOET controller as a containerized service while preserving the existing controller responsibilities, database ownership model, worker orchestration behavior, and client/controller boundary.

This SC exists to create a stable planning surface before implementation slices are written.

## Problem Statement

The controller is expected to become a long-running orchestration process that manages workflow admission, work dispatch, worker state, execution records, artifact metadata, and operational observability.

As the system moves toward reproducible deployments and remote worker execution, the controller should have a predictable runtime environment that can be started, stopped, inspected, upgraded, and tested consistently.

Containerizing the controller should reduce host-environment drift without hiding the controller’s operational state or making local development harder.

## Goals

- Package the controller runtime into a repeatable container image.
- Preserve clear separation between client, controller, worker, and shared/common code.
- Provide a predictable controller startup path for local development.
- Support persistent controller state through explicit mounted volumes or configured database connections.
- Keep secrets and environment-specific configuration outside the image.
- Make controller logs, health checks, and failure modes visible.
- Prepare the controller for later deployment patterns without prematurely committing to Kubernetes, cloud hosting, or a specific production orchestrator.

## Non-Goals

- Do not redesign the controller architecture.
- Do not containerize workers in this SC unless needed as a test fixture.
- Do not introduce Kubernetes, Nomad, ECS, or other production schedulers yet.
- Do not move client responsibilities into the controller container.
- Do not bake credentials, project files, workflow files, or data assets into the controller image.
- Do not require LandCore-specific assumptions in the generic controller container design.

## Current Assumptions

- The controller remains the long-running orchestration process.
- The client remains a lightweight interface for submitting workflows and inspecting state.
- Workers may be local, SSH-based, or container-backed, but worker containerization is handled separately.
- Controller state must survive container restarts.
- Configuration should be explicit, inspectable, and overrideable by environment variables, config files, or mounted paths.
- Local development should remain possible without a production deployment stack.

## Expected Outputs

This SC should eventually produce operational slices covering:

1. Minimal controller container image.
2. Controller configuration surface for container execution.
3. Persistent state and volume contract.
4. Health check and readiness endpoint or command.
5. Local `docker compose` development harness.
6. Logging and diagnostics expectations.
7. Controller startup/shutdown behavior.
8. Documentation updates for running the controller locally.

## Candidate Operational Slices

### OS-001: Minimal Controller Image

Create a minimal container image that builds or packages the controller binary and starts it with explicit configuration.

Expected implementation surface:

- `Dockerfile.controller` or equivalent
- build instructions
- controller entrypoint
- basic smoke test
- documentation update

### OS-002: Container Configuration Contract

Define how the controller receives runtime configuration in container mode.

Expected implementation surface:

- environment variable names
- config file path conventions
- defaults suitable for local development
- failure behavior for missing required config
- documentation update

### OS-003: Persistent Controller State

Define how controller state is persisted outside the container lifecycle.

Expected implementation surface:

- mounted data directory or database URL
- migration/init expectations
- restart behavior
- local development example
- documentation update

### OS-004: Health and Readiness

Add a simple health/readiness surface suitable for local compose usage and future deployment automation.

Expected implementation surface:

- health command or HTTP endpoint
- readiness definition
- failure cases
- tests where practical
- documentation update

### OS-005: Local Compose Harness

Create a local development harness for running the controller container with persistent state and inspectable logs.

Expected implementation surface:

- `docker-compose.yml` or compose fragment
- named volume or bind mount
- local configuration example
- smoke command
- documentation update

### OS-006: Controller Container Runbook

Document how to build, run, stop, inspect, and troubleshoot the controller container.

Expected implementation surface:

- README updates
- common commands
- log inspection
- state reset instructions
- known limitations

## Design Questions

- Should the controller image build from source inside Docker, or copy a locally built binary?
- Should the default local state use SQLite, a file-backed DB, or an external database service?
- Should controller health be exposed through HTTP, CLI command, or both?
- What is the minimum useful controller configuration for local development?
- How should the controller distinguish development, test, and production-like container modes?
- Which paths are considered stable public runtime contracts?
- Which logs must be structured versus plain text?

## Risks

- Containerization could obscure controller state if persistence is not explicit.
- Docker-specific assumptions could leak into core controller logic.
- Local development could become slower if the container path is required too early.
- Secrets could accidentally be passed through logs, environment dumps, or compose files.
- Health checks could report process liveness without confirming controller readiness.
- Controller image layout could become coupled to LandCore-specific project structure.

## Readiness Checklist

This SC is ready to split into OS files when the following are known:

- Controller binary/package entrypoint is identified.
- Required runtime configuration is listed.
- Controller state storage mechanism is confirmed.
- Local development command is defined.
- Expected smoke test behavior is defined.
- Ownership boundary between controller containerization and worker containerization is explicit.

## Implementation Boundary

Allowed future implementation work should stay focused on making the controller runnable in a container without changing workflow semantics.

Changes that alter workflow admission, dispatch semantics, worker contracts, artifact promotion, data asset behavior, or LandCore-specific processing should be handled in separate SCs or explicitly called out as prerequisites.

## Notes

This README is intentionally a placeholder. It should be revised after the current controller entrypoint, configuration model, and persistence model are inspected.


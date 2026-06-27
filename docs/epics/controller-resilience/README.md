# Controller Resilience Epic

Status: Proposed

## Purpose

Define how GOET identifies a controller process instance and what happens to
work, reports, and orchestration state when that instance stops or restarts.

This is a placeholder epic. Its scope requires a dedicated planning session
before implementation slices are proposed.

## Goals

- Give each running controller instance an identity.
- Associate assigned work with the controller instance that issued it.
- Prevent late worker reports from an abandoned controller instance from being
  accepted by a replacement instance.
- Define whether controller restart abandons, cancels, recovers, or reconciles
  previously assigned compute.
- Define which orchestration state must survive restart.
- Make restart and recovery outcomes observable.

## Non-Goals

- Implementing resource constraints.
- Coordinating multiple active controllers.
- Designing high availability or leader election before single-controller
  restart semantics are agreed.
- Assuming that ledger history alone represents active orchestration state.

## Architectural Context

The controller is authoritative for orchestration state, as described in
`docs/ARCHITECTURE_OVERVIEW.md`. A process restart creates a boundary between
the old controller's assignments and the replacement controller's authority.

The resource-constraint epic currently assumes that restart abandons existing
compute and clears active resource holders. A controller instance ID would let
the replacement controller recognize and reject reports from that abandoned
compute. This assumption must be reviewed here before it becomes a stable
contract.

## Proposed Slices

No implementation slices are proposed yet. The restart model and persistence
boundary must be agreed first.

## Open Questions

- Does restart abandon all assigned compute, or should any work be recovered?
- Should the controller attempt to cancel compute before shutting down or after
  recovering from an unexpected stop?
- Which requests and reports must carry the controller instance ID?
- Where is durable orchestration state stored?
- How are graceful shutdown, process crash, host failure, and network partition
  distinguished?
- When may abandoned work be safely retried?

## Completion Criteria

- Controller instance identity has an agreed lifecycle and transport contract.
- Restart behavior is defined for pending, assigned, completed, failed, and
  late-reporting work.
- The durable state boundary is explicit.
- Resource-holder recovery behavior is consistent with the restart model.
- The agreed implementation slices are complete and relevant tests pass.

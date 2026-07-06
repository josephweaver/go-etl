# Miscellaneous Architecture Refinements

## Purpose

This Strategic Concept captures small architectural refinements, consistency improvements, and technical debt discovered during implementation or Epistemic Control (EC) reviews.

These items are intentionally:
- Small in scope.
- Independently implementable.
- Low-risk.
- Valuable for improving architectural consistency, maintainability, and long-term evolution.

Unlike larger Strategic Concepts, these Operational Slices are not expected to introduce major user-visible features. Their goal is to reduce ambiguity, remove accidental complexity, and align the implementation with the intended architecture.

## Selection Criteria

An item belongs here if it:

- Closes a gap between the intended architecture and the current implementation.
- Removes an outdated assumption.
- Simplifies the mental model.
- Improves provenance, persistence, scheduling, or execution consistency.
- Is too small to justify its own Strategic Concept.

## Operational Slices

| ID | Title | Status | Notes |
|----|-------|--------|-------|
| 001 | Decouple workers from `run_id` | Implemented | Treat workers as reusable execution capacity rather than resources owned by a workflow run. `workers.run_id` is preserved as a legacy/non-authoritative launch-context field. |

## Candidate Backlog

Future EC reviews may add items such as:

- Normalize inconsistent terminology.
- Remove obsolete fields left from earlier designs.
- Improve persistence invariants.
- Strengthen controller recovery behavior.
- Eliminate duplicate state.
- Clarify ownership boundaries.
- Improve schema documentation.
- Improve provenance metadata.
- Refactor internal APIs for consistency.

## Exit Criteria

A slice is complete when:

1. The implementation matches the intended architecture.
2. Documentation is updated.
3. Tests cover the new behavior.
4. Any obsolete assumptions are removed.
5. PROJECT_STATE.md is updated where appropriate.

## Notes

This Strategic Concept is expected to grow gradually as implementation reviews and Epistemic Control sessions identify opportunities for refinement.

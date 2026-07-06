# OS 007+ Stage-Level `output_json` Review Package

This package contains a focused review and fast-follower implementation prompt for the `dependency-aware-workflows` operational slices, specifically around whether `output_json` should be persisted at the **stage** level.

## Files

- `01-review-stage-output-json-007-012.md` — review of OS 007 through OS 012 and the current stage-level schema concern.
- `02-codex-amend-current-os007-stage-output-json.md` — small amendment to apply to the in-flight OS 007 implementation.
- `03-codex-fast-follower-output-retention.md` — fast-follower prompt for bounded/pruned output retention.
- `04-implementation-checklist.md` — concise checklist for PR review.

## Bottom Line

Do **not** use `workflow_stages.output_json` as the canonical persistence location for `workflow.step[index]`.

The docs define outputs at the logical **step** level:

- `workflow.step[index]` is a list in workflow-definition order.
- A non-fanout step produces one object.
- A fanout step produces a list ordered by `work_item_index`.
- A stage is only a readiness/completion boundary and may contain multiple steps.

Therefore, one `output_json` column on `workflow_stages` cannot represent the output contract safely. It would collapse multiple step outputs into one ambiguous stage value and create unnecessary database duplication.

Recommended policy:

1. Persist logical outputs on dependency **step** state while the workflow is running.
2. Do not write step outputs into `workflow_stages.output_json`.
3. Keep `workflow_stages.output_json` null/unused for dependency-aware workflow output semantics.
4. Bound raw completion output size before persistence.
5. Prune membership-level output JSON after step aggregation.
6. Prune step-level output JSON only after the workflow is terminal, unless a later dependency analysis proves it is no longer referenced.
7. Preserve hashes, byte counts, IDs, states, timestamps, and failure reasons.

## Suggested PR Sequence

```text
PR 1: OS 007 functional implementation
      - step output capture
      - fanout aggregation
      - workflow.step scope
      - no canonical use of workflow_stages.output_json

PR 2: output retention fast follower
      - byte limits
      - prune membership outputs after aggregation
      - prune step outputs at terminal state
      - ensure status/logs do not expose full output_json
```

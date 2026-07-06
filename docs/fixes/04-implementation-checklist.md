# Implementation Checklist: Stage-Level `output_json`

Use this during code review.

## Must Be True

- [ ] `workflow.step[index]` is built from dependency step outputs.
- [ ] `workflow.step[index]` is not built from `workflow_stages.output_json`.
- [ ] Logical step outputs are not duplicated into `workflow_stages.output_json`.
- [ ] A stage with multiple steps preserves each step output separately.
- [ ] Fanout output order is by `work_item_index`, not completion order.
- [ ] Empty fanout step output is `[]` at the step level.
- [ ] Stage completion updates stage state, not a stage output payload.
- [ ] Output-capture failure fails the workflow and prevents downstream activation.
- [ ] Status/logs do not include full output JSON.

## Storage Retention

- [ ] Raw completed output has a byte limit before persistence.
- [ ] Logical step output has a byte limit before persistence.
- [ ] Membership output JSON is pruned after step aggregation.
- [ ] Step output JSON is retained while the workflow is running.
- [ ] Step output JSON is pruned at terminal workflow state.
- [ ] Hash and byte metadata survive pruning.
- [ ] Pruned output is not represented as `{}` or `null`.

## Tests To Look For

- [ ] `workflow.step` ignores bogus stage output.
- [ ] Parallel stage with two steps resolves two distinct step outputs.
- [ ] Membership output prunes after aggregation.
- [ ] Step output remains available before terminal state.
- [ ] Step output prunes after terminal state.
- [ ] Oversized output is rejected with artifact-reference guidance.
- [ ] Status/logs avoid dumping full output JSON.

## Red Flags

- [ ] New code writes logical output to `workflow_stages.output_json`.
- [ ] New code reads `workflow_stages.output_json` inside `workflowStepScope`.
- [ ] New code creates a synthetic stage output wrapper like `{ "steps": [...] }`.
- [ ] New code prunes step output immediately after stage completion.
- [ ] New code silently substitutes `{}` or `null` for pruned output.
- [ ] New status/log payloads include `OutputJSON`.

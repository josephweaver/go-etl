# Development Governance

Last updated: 2026-07-07

This file preserves the moved development-governance section from the pre-split root state file.

## Development Governance

`../epistemic-control/EPI_CTL.md` now uses a three-category epistemic-control model:

```text
Strategic Understanding (SU) /20
Operational Control (OC) /10
Implementation Recall (IR) /5
Surprise Penalty -/5
Total EC /35
```

The protocol distinguishes architectural and causal understanding from practical codebase control and from short- or medium-term recall of implementation details. Low implementation recall is explicitly acceptable when Strategic Understanding and Operational Control remain strong.

`../epistemic-control/EPI_CTL.md` also now defines longitudinal retention reviews. Same-day audits are `T`; follow-up retention-chain reviews are `T+3`, `T+14`, and `T+180`, named with the original session date, such as:

```text
../epistemic-control/epi_ctl/20260624.md
../epistemic-control/epi_ctl/20260624_T3.md
../epistemic-control/epi_ctl/20260624_T14.md
../epistemic-control/epi_ctl/20260624_T180.md
```

Retention reviews are first-class audits and are treated as the primary evidence for durable ownership. The protocol also records Codex usage indicators and ActivityWatch distraction/context-switch metrics when available.
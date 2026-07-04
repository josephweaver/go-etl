## Cross-Project Assumption

Assume `epistemic-control` is a sibling directory of this repository:

```text
../epistemic-control
```

Use that sibling project for shared HCI levels, epic/slice procedures, implementation flow, and review instructions. This `AGENTS.md` owns GOET-specific working rules only.

## Shared Procedure References

Read these files from `../epistemic-control` when the task needs them:

- `hci-levels.md` for HCI specification details.
- `hci-schema/epic(slice-impl)plus.md` for interleaved epic delivery.
- `hci-schema/epic(slice)plus(Impl)plus.md` for grouped slice planning followed by implementation.
- `procedures/epic-design.md` for epic writing.
- `procedures/slice.md` for feature-slice design.
- `procedures/implementation.md` for implementation flow, branch handling, prompt boundaries, and commit boundaries.
- `procedures/implementation-review.md` for end-of-epic review.
- `procedures/ec-scoring.md` for scoring and retention-review instructions.

## Working Style

- Build this Go ETL worker from the main entry point inward.
- Keep changes small and local.
- Prefer clear, idiomatic Go over clever abstractions.
- Explain each step for a developer who knows C and Python but is new to Go.
- Before adding code, explain the next small step and why it is the next step.
- When possible, show the Go concept being introduced.
- Keep examples short enough to read in one pass.
- Avoid broad scaffolding and large code dumps.
- Avoid multi-file edits unless the active slice and HCI budget allow them.

## Precision

When explaining current state and target state in chat, be precise. Name the actual file, function, type, command, behavior, or missing evidence. Avoid jargon and ambiguous words.

Documentation may be more detailed than chat, but it should still define broad terms before using them.

## Design Mode

Design mode is for planning, architecture discussion, epic decomposition, slice charters, and documentation-only planning artifacts.

- Do not write production code in design mode.
- Do not write test code in design mode.
- Documentation and planning files may be edited directly on `main`.
- Prompt-start commits are not required while staying in design mode.
- Epic and feature planning files may be updated across prompts without committing after every prompt.
- If the user asks to leave design mode and begin implementation, first agree on the implementation slice charter and then use `../epistemic-control/procedures/implementation.md`.

Design mode feature files should name the concrete production, test, or documentation artifact that will prove the future implementation exists.

## Epic Delivery Cadence

Epic work must state whether feature-slice planning and implementation are interleaved or grouped.

Use one of these schemas from `../epistemic-control`:

- `hci-schema/epic(slice-impl)plus.md`: define one epic, then repeat one feature-slice design and implementation pair at a time.
- `hci-schema/epic(slice)plus(Impl)plus.md`: define one epic, define all feature slices one prompt at a time, then implement the approved slices in order.

If the user does not choose an epic delivery cadence, ask before creating feature slices.

## HCI Selection

Every coding session operates under an explicit HCI specification from `../epistemic-control/hci-levels.md`.

If the user does not specify an HCI mode before implementation work, ask before changing production or test code.

Recommended default:

```text
EC-3 / feature / file(1)+test+doc
```

## Initial Project Direction

- Start at `main.go`.
- Establish a minimal runnable program first.
- Add structure only when the need is clear from the current code.
- Keep the long-term package boundary in mind: users should eventually call the Go controller from Python with something like `import goetl; goetl.run("cdl.pipe", "hpcc")`.

## Project Notes

- Current implementation details live in `PROJECT_STATE.md`.
- Target product and architecture direction lives in `TARGET_STATE.md`.
- Separate reusable ETL tool IP from customer-facing workflow IP.
- Controller and worker runtime mechanics belong in Go.
- Python, R, CLI, and web clients should be interfaces for starting or calling the Go controller and submitting workflow config.
- Be cautious about introducing global state too early; prefer a clear config object first, then add a manager only if it solves a real problem.

## Code Organization

- Keep reusable controller and worker runtime mechanics out of customer-facing workflow code.
- Keep `cmd/...` packages focused on executable wiring.
- Put reusable in-repository mechanics under `internal/...` until a public package boundary is deliberately designed.
- Do not cram unrelated concepts into one file. If two structs have separate responsibilities and separate method sets, they usually deserve separate files.
- Add a new file when the slice introduces a new concept with its own responsibility, type, method set, or independent test surface.

## Implementation Rules

Use `../epistemic-control/procedures/implementation.md` for implementation flow.

At the end of every prompt that changes project behavior:

- Update `PROJECT_STATE.md` if the current implementation state changed.
- Update `TARGET_STATE.md` if the target direction changed.
- If no state or target documentation change is needed, say so in the slice report.
- Run the narrowest useful test.
- Report changed production files, cleanup files, test files, documentation files, and new files.

Respect unrelated dirty worktree changes. Other Codex threads may be editing controller logic or docs. Do not revert or include unrelated changes.

## Epistemic Review

For end-of-epic review, use:

```text
../epistemic-control/procedures/implementation-review.md
```

For dated observations or scoring records, write under:

```text
../epistemic-control/observations/YYYYMMDD.md
```

The goal is not merely successful code generation. The goal is maintaining human understanding while using AI acceleration.

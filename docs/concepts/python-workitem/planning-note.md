# Python WorkItem Operational Slice Planning Note

## Recommendation

Create and implement Python WorkItem Operational Slices one at a time.

The Strategic Concept can name the whole planned sequence, but the Operational Slice procedure is deliberately narrower. Each slice should be concrete enough that Codex can change a small set of files without reading the whole repository.

The safest cadence is:

1. Approve the Strategic Concept README.
2. Add `001-workitem-source-and-python-operation-contract.md`.
3. Run Codex on slice 001 in a fresh context.
4. Review the diff and tests.
5. Create slice 002 using the actual state after slice 001 lands.
6. Repeat.

## Can all slice files be created in one go?

Yes, but only as proposed planning drafts under a grouped planning cadence.

That is useful if the human wants a visible backlog in `docs/concepts/python-workitem/`, but it is less precise because later slice files may need adjustment after earlier implementation changes.

For this project, prefer:

```text
Create all slice names in the Strategic Concept README.
Create individual Operational Slice files one at a time.
Implement individual Operational Slice files one at a time.
```

## Why one at a time is better for Codex token usage

A single Codex context should only read the active Operational Slice and the few files named in that slice. This avoids carrying controller, worker, source-cache, and demo-project context into every task.

Use fresh contexts for each slice. Ask Codex for a short report at the end, then start the next context with that report plus the next Operational Slice file.

## Suggested model/effort pattern

Use lower-cost models for local slices:

```text
001 shared model contract:      gpt-5.4-mini / medium / standard speed
003 worker bundle staging:      gpt-5.4-mini / medium / standard speed
004 Python subprocess runner:   gpt-5.4-mini / medium / standard speed
005 output/evidence contract:   gpt-5.4-mini / medium / standard speed
007 demo project fixture:       gpt-5.4-mini / low or medium / standard speed
```

Use a stronger model for controller/source-admission slices:

```text
002 controller source bundle:   gpt-5.4 / medium or high / standard speed
006 workflow compilation:       gpt-5.4 / high / standard speed
```

Reserve the highest-capability model for final review or hard failures.

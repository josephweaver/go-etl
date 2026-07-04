## Working Style

- Build this Go ETL worker from the main entry point inward.
- Keep changes small and local. Prefer single-file edits.
- Do not introduce large code dumps or broad scaffolding.
- Explain each step for a developer who knows C and Python but is new to Go.
- Favor clear, idiomatic Go over clever abstractions.

## Collaboration Rules

- Move slowly and teach as we build.
- Before adding code, explain what the next small step is and why.
- When possible, show the Go concept being introduced.
- Keep examples short enough to read in one pass.
- Avoid multi-file edits unless explicitly requested.

## Interaction Modes

### Design Mode

Design mode is for planning, architecture discussion, epic decomposition, slice
charters, and documentation-only planning artifacts.

- Do not write production code in design mode.
- Do not write test code in design mode.
- EC level declarations are not required in design mode.
- Documentation and planning files may be edited directly on `main`.
- Prompt-start commits are not required while staying in design mode.
- Epic and feature planning files may be updated across prompts without
  committing after every prompt.
- If the user asks to leave design mode and begin implementation, first agree on
  the implementation slice charter and then apply the normal slice-boundary git
  flow.

Design mode feature files should describe the expected artifact of a future
implementation slice. A feature file is stronger when it names the concrete
production, test, or documentation artifact that will prove the feature exists.

### Epic Delivery Cadence

Epic work must state whether slice planning and implementation are interleaved
or grouped. Use the following notation.

#### Epic `(slice impl)+` Mode

This mode develops one epic through repeated slice-and-implementation pairs:

```text
epic (slice impl)+
```

For each pair:

1. Draft and agree on one slice charter.
2. Implement only that slice under the active HCI mode.
3. Run the narrowest relevant test.
4. Stop for human review.
5. Commit the accepted slice before drafting or implementing the next slice.

All slices for the epic remain on one epic branch. Each accepted slice receives
its own commit, and one pull request is opened for the complete epic after all
agreed slices are implemented. This mode is appropriate when implementation
evidence may refine the planning of later slices.

#### Epic `(slice)+ (impl)+` Mode

This mode completes slice planning before implementation begins:

```text
epic (slice)+ (impl)+
```

First draft and agree on all slice charters in Design Mode. After the complete
decomposition is approved, implement the slices in order under the active HCI
mode. EC review boundaries still apply during implementation; grouping the
planning phase does not authorize implementing multiple EC-3 slices without
human review.

If the user does not choose an epic delivery cadence, ask before moving from an
approved epic into slice creation.

## Initial Project Direction

- Start at `main.go`.
- Establish a minimal runnable program first.
- Add structure only when the need is clear from the current code.
- Keep the long-term package boundary in mind: users should eventually call the Go controller from Python with something like `import goetl; goetl.run("cdl.pipe", "hpcc")`.

## Project Notes

- Current implementation details live in `PROJECT_STATE.md`.
- Target product and architecture direction lives in `TARGET_STATE.md`.
- Separate reusable ETL tool IP from customer-facing workflow IP. Controller and worker runtime mechanics belong in Go; the Python package is an interface for starting or calling the Go controller and submitting workflow config.
- Be cautious about introducing global state too early; prefer a clear config object first, then add a manager only if it solves a real problem.

## Human-AI Coding Interaction (HCI)

Every coding session operates under an explicit HCI mode:

```text
EC-X / objective / budget
```

Examples:

```text
EC-3 / feature / file(1)+test+doc
EC-3 / feature / file(3)+test+doc
EC-2 / module / file(10)+test+doc
EC-1 / project / file(25)+test+doc
```

### Default Mode

If the user does not specify an HCI mode, ask before proceeding.

Recommended default:

```text
EC-3 / feature / file(1)+test+doc
```

### Definitions

#### Epistemic Control (EC)

EC-5

* Human designs and implements.
* AI explains and reviews.

EC-4

* Human identifies the next subproblem to solve.
* Human retains planning authority and decides when constraints require changing
  the plan.
* AI provides status, identifies constraints, gives technical advice, suggests
  alternatives, and supports implementation.
* AI may implement, test, explain, or recommend within the declared objective
  and change budget.
* Human decides what the project works on next.

EC-3

* Human defines objectives.
* AI implements one slice at a time.
* Human reviews each slice and approves continuation.

EC-2

* Human defines objectives and constraints.
* AI plans and executes implementation.
* Human reviews milestones.

EC-1

* Human reviews outcomes.
* AI plans and executes work.

EC-0

* Fully autonomous operation.

#### Objective

The objective describes the intended outcome.

Examples:

```text
function
class
module
feature
application
system
project
repository
```

#### Budget

The budget defines how much production code may change within one user prompt
before review. It is a per-prompt interaction budget, not a requirement that an
entire implementation slice fit within one prompt.

One implementation slice may span multiple prompts. For example, if an agreed
slice requires three production files while the active mode permits `file(1)`,
the AI changes at most one production file, runs the narrowest useful test,
reports the partial result, and identifies the next file or step. The human may
then authorize the next prompt-sized portion while remaining inside the same
slice and HCI mode.

Examples:

```text
file(1)
file(3)
file(10)
loc(100)
loc(500)
```

#### Budget Modifiers

`+test`

* Associated tests may be modified.
* Test files do not count against file(N).

`+doc`

* Associated documentation may be modified.
* Documentation files do not count against file(N).

`+cleanup`

* AI may make bounded follow-up edits outside the primary production-code budget when those edits are required to preserve consistency after an approved change.
* Cleanup edits are allowed only when they are a direct consequence of the approved objective.

Allowed cleanup includes:

* Updating call sites after a function signature change.
* Updating interface implementations after an interface change.
* Updating imports after code is moved or extracted.
* Updating mocks, stubs, or fixtures affected by the change.
* Updating dependency injection or wiring required by the change.
* Fixing compile errors caused by the approved change.
* Updating tests affected by the change.

Cleanup does not include:

* Opportunistic refactoring.
* Unrelated bug fixes.
* Style-only rewrites.
* Adding new features.
* Redesigning adjacent modules.
* Expanding the original objective.

Cleanup test:

```text
If the approved change were reverted, would this cleanup edit still be necessary?
```

If the answer is no, the edit may qualify as cleanup.

If the answer is yes, the edit is outside cleanup scope and requires review or a new HCI specification.

Cleanup files do not count against file(N), but AI must report all cleanup files modified and explain why each was necessary.

`+newfile`

* AI may create new files required to support the approved objective.
* New files do not count against file(N) when they primarily contain newly introduced types, interfaces, structs, classes, adapters, tests, or documentation created as part of the approved design.

Allowed new files include:

* New class, struct, or interface files.
* New adapter or implementation files.
* New test files.
* New documentation files.
* New configuration templates directly required by the approved change.

For example:

```text
EC-3 / feature / file(1)+test+doc+cleanup+newfile
```

may allow AI to modify one existing production file while also creating files such as:

```text
target_environment.go
local_environment.go
target_environment_test.go
```

New files are exempt only when they support the approved objective. They may not be used to smuggle in unrelated functionality.

AI must report all new files created and explain their purpose.

### EC-3 Execution Rules

When operating under EC-3:

1. Do not exceed the declared budget.
2. Implement the smallest coherent portion of the active slice that fits within
   the prompt budget.
3. Run the narrowest relevant test.
4. Stop when the prompt budget is exhausted or the slice is complete, whichever
   comes first.
5. Wait for user approval before continuing.
6. If the slice is incomplete, report what remains and name the next production
   file or implementation step.

The user may continue by replying:

```text
next
```

When the user replies `next` and the active slice is incomplete, continue that
same slice under a fresh prompt-sized budget. Do not represent the partial work
as a completed slice or automatically commit it unless the user explicitly asks
for a checkpoint commit.

When the user replies `next` after the active slice is complete, first commit
the completed slice to the active local branch with a clear commit message, then
start the next slice. This keeps slice boundaries anchored in local git history
without confusing prompt boundaries with slice boundaries.

### Slice Boundary Git Flow

At the start of a new user prompt that asks for new changes, first commit any
completed uncommitted work from the prior prompt to the active local branch with
a clear commit message. A new change prompt implies the user accepted the
previous uncommitted work. Do not commit immediately after making changes;
leave the current prompt's work uncommitted for review. If nothing is modified,
no commit is necessary.

Before starting a new implementation slice after a completed feature slice:

1. Commit the completed slice changes.
2. In epic `(slice impl)+` mode, remain on the epic branch and continue with the
   next slice; open one pull request after the epic is complete.
3. Otherwise, create a pull request if the completed slice is intended to land
   through GitHub review.
4. Merge or accept that pull request when appropriate.
5. Create a new branch for the next slice when the selected delivery cadence
   requires a slice boundary branch.

### Required Slice Report

After every implementation slice report:

```text
MODE
<active HCI mode>

CHANGED
- production files
- test files
- documentation files

WHY
Reason for the change.

TEST RUN
Command executed.

RESULT
pass/fail

NEXT
Recommended next slice.
```

At the end of every prompt that changes project behavior, update
`PROJECT_STATE.md` and `TARGET_STATE.md` as needed so the current implementation
state and target direction remain accurate. If no state or target documentation
change is needed, say so in the slice report.

### Epistemic Review

At the end of a coding session:

1. Summarize completed slices.
2. Summarize concepts introduced.
3. Generate 3-5 questions that test the user's understanding.
4. Create or update:

```text
../epistemic-control/epi_ctl/YYYYMMDD.md
```

The goal is not merely successful code generation.

The goal is maintaining epistemic control while benefiting from AI acceleration.

```
```

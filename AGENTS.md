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

* Human designs.
* AI drafts code.
* Human applies changes.

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

The budget defines how much production code may change before review.

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
2. Implement the smallest coherent slice.
3. Run the narrowest relevant test.
4. Stop after the slice.
5. Wait for user approval before continuing.

The user may continue by replying:

```text
next
```

When the user replies `next`, first commit the current completed slice to the active local branch with a clear commit message, then start the next slice. This keeps each continuation anchored in local git history before new changes are introduced.

### Slice Boundary Git Flow

At the start of a new user prompt that asks for new changes, first commit any
completed uncommitted work from the prior prompt to the active local branch with
a clear commit message. A new change prompt implies the user accepted the
previous uncommitted work. Do not commit immediately after making changes;
leave the current prompt's work uncommitted for review. If nothing is modified,
no commit is necessary.

Before starting a new implementation slice after a completed feature slice:

1. Commit the completed slice changes.
2. Create a pull request if the work is intended to land through GitHub review.
3. Merge or accept the pull request when appropriate.
4. Create a new branch for the next slice.

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

### Epistemic Review

At the end of a coding session:

1. Summarize completed slices.
2. Summarize concepts introduced.
3. Generate 3-5 questions that test the user's understanding.
4. Create or update:

```text
epi_ctl/YYYYMMDD.md
```

The goal is not merely successful code generation.

The goal is maintaining epistemic control while benefiting from AI acceleration.

```
```

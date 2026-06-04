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

* Bounded follow-up edits outside the primary target file are allowed only when required to restore consistency after an interface or signature change.
* Cleanup must be explainable as a direct consequence of the approved change.

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

### Slice Boundary Git Flow

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

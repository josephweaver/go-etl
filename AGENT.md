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

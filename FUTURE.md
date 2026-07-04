# Future Work

## Docker Environment Detection

On the first Docker call, detect whether a usable Docker environment exists.
If one does not exist, prompt the user with a yes/no confirmation before
installing one.

## PowerShell Dialect

Add a `PowerShellDialect` only when native Windows execution becomes a real
target. Current HPCC-facing work should stay on the Bash/Linux dialect through
WSL, Dockerized Slurm, and SingularityCE so Windows quoting and path rules do
not distract from the production runtime path.

## Local Git Source Negotiation

Consider explicit local Git source support after the repository-source cache is
working for GitHub and local filesystem providers.

The current repository-source Strategic Concept treats local paths as local filesystem
sources. If a local path happens to be inside a Git checkout, GOET should not
silently infer Git provenance. A future Operational Slice may let a submission
or controller configuration explicitly request local Git behavior, resolve a
local ref to a commit, and then publish the same pinned file shape into the
controller repository cache.

## Previous Workflow Stage Alias

Consider adding a generated read-only `workflow.previous` convenience variable
for dependency-aware workflows. It would resolve to the immediately preceding
completed stage's output: one typed object or fan-out list after a standalone
step, and the ordered aggregate list after a parallel group. Steps inside a
parallel group would see the stage before the group, never a concurrently
running sibling.

The initial dependency-aware workflow implementation should use explicit
`workflow.step[index]` references so output identity remains unambiguous while
the execution model is established.

## Large Output Manifests

Consider an immutable, content-addressed manifest artifact model when typed
workflow outputs become too large to persist inline. A worker could stream a
manifest to a controller-owned artifact API, while SQLite stores a compact
typed descriptor containing the artifact ID, SHA-256 hash, format, item count,
and size. The controller could materialize the verified manifest during JIT
fan-out compilation. The first workflow execution persistence model stores
typed outputs directly and does not require an artifact store.

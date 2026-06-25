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

# Direct Python Fixture

This development-only fixture exercises `worker execute` with one resolved
`python_script` work item. The Go integration test builds `source-bundle.zip`
from `source/main.py` at runtime; no binary ZIP is committed.

The work item intentionally omits attempt and source bookkeeping so direct mode
must supply the documented defaults. Passing `fail` instead of `fixture-value`
makes the script exit with status 7 after writing stdout and stderr.

# Execution Observability Issues

## 2026-07-07

- OS-010 `published-data-asset-copy-to-named-location` is not ready for implementation as written.
- Blocker: the worker has no artifact-promotion phase or promoted-artifact lookup surface yet. Current `cmd/worker/work_python.go` promotes only `GOET_OUTPUT_JSON` into `DataDir/<output_filename>`, so `from_artifact` publication would have no safe, named source artifact to copy.
- Required prerequisite: add or confirm the worker artifact-manifest/promotion step that records promoted artifact names and final paths before publication.
- Follow-up: the artifact-promotion prerequisite was repaired by implementing OS-002 and OS-003. OS-010 remains unimplemented and should be rechecked against the new promoted artifact manifest path before publication work starts.

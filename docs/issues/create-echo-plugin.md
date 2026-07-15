# Create Echo Plugin

## Status

Proposed.

## Problem

The current `write_demo_output` work item is useful as a controller/worker smoke test, but it behaves like a built-in worker operation rather than a plugin. It writes deterministic output to the worker data directory and records worker lifecycle messages, but it does not exercise the plugin stdout/stderr capture path.

That makes simple demo workflows less representative than real plugin-backed workflows. A user running a hello-world demo expects the work item to emit a visible message such as `hello-world`, and expects that message to appear in worker logs and in the client experience when running in wait mode.

## Proposal

Replace or supersede `write_demo_output` with a general Echo worker plugin.

The Echo plugin should:

1. Accept a small typed request, including a message string and optional output file settings.
2. Print the requested message to stdout.
3. Optionally write the same message, or a structured echo result, to the configured output artifact.
4. Return normal plugin result metadata through the existing worker result contract.
5. Use the same stdout/stderr capture path as other plugins.

The workflow-level operation should be named generally, for example:

```json
{
  "type": "echo",
  "parameters": {
    "message": "hello-world",
    "output_filename": "hello-world.txt"
  }
}
```

The old `write_demo_output` behavior can remain temporarily as a compatibility alias, but new demo workflows should use the Echo plugin.

## Client Wait-Mode Behavior

`goet submit --wait` should not only wait for terminal status. It should also surface plugin stdout/stderr observations that are associated with the submitted run.

Target behavior:

1. The worker runs the Echo plugin.
2. The Echo plugin writes `hello-world` to stdout.
3. The worker captures stdout using the standard plugin capture mechanism.
4. The worker sends captured stdout observations to the controller.
5. The controller persists those observations under the submission log stream.
6. The client app, while in wait mode, prints those observations as they become available or prints the bounded log tail before the final status.

For OS-01, the user-facing result should make this visible without a separate `goet logs` command. A successful single-item demo should show both the final completion status and the plugin-emitted stdout line.

## Acceptance Criteria

1. A workflow can define an Echo plugin work item with message `hello-world`.
2. Running the workflow through local controller/local worker completes successfully.
3. The worker-local captured stdout contains `hello-world`.
4. The controller submission logs include a stdout observation containing `hello-world`.
5. `goet logs <submission-id> --stream stdout` prints the Echo plugin stdout line.
6. `goet submit --wait` prints the Echo plugin stdout line during the wait flow or immediately before final status.
7. The final status still reports the expected completed work-item counts.
8. Existing `write_demo_output` tests either continue passing through a compatibility alias or are intentionally migrated to Echo plugin tests.

## Implementation Notes

- Keep Echo generic. It should not be tied to the demo project, OS-01, fake HPCC, or any one controller configuration.
- Prefer the standard plugin stdout/stderr capture path instead of adding a special case for demo work items.
- Keep output small and deterministic so Echo remains useful for controller, scheduler, worker, and client smoke tests.
- If wait-mode log streaming is not ready, implement a bounded log-tail fetch after terminal status as the first client-visible step.
- Update demo-project OS-01 workflows to use the Echo plugin once the plugin is available.

# Python WorkItem Operational Slice Drafts

These are draft Operational Slice charters for `docs/concepts/python-workitem/`.

Use them with the `Python WorkItem and Staged Source Execution` Strategic Concept. They are written so Codex can consume one slice at a time with a fresh context.

## Recommended Process

Create or review all slice drafts in planning mode, but implement only one slice per Codex task.

The operational-slice procedure says slice creation is interactive and each slice should be concrete before it is committed. Treat these files as proposed drafts until the human approves their scope.

## Draft Slice Order

```text
001-workitem-source-and-python-operation-contract.md
002-controller-source-bundle-api.md
003-worker-source-bundle-client-and-staging.md
004-python-subprocess-runner-no-environment-creation.md
005-python-output-evidence-contract.md
006-workflow-compilation-integration-for-python-source-workitems.md
007-python-demo-project-fixture.md
```

## Codex Usage Pattern

For each implementation task, start a fresh Codex context and give it:

```text
Read AGENTS.md.
Read docs/concepts/python-workitem/README.md.
Read docs/concepts/python-workitem/<slice-file>.md.
Implement only the approved slice.
Respect Required Context, Allowed Files, Out Of Scope, and Acceptance Criteria.
Run the narrowest tests listed in the slice.
Report changed files, tests run, behavior implemented, and unresolved issues.
```

Do not feed all implementation slices to Codex at once unless the task is explicitly documentation-only. The purpose of these slices is to prevent a large multi-file, multi-concept implementation from consuming a large context window.

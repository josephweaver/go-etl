# Execution Observability Documentation Bundle

This bundle is replacement-ready for:

```text
docs/concepts/execution-observability/
```

It overwrites the existing README and existing slice files:

```text
001-logging-model.md
002-log-configuration.md
003-controller-logging-endpoint.md
004-worker-logging-client.md
005-controller-filesystem-log-sinks.md
006-worker-fallback-logging.md
008-log-levels-and-filtering.md
```

It adds:

```text
007-python-subprocess-log-emission.md
009-submission-log-read-api.md
010-cli-logs-command.md
011-update-observability-docs-and-smoke.md
MODEL_RECOMMENDATIONS.md
```

The slice numbering intentionally preserves the existing `008-log-levels-and-filtering.md` filename while filling the missing 007 slot and adding the submission-addressable read/CLI slices that become possible after Submission CLI Status is complete.

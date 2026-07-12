# `<worker-plugin-name>` Workflow JSON Authoring Guide

Status: `<draft | stable | experimental>`
Applies to: GOET `<commit/date/version>`
Worker image/runtime: `<worker image name or runtime requirement>`
Plugin executable or handler: `<exact executable path or work-item type>`

## 1. Purpose

This document tells workflow authors and AI coding agents how to write a GOET `workflow.json` that invokes the `<worker-plugin-name>` worker plugin safely and reproducibly.

It documents:

1. the workflow JSON shape that GOET should compile;
2. the concrete work-item parameters the worker should receive;
3. the plugin request JSON contract;
4. valid operations, inputs, outputs, options, and defaults;
5. validation and smoke-test expectations.

## 2. AI Authoring Contract

When generating workflow JSON for this plugin, an AI agent must follow these rules.

1. Use only operation names listed in this document.
2. Use only documented input names, output names, parameter names, and option names.
3. Do not invent defaults. If a default affects scientific or operational meaning, require it to be explicit.
4. Do not use private absolute paths, controller cache paths, or developer-local paths.
5. Use materialized data-asset paths such as `${data.<binding>.path[0]}` only where the runtime explicitly supports that interpolation.
6. Plugin output paths must be artifact-relative slash paths. Do not use absolute paths, `..`, backslashes, drive letters, or trailing slashes.
7. Sensitive values must use protected references and explicit materialization. Never place plaintext secrets in workflow JSON.
8. If the request involves CRS, raster alignment, resampling, nodata handling, polygon inclusion, units, or output explosion risk, do not silently choose semantics. Mark the workflow as blocked until those choices are explicit.
9. Include a runnable smoke example and expected outputs for every documented operation family.

## 3. Current Integration Mode

Select one mode and delete the other.

### Mode A: Direct plugin work item

Use this when GOET has a first-class worker dispatch type for the plugin.

```text
workflow.json
  -> compiled work item type: <exact_work_item_type>
  -> worker dispatches plugin handler
  -> plugin writes artifacts
  -> worker reports artifact manifest/evidence
```

Required direct work-item type:

```text
<exact_work_item_type>
```

### Mode B: Wrapper work item

Use this when the plugin currently exists as an executable but is not yet a first-class work-item type.

```text
workflow.json
  -> compiled python_script or other wrapper work item
  -> wrapper writes plugin request JSON
  -> wrapper invokes plugin executable
  -> wrapper declares plugin outputs as GOET artifacts
```

Wrapper work-item type:

```text
python_script
```

Wrapper entrypoint:

```text
<relative/source/path/to/wrapper.py>
```

Plugin executable expected in worker image:

```text
<absolute/path/to/plugin-executable>
```

## 4. Ownership Boundary

| Layer                          | Owns                                                                                                                                                | Must not own                                                                       |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Workflow author                | Declaring data bindings, workflow steps, plugin operation, explicit options, expected artifacts                                                     | Runtime queue state, worker-local cache paths, controller internals                |
| Controller                     | Source admission, workflow compilation, dependency readiness, work-item queueing, attempt records                                                   | Plugin scientific semantics                                                        |
| Worker                         | Receiving one work item, preparing attempt directories, resolving materialized inputs/secrets, invoking the handler/executable, collecting evidence | Workflow expression resolution, source-control fetching, undeclared data discovery |
| Plugin                         | Transforming declared local inputs into declared artifacts and compact result metadata                                                              | Orchestration, queueing, publication policy, private data acquisition              |
| Post-processing script, if any | Domain-specific summaries derived from plugin artifacts                                                                                             | Reinterpreting plugin inputs without documented semantics                          |

## 5. Workflow JSON Fields

Use the project’s current accepted workflow JSON casing. Recommended public casing is `snake_case`. Do not mix `snake_case` and Go struct field names in the same document.

Minimum workflow structure:

```json
{
  "id": "<workflow_id>",
  "variables": [],
  "steps": []
}
```

Each plugin-producing step should document these fields.

| Field                                          |        Required | Meaning                                                  | Example                     |
| ---------------------------------------------- | --------------: | -------------------------------------------------------- | --------------------------- |
| `steps[].id`                                   |             yes | Stable workflow step identifier                          | `"field_crop_counts"`       |
| `steps[].parallel_with`                        |              no | Step ID this step may run beside, if supported           | `"other_independent_step"`  |
| `steps[].fan_out.work_item.fan_out_expression` | yes for fan-out | List/object expression used to generate work items       | `"${workflow.tiles}"`       |
| `id_token_accessor`                            | yes for fan-out | Accessor for stable work-item ID token                   | `".tile_id"`                |
| `output_accessor`                              | yes for fan-out | Accessor for stable output token                         | `".tile_id"`                |
| `type`                                         |             yes | Work-item dispatch type                                  | `"<exact_work_item_type>"`  |
| `output_prefix`                                |             yes | Filename prefix for logical output                       | `"field-crop-counts-"`      |
| `output_extension`                             |             yes | Filename extension for logical output                    | `".json"`                   |
| `parameters`                                   |             yes | Typed worker/plugin parameters                           | See below                   |
| `parameter_accessors`                          |              no | Accessors that bind fan-out values into parameter values | `{ "tile_id": ".tile_id" }` |
| `resource_constraints`                         |              no | Admission limits for shared resources                    | See below                   |

## 6. Work-Item Parameter Contract

Document every parameter the worker/plugin expects.

| Parameter           | Type     | Required | Sensitive | Materialization | Meaning                    | Example                |
| ------------------- | -------- | -------: | --------: | --------------- | -------------------------- | ---------------------- |
| `<param_name>`      | `string` |      yes |        no | none            | `<meaning>`                | `"<value>"`            |
| `<secret_param>`    | `string` |       no |       yes | `env` or `file` | `<meaning>`                | protected ref only     |
| `operation_request` | `object` |      yes |        no | none            | Plugin request JSON object | See operation examples |

Example public parameter:

```json
{
  "parameters": {
    "operation_request": {
      "type": "object",
      "value": {
        "api_version": "<plugin.api/version>",
        "kind": "<PluginOperationRequest>",
        "operation": "<operation_name>",
        "inputs": {},
        "outputs": {},
        "options": {}
      }
    }
  }
}
```

Example protected parameter:

```json
{
  "parameters": {
    "api_token": {
      "type": "string",
      "protected_ref": {
        "provider": "worker_env",
        "key": "MY_PLUGIN_TOKEN",
        "redaction_label": "my_plugin_token"
      },
      "materialize": {
        "mode": "env",
        "target": "MY_PLUGIN_TOKEN"
      }
    }
  }
}
```

## 7. Plugin Request JSON Contract

Every plugin request must use this envelope.

```json
{
  "api_version": "<plugin.api/version>",
  "kind": "<PluginOperationRequest>",
  "operation": "<operation_name>",
  "inputs": {},
  "outputs": {},
  "options": {}
}
```

Every plugin result must use this envelope.

```json
{
  "api_version": "<plugin.api/version>",
  "kind": "<PluginOperationResult>",
  "operation": "<operation_name>",
  "artifacts": [
    {
      "name": "<artifact_name>",
      "path": "<artifact-relative/path.ext>",
      "kind": "<table | raster | vector | metadata | directory>",
      "format": "<csv | json | tif | gpkg | parquet | txt>"
    }
  ],
  "summary": {},
  "warnings": []
}
```

## 8. Operation Matrix

| Operation          | Purpose          | Required inputs | Required outputs | Options          | Defaults               | Must not guess        |
| ------------------ | ---------------- | --------------- | ---------------- | ---------------- | ---------------------- | --------------------- |
| `<operation_name>` | `<one sentence>` | `<input names>` | `<output names>` | `<option names>` | `<safe defaults only>` | `<blocked decisions>` |

### Example: geospatial `raster_pair_value_counts`

| Field                 | Contract                                                                                   |
| --------------------- | ------------------------------------------------------------------------------------------ |
| Operation             | `raster_pair_value_counts`                                                                 |
| Purpose               | Count aligned categorical raster pairs into `field_id,crop_id,count` rows                  |
| Input mode 1          | `field_raster` and `value_raster`                                                          |
| Input mode 2          | `stacked_raster` with separate `field_band` and `value_band`                               |
| Required outputs      | `counts_csv`, `metadata_json`                                                              |
| Required options      | `require_aligned_grid: true`                                                               |
| Optional options      | `chunk_rows`, `field_dtype`, `value_dtype`, `include_value_nodata`                         |
| Safe dtype assumption | `uint16` only, unless implementation says otherwise                                        |
| Must not guess        | CRS, pixel alignment, nodata values, categorical resampling, inclusion/exclusion of nodata |

Request example:

```json
{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {
    "field_raster": {
      "path": "${data.yanroy_field.path[0]}",
      "band": 1,
      "nodata": 0
    },
    "value_raster": {
      "path": "${data.cdl_year.path[0]}",
      "band": 1,
      "nodata": 0
    }
  },
  "outputs": {
    "counts_csv": "results/field_crop_counts.csv",
    "metadata_json": "results/field_crop_counts.metadata.json"
  },
  "options": {
    "require_aligned_grid": true,
    "chunk_rows": 1024,
    "field_dtype": "uint16",
    "value_dtype": "uint16"
  }
}
```

Expected result shape:

```json
{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationResult",
  "operation": "raster_pair_value_counts",
  "artifacts": [
    {
      "name": "counts_csv",
      "path": "results/field_crop_counts.csv",
      "kind": "table",
      "format": "csv"
    },
    {
      "name": "metadata_json",
      "path": "results/field_crop_counts.metadata.json",
      "kind": "metadata",
      "format": "json"
    }
  ],
  "summary": {
    "valid_pixels": 0,
    "skipped_field_nodata": 0,
    "skipped_value_nodata": 0,
    "distinct_fields": 0,
    "distinct_values": 0,
    "distinct_pairs": 0,
    "count_dtype": "uint64"
  },
  "warnings": []
}
```

## 9. Target Direct Workflow Fragment

Use this section only after the plugin has a first-class work-item type.

```json
{
  "id": "example-plugin-workflow",
  "variables": [
    {
      "name": "tiles",
      "type": "list",
      "value": [
        {
          "tile_id": "fixture_tile",
          "field_binding": "yanroy_field",
          "value_binding": "cdl_year"
        }
      ]
    }
  ],
  "steps": [
    {
      "id": "field_crop_counts",
      "fan_out": {
        "work_item": {
          "fan_out_expression": "${workflow.tiles}",
          "id_token_accessor": ".tile_id",
          "output_accessor": ".tile_id",
          "type": "geospatial_operation",
          "output_prefix": "field-crop-counts-",
          "output_extension": ".json",
          "parameters": {
            "operation_request": {
              "type": "object",
              "value": {
                "api_version": "goet.geospatial/v1alpha1",
                "kind": "GeospatialOperationRequest",
                "operation": "raster_pair_value_counts",
                "inputs": {
                  "field_raster": {
                    "path": "${data.yanroy_field.path[0]}",
                    "band": 1,
                    "nodata": 0
                  },
                  "value_raster": {
                    "path": "${data.cdl_year.path[0]}",
                    "band": 1,
                    "nodata": 0
                  }
                },
                "outputs": {
                  "counts_csv": "results/field_crop_counts.csv",
                  "metadata_json": "results/field_crop_counts.metadata.json"
                },
                "options": {
                  "require_aligned_grid": true,
                  "chunk_rows": 1024,
                  "field_dtype": "uint16",
                  "value_dtype": "uint16"
                }
              }
            }
          }
        }
      }
    }
  ]
}
```

Before making this example authoritative, confirm that `geospatial_operation` exists as a supported work-item type and that the workflow compiler accepts the shown public JSON casing.

## 10. Current Wrapper Workflow Pattern

Use this section when the plugin is executable-only and must be called through a wrapper such as `python_script`.

The wrapper should:

1. read its typed parameters from `GOET_INPUT_JSON`;
2. resolve any data-asset paths already materialized by GOET;
3. write a plugin request JSON under `GOET_WORK_DIR`;
4. invoke the plugin executable with `--request <path>` and `--response <path>`;
5. copy or leave plugin artifacts under `GOET_ARTIFACT_DIR`;
6. write `GOET_OUTPUT_JSON` with an `artifacts` array declaring all output files.

Required Python parameters:

| Parameter           | Type               | Meaning                                                               |
| ------------------- | ------------------ | --------------------------------------------------------------------- |
| `python_entrypoint` | `string` or `path` | Wrapper script path inside admitted source                            |
| `python_args`       | `list`             | Wrapper arguments; may include supported data/artifact interpolations |

Data inputs should be declared through workflow `data` sections and explicit `asset.materialize` steps. Wrapper arguments may reference materialized paths with `${data.<binding>.path[0]}` or named file-role paths such as `${data.<binding>.files.<role>.path}`.

Wrapper output JSON example:

```json
{
  "plugin_result": {
    "api_version": "goet.geospatial/v1alpha1",
    "kind": "GeospatialOperationResult",
    "operation": "raster_pair_value_counts",
    "summary": {
      "valid_pixels": 224191,
      "distinct_pairs": 1482
    }
  },
  "artifacts": [
    {
      "name": "counts_csv",
      "path": "results/field_crop_counts.csv",
      "kind": "table",
      "format": "csv"
    },
    {
      "name": "metadata_json",
      "path": "results/field_crop_counts.metadata.json",
      "kind": "metadata",
      "format": "json"
    }
  ]
}
```

## 11. Resource Constraints

Use resource constraints when the plugin competes for shared devices, transfer sources, write targets, memory-heavy files, or single-writer locations.

| Resource                 | Recommended key            | Units | Operator |       Target | Notes                                  |
| ------------------------ | -------------------------- | ----: | -------- | -----------: | -------------------------------------- |
| GDAL CPU-heavy operation | `worker.gdal.cpu`          |   `1` | `<=`     | `<capacity>` | Use if GDAL jobs should be throttled   |
| Source transfer          | `source.<provider>.<name>` |   `1` | `<=`     | `<capacity>` | Use for shared HTTP/rclone/local roots |
| Publish target           | `publish.<target_name>`    |   `1` | `<=`     |          `1` | Use for named-location writes          |

Example:

```json
{
  "resource_constraints": [
    {
      "resource_key": "worker.gdal.cpu",
      "requested_units": 1,
      "operator": "<=",
      "target_units": 2
    }
  ]
}
```

## 12. Validation Checklist

Before accepting a generated workflow JSON, verify:

* [ ] Workflow ID is stable and descriptive.
* [ ] Step IDs are unique.
* [ ] Generated work-item IDs are deterministic.
* [ ] Work-item `type` is supported by the current runtime.
* [ ] `output_filename` is a filename only, not a path.
* [ ] Every parameter has a documented type.
* [ ] Sensitive parameters use `protected_ref`; plaintext secrets are absent.
* [ ] Data inputs use declared data bindings, not private paths.
* [ ] Plugin `api_version`, `kind`, and `operation` are exact.
* [ ] Every required input and output is present.
* [ ] Output artifact paths are safe relative paths.
* [ ] Options affecting scientific meaning are explicit.
* [ ] Expected output artifacts are documented.
* [ ] A fixture smoke test exists.
* [ ] The fixture test has deterministic expected rows or metadata.

## 13. Smoke Test

Each plugin authoring guide must include one minimal fixture.

### Command

```bash
<command that builds or runs the worker/plugin fixture>
```

### Expected artifacts

| Artifact          | Path              | Format     | Required |
| ----------------- | ----------------- | ---------- | -------: |
| `<artifact_name>` | `<relative/path>` | `<format>` |      yes |

### Expected output sample

```csv
<small deterministic expected output>
```

### Failure cases the smoke must catch

* unsupported operation;
* missing required input;
* unsafe output path;
* missing required output;
* invalid option value;
* semantic ambiguity that must not be guessed.

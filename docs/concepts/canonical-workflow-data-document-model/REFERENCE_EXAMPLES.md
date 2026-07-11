# Reference Examples

These examples are normative illustrations for the target authoring model. Exact Go field names may change, but the ownership and state transitions must not.

## Project YAML: General Yan-Roy definition

```yaml
api_version: goet/v1alpha1
kind: Project
id: landcore-rci

variables:
  target_environment_id: msu-hpcc

# General physical definition: both files are available.
data:
  inputs:
    yan_roy_field_segments:
      kind: envi_field_segments
      parameters:
        tile:
          type: string

      files:
        raster:
          member: "${asset.tile}/WELD_${asset.tile}_2010_field_segments"
          as: "WELD_${asset.tile}_2010_field_segments"
          required: true
        header:
          member: "${asset.tile}/WELD_${asset.tile}_2010_field_segments.hdr"
          as: "WELD_${asset.tile}_2010_field_segments.hdr"
          required: true

      select:
        - raster
        - header

      binding:
        provider_name: gdrive_release_data
        provider: gdrive_rclone
        location:
          remote: gdrive
          drive_path: Data/Field_Boundaries/ReleaseData.7z
        archive:
          type: seven_zip
          expose: selected_directory
        integrity:
          size_bytes: 261861012
          required: true
        cache:
          strategy: worker_cache
          cache_key: gdrive/field_boundaries/release-data/source.7z
          immutable: true
        materialization:
          scope: shared
          strategy: worker_cache
```

## Workflow YAML: Header-only override and explicit cache

```yaml
api_version: goet/v1alpha1
kind: Workflow
id: yan-roy-header-analysis

variables:
  tiles:
    - h18v07
    - h18v08

data:
  inputs:
    yan_roy_field_segments:
      select:
        - header

steps:
  - id: cache-field-segment-headers
    fan_out:
      over: "${workflow.tiles[*]}"
      as: tile
      id: "${fanout.tile}"
    data:
      materialize:
        field_segments:
          asset: yan_roy_field_segments
          with:
            tile: "${fanout.tile}"
    work:
      type: cache_data

  - id: analyze-field-segment-headers
    fan_out:
      over: "${workflow.tiles[*]}"
      as: tile
      id: "${fanout.tile}"
    data:
      inputs:
        field_segments:
          asset: yan_roy_field_segments
          with:
            tile: "${fanout.tile}"
    work:
      type: python_script
      parameters:
        python_entrypoint: scripts/analyze_header.py
        args:
          - --header
          - "${data.field_segments.path[0]}"

source_manifest:
  files:
    - role: python_entrypoint
      path: scripts/analyze_header.py
      content_type: text/x-python
```

## Equivalent Workflow JSON

```json
{
  "api_version": "goet/v1alpha1",
  "kind": "Workflow",
  "id": "yan-roy-header-analysis",
  "variables": {
    "tiles": ["h18v07", "h18v08"]
  },
  "data": {
    "inputs": {
      "yan_roy_field_segments": {
        "select": ["header"]
      }
    }
  },
  "steps": [
    {
      "id": "cache-field-segment-headers",
      "fan_out": {
        "over": "${workflow.tiles[*]}",
        "as": "tile",
        "id": "${fanout.tile}"
      },
      "data": {
        "materialize": {
          "field_segments": {
            "asset": "yan_roy_field_segments",
            "with": {
              "tile": "${fanout.tile}"
            }
          }
        }
      },
      "work": {
        "type": "cache_data"
      }
    },
    {
      "id": "analyze-field-segment-headers",
      "fan_out": {
        "over": "${workflow.tiles[*]}",
        "as": "tile",
        "id": "${fanout.tile}"
      },
      "data": {
        "inputs": {
          "field_segments": {
            "asset": "yan_roy_field_segments",
            "with": {
              "tile": "${fanout.tile}"
            }
          }
        }
      },
      "work": {
        "type": "python_script",
        "parameters": {
          "python_entrypoint": "scripts/analyze_header.py",
          "args": [
            "--header",
            "${data.field_segments.path[0]}"
          ]
        }
      }
    }
  ],
  "source_manifest": {
    "files": [
      {
        "role": "python_entrypoint",
        "path": "scripts/analyze_header.py",
        "content_type": "text/x-python"
      }
    ]
  }
}
```

## Mixed selection by step

The workflow may retain the project default. One step may select only the header while another selects both roles:

```yaml
steps:
  - id: inspect-header
    data:
      inputs:
        field_segments:
          asset: yan_roy_field_segments
          select:
            - header
          with:
            tile: "${fanout.tile}"

  - id: analyze-raster
    data:
      inputs:
        field_segments:
          asset: yan_roy_field_segments
          select:
            - raster
            - header
          with:
            tile: "${fanout.tile}"
```

## Structured function call

```yaml
variables:
  years:
    - 2020
    - 2021
  regions:
    - north
    - south
  year_regions:
    $type: list
    $call: list.crossproduct
    args:
      - $ref: years
      - $ref: regions
```

The loader normalizes this into an internal semantic function call. It does not preserve `$call` as a runtime JSON payload.

## Output publication

```yaml
# Project physical target.
data:
  outputs:
    report_archive:
      kind: archive
      format: zip
      binding:
        provider: gdrive_rclone
        location:
          remote: gdrive
          drive_path: Data/ETL/reports/report.zip
        overwrite_policy: fail_if_exists
```

```yaml
# Workflow visible publication step.
steps:
  - id: publish-report
    data:
      outputs:
        report_archive:
          from:
            step: build-report
            artifact: report_archive
          target: report_archive
    work:
      type: commit_data
```

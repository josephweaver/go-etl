package workflow

import (
	"bytes"
	"encoding/json"
	"testing"

	"goetl/internal/document"
	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCanonicalJSONAndYAMLWorkflowCompileEquivalently(t *testing.T) {
	jsonSource := []byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "canonical-equivalence-smoke",
		"variables": {
			"years": ["2024"]
		},
		"data": {
			"inputs": {
				"field_segments": {
					"kind": "fixture_matrix",
					"format": "csv",
					"parameters": {
						"year": {"type": "string"}
					},
					"files": {
						"header": {
							"member": "field_segments_${asset.year}.csv",
							"required": true
						}
					},
					"select": ["header"],
					"binding": {
						"provider_name": "fixture_data",
						"provider": "local_file",
						"location": {
							"name": "fixture_data",
							"path": "field_segments_${asset.year}.csv"
						},
						"cache": {
							"strategy": "worker_cache",
							"cache_key": "fixtures/field_segments_${asset.year}.csv",
							"immutable": true
						},
						"materialization": {
							"scope": "shared",
							"strategy": "worker_cache"
						}
					}
				}
			},
			"outputs": {
				"report_archive": {
					"kind": "archive",
					"format": "zip",
					"parameters": {
						"year": {"type": "string"}
					},
					"binding": {
						"provider": "registered_location",
						"location": {
							"name": "published_data",
							"path_template": "reports/${year}/report.zip"
						},
						"overwrite_policy": "fail_if_exists"
					}
				}
			}
		},
		"steps": [
			{
				"id": "cache-field-segments",
				"fan_out": {
					"over": "${workflow.years[*]}",
					"as": "year",
					"id": "${fanout}"
				},
				"data": {
					"materialize": {
						"field_segments": {
							"asset": "field_segments",
							"select": ["header"],
							"with": {
								"year": "${fanout}"
							}
						}
					}
				},
				"work": {
					"type": "cache_data",
					"parameters": {
						"target_environment_id": "target-local"
					}
				}
			},
			{
				"id": "build-report",
				"fan_out": {
					"over": "${workflow.years[*]}",
					"as": "year",
					"id": "${fanout}"
				},
				"work": {
					"type": "python_script",
					"parameters": {
						"python_entrypoint": "scripts/build_report.py",
						"python_args": ["--header", "${data.field_segments.path[0]}"]
					}
				}
			},
			{
				"id": "publish-report",
				"fan_out": {
					"over": "${workflow.years[*]}",
					"as": "year",
					"id": "${fanout}"
				},
				"data": {
					"outputs": {
						"report_archive": {
							"from": {
								"step": "build-report",
								"artifact": "report_archive"
							},
							"target": "report_archive",
							"with": {
								"year": "${fanout}"
							}
						}
					}
				},
				"work": {
					"type": "commit_data",
					"parameters": {
						"target_environment_id": "target-local"
					}
				}
			}
		]
	}`)
	yamlSource := []byte(`
api_version: goet/v1alpha1
kind: Workflow
id: canonical-equivalence-smoke
variables:
  years:
    - "2024"
data:
  inputs:
    field_segments:
      kind: fixture_matrix
      format: csv
      parameters:
        year:
          type: string
      files:
        header:
          member: field_segments_${asset.year}.csv
          required: true
      select:
        - header
      binding:
        provider_name: fixture_data
        provider: local_file
        location:
          name: fixture_data
          path: field_segments_${asset.year}.csv
        cache:
          strategy: worker_cache
          cache_key: fixtures/field_segments_${asset.year}.csv
          immutable: true
        materialization:
          scope: shared
          strategy: worker_cache
  outputs:
    report_archive:
      kind: archive
      format: zip
      parameters:
        year:
          type: string
      binding:
        provider: registered_location
        location:
          name: published_data
          path_template: reports/${year}/report.zip
        overwrite_policy: fail_if_exists
steps:
  - id: cache-field-segments
    fan_out:
      over: ${workflow.years[*]}
      as: year
      id: ${fanout}
    data:
      materialize:
        field_segments:
          asset: field_segments
          select:
            - header
          with:
            year: ${fanout}
    work:
      type: cache_data
      parameters:
        target_environment_id: target-local
  - id: build-report
    fan_out:
      over: ${workflow.years[*]}
      as: year
      id: ${fanout}
    work:
      type: python_script
      parameters:
        python_entrypoint: scripts/build_report.py
        python_args:
          - --header
          - ${data.field_segments.path[0]}
  - id: publish-report
    fan_out:
      over: ${workflow.years[*]}
      as: year
      id: ${fanout}
    data:
      outputs:
        report_archive:
          from:
            step: build-report
            artifact: report_archive
          target: report_archive
          with:
            year: ${fanout}
    work:
      type: commit_data
      parameters:
        target_environment_id: target-local
`)

	jsonDoc := decodeCanonicalWorkflowForEquivalence(t, jsonSource, document.SourceFormatJSON)
	yamlDoc := decodeCanonicalWorkflowForEquivalence(t, yamlSource, document.SourceFormatYAML)

	if got, want := canonicalHash(t, jsonDoc.Data), canonicalHash(t, yamlDoc.Data); got != want {
		t.Fatalf("data hash mismatch: json=%s yaml=%s", got, want)
	}

	jsonCompiled := compileCanonicalWorkflowForEquivalence(t, jsonDoc)
	yamlCompiled := compileCanonicalWorkflowForEquivalence(t, yamlDoc)
	if got, want := canonicalHash(t, jsonCompiled), canonicalHash(t, yamlCompiled); got != want {
		t.Fatalf("compiled workflow hash mismatch: json=%s yaml=%s", got, want)
	}

	counts := map[model.WorkItemType]int{}
	for _, item := range jsonCompiled {
		counts[item.Type]++
	}
	if counts[model.WorkItemTypeCacheData] != 1 || counts[model.WorkItemTypePythonScript] != 1 || counts[model.WorkItemTypeCommitData] != 1 {
		t.Fatalf("compiled type counts = %+v, want one cache_data, one python_script, one commit_data", counts)
	}
}

func decodeCanonicalWorkflowForEquivalence(t *testing.T, source []byte, format document.SourceFormat) document.CanonicalWorkflowDocument {
	t.Helper()
	doc, err := document.DecodeCanonicalWorkflowSource(source, document.DecodeOptions{Format: format})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource(%s) error = %v", format, err)
	}
	return doc
}

func compileCanonicalWorkflowForEquivalence(t *testing.T, doc document.CanonicalWorkflowDocument) []model.WorkItem {
	t.Helper()
	workflow, err := WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v", err)
	}
	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}

	items := []model.WorkItem{}
	for stageIndex := range plan.Stages {
		stage, err := CompileWorkflowStage(resolver, workflow, plan, stageIndex)
		if err != nil {
			t.Fatalf("CompileWorkflowStage(%d) error = %v", stageIndex, err)
		}
		for _, item := range stage.WorkItems {
			items = append(items, item.WorkItem)
		}
	}
	return items
}

func canonicalHash(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal canonical value: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("decode canonical value: %v", err)
	}
	_, hash, err := fp.CanonicalJSONSHA256(decoded)
	if err != nil {
		t.Fatalf("canonical hash: %v", err)
	}
	return hash
}

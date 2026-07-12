package workflow

import (
	"testing"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCanonicalComputeDataInputsCompileRequirementWithoutHiddenMaterialization(t *testing.T) {
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "compute-data-inputs",
		"variables": {
			"years": [2010]
		},
		"data": {
			"inputs": {
				"cdl_zip": {
					"kind": "cdl_archive",
					"format": "zip",
					"parameters": {
						"year": {"type": "int"}
					},
					"binding": {
						"provider_name": "nass_cdl",
						"provider": "http",
						"location": {
							"uri_template": "https://example.invalid/${year}_30m_cdls.zip"
						},
						"cache": {
							"strategy": "worker_cache",
							"cache_key": "cdl/${year}_30m_cdls.zip",
							"immutable": true
						},
						"materialization": {
							"scope": "shared",
							"strategy": "worker_cache"
						}
					}
				}
			}
		},
		"steps": [
			{
				"id": "extract-cdl",
				"fan_out": {
					"over": "${workflow.years[*]}",
					"as": "year",
					"id": "${fanout}"
				},
				"data": {
					"inputs": {
						"cdl_zip": {
							"asset": "cdl_zip",
							"with": {
								"year": "${fanout}"
							}
						}
					}
				},
				"work": {
					"type": "python_script",
					"parameters": {
						"target_environment_id": "target-local",
						"python_entrypoint": "scripts/extract_cdl.py",
						"python_args": ["--zip", "${data.cdl_zip.path[0]}"]
					}
				}
			}
		]
	}`), document.DecodeOptions{Path: "workflow.json"})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
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
	stage, err := CompileWorkflowStage(resolver, workflow, plan, 0)
	if err != nil {
		t.Fatalf("CompileWorkflowStage() error = %v", err)
	}
	planned, err := PlanStageAssetMaterializeWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanStageAssetMaterializeWorkItems() error = %v", err)
	}

	if len(planned.WorkItems) != 1 {
		t.Fatalf("planned work item count = %d, want authored compute only", len(planned.WorkItems))
	}
	compute := planned.WorkItems[0].WorkItem
	if compute.Type != model.WorkItemTypePythonScript {
		t.Fatalf("work item type = %q, want python_script", compute.Type)
	}
	if len(compute.DependsOn) != 0 {
		t.Fatalf("compute depends_on = %+v, want no hidden materialization dependency", compute.DependsOn)
	}
	assets, err := boundDataAssetsFromParameters(compute.Parameters)
	if err != nil {
		t.Fatalf("boundDataAssetsFromParameters() error = %v", err)
	}
	if len(assets) != 1 || assets[0].BindingName != "cdl_zip" || assets[0].Location.URI != "https://example.invalid/2010_30m_cdls.zip" {
		t.Fatalf("compute data assets = %+v", assets)
	}
}

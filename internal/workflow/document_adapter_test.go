package workflow

import (
	"strings"
	"testing"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestWorkflowFromCanonicalDocumentCompilesNoDataFixture(t *testing.T) {
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "canonical-demo",
		"variables": {
			"years": [2024, 2025]
		},
		"steps": [
			{
				"id": "download",
				"fan_out": {
					"over": "${workflow.years[*]}",
					"as": "year",
					"id": "${fanout}"
				},
				"work": {
					"type": "write_demo_output",
					"output_prefix": "demo",
					"output_extension": ".txt",
					"parameters": {
						"label": "canonical"
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

	items, err := CompileWorkflow(resolver, workflow)
	if err != nil {
		t.Fatalf("CompileWorkflow() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("work item count = %d, want 2", len(items))
	}
	if items[0].ID != "download-2024" || items[0].OutputFilename != "demo-2024.txt" {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[0].Type != model.WorkItemTypeWriteDemoOutput {
		t.Fatalf("first item type = %q", items[0].Type)
	}
	if items[0].Parameters["label"].Type != "string" || items[0].Parameters["label"].Value != "canonical" {
		t.Fatalf("label parameter = %+v", items[0].Parameters["label"])
	}
}

func TestWorkflowFromCanonicalDocumentRejectsRetiredAssetMaterializeTypes(t *testing.T) {
	tests := []struct {
		name        string
		workType    string
		wantMessage string
	}{
		{
			name:        "old cache_data",
			workType:    "cache_data",
			wantMessage: `work.type "cache_data" was renamed to "asset.materialize"`,
		},
		{
			name:        "asset materialization near miss",
			workType:    "asset.materialization",
			wantMessage: `unsupported work.type "asset.materialization"; use "asset.materialize"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := document.DecodeCanonicalWorkflowSource([]byte(`{
				"api_version": "goet/v1alpha1",
				"kind": "Workflow",
				"id": "canonical-demo",
				"variables": {
					"items": ["one"]
				},
				"steps": [
					{
						"id": "materialize",
						"fan_out": {
							"over": "${workflow.items[*]}",
							"as": "item",
							"id": "${fanout}"
						},
						"work": {
							"type": "`+test.workType+`"
						}
					}
				]
			}`), document.DecodeOptions{Path: "workflow.json"})
			if err != nil {
				t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
			}

			_, err = WorkflowFromCanonicalDocument(doc)
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("WorkflowFromCanonicalDocument() error = %v, want %q", err, test.wantMessage)
			}
		})
	}
}

func TestWorkflowFromCanonicalDocumentUsesFanoutFieldID(t *testing.T) {
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "canonical-demo",
		"variables": {
			"tiles": [{"tile": "h18v07"}]
		},
		"steps": [
			{
				"id": "inspect",
				"fan_out": {
					"over": "${workflow.tiles[*]}",
					"as": "tile",
					"id": "${fanout.tile}"
				},
				"work": {
					"type": "write_demo_output",
					"output_prefix": "tile",
					"output_extension": ".txt"
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

	items, err := CompileWorkflow(variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{}), workflow)
	if err != nil {
		t.Fatalf("CompileWorkflow() error = %v", err)
	}
	if items[0].ID != "inspect-h18v07" {
		t.Fatalf("item id = %q, want inspect-h18v07", items[0].ID)
	}
}

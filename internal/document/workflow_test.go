package document

import (
	"errors"
	"strings"
	"testing"

	"goetl/internal/variable"
)

func TestDecodeCanonicalWorkflowSourceJSON(t *testing.T) {
	doc, err := DecodeCanonicalWorkflowSource([]byte(`{
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
					"id": "${fanout}",
					"output": "year-${fanout}"
				},
				"work": {
					"type": "write_demo_output",
					"output_prefix": "demo",
					"output_extension": ".txt"
				}
			}
		],
		"source_manifest": {
			"files": []
		}
	}`), DecodeOptions{Path: "workflow.json"})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
	if doc.ID != "canonical-demo" {
		t.Fatalf("workflow id = %q", doc.ID)
	}
	if len(doc.Variables) != 1 || doc.Variables[0].Name != (variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"}) {
		t.Fatalf("variables = %+v", doc.Variables)
	}
	if len(doc.Steps) != 1 || doc.Steps[0].FanOut.As != "year" || doc.Steps[0].Work.Type != "write_demo_output" {
		t.Fatalf("steps = %+v", doc.Steps)
	}
	if doc.Steps[0].FanOut.Output != "year-${fanout}" {
		t.Fatalf("fan_out.output = %q", doc.Steps[0].FanOut.Output)
	}
}

func TestDecodeCanonicalWorkflowSourceYAML(t *testing.T) {
	doc, err := DecodeCanonicalWorkflowSource([]byte(`
api_version: goet/v1alpha1
kind: Workflow
id: canonical-demo
variables:
  years:
    - 2024
steps:
  - id: download
    fan_out:
      over: ${workflow.years[*]}
      as: year
      id: ${fanout}
    work:
      type: write_demo_output
      output_prefix: demo
      output_extension: .txt
`), DecodeOptions{Path: "workflow.yaml"})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
	if doc.Steps[0].FanOut.Over != "${workflow.years[*]}" {
		t.Fatalf("fan_out.over = %q", doc.Steps[0].FanOut.Over)
	}
	if doc.Steps[0].FanOut.Output != "" {
		t.Fatalf("fan_out.output = %q, want omitted value", doc.Steps[0].FanOut.Output)
	}
}

func TestDecodeCanonicalWorkflowRejectsInvalidFanOutAlias(t *testing.T) {
	tests := []string{
		"",
		" ",
		"2024year",
		"year-tile",
		"fanout",
		"workflow",
		"override",
		"step",
		"asset",
		"data",
		"work_item",
		"runtime",
	}

	for _, alias := range tests {
		t.Run(alias, func(t *testing.T) {
			source := strings.ReplaceAll(`{
				"api_version": "goet/v1alpha1",
				"kind": "Workflow",
				"id": "invalid-alias",
				"variables": {
					"years": [2024]
				},
				"steps": [
					{
						"id": "download",
						"fan_out": {
							"over": "${workflow.years[*]}",
							"as": "__ALIAS__",
							"id": "${fanout}"
						},
						"work": {
							"type": "write_demo_output"
						}
					}
				]
			}`, "__ALIAS__", alias)
			_, err := DecodeCanonicalWorkflowSource([]byte(source), DecodeOptions{Path: "workflow.json"})
			if err == nil {
				t.Fatal("DecodeCanonicalWorkflowSource() expected error")
			}
			if !strings.Contains(err.Error(), "workflow document steps[0].fan_out.as must be a valid non-reserved identifier") {
				t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
			}
		})
	}
}

func TestDecodeCanonicalWorkflowRejectsUnknownFanOutField(t *testing.T) {
	_, err := DecodeCanonicalWorkflowSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "unknown-fanout",
		"variables": {
			"years": [2024]
		},
		"steps": [
			{
				"id": "download",
				"fan_out": {
					"over": "${workflow.years[*]}",
					"as": "year",
					"id": "${fanout}",
					"token": "${fanout}"
				},
				"work": {
					"type": "write_demo_output"
				}
			}
		]
	}`), DecodeOptions{Path: "workflow.json"})
	if err == nil {
		t.Fatal("DecodeCanonicalWorkflowSource() expected error")
	}
	if !strings.Contains(err.Error(), `workflow document steps[0].fan_out has unknown field "token"`) {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
}

func TestDecodeCanonicalWorkflowRejectsMixedGoCasing(t *testing.T) {
	_, err := DecodeCanonicalWorkflowSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "mixed",
		"Steps": []
	}`), DecodeOptions{Path: "workflow.json"})
	if err == nil {
		t.Fatal("DecodeCanonicalWorkflowSource() expected error")
	}
	if errors.Is(err, ErrNotCanonicalWorkflow) {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v, want canonical validation error", err)
	}
}

func TestDecodeCanonicalWorkflowSkipsLegacyWrapper(t *testing.T) {
	_, err := DecodeCanonicalWorkflowSource([]byte(`{
		"workflow": {
			"ID": "legacy",
			"Steps": []
		}
	}`), DecodeOptions{Path: "workflow.json"})
	if !errors.Is(err, ErrNotCanonicalWorkflow) {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v, want ErrNotCanonicalWorkflow", err)
	}
}

func TestDecodeCanonicalWorkflowRejectsLegacyWrapperWithMigrationError(t *testing.T) {
	_, err := DecodeCanonicalWorkflowSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"workflow": {
			"ID": "legacy",
			"Steps": []
		}
	}`), DecodeOptions{Path: "workflow.json"})
	if err == nil {
		t.Fatal("DecodeCanonicalWorkflowSource() expected error")
	}
	if errors.Is(err, ErrNotCanonicalWorkflow) {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v, want migration validation error", err)
	}
	if !strings.Contains(err.Error(), "legacy workflow wrapper is not supported in canonical Workflow documents") {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v, want migration error", err)
	}
}

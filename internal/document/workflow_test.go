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
					"id": "${fanout}"
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

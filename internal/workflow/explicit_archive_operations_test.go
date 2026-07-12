package workflow

import (
	"strings"
	"testing"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCanonicalArchiveExtractStepCompilesPayload(t *testing.T) {
	workflow := workflowFromCanonicalArchiveForTest(t, `{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "archive-work",
		"variables": {"years": [2024]},
		"steps": [
			{
				"id": "extract-aqi",
				"fan_out": {"over": "${workflow.years[*]}", "as": "year", "id": "${fanout}"},
				"work": {
					"type": "archive.extract",
					"resource_constraints": [
						{"resource_key": "target:local/archive-extract", "requested_units": 1, "operator": "<=", "target_units": 1}
					]
				},
				"data": {
					"archive": {
						"extract": {
							"type": "zip",
							"source": {
								"materialized_asset": {"step": "materialize-aqi", "binding": "annual_aqi_zip"}
							},
							"members": [
								{"member": "annual_aqi_by_county_${fanout}.csv", "as": "annual_aqi_by_county_${fanout}.csv", "required": true}
							],
							"output": {"path": "annual_aqi_by_county_${fanout}.csv"}
						}
					}
				}
			}
		]
	}`)

	items, err := CompileWorkflowItems(archiveWorkflowResolverForTest(t, workflow), workflow)
	if err != nil {
		t.Fatalf("CompileWorkflowItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1", len(items))
	}
	item := items[0]
	if item.WorkItem.ID != "extract-aqi-2024" || item.WorkItem.Type != model.WorkItemTypeArchiveExtract {
		t.Fatalf("work item = %+v", item.WorkItem)
	}
	if len(item.WorkItem.DependsOn) != 1 || item.WorkItem.DependsOn[0] != "materialize-aqi-2024" {
		t.Fatalf("depends_on = %+v, want materialize-aqi-2024", item.WorkItem.DependsOn)
	}
	payload := decodeArchiveExtractPayload(t, item.WorkItem)
	if payload.Source.MaterializedAsset == nil ||
		payload.Source.MaterializedAsset.FromWorkItemID != "materialize-aqi-2024" ||
		payload.Source.MaterializedAsset.BindingName != "annual_aqi_zip" {
		t.Fatalf("source = %+v", payload.Source)
	}
	if len(payload.Members) != 1 ||
		payload.Members[0].Member != "annual_aqi_by_county_2024.csv" ||
		payload.Members[0].As != "annual_aqi_by_county_2024.csv" ||
		!payload.Members[0].Required {
		t.Fatalf("members = %+v", payload.Members)
	}
	if payload.OutputPath != "annual_aqi_by_county_2024.csv" {
		t.Fatalf("output_path = %q", payload.OutputPath)
	}
	if len(payload.ResourceConstraints) != 1 || payload.ResourceConstraints[0].ResourceKey != "target:local/archive-extract" {
		t.Fatalf("payload resource constraints = %+v", payload.ResourceConstraints)
	}
}

func TestCanonicalArchiveCreateStepCompilesPayload(t *testing.T) {
	workflow := workflowFromCanonicalArchiveForTest(t, `{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "archive-work",
		"variables": {"years": [2024]},
		"steps": [
			{
				"id": "create-aqi",
				"fan_out": {"over": "${workflow.years[*]}", "as": "year", "id": "${fanout}"},
				"work": {"type": "archive.create"},
				"data": {
					"archive": {
						"create": {
							"type": "zip",
							"entries": [
								{
									"from": {
										"artifact": {"step": "extract-aqi", "name": "annual_aqi_by_county_${fanout}.csv"}
									},
									"as": "annual_aqi_by_county_${fanout}.csv"
								}
							],
							"output": {"path": "annual_aqi_by_county_${fanout}.zip"}
						}
					}
				}
			}
		]
	}`)

	items, err := CompileWorkflowItems(archiveWorkflowResolverForTest(t, workflow), workflow)
	if err != nil {
		t.Fatalf("CompileWorkflowItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1", len(items))
	}
	item := items[0]
	if item.WorkItem.ID != "create-aqi-2024" || item.WorkItem.Type != model.WorkItemTypeArchiveCreate {
		t.Fatalf("work item = %+v", item.WorkItem)
	}
	if len(item.WorkItem.DependsOn) != 1 || item.WorkItem.DependsOn[0] != "extract-aqi-2024" {
		t.Fatalf("depends_on = %+v, want extract-aqi-2024", item.WorkItem.DependsOn)
	}
	payload := decodeArchiveCreatePayload(t, item.WorkItem)
	if len(payload.Entries) != 1 ||
		payload.Entries[0].From.Artifact == nil ||
		payload.Entries[0].From.Artifact.FromWorkItemID != "extract-aqi-2024" ||
		payload.Entries[0].From.Artifact.Name != "annual_aqi_by_county_2024.csv" ||
		payload.Entries[0].As != "annual_aqi_by_county_2024.csv" {
		t.Fatalf("entries = %+v", payload.Entries)
	}
	if payload.OutputPath != "annual_aqi_by_county_2024.zip" {
		t.Fatalf("output_path = %q", payload.OutputPath)
	}
}

func TestCanonicalStandaloneArchiveCreateStepCompiles(t *testing.T) {
	workflow := workflowFromCanonicalArchiveForTest(t, `{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "archive-work",
		"steps": [
			{
				"id": "create-fixture",
				"work": {"type": "archive.create"},
				"data": {
					"archive": {
						"create": {
							"type": "zip",
							"entries": [
								{"from": {"local_path": "fixtures/report.csv"}, "as": "report.csv"}
							],
							"output": {"path": "reports.zip"}
						}
					}
				}
			}
		]
	}`)

	items, err := CompileWorkflowItems(archiveWorkflowResolverForTest(t, workflow), workflow)
	if err != nil {
		t.Fatalf("CompileWorkflowItems() error = %v", err)
	}
	if len(items) != 1 || items[0].WorkItem.ID != "create-fixture" {
		t.Fatalf("items = %+v", items)
	}
	payload := decodeArchiveCreatePayload(t, items[0].WorkItem)
	if payload.Entries[0].From.LocalPath != "fixtures/report.csv" || payload.Entries[0].As != "report.csv" {
		t.Fatalf("entries = %+v", payload.Entries)
	}
}

func TestCanonicalArchiveOperationsRejectMismatchedDataAndWorkType(t *testing.T) {
	tests := []struct {
		name        string
		workType    string
		dataBody    string
		wantMessage string
	}{
		{
			name:        "extract work without data",
			workType:    "archive.extract",
			dataBody:    ``,
			wantMessage: "archive.extract step requires data.archive.extract",
		},
		{
			name:        "create work without data",
			workType:    "archive.create",
			dataBody:    ``,
			wantMessage: "archive.create step requires data.archive.create",
		},
		{
			name:        "extract data with create work",
			workType:    "archive.create",
			dataBody:    `,"data":{"archive":{"extract":{"type":"zip","source":{"local_path":"fixtures/source.zip"},"members":[{"member":"a.csv","as":"a.csv"}],"output":{"path":"a.csv"}}}}`,
			wantMessage: `data.archive.extract requires work.type "archive.extract"`,
		},
		{
			name:        "create data with extract work",
			workType:    "archive.extract",
			dataBody:    `,"data":{"archive":{"create":{"type":"zip","entries":[{"from":{"local_path":"fixtures/a.csv"},"as":"a.csv"}],"output":{"path":"a.zip"}}}}`,
			wantMessage: `data.archive.create requires work.type "archive.create"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := `{
				"api_version": "goet/v1alpha1",
				"kind": "Workflow",
				"id": "archive-work",
				"steps": [{"id": "archive-step", "work": {"type": "` + test.workType + `"} ` + test.dataBody + `}]
			}`
			doc, err := document.DecodeCanonicalWorkflowSource([]byte(source), document.DecodeOptions{Format: document.SourceFormatJSON})
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

func workflowFromCanonicalArchiveForTest(t *testing.T, source string) Workflow {
	t.Helper()
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(source), document.DecodeOptions{Format: document.SourceFormatJSON})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
	workflow, err := WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v", err)
	}
	return workflow
}

func archiveWorkflowResolverForTest(t *testing.T, workflow Workflow) variable.Resolver {
	t.Helper()
	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{FunctionRegistry: variable.DefaultFunctionRegistry()})
}

func decodeArchiveExtractPayload(t *testing.T, item model.WorkItem) model.ArchiveExtractWorkItemPayload {
	t.Helper()
	parameter, ok := item.Parameters["archive_extract"]
	if !ok {
		t.Fatalf("archive_extract parameter missing: %+v", item.Parameters)
	}
	payload, ok := parameter.Value.(model.ArchiveExtractWorkItemPayload)
	if !ok {
		t.Fatalf("archive_extract parameter type = %T", parameter.Value)
	}
	return payload
}

func decodeArchiveCreatePayload(t *testing.T, item model.WorkItem) model.ArchiveCreateWorkItemPayload {
	t.Helper()
	parameter, ok := item.Parameters["archive_create"]
	if !ok {
		t.Fatalf("archive_create parameter missing: %+v", item.Parameters)
	}
	payload, ok := parameter.Value.(model.ArchiveCreateWorkItemPayload)
	if !ok {
		t.Fatalf("archive_create parameter type = %T", parameter.Value)
	}
	return payload
}

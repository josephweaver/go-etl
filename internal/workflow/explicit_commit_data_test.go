package workflow

import (
	"strings"
	"testing"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCanonicalExplicitCommitDataStepCompilesVisibleCommitWork(t *testing.T) {
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "publish-archive",
		"variables": {
			"years": ["2024"]
		},
		"data": {
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
				"id": "build-report",
				"fan_out": {
					"over": "${workflow.years[*]}",
					"as": "year",
					"id": "${fanout}"
				},
				"work": {
					"type": "python_script",
					"parameters": {
						"python_entrypoint": "scripts/build.py"
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
	}`), document.DecodeOptions{Format: document.SourceFormatJSON})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
	workflow, err := WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v", err)
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}
	resolver := explicitCommitResolverForTest(t, workflow)

	result, err := CompileWorkflowStage(resolver, workflow, plan, 1)
	if err != nil {
		t.Fatalf("CompileWorkflowStage() error = %v", err)
	}
	if len(result.WorkItems) != 1 {
		t.Fatalf("work item count = %d, want 1", len(result.WorkItems))
	}
	commit := result.WorkItems[0]
	if commit.WorkItem.ID != "publish-report-2024" || commit.WorkItem.Type != model.WorkItemTypeCommitData {
		t.Fatalf("commit item = %+v", commit.WorkItem)
	}
	if len(commit.WorkItem.DependsOn) != 1 || commit.WorkItem.DependsOn[0] != "build-report-2024" {
		t.Fatalf("depends_on = %+v, want build-report-2024", commit.WorkItem.DependsOn)
	}
	payload := decodeCommitDataPayload(t, commit)
	if payload.Source.FromWorkItemID != "build-report-2024" || payload.Source.FromArtifact != "report_archive" {
		t.Fatalf("source = %+v", payload.Source)
	}
	if payload.PublishTarget.Location.LocationName != "published_data" || payload.PublishTarget.Location.Path != "reports/2024/report.zip" {
		t.Fatalf("publish location = %+v", payload.PublishTarget.Location)
	}
	if len(commit.ResourceConstraints) != 1 || commit.ResourceConstraints[0].ResourceKey != "target:target-local/published-data-write:published_data" {
		t.Fatalf("resource constraints = %+v", commit.ResourceConstraints)
	}
}

func TestCanonicalExplicitCommitDataStepRejectsMissingTarget(t *testing.T) {
	_, err := workflowFromCanonicalExplicitCommitForTest(t, `"from":{"step":"build-report","artifact":"report_archive"}`)
	if err == nil || !strings.Contains(err.Error(), "data.outputs.report_archive target is required") {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v, want missing target", err)
	}
}

func TestCanonicalExplicitCommitDataStepRejectsMissingArtifact(t *testing.T) {
	_, err := workflowFromCanonicalExplicitCommitForTest(t, `"from":{"step":"build-report"},"target":"report_archive"`)
	if err == nil || !strings.Contains(err.Error(), "data.outputs.report_archive.from artifact is required") {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v, want missing artifact", err)
	}
}

func TestCanonicalComputeStepRejectsImplicitPublishParameter(t *testing.T) {
	_, err := workflowFromCanonicalComputeParametersForTest(t, `"publish": []`)
	if err == nil || !strings.Contains(err.Error(), `canonical work parameter "publish" is not allowed`) {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v, want publish parameter rejection", err)
	}
}

func TestCanonicalComputeStepRejectsImplicitDataAssetsParameter(t *testing.T) {
	_, err := workflowFromCanonicalComputeParametersForTest(t, `"data_assets": []`)
	if err == nil || !strings.Contains(err.Error(), `canonical work parameter "data_assets" is not allowed`) {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v, want data_assets parameter rejection", err)
	}
}

func workflowFromCanonicalExplicitCommitForTest(t *testing.T, outputBody string) (Workflow, error) {
	t.Helper()
	source := `{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "publish-archive",
		"variables": {"years": ["2024"]},
		"data": {
			"outputs": {
				"report_archive": {
					"kind": "archive",
					"binding": {
						"provider": "registered_location",
						"location": {"name": "published_data", "path": "reports/report.zip"}
					}
				}
			}
		},
		"steps": [{
			"id": "publish-report",
			"fan_out": {"over": "${workflow.years[*]}", "as": "year", "id": "${fanout}"},
			"data": {"outputs": {"report_archive": {` + outputBody + `}}},
			"work": {"type": "commit_data"}
		}]
	}`
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(source), document.DecodeOptions{Format: document.SourceFormatJSON})
	if err != nil {
		return Workflow{}, err
	}
	return WorkflowFromCanonicalDocument(doc)
}

func workflowFromCanonicalComputeParametersForTest(t *testing.T, parameterBody string) (Workflow, error) {
	t.Helper()
	source := `{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "compute",
		"variables": {"years": ["2024"]},
		"steps": [{
			"id": "build-report",
			"fan_out": {"over": "${workflow.years[*]}", "as": "year", "id": "${fanout}"},
			"work": {
				"type": "python_script",
				"parameters": {` + parameterBody + `}
			}
		}]
	}`
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(source), document.DecodeOptions{Format: document.SourceFormatJSON})
	if err != nil {
		return Workflow{}, err
	}
	return WorkflowFromCanonicalDocument(doc)
}

func explicitCommitResolverForTest(t *testing.T, workflow Workflow) variable.Resolver {
	t.Helper()
	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
}

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goetl/internal/model"
	"goetl/internal/persistence"
)

func TestSubmitWorkflowRunToStoreAttachesPythonWorkItemSource(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()

	controller := newController()
	controller.workflowStore = store
	root := setupLocalWorkflowSource(t, controller)
	writeLocalPythonSourceFiles(t, root)
	writeLocalPythonWorkflowSource(t, root, pythonWorkflowFixture{})

	if _, err := controller.submitWorkflowRunToStore(context.Background(), localWorkflowRunSubmission(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("submitWorkflowRunToStore() error = %v", err)
	}

	runs, err := store.ListActiveWorkflowRuns(context.Background())
	if err != nil {
		t.Fatalf("ListActiveWorkflowRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("active run count = %d, want 1", len(runs))
	}

	var submissionContext workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(runs[0].SubmissionContextJSON), &submissionContext); err != nil {
		t.Fatalf("decode submission context: %v", err)
	}

	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("queued work count = %d, want 1", len(queued))
	}

	var item model.WorkItem
	if err := json.Unmarshal([]byte(queued[0].WorkerPayloadJSON), &item); err != nil {
		t.Fatalf("decode queued work item: %v", err)
	}
	if item.Type != model.WorkItemTypePythonScript {
		t.Fatalf("work item type = %q, want python_script", item.Type)
	}
	if item.Source == nil {
		t.Fatal("work item source = nil, want controller-generated source")
	}
	if item.Source.Schema != workItemSourceSchemaV1 {
		t.Fatalf("source schema = %q, want %q", item.Source.Schema, workItemSourceSchemaV1)
	}
	if item.Source.RunID != runs[0].ID {
		t.Fatalf("source run id = %q, want %q", item.Source.RunID, runs[0].ID)
	}
	if item.Source.ManifestPath != submissionContext.SourceAdmission.ManifestRef {
		t.Fatalf("source manifest path = %q, want %q", item.Source.ManifestPath, submissionContext.SourceAdmission.ManifestRef)
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("strict work item validation failed after source attachment: %v", err)
	}
}

func TestSubmitWorkflowRunToStoreRejectsInvalidPythonSourceAdmission(t *testing.T) {
	tests := []struct {
		name           string
		fixture        pythonWorkflowFixture
		wantErrContain string
	}{
		{
			name: "missing python_entrypoint",
			fixture: pythonWorkflowFixture{
				omitEntrypointParameter: true,
			},
			wantErrContain: "parameter python_entrypoint is required",
		},
		{
			name: "python_entrypoint unsupported type",
			fixture: pythonWorkflowFixture{
				entrypointParameterJSON: `{"type":"int","value":1}`,
			},
			wantErrContain: "parameter python_entrypoint has type int, want string or path",
		},
		{
			name: "undeclared python_entrypoint path",
			fixture: pythonWorkflowFixture{
				entrypointParameterJSON: `{"type":"path","value":"scripts/other.py"}`,
			},
			wantErrContain: "parameter python_entrypoint path scripts/other.py is not declared",
		},
		{
			name: "wrong-role python_entrypoint path",
			fixture: pythonWorkflowFixture{
				manifestFilesJSON: `{"role":"support_file","path":"scripts/run.py","content_type":"text/x-python"}`,
			},
			wantErrContain: "parameter python_entrypoint path scripts/run.py has role support_file, want python_entrypoint",
		},
		{
			name: "python_environment unsupported type",
			fixture: pythonWorkflowFixture{
				includeEnvironmentParameter: true,
				environmentParameterJSON:    `{"type":"int","value":1}`,
			},
			wantErrContain: "parameter python_environment has type int, want string or path",
		},
		{
			name: "undeclared python_environment path",
			fixture: pythonWorkflowFixture{
				includeEnvironmentParameter: true,
				environmentParameterJSON:    `{"type":"path","value":"environments/other.json"}`,
			},
			wantErrContain: "parameter python_environment path environments/other.json is not declared",
		},
		{
			name: "wrong-role python_environment path",
			fixture: pythonWorkflowFixture{
				includeEnvironmentParameter: true,
				manifestFilesJSON: `{"role":"python_entrypoint","path":"scripts/run.py","content_type":"text/x-python"},
				{"role":"support_file","path":"environments/default.json","content_type":"application/json"}`,
			},
			wantErrContain: "parameter python_environment path environments/default.json has role support_file, want python_environment",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openTestWorkflowExecutionStore(t)
			defer store.Close()

			controller := newController()
			controller.workflowStore = store
			root := setupLocalWorkflowSource(t, controller)
			writeLocalPythonSourceFiles(t, root)
			writeLocalPythonWorkflowSource(t, root, test.fixture)

			_, err := controller.submitWorkflowRunToStore(context.Background(), localWorkflowRunSubmission(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC))
			if err == nil || !strings.Contains(err.Error(), test.wantErrContain) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErrContain)
			}

			assertNoStoredWorkflowRun(t, store)
		})
	}
}

func TestSubmitWorkflowRunToStoreReplacesUserSuppliedPythonSourceLocator(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()

	controller := newController()
	controller.workflowStore = store
	root := setupLocalWorkflowSource(t, controller)
	writeLocalPythonSourceFiles(t, root)
	writeLocalPythonWorkflowSource(t, root, pythonWorkflowFixture{
		extraWorkItemFields: `"Source":{"schema":"evil/schema","run_id":"evil-run","manifest_path":"evil-manifest"},`,
	})

	if _, err := controller.submitWorkflowRunToStore(context.Background(), localWorkflowRunSubmission(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("submitWorkflowRunToStore() error = %v", err)
	}

	runs, err := store.ListActiveWorkflowRuns(context.Background())
	if err != nil {
		t.Fatalf("ListActiveWorkflowRuns() error = %v", err)
	}
	var submissionContext workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(runs[0].SubmissionContextJSON), &submissionContext); err != nil {
		t.Fatalf("decode submission context: %v", err)
	}

	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	var item model.WorkItem
	if err := json.Unmarshal([]byte(queued[0].WorkerPayloadJSON), &item); err != nil {
		t.Fatalf("decode queued work item: %v", err)
	}

	if item.Source == nil {
		t.Fatal("work item source = nil, want controller-generated source")
	}
	if item.Source.Schema == "evil/schema" || item.Source.RunID == "evil-run" || item.Source.ManifestPath == "evil-manifest" {
		t.Fatalf("user-authored source locator leaked into queued payload: %+v", item.Source)
	}
	if item.Source.RunID != runs[0].ID || item.Source.ManifestPath != submissionContext.SourceAdmission.ManifestRef {
		t.Fatalf("source locator = %+v, want controller-generated run/manifest", item.Source)
	}
}

func TestSubmitWorkflowRunToStoreLeavesNonPythonWorkItemsUnchanged(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()

	controller := newController()
	controller.workflowStore = store
	root := setupLocalWorkflowSource(t, controller)
	writeLocalWorkflowSource(t, root, []int{2024}, "")

	if _, err := controller.submitWorkflowRunToStore(context.Background(), localWorkflowRunSubmission(), time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("submitWorkflowRunToStore() error = %v", err)
	}

	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("queued work count = %d, want 1", len(queued))
	}

	var item model.WorkItem
	if err := json.Unmarshal([]byte(queued[0].WorkerPayloadJSON), &item); err != nil {
		t.Fatalf("decode queued work item: %v", err)
	}
	if item.Type != model.WorkItemTypeWriteDemoOutput {
		t.Fatalf("work item type = %q, want write_demo_output", item.Type)
	}
	if item.Source != nil {
		t.Fatalf("non-python work item source = %+v, want nil", item.Source)
	}
}

type pythonWorkflowFixture struct {
	omitEntrypointParameter     bool
	entrypointParameterJSON     string
	includeEnvironmentParameter bool
	environmentParameterJSON    string
	manifestFilesJSON           string
	extraWorkItemFields         string
}

func writeLocalPythonSourceFiles(t *testing.T, root string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "environments"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("print('ok')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "environments", "default.json"), []byte(`{"name":"default"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeLocalPythonWorkflowSource(t *testing.T, root string, fixture pythonWorkflowFixture) {
	t.Helper()

	entrypointJSON := fixture.entrypointParameterJSON
	if entrypointJSON == "" {
		entrypointJSON = `{"type":"path","value":"scripts/run.py"}`
	}
	environmentJSON := fixture.environmentParameterJSON
	if environmentJSON == "" {
		environmentJSON = `{"type":"path","value":"environments/default.json"}`
	}
	manifestFilesJSON := fixture.manifestFilesJSON
	if manifestFilesJSON == "" {
		manifestFilesJSON = `{"role":"python_entrypoint","path":"scripts/run.py","content_type":"text/x-python"},
				{"role":"python_environment","path":"environments/default.json","content_type":"application/json"}`
	}

	parameters := make([]string, 0, 2)
	if !fixture.omitEntrypointParameter {
		parameters = append(parameters, `"python_entrypoint": `+entrypointJSON)
	}
	if fixture.includeEnvironmentParameter {
		parameters = append(parameters, `"python_environment": `+environmentJSON)
	}

	workflowJSON := `{
		"workflow": {
			"ID": "python-demo",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}]
				}
			],
			"Steps": [
				{
					"ID": "python-step",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "python_script",
							` + fixture.extraWorkItemFields + `
							"OutputPrefix": "python",
							"OutputExtension": ".json",
							"Parameters": {
								` + strings.Join(parameters, ",") + `
							}
						}
					}
				}
			]
		},
		"source_manifest": {
			"files": [
				` + manifestFilesJSON + `
			]
		},
		"variables": []
	}`

	if err := os.WriteFile(filepath.Join(root, "workflows", "demo-workflow.json"), []byte(workflowJSON), 0o600); err != nil {
		t.Fatal(err)
	}
}

func localWorkflowRunSubmission() WorkflowRunSubmission {
	return WorkflowRunSubmission{
		Project: SourceDocumentReference{
			Repository: "local:test",
			Ref:        "main",
			Path:       "project.json",
		},
		Workflow: SourceDocumentReference{
			Repository: "local:test",
			Ref:        "main",
			Path:       "workflows/demo-workflow.json",
		},
	}
}

func assertNoStoredWorkflowRun(t *testing.T, store *persistence.Store) {
	t.Helper()

	runs, err := store.ListActiveWorkflowRuns(context.Background())
	if err != nil {
		t.Fatalf("ListActiveWorkflowRuns() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("active run count = %d, want 0", len(runs))
	}

	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("queued work count = %d, want 0", len(queued))
	}
}

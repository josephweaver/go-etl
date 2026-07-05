package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/variable"
)

func TestLoadCLIInputsWithControllerFile(t *testing.T) {
	dir := t.TempDir()
	controllerPath := writeTestFile(t, dir, "controller.json", `{"kind":"Controller"}`)
	projectPath := writeTestFile(t, dir, "project.json", `{
		"id": "go-etl-demo",
		"name": "GO ETL Demo Project",
		"year": 2026,
		"enabled": true,
		"labels": ["cdl", 2026, true],
		"metadata": {"state": "MI"}
	}`)
	workflowPath := writeTestFile(t, dir, "workflow.json", `{
		"workflow": {
			"ID": "cdl-demo",
			"Variables": [
				{"name":{"namespace":"workflow","key":"years"},"type":"list","expression":[{"type":"int","expression":2026}]}
			],
			"Steps": []
		},
		"source_manifest": {},
		"variables": [
			{"name":{"namespace":"override","key":"code_version"},"type":"string","expression":"test"}
		]
	}`)

	inputs, err := LoadCLIInputs(CLIInputPaths{
		ControllerPath: controllerPath,
		ProjectPath:    projectPath,
		WorkflowPath:   workflowPath,
	})
	if err != nil {
		t.Fatalf("LoadCLIInputs() error = %v", err)
	}

	if inputs.ControllerPath != controllerPath {
		t.Fatalf("ControllerPath = %q, want %q", inputs.ControllerPath, controllerPath)
	}
	if inputs.ControllerURL != defaultCLIControllerURL {
		t.Fatalf("ControllerURL = %q, want %q", inputs.ControllerURL, defaultCLIControllerURL)
	}
	if inputs.Starter == nil {
		t.Fatal("Starter = nil, want local starter")
	}

	gotURL, err := inputs.Resolver.String("controller_config.controller_url")
	if err != nil {
		t.Fatalf("resolve controller URL: %v", err)
	}
	if gotURL != defaultCLIControllerURL {
		t.Fatalf("controller URL = %q, want %q", gotURL, defaultCLIControllerURL)
	}

	args, err := inputs.Resolver.StringList("controller_config.controller_start_args")
	if err != nil {
		t.Fatalf("resolve controller start args: %v", err)
	}
	wantArgs := []string{"run", "./cmd/controller", "--config", controllerPath}
	if strings.Join(args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("controller start args = %#v, want %#v", args, wantArgs)
	}

	projectID, err := inputs.Resolver.String("project_config.id")
	if err != nil {
		t.Fatalf("resolve project id: %v", err)
	}
	if projectID != "go-etl-demo" {
		t.Fatalf("project_config.id = %q, want go-etl-demo", projectID)
	}

	metadata, err := inputs.Resolver.Object("project_config.metadata")
	if err != nil {
		t.Fatalf("resolve project metadata: %v", err)
	}
	if metadata["state"].Type != variable.TypeString || metadata["state"].Value != "MI" {
		t.Fatalf("project_config.metadata.state = %#v, want string MI", metadata["state"])
	}

	if inputs.Submission.Workflow.ID != "cdl-demo" {
		t.Fatalf("workflow ID = %q, want cdl-demo", inputs.Submission.Workflow.ID)
	}
	if len(inputs.Submission.Workflow.Variables) != 1 {
		t.Fatalf("workflow variable count = %d, want 1", len(inputs.Submission.Workflow.Variables))
	}
	if len(inputs.Submission.Variables) != 7 {
		t.Fatalf("top-level submission variable count = %d, want 7", len(inputs.Submission.Variables))
	}
}

func TestLoadCLIInputsWithControllerURLDoesNotCreateStarter(t *testing.T) {
	dir := t.TempDir()
	projectPath := writeTestFile(t, dir, "project.json", `{"id": "go-etl-demo"}`)
	workflowPath := writeTestFile(t, dir, "workflow.json", `{"workflow":{"ID":"cdl-demo","Steps":[]},"source_manifest":{},"variables":[]}`)

	inputs, err := LoadCLIInputs(CLIInputPaths{
		ControllerURL: "http://controller:8080",
		ProjectPath:   projectPath,
		WorkflowPath:  workflowPath,
	})
	if err != nil {
		t.Fatalf("LoadCLIInputs() error = %v", err)
	}
	if inputs.Starter != nil {
		t.Fatal("Starter is set, want nil for --controller-url")
	}

	gotURL, err := inputs.Resolver.String("controller_config.controller_url")
	if err != nil {
		t.Fatalf("resolve controller URL: %v", err)
	}
	if gotURL != "http://controller:8080" {
		t.Fatalf("controller URL = %q, want explicit URL", gotURL)
	}
}

func TestLoadCLIInputsErrorsIdentifyInputFiles(t *testing.T) {
	dir := t.TempDir()
	validControllerPath := writeTestFile(t, dir, "controller.json", `{}`)
	validProjectPath := writeTestFile(t, dir, "project.json", `{"id": "go-etl-demo"}`)
	validWorkflowPath := writeTestFile(t, dir, "workflow.json", `{"workflow":{"ID":"cdl-demo","Steps":[]},"source_manifest":{},"variables":[]}`)

	tests := []struct {
		name    string
		paths   CLIInputPaths
		wantErr string
	}{
		{
			name: "invalid controller",
			paths: CLIInputPaths{
				ControllerPath: writeTestFile(t, dir, "bad-controller.json", `{`),
				ProjectPath:    validProjectPath,
				WorkflowPath:   validWorkflowPath,
			},
			wantErr: "bad-controller.json",
		},
		{
			name: "invalid project",
			paths: CLIInputPaths{
				ControllerPath: validControllerPath,
				ProjectPath:    writeTestFile(t, dir, "bad-project.json", `{`),
				WorkflowPath:   validWorkflowPath,
			},
			wantErr: "bad-project.json",
		},
		{
			name: "invalid workflow",
			paths: CLIInputPaths{
				ControllerPath: validControllerPath,
				ProjectPath:    validProjectPath,
				WorkflowPath:   writeTestFile(t, dir, "bad-workflow.json", `{`),
			},
			wantErr: "bad-workflow.json",
		},
		{
			name: "unsupported project null",
			paths: CLIInputPaths{
				ControllerPath: validControllerPath,
				ProjectPath:    writeTestFile(t, dir, "null-project.json", `{"id": null}`),
				WorkflowPath:   validWorkflowPath,
			},
			wantErr: "null is not supported",
		},
		{
			name: "unsupported project decimal",
			paths: CLIInputPaths{
				ControllerPath: validControllerPath,
				ProjectPath:    writeTestFile(t, dir, "decimal-project.json", `{"ratio": 1.25}`),
				WorkflowPath:   validWorkflowPath,
			},
			wantErr: "not a supported integer",
		},
		{
			name: "missing project",
			paths: CLIInputPaths{
				ControllerPath: validControllerPath,
				ProjectPath:    filepath.Join(dir, "missing-project.json"),
				WorkflowPath:   validWorkflowPath,
			},
			wantErr: "missing-project.json",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadCLIInputs(test.paths)
			if err == nil {
				t.Fatal("LoadCLIInputs() error = nil, want error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadCLIInputs() error = %q, want substring %q", err.Error(), test.wantErr)
			}
		})
	}
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
	return path
}

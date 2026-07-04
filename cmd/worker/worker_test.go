package main

import (
	"os"
	"path/filepath"
	"testing"

	"goetl/internal/model"
)

func TestRequireDir(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "file.txt")

	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "existing directory", path: root},
		{name: "missing path", path: filepath.Join(root, "missing"), wantErr: true},
		{name: "regular file", path: filePath, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := requireDir(test.path)

			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWorkerRunWorkItemRejectsInvalidItem(t *testing.T) {
	worker := newTestWorker(t)

	item := model.WorkItem{
		ID:             "test-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "../outside.txt",
	}

	if _, err := worker.runWorkItem(item); err == nil {
		t.Fatal("expected an error")
	}
}

func newTestWorker(t *testing.T) Worker {
	t.Helper()

	root := t.TempDir()

	config := Config{
		LogDir:        filepath.Join(root, "logs"),
		TmpDir:        filepath.Join(root, "tmp"),
		DataDir:       filepath.Join(root, "data"),
		ControllerURL: "https://controller.local",
	}

	for _, dir := range []string{config.LogDir, config.TmpDir, config.DataDir} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("create directory %s: %v", dir, err)
		}
	}

	return Worker{Config: config}
}

func TestWorkerRun(t *testing.T) {
	worker := newTestWorker(t)

	item := model.WorkItem{
		ID:             "test-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
	}

	evidence, err := worker.Run(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evidence.OutputJSON == "" || evidence.PreStateJSON == "" || evidence.PostStateJSON == "" {
		t.Fatalf("expected run evidence: %+v", evidence)
	}

	dataPath := filepath.Join(worker.Config.DataDir, item.OutputFilename)
	if _, err := os.Stat(dataPath); err != nil {
		t.Fatalf("completed output does not exist: %v", err)
	}
}

func TestWorkerRunSummarizeInputFile(t *testing.T) {
	worker := newTestWorker(t)
	inputPath := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(inputPath, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	item := model.WorkItem{
		ID:             "summary-001",
		Type:           model.WorkItemTypeSummarizeInputFile,
		OutputFilename: "summary.txt",
		Parameters: model.Parameters{
			"input_path": {Type: "path", Value: inputPath},
		},
	}

	evidence, err := worker.Run(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evidence.OutputJSON == "" || evidence.PreStateJSON == "" || evidence.PostStateJSON == "" {
		t.Fatalf("expected summary evidence: %+v", evidence)
	}

	dataPath := filepath.Join(worker.Config.DataDir, item.OutputFilename)
	if _, err := os.Stat(dataPath); err != nil {
		t.Fatalf("completed output does not exist: %v", err)
	}
}

func TestWorkerRunWorkItemRejectsUnsupportedType(t *testing.T) {
	worker := newTestWorker(t)

	item := model.WorkItem{
		ID:             "test-001",
		Type:           "unknown",
		OutputFilename: "result.txt",
	}

	if _, err := worker.runWorkItem(item); err == nil {
		t.Fatal("expected an error")
	}
}

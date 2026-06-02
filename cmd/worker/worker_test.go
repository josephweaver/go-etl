package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestWorkerRunWorkItem(t *testing.T) {
	worker := newTestWorker(t)

	item := WorkItem{
		ID:             "test-001",
		Type:           WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
	}

	if err := worker.runWorkItem(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tmpPath := filepath.Join(worker.Config.TmpDir, item.OutputFilename)
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("temporary output still exists: %s", tmpPath)
	}

	dataPath := filepath.Join(worker.Config.DataDir, item.OutputFilename)
	output, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("read completed output: %v", err)
	}

	if !strings.Contains(string(output), item.ID) {
		t.Fatalf("output does not contain work item id: %q", output)
	}

	logPath := filepath.Join(worker.Config.LogDir, "worker.log")
	logOutput, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	if !strings.Contains(string(logOutput), "starting work item: "+item.ID) {
		t.Fatalf("log does not contain start message: %q", logOutput)
	}

	if !strings.Contains(string(logOutput), "completed work item: "+item.ID) {
		t.Fatalf("log does not contain completion message: %q", logOutput)
	}
}

func TestWorkerRunWorkItemRejectsInvalidItem(t *testing.T) {
	worker := newTestWorker(t)

	item := WorkItem{
		ID:             "test-001",
		Type:           WorkItemTypeWriteDemoOutput,
		OutputFilename: "../outside.txt",
	}

	if err := worker.runWorkItem(item); err == nil {
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

	item := WorkItem{
		ID:             "test-001",
		Type:           WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
	}

	if err := worker.Run(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dataPath := filepath.Join(worker.Config.DataDir, item.OutputFilename)
	if _, err := os.Stat(dataPath); err != nil {
		t.Fatalf("completed output does not exist: %v", err)
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerWriteDemoOutput(t *testing.T) {
	worker := newTestWorker(t)

	item := WorkItem{
		ID:             "test-001",
		Type:           WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
	}

	if err := worker.writeDemoOutput(item); err != nil {
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

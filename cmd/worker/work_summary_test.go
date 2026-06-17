package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestWorkerSummarizeInputFile(t *testing.T) {
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

	if err := worker.summarizeInputFile(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, err := os.ReadFile(filepath.Join(worker.Config.DataDir, item.OutputFilename))
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}

	if !strings.Contains(string(output), "input_path="+inputPath) {
		t.Fatalf("summary missing input path: %q", output)
	}

	if !strings.Contains(string(output), "size_bytes=6") {
		t.Fatalf("summary missing size: %q", output)
	}
}

func TestWorkerSummarizeInputFileRequiresInputPath(t *testing.T) {
	worker := newTestWorker(t)
	item := model.WorkItem{
		ID:             "summary-001",
		Type:           model.WorkItemTypeSummarizeInputFile,
		OutputFilename: "summary.txt",
	}

	if err := worker.summarizeInputFile(item); err == nil {
		t.Fatal("expected an error")
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkItem(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "item.json")

	content := []byte(`{
		"id": "test-001",
		"type": "write_demo_output",
		"output_filename": "result.txt"
	}`)

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	item, err := loadWorkItem(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if item.ID != "test-001" {
		t.Fatalf("unexpected id: %q", item.ID)
	}

	if item.Type != WorkItemTypeWriteDemoOutput {
		t.Fatalf("unexpected type: %q", item.Type)
	}

	if item.OutputFilename != "result.txt" {
		t.Fatalf("unexpected output filename: %q", item.OutputFilename)
	}
}

func TestLoadWorkItemRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	if _, err := loadWorkItem(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoadWorkItemRejectsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "item.json")

	if err := os.WriteFile(path, []byte(`{"id":`), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadWorkItem(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoadWorkItemRejectsInvalidItem(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "item.json")

	content := []byte(`{
		"id": "test-001",
		"type": "write_demo_output",
		"output_filename": "../outside.txt"
	}`)

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadWorkItem(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestWorkItemValidate(t *testing.T) {
	tests := []struct {
		name    string
		item    WorkItem
		wantErr bool
	}{
		{
			name: "valid item",
			item: WorkItem{
				ID:             "local-demo-001",
				Type:           WorkItemTypeWriteDemoOutput,
				OutputFilename: "output.txt",
			},
		},
		{
			name: "missing id",
			item: WorkItem{
				Type:           WorkItemTypeWriteDemoOutput,
				OutputFilename: "output.txt",
			},
			wantErr: true,
		},
		{
			name: "missing type",
			item: WorkItem{
				ID:             "local-demo-001",
				OutputFilename: "output.txt",
			},
			wantErr: true,
		},
		{
			name: "unknown type is structurally valid",
			item: WorkItem{
				ID:             "local-demo-001",
				Type:           "unknown",
				OutputFilename: "output.txt",
			},
		},
		{
			name: "missing output filename",
			item: WorkItem{
				ID:   "local-demo-001",
				Type: WorkItemTypeWriteDemoOutput,
			},
			wantErr: true,
		},
		{
			name: "output filename contains directory",
			item: WorkItem{
				ID:             "local-demo-001",
				Type:           WorkItemTypeWriteDemoOutput,
				OutputFilename: "../outside.txt",
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.item.Validate()

			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

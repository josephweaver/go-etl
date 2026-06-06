package model

import (
	"encoding/json"
	"testing"
)

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

func TestWorkCompletionJSONIncludesAttemptMetadata(t *testing.T) {
	completion := WorkCompletion{
		ID:                  "work-item-001",
		AttemptID:           "attempt-001",
		WorkflowInstanceID:  "workflow-instance-001",
		StepInstanceID:      "step-instance-001",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
		StartedAt:           "2026-06-06T12:00:00Z",
		CompletedAt:         "2026-06-06T12:01:00Z",
	}

	data, err := json.Marshal(completion)
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode completion: %v", err)
	}

	if decoded["attempt_id"] != completion.AttemptID {
		t.Fatalf("attempt_id = %q, want %q", decoded["attempt_id"], completion.AttemptID)
	}

	if decoded["work_item_fingerprint"] != completion.WorkItemFingerprint {
		t.Fatalf("work_item_fingerprint = %q, want %q", decoded["work_item_fingerprint"], completion.WorkItemFingerprint)
	}
}

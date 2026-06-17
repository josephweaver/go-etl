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
		{
			name: "valid parameters",
			item: WorkItem{
				ID:             "local-demo-001",
				Type:           WorkItemTypeWriteDemoOutput,
				OutputFilename: "output.txt",
				Parameters: Parameters{
					"input_path": {Type: "path", Value: "/data/input.tif"},
				},
			},
		},
		{
			name: "parameter missing type",
			item: WorkItem{
				ID:             "local-demo-001",
				Type:           WorkItemTypeWriteDemoOutput,
				OutputFilename: "output.txt",
				Parameters: Parameters{
					"input_path": {Value: "/data/input.tif"},
				},
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

func TestWorkItemJSONIncludesRuntimeMetadata(t *testing.T) {
	item := WorkItem{
		ID:                   "work-item-001",
		Type:                 WorkItemTypeWriteDemoOutput,
		OutputFilename:       "output.txt",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		Parameters: Parameters{
			"input_path": {Type: "path", Value: "/data/input.tif"},
		},
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}

	var decodedItem WorkItem
	if err := json.Unmarshal(data, &decodedItem); err != nil {
		t.Fatalf("decode work item: %v", err)
	}

	if decodedItem.WorkflowInstanceID != item.WorkflowInstanceID {
		t.Fatalf("workflow_instance_id = %q, want %q", decodedItem.WorkflowInstanceID, item.WorkflowInstanceID)
	}

	if decodedItem.StepDefinitionID != item.StepDefinitionID {
		t.Fatalf("step_definition_id = %q, want %q", decodedItem.StepDefinitionID, item.StepDefinitionID)
	}

	if decodedItem.StepFingerprint != item.StepFingerprint {
		t.Fatalf("step_fingerprint = %q, want %q", decodedItem.StepFingerprint, item.StepFingerprint)
	}

	if decodedItem.WorkItemFingerprint != item.WorkItemFingerprint {
		t.Fatalf("work_item_fingerprint = %q, want %q", decodedItem.WorkItemFingerprint, item.WorkItemFingerprint)
	}

	if decodedItem.Parameters["input_path"].Value != "/data/input.tif" {
		t.Fatalf("unexpected input_path parameter: %+v", decodedItem.Parameters["input_path"])
	}
}

func TestWorkCompletionJSONIncludesAttemptMetadata(t *testing.T) {
	completion := WorkCompletion{
		ID:                   "work-item-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
		Parameters: Parameters{
			"input_path": {Type: "path", Value: "demo-summary-input.txt"},
		},
	}

	data, err := json.Marshal(completion)
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}

	var decodedCompletion WorkCompletion
	if err := json.Unmarshal(data, &decodedCompletion); err != nil {
		t.Fatalf("decode completion: %v", err)
	}

	if decodedCompletion.AttemptID != completion.AttemptID {
		t.Fatalf("attempt_id = %q, want %q", decodedCompletion.AttemptID, completion.AttemptID)
	}

	if decodedCompletion.WorkflowDefinitionID != completion.WorkflowDefinitionID {
		t.Fatalf("workflow_definition_id = %q, want %q", decodedCompletion.WorkflowDefinitionID, completion.WorkflowDefinitionID)
	}

	if decodedCompletion.WorkflowFingerprint != completion.WorkflowFingerprint {
		t.Fatalf("workflow_fingerprint = %q, want %q", decodedCompletion.WorkflowFingerprint, completion.WorkflowFingerprint)
	}

	if decodedCompletion.WorkItemFingerprint != completion.WorkItemFingerprint {
		t.Fatalf("work_item_fingerprint = %q, want %q", decodedCompletion.WorkItemFingerprint, completion.WorkItemFingerprint)
	}

	if decodedCompletion.Parameters["input_path"].Value != "demo-summary-input.txt" {
		t.Fatalf("unexpected input_path parameter: %+v", decodedCompletion.Parameters["input_path"])
	}
}

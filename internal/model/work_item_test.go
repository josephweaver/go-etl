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
		AttemptID:            "attempt-001",
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
		ReuseCandidates: []WorkReuseCandidate{
			{
				AttemptID:        "attempt-prior",
				InputSHA256:      "input-sha",
				OutputSHA256:     "output-sha",
				PostStateSHA256:  "post-state-sha",
				OutputJSONSHA256: "output-json-sha",
			},
		},
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

	if decodedItem.AttemptID != item.AttemptID {
		t.Fatalf("attempt_id = %q, want %q", decodedItem.AttemptID, item.AttemptID)
	}
	if len(decodedItem.ReuseCandidates) != 1 || decodedItem.ReuseCandidates[0].AttemptID != "attempt-prior" {
		t.Fatalf("reuse_candidates = %+v, want prior attempt", decodedItem.ReuseCandidates)
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
		Skipped:              true,
		SkippedParentID:      "attempt-prior",
		SkipReason:           "matched_worker_observed_state",
		InputSHA256:          "input-sha",
		OutputSHA256:         "output-sha",
		PreStateSHA256:       "pre-state-sha",
		PostStateSHA256:      "post-state-sha",
		OutputJSON:           `{"result":"ok"}`,
		PreStateJSON:         `{"output_exists":false}`,
		PostStateJSON:        `{"output_exists":true}`,
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
	if !decodedCompletion.Skipped || decodedCompletion.SkippedParentID != completion.SkippedParentID {
		t.Fatalf("skip metadata = %+v, want skipped parent %q", decodedCompletion, completion.SkippedParentID)
	}
	if decodedCompletion.InputSHA256 != completion.InputSHA256 || decodedCompletion.OutputSHA256 != completion.OutputSHA256 {
		t.Fatalf("observed hashes = %+v, want input/output hashes", decodedCompletion)
	}

	if decodedCompletion.OutputJSON != completion.OutputJSON {
		t.Fatalf("output_json = %q, want %q", decodedCompletion.OutputJSON, completion.OutputJSON)
	}

	if decodedCompletion.PreStateJSON != completion.PreStateJSON {
		t.Fatalf("pre_state_json = %q, want %q", decodedCompletion.PreStateJSON, completion.PreStateJSON)
	}

	if decodedCompletion.PostStateJSON != completion.PostStateJSON {
		t.Fatalf("post_state_json = %q, want %q", decodedCompletion.PostStateJSON, completion.PostStateJSON)
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

func TestWorkFailureJSONIncludesAttemptID(t *testing.T) {
	failure := WorkFailure{
		ID:        "work-item-001",
		AttemptID: "attempt-001",
		FailedAt:  "2026-07-03T12:00:00Z",
		Error:     "boom",
	}

	data, err := json.Marshal(failure)
	if err != nil {
		t.Fatalf("marshal failure: %v", err)
	}

	var decoded WorkFailure
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode failure: %v", err)
	}

	if decoded.AttemptID != failure.AttemptID {
		t.Fatalf("attempt_id = %q, want %q", decoded.AttemptID, failure.AttemptID)
	}
	if decoded.FailedAt != failure.FailedAt {
		t.Fatalf("failed_at = %q, want %q", decoded.FailedAt, failure.FailedAt)
	}
}

func TestControllerStatusJSONIncludesReuseCandidates(t *testing.T) {
	status := ControllerStatus{
		Pending:                2,
		PendingReuseCandidates: 1,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}

	var decoded ControllerStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode status: %v", err)
	}

	if decoded.PendingReuseCandidates != 1 {
		t.Fatalf("pending_reuse_candidates = %d, want 1", decoded.PendingReuseCandidates)
	}
}

func TestWorkSkipValidate(t *testing.T) {
	tests := []struct {
		name    string
		skip    WorkSkip
		wantErr bool
	}{
		{
			name: "valid skip",
			skip: WorkSkip{
				ID:             "work-item-001",
				PriorAttemptID: "attempt-001",
				Reason:         "matched_prior_completed_attempt",
			},
		},
		{
			name: "missing id",
			skip: WorkSkip{
				PriorAttemptID: "attempt-001",
				Reason:         "matched_prior_completed_attempt",
			},
			wantErr: true,
		},
		{
			name: "missing prior attempt id",
			skip: WorkSkip{
				ID:     "work-item-001",
				Reason: "matched_prior_completed_attempt",
			},
			wantErr: true,
		},
		{
			name: "missing reason",
			skip: WorkSkip{
				ID:             "work-item-001",
				PriorAttemptID: "attempt-001",
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.skip.Validate()

			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWorkSkipJSON(t *testing.T) {
	skip := WorkSkip{
		ID:             "work-item-001",
		PriorAttemptID: "attempt-001",
		Reason:         "matched_prior_completed_attempt",
	}

	data, err := json.Marshal(skip)
	if err != nil {
		t.Fatalf("marshal skip: %v", err)
	}

	var decoded WorkSkip
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode skip: %v", err)
	}

	if decoded.PriorAttemptID != skip.PriorAttemptID {
		t.Fatalf("prior_attempt_id = %q, want %q", decoded.PriorAttemptID, skip.PriorAttemptID)
	}
}

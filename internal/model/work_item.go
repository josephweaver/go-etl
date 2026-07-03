package model

import (
	"fmt"
	"path/filepath"
)

type WorkItemType string

const (
	WorkItemTypeWriteDemoOutput    WorkItemType = "write_demo_output"
	WorkItemTypeSummarizeInputFile WorkItemType = "summarize_input_file"
)

type WorkItem struct {
	ID                   string       `json:"id"`
	AttemptID            string       `json:"attempt_id,omitempty"`
	Type                 WorkItemType `json:"type"`
	OutputFilename       string       `json:"output_filename"`
	Parameters           Parameters   `json:"parameters,omitempty"`
	WorkflowDefinitionID string       `json:"workflow_definition_id,omitempty"`
	WorkflowFingerprint  string       `json:"workflow_fingerprint,omitempty"`
	WorkflowInstanceID   string       `json:"workflow_instance_id,omitempty"`
	StepDefinitionID     string       `json:"step_definition_id,omitempty"`
	StepFingerprint      string       `json:"step_fingerprint,omitempty"`
	StepInstanceID       string       `json:"step_instance_id,omitempty"`
	WorkItemFingerprint  string       `json:"work_item_fingerprint,omitempty"`
	InputFingerprint     string       `json:"input_fingerprint,omitempty"`
	OutputFingerprint    string       `json:"output_fingerprint,omitempty"`
	CodeVersion          string       `json:"code_version,omitempty"`
}

type Parameters map[string]Parameter

type Parameter struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type WorkCompletion struct {
	ID                   string     `json:"id"`
	AttemptID            string     `json:"attempt_id,omitempty"`
	OutputJSON           string     `json:"output_json,omitempty"`
	PreStateJSON         string     `json:"pre_state_json,omitempty"`
	PostStateJSON        string     `json:"post_state_json,omitempty"`
	WorkflowDefinitionID string     `json:"workflow_definition_id,omitempty"`
	WorkflowFingerprint  string     `json:"workflow_fingerprint,omitempty"`
	WorkflowInstanceID   string     `json:"workflow_instance_id,omitempty"`
	StepDefinitionID     string     `json:"step_definition_id,omitempty"`
	StepFingerprint      string     `json:"step_fingerprint,omitempty"`
	StepInstanceID       string     `json:"step_instance_id,omitempty"`
	WorkItemFingerprint  string     `json:"work_item_fingerprint,omitempty"`
	InputFingerprint     string     `json:"input_fingerprint,omitempty"`
	OutputFingerprint    string     `json:"output_fingerprint,omitempty"`
	CodeVersion          string     `json:"code_version,omitempty"`
	StartedAt            string     `json:"started_at,omitempty"`
	CompletedAt          string     `json:"completed_at,omitempty"`
	Parameters           Parameters `json:"parameters,omitempty"`
}

type WorkFailure struct {
	ID        string `json:"id"`
	AttemptID string `json:"attempt_id,omitempty"`
	FailedAt  string `json:"failed_at,omitempty"`
	Error     string `json:"error"`
}

type WorkSkip struct {
	ID             string `json:"id"`
	PriorAttemptID string `json:"prior_attempt_id"`
	Reason         string `json:"reason"`
}

type ControllerStatus struct {
	Pending                int `json:"pending"`
	Assigned               int `json:"assigned"`
	Failed                 int `json:"failed"`
	PendingReuseCandidates int `json:"pending_reuse_candidates"`
	Attempts               int `json:"attempts"`
	AttemptVariables       int `json:"attempt_variables"`
}

func (item WorkItem) Validate() error {
	if item.ID == "" {
		return fmt.Errorf("work item id is required")
	}

	if item.Type == "" {
		return fmt.Errorf("work item type is required")
	}

	if item.OutputFilename == "" {
		return fmt.Errorf("output filename is required")
	}

	if filepath.Base(item.OutputFilename) != item.OutputFilename {
		return fmt.Errorf("output filename must not contain a directory: %s", item.OutputFilename)
	}

	for name, parameter := range item.Parameters {
		if name == "" {
			return fmt.Errorf("parameter name is required")
		}
		if parameter.Type == "" {
			return fmt.Errorf("parameter %s type is required", name)
		}
		if parameter.Value == nil {
			return fmt.Errorf("parameter %s value is required", name)
		}
	}

	return nil
}

func (skip WorkSkip) Validate() error {
	if skip.ID == "" {
		return fmt.Errorf("work item id is required")
	}
	if skip.PriorAttemptID == "" {
		return fmt.Errorf("prior attempt id is required")
	}
	if skip.Reason == "" {
		return fmt.Errorf("skip reason is required")
	}

	return nil
}

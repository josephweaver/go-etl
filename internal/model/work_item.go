package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

type WorkItemType string

const (
	WorkItemTypeWriteDemoOutput    WorkItemType = "write_demo_output"
	WorkItemTypeSummarizeInputFile WorkItemType = "summarize_input_file"
	WorkItemTypePythonScript       WorkItemType = "python_script"
)

type WorkItem struct {
	ID                   string               `json:"id"`
	AttemptID            string               `json:"attempt_id,omitempty"`
	Type                 WorkItemType         `json:"type"`
	Source               *WorkItemSource      `json:"source,omitempty"`
	OutputFilename       string               `json:"output_filename"`
	Parameters           Parameters           `json:"parameters,omitempty"`
	ReuseCandidates      []WorkReuseCandidate `json:"reuse_candidates,omitempty"`
	WorkflowDefinitionID string               `json:"workflow_definition_id,omitempty"`
	WorkflowFingerprint  string               `json:"workflow_fingerprint,omitempty"`
	WorkflowInstanceID   string               `json:"workflow_instance_id,omitempty"`
	StepDefinitionID     string               `json:"step_definition_id,omitempty"`
	StepFingerprint      string               `json:"step_fingerprint,omitempty"`
	StepInstanceID       string               `json:"step_instance_id,omitempty"`
	WorkItemFingerprint  string               `json:"work_item_fingerprint,omitempty"`
	InputFingerprint     string               `json:"input_fingerprint,omitempty"`
	OutputFingerprint    string               `json:"output_fingerprint,omitempty"`
	CodeVersion          string               `json:"code_version,omitempty"`
}

type WorkItemResourceConstraintOperator string

const (
	WorkItemResourceConstraintOperatorEqual     WorkItemResourceConstraintOperator = "="
	WorkItemResourceConstraintOperatorNotEqual  WorkItemResourceConstraintOperator = "!="
	WorkItemResourceConstraintOperatorLessThan  WorkItemResourceConstraintOperator = "<"
	WorkItemResourceConstraintOperatorGreater   WorkItemResourceConstraintOperator = ">"
	WorkItemResourceConstraintOperatorLessEq    WorkItemResourceConstraintOperator = "<="
	WorkItemResourceConstraintOperatorGreaterEq WorkItemResourceConstraintOperator = ">="
)

type WorkItemResourceConstraint struct {
	WorkItemID      string                             `json:"work_item_id"`
	ConstraintIndex int                                `json:"constraint_index"`
	ResourceKey     string                             `json:"resource_key"`
	RequestedUnits  int                                `json:"requested_units"`
	Operator        WorkItemResourceConstraintOperator `json:"operator"`
	TargetUnits     int                                `json:"target_units"`
	CreatedAt       string                             `json:"created_at"`
}

type WorkItemSource struct {
	Schema       string `json:"schema,omitempty"`
	RunID        string `json:"run_id"`
	ManifestPath string `json:"manifest_path"`
}

type WorkReuseCandidate struct {
	AttemptID        string `json:"attempt_id"`
	InputSHA256      string `json:"input_sha256,omitempty"`
	OutputSHA256     string `json:"output_sha256,omitempty"`
	PreStateSHA256   string `json:"pre_state_sha256,omitempty"`
	PostStateSHA256  string `json:"post_state_sha256,omitempty"`
	OutputJSONSHA256 string `json:"output_json_sha256,omitempty"`
	ControllerSHA256 string `json:"controller_sha256,omitempty"`
	PluginSHA256     string `json:"plugin_sha256,omitempty"`
}

type Parameters map[string]Parameter

type Parameter struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type WorkCompletion struct {
	ID                   string     `json:"id"`
	AttemptID            string     `json:"attempt_id,omitempty"`
	Skipped              bool       `json:"skipped,omitempty"`
	SkippedParentID      string     `json:"skipped_parent_id,omitempty"`
	SkipReason           string     `json:"skip_reason,omitempty"`
	InputSHA256          string     `json:"input_sha256,omitempty"`
	OutputSHA256         string     `json:"output_sha256,omitempty"`
	PreStateSHA256       string     `json:"pre_state_sha256,omitempty"`
	PostStateSHA256      string     `json:"post_state_sha256,omitempty"`
	ControllerSHA256     string     `json:"controller_sha256,omitempty"`
	PluginSHA256         string     `json:"plugin_sha256,omitempty"`
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

func (source WorkItemSource) Validate() error {
	if strings.TrimSpace(source.RunID) == "" {
		return fmt.Errorf("work item source run id is required")
	}
	if strings.TrimSpace(source.ManifestPath) == "" {
		return fmt.Errorf("work item source manifest path is required")
	}
	if strings.TrimSpace(source.Schema) == "" && source.Schema != "" {
		return fmt.Errorf("work item source schema must not be empty when set")
	}

	return nil
}

func (item WorkItem) Validate() error {
	return item.validate(false)
}

func (item WorkItem) ValidateForWorkflowCompile() error {
	return item.validate(true)
}

func (item WorkItem) validate(allowMissingPythonSource bool) error {
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

	if item.Type == WorkItemTypePythonScript {
		if item.Source == nil && !allowMissingPythonSource {
			return fmt.Errorf("work item source is required for %s", item.Type)
		}
		if item.Source != nil {
			if err := item.Source.Validate(); err != nil {
				return err
			}
		}
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

func (c WorkItemResourceConstraint) Validate() error {
	if strings.TrimSpace(c.WorkItemID) == "" {
		return fmt.Errorf("work item id is required")
	}
	if c.ConstraintIndex < 0 {
		return fmt.Errorf("constraint index must be non-negative")
	}
	if strings.TrimSpace(c.ResourceKey) == "" {
		return fmt.Errorf("resource key is required")
	}
	if c.RequestedUnits <= 0 {
		return fmt.Errorf("requested units must be greater than 0")
	}
	if !isSupportedWorkItemResourceConstraintOperator(c.Operator) {
		return fmt.Errorf("unsupported resource constraint operator %q", c.Operator)
	}
	if c.TargetUnits < 0 {
		return fmt.Errorf("target units must be non-negative")
	}
	if strings.TrimSpace(c.CreatedAt) == "" {
		return fmt.Errorf("created at is required")
	}
	return nil
}

func isSupportedWorkItemResourceConstraintOperator(operator WorkItemResourceConstraintOperator) bool {
	switch operator {
	case WorkItemResourceConstraintOperatorEqual,
		WorkItemResourceConstraintOperatorNotEqual,
		WorkItemResourceConstraintOperatorLessThan,
		WorkItemResourceConstraintOperatorGreater,
		WorkItemResourceConstraintOperatorLessEq,
		WorkItemResourceConstraintOperatorGreaterEq:
		return true
	default:
		return false
	}
}

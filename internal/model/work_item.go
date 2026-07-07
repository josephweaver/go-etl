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
	WorkItemTypeCacheData          WorkItemType = "cache_data"
)

type WorkItem struct {
	ID                   string               `json:"id"`
	AttemptID            string               `json:"attempt_id,omitempty"`
	Type                 WorkItemType         `json:"type"`
	Source               *WorkItemSource      `json:"source,omitempty"`
	OutputFilename       string               `json:"output_filename"`
	Parameters           Parameters           `json:"parameters,omitempty"`
	DependsOn            []string             `json:"depends_on,omitempty"`
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

type CacheDataWorkItemPayload struct {
	Operator            string             `json:"operator"`
	TargetEnvironmentID string             `json:"target_environment_id"`
	AssetKey            string             `json:"asset_key"`
	DedupeKey           string             `json:"dedupe_key"`
	BindingName         string             `json:"binding_name"`
	ProviderName        string             `json:"provider_name"`
	ProviderType        string             `json:"provider_type"`
	Kind                string             `json:"kind"`
	Format              string             `json:"format,omitempty"`
	ResolvedLocation    DataAssetLocation  `json:"resolved_location"`
	Cache               DataAssetCache     `json:"cache,omitempty"`
	Integrity           DataAssetIntegrity `json:"integrity,omitempty"`
	Archive             *DataAssetArchive  `json:"archive,omitempty"`
	Parameters          map[string]any     `json:"parameters,omitempty"`
	Metadata            map[string]any     `json:"metadata,omitempty"`
}

type ControllerStatus struct {
	Pending                     int                         `json:"pending"`
	Assigned                    int                         `json:"assigned"`
	Failed                      int                         `json:"failed"`
	PendingReuseCandidates      int                         `json:"pending_reuse_candidates"`
	Attempts                    int                         `json:"attempts"`
	AttemptVariables            int                         `json:"attempt_variables"`
	QueuedResourceEligibleCount int                         `json:"queued_resource_eligible_count,omitempty"`
	QueuedResourceBlockedCount  int                         `json:"queued_resource_blocked_count,omitempty"`
	RunningResourceClaimCount   int                         `json:"running_resource_claim_count,omitempty"`
	ResourceConstraintSummaries []ResourceConstraintSummary `json:"resource_constraint_summaries,omitempty"`
}

type ResourceConstraintSummary struct {
	ResourceKey           string `json:"resource_key"`
	TotalUnits            int64  `json:"total_units"`
	BlockedCandidateCount int    `json:"blocked_candidate_count"`
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
	for i, dependency := range item.DependsOn {
		if strings.TrimSpace(dependency) == "" {
			return fmt.Errorf("depends_on[%d] is required", i)
		}
	}

	return nil
}

func (payload CacheDataWorkItemPayload) Validate() error {
	if payload.Operator != string(WorkItemTypeCacheData) {
		return fmt.Errorf("cache_data operator must be %q", WorkItemTypeCacheData)
	}
	if strings.TrimSpace(payload.TargetEnvironmentID) == "" {
		return fmt.Errorf("cache_data target_environment_id is required")
	}
	if !strings.HasPrefix(payload.AssetKey, "sha256:") {
		return fmt.Errorf("cache_data asset_key must use sha256: prefix")
	}
	if err := validateOptionalSHA256("cache_data asset_key", strings.TrimPrefix(payload.AssetKey, "sha256:")); err != nil {
		return err
	}
	if strings.TrimSpace(payload.DedupeKey) == "" {
		return fmt.Errorf("cache_data dedupe_key is required")
	}
	if err := validateDataName(payload.BindingName, "cache_data binding_name"); err != nil {
		return err
	}
	if err := validateDataName(payload.ProviderName, "cache_data provider_name"); err != nil {
		return err
	}
	if !isSupportedDataProvider(payload.ProviderType) {
		return fmt.Errorf("unsupported cache_data provider_type %q", payload.ProviderType)
	}
	if strings.TrimSpace(payload.Kind) == "" {
		return fmt.Errorf("cache_data kind is required")
	}
	if err := payload.ResolvedLocation.Validate(); err != nil {
		return err
	}
	if err := payload.Cache.Validate(); err != nil {
		return err
	}
	if err := payload.Integrity.Validate(); err != nil {
		return err
	}
	if payload.Archive != nil {
		if err := payload.Archive.Validate(); err != nil {
			return err
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

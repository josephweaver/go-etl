package model

import (
	"fmt"
	"path/filepath"
	"strings"

	"goetl/internal/variable"
)

type WorkItemType string

const (
	WorkItemTypeWriteDemoOutput    WorkItemType = "write_demo_output"
	WorkItemTypeSummarizeInputFile WorkItemType = "summarize_input_file"
	WorkItemTypePythonScript       WorkItemType = "python_script"
	WorkItemTypeCacheData          WorkItemType = "cache_data"
	WorkItemTypeCommitData         WorkItemType = "commit_data"
)

type WorkItem struct {
	ID                   string               `json:"id"`
	AttemptID            string               `json:"attempt_id,omitempty"`
	Type                 WorkItemType         `json:"type"`
	Source               *WorkItemSource      `json:"source,omitempty"`
	OutputFilename       string               `json:"output_filename"`
	Parameters           Parameters           `json:"parameters,omitempty"`
	ExecutionEnvelope    *ExecutionEnvelope   `json:"execution_envelope,omitempty"`
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
	Type           string                    `json:"type"`
	Value          any                       `json:"value,omitempty"`
	Sensitive      bool                      `json:"sensitive,omitempty"`
	RedactionLabel string                    `json:"redaction_label,omitempty"`
	ProtectedRef   *variable.ProtectedRef    `json:"protected_ref,omitempty"`
	Materialize    *ParameterMaterialization `json:"materialize,omitempty"`
}

type ParameterMaterialization struct {
	Mode   string `json:"mode"`
	Target string `json:"target"`
}

type ExecutionEnvelope struct {
	Schema    string                     `json:"schema"`
	WorkItem  ExecutionEnvelopeWorkItem  `json:"work_item,omitempty"`
	Variables ExecutionEnvelopeVariables `json:"variables"`
}

type ExecutionEnvelopeWorkItem struct {
	ID   string       `json:"id"`
	Type WorkItemType `json:"type"`
}

type ExecutionEnvelopeVariables struct {
	Public        map[string]ExecutionEnvelopePublicValue        `json:"public,omitempty"`
	ProtectedRefs map[string]ExecutionEnvelopeProtectedReference `json:"protected_refs,omitempty"`
}

type ExecutionEnvelopePublicValue struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type ExecutionEnvelopeProtectedReference struct {
	Type           string                    `json:"type"`
	Provider       string                    `json:"provider"`
	Key            string                    `json:"key"`
	RedactionLabel string                    `json:"redaction_label"`
	Materialize    *ParameterMaterialization `json:"materialize,omitempty"`
}

type WorkCompletion struct {
	ID                   string     `json:"id"`
	AttemptID            string     `json:"attempt_id,omitempty"`
	WorkerID             string     `json:"worker_id,omitempty"`
	WorkerSessionID      string     `json:"worker_session_id,omitempty"`
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
	ID              string `json:"id"`
	AttemptID       string `json:"attempt_id,omitempty"`
	WorkerID        string `json:"worker_id,omitempty"`
	WorkerSessionID string `json:"worker_session_id,omitempty"`
	FailedAt        string `json:"failed_at,omitempty"`
	Error           string `json:"error"`
}

type WorkSkip struct {
	ID             string `json:"id"`
	PriorAttemptID string `json:"prior_attempt_id"`
	Reason         string `json:"reason"`
}

type CacheDataWorkItemPayload struct {
	Operator            string                       `json:"operator"`
	TargetEnvironmentID string                       `json:"target_environment_id"`
	AssetKey            string                       `json:"asset_key"`
	DedupeKey           string                       `json:"dedupe_key"`
	BindingName         string                       `json:"binding_name"`
	ProviderName        string                       `json:"provider_name"`
	ProviderType        string                       `json:"provider_type"`
	Kind                string                       `json:"kind"`
	Format              string                       `json:"format,omitempty"`
	ResolvedLocation    DataAssetLocation            `json:"resolved_location"`
	Cache               DataAssetCache               `json:"cache,omitempty"`
	Integrity           DataAssetIntegrity           `json:"integrity,omitempty"`
	Archive             *DataAssetArchive            `json:"archive,omitempty"`
	ResourceConstraints []WorkItemResourceConstraint `json:"resource_constraints,omitempty"`
	TransferPolicy      DataAssetTransferPolicy      `json:"transfer_policy,omitempty"`
	TransferLimits      DataAssetTransferLimits      `json:"transfer_limits,omitempty"`
	Parameters          map[string]any               `json:"parameters,omitempty"`
	Metadata            map[string]any               `json:"metadata,omitempty"`
}

type CommitDataWorkItemPayload struct {
	Operator            string                       `json:"operator"`
	TargetEnvironmentID string                       `json:"target_environment_id"`
	Source              CommitDataSource             `json:"source"`
	PublishTarget       BoundPublishTarget           `json:"publish_target"`
	ResourceConstraints []WorkItemResourceConstraint `json:"resource_constraints,omitempty"`
}

type CommitDataSource struct {
	FromWorkItemID string `json:"from_work_item_id"`
	FromArtifact   string `json:"from_artifact"`
}

type DataAssetTransferLimits struct {
	MaxBytesPerSecond int64 `json:"max_bytes_per_second,omitempty"`
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

func (item WorkItem) WithExecutionEnvelope() (WorkItem, error) {
	envelope, err := NewExecutionEnvelope(item)
	if err != nil {
		return WorkItem{}, err
	}
	item.ExecutionEnvelope = &envelope
	return item, nil
}

func NewExecutionEnvelope(item WorkItem) (ExecutionEnvelope, error) {
	envelope := ExecutionEnvelope{
		Schema: "goet/execution-envelope/v1",
		WorkItem: ExecutionEnvelopeWorkItem{
			ID:   item.ID,
			Type: item.Type,
		},
		Variables: ExecutionEnvelopeVariables{
			Public:        map[string]ExecutionEnvelopePublicValue{},
			ProtectedRefs: map[string]ExecutionEnvelopeProtectedReference{},
		},
	}

	for name, parameter := range item.Parameters {
		if err := parameter.Validate(); err != nil {
			return ExecutionEnvelope{}, fmt.Errorf("parameter %s: %w", name, err)
		}
		if parameter.ProtectedRef != nil {
			ref := parameter.ProtectedRef.Normalize()
			envelope.Variables.ProtectedRefs[name] = ExecutionEnvelopeProtectedReference{
				Type:           parameter.Type,
				Provider:       ref.Provider,
				Key:            ref.Key,
				RedactionLabel: ref.RedactionLabelValue(),
				Materialize:    parameter.Materialize,
			}
			continue
		}
		envelope.Variables.Public[name] = ExecutionEnvelopePublicValue{
			Type:  parameter.Type,
			Value: parameter.Value,
		}
	}

	if len(envelope.Variables.Public) == 0 {
		envelope.Variables.Public = nil
	}
	if len(envelope.Variables.ProtectedRefs) == 0 {
		envelope.Variables.ProtectedRefs = nil
	}
	return envelope, nil
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
		if err := parameter.Validate(); err != nil {
			return fmt.Errorf("parameter %s: %w", name, err)
		}
	}
	if item.ExecutionEnvelope != nil {
		if err := item.ExecutionEnvelope.Validate(); err != nil {
			return fmt.Errorf("execution envelope: %w", err)
		}
	}
	for i, dependency := range item.DependsOn {
		if strings.TrimSpace(dependency) == "" {
			return fmt.Errorf("depends_on[%d] is required", i)
		}
	}

	return nil
}

func (p Parameter) Validate() error {
	if p.Type == "" {
		return fmt.Errorf("type is required")
	}
	if p.ProtectedRef != nil {
		if err := p.ProtectedRef.Validate(); err != nil {
			return err
		}
		if p.Value != nil {
			return fmt.Errorf("protected reference value must be omitted")
		}
		if p.Materialize != nil {
			if err := p.Materialize.Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if p.Sensitive {
		return fmt.Errorf("sensitive parameter must use protected_ref")
	}
	if p.Value == nil {
		return fmt.Errorf("value is required")
	}
	if p.Materialize != nil {
		return fmt.Errorf("materialize requires protected_ref")
	}
	return nil
}

func (m ParameterMaterialization) Validate() error {
	if strings.TrimSpace(m.Mode) == "" {
		return fmt.Errorf("materialize mode is required")
	}
	if m.Mode != "env" && m.Mode != "file" {
		return fmt.Errorf("unsupported materialize mode %q", m.Mode)
	}
	if strings.TrimSpace(m.Target) == "" {
		return fmt.Errorf("materialize target is required")
	}
	if strings.TrimSpace(m.Target) != m.Target || strings.ContainsAny(m.Target, " \t\r\n=") {
		return fmt.Errorf("materialize target must be an environment variable name")
	}
	return nil
}

func (e ExecutionEnvelope) Validate() error {
	if e.Schema != "goet/execution-envelope/v1" {
		return fmt.Errorf("unsupported schema %q", e.Schema)
	}
	for name, value := range e.Variables.Public {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("public variable name is required")
		}
		if strings.TrimSpace(value.Type) == "" {
			return fmt.Errorf("public variable %s type is required", name)
		}
		if value.Value == nil {
			return fmt.Errorf("public variable %s value is required", name)
		}
	}
	for name, ref := range e.Variables.ProtectedRefs {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("protected reference name is required")
		}
		if strings.TrimSpace(ref.Type) == "" {
			return fmt.Errorf("protected reference %s type is required", name)
		}
		protectedRef := variable.ProtectedRef{
			Provider:       ref.Provider,
			Key:            ref.Key,
			RedactionLabel: ref.RedactionLabel,
		}
		if err := protectedRef.Validate(); err != nil {
			return fmt.Errorf("protected reference %s: %w", name, err)
		}
		if ref.Materialize != nil {
			if err := ref.Materialize.Validate(); err != nil {
				return fmt.Errorf("protected reference %s: %w", name, err)
			}
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
	for i, constraint := range payload.ResourceConstraints {
		if strings.TrimSpace(constraint.ResourceKey) == "" {
			return fmt.Errorf("cache_data resource_constraints[%d] resource_key is required", i)
		}
		if constraint.RequestedUnits <= 0 {
			return fmt.Errorf("cache_data resource_constraints[%d] requested_units must be greater than 0", i)
		}
		if !isSupportedWorkItemResourceConstraintOperator(constraint.Operator) {
			return fmt.Errorf("cache_data resource_constraints[%d] unsupported operator %q", i, constraint.Operator)
		}
		if constraint.TargetUnits < 0 {
			return fmt.Errorf("cache_data resource_constraints[%d] target_units must be non-negative", i)
		}
	}
	if err := payload.TransferPolicy.Validate(); err != nil {
		return fmt.Errorf("cache_data transfer_policy: %w", err)
	}
	if payload.TransferLimits.MaxBytesPerSecond < 0 {
		return fmt.Errorf("cache_data transfer_limits max_bytes_per_second must be non-negative")
	}
	return nil
}

func (payload CommitDataWorkItemPayload) Validate() error {
	if payload.Operator != string(WorkItemTypeCommitData) {
		return fmt.Errorf("commit_data operator must be %q", WorkItemTypeCommitData)
	}
	if strings.TrimSpace(payload.TargetEnvironmentID) == "" {
		return fmt.Errorf("commit_data target_environment_id is required")
	}
	if strings.TrimSpace(payload.Source.FromWorkItemID) == "" {
		return fmt.Errorf("commit_data source from_work_item_id is required")
	}
	if err := validateDataName(payload.Source.FromArtifact, "commit_data source from_artifact"); err != nil {
		return err
	}
	if err := payload.PublishTarget.Validate(); err != nil {
		return fmt.Errorf("commit_data publish_target: %w", err)
	}
	if payload.PublishTarget.FromArtifact != payload.Source.FromArtifact {
		return fmt.Errorf("commit_data publish_target from_artifact must match source from_artifact")
	}
	for i, constraint := range payload.ResourceConstraints {
		if strings.TrimSpace(constraint.ResourceKey) == "" {
			return fmt.Errorf("commit_data resource_constraints[%d] resource_key is required", i)
		}
		if constraint.RequestedUnits <= 0 {
			return fmt.Errorf("commit_data resource_constraints[%d] requested_units must be greater than 0", i)
		}
		if !isSupportedWorkItemResourceConstraintOperator(constraint.Operator) {
			return fmt.Errorf("commit_data resource_constraints[%d] unsupported operator %q", i, constraint.Operator)
		}
		if constraint.TargetUnits < 0 {
			return fmt.Errorf("commit_data resource_constraints[%d] target_units must be non-negative", i)
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	fp "goetl/internal/fingerprint"
	"goetl/internal/ledger"
	"goetl/internal/model"
	"goetl/internal/persistence"
)

func (c *Controller) recordAttempt(ctx context.Context, attempt ledger.Attempt) error {
	if c.ledger == nil {
		return nil
	}

	return ledger.InsertAttempt(ctx, c.ledger, attempt)
}

func completeAttemptRequestFromCompletion(completion model.WorkCompletion, fallbackCompletedAt time.Time) (persistence.CompleteAttemptRequest, error) {
	if completion.AttemptID == "" {
		return persistence.CompleteAttemptRequest{}, fmt.Errorf("attempt_id is required")
	}
	if completion.Skipped && completion.SkippedParentID == "" {
		return persistence.CompleteAttemptRequest{}, fmt.Errorf("skipped_parent_id is required when skipped is true")
	}

	outputJSON, outputJSONSHA256, err := canonicalJSONTextAndHash("output_json", completion.OutputJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	if err := validateArtifactManifestOutputJSON(outputJSON); err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	_, preStateEvidenceSHA256, err := canonicalJSONTextAndHash("pre_state_json", completion.PreStateJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	_, postStateEvidenceSHA256, err := canonicalJSONTextAndHash("post_state_json", completion.PostStateJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	preStateSHA256, err := reportedOrEvidenceSHA256("pre_state_sha256", completion.PreStateSHA256, preStateEvidenceSHA256)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	postStateSHA256, err := reportedOrEvidenceSHA256("post_state_sha256", completion.PostStateSHA256, postStateEvidenceSHA256)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}

	completedAt, err := reportTimestamp("completed_at", completion.CompletedAt, fallbackCompletedAt)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}

	return persistence.CompleteAttemptRequest{
		AttemptID:        completion.AttemptID,
		WorkerID:         completion.WorkerID,
		WorkerSessionID:  completion.WorkerSessionID,
		SkippedParentID:  completion.SkippedParentID,
		OutputJSON:       outputJSON,
		OutputJSONSHA256: outputJSONSHA256,
		PreStateSHA256:   preStateSHA256,
		PostStateSHA256:  postStateSHA256,
		CompletedAt:      completedAt,
	}, nil
}

func reportedOrEvidenceSHA256(name string, reported string, evidence string) (string, error) {
	if reported == "" {
		return evidence, nil
	}
	if err := fp.ValidateSHA256Hex(reported); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return reported, nil
}

func canonicalJSONTextAndHash(name string, value string) (string, string, error) {
	if strings.TrimSpace(value) == "" {
		return "", "", fmt.Errorf("%s is required", name)
	}

	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return "", "", fmt.Errorf("decode %s: %w", name, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", "", fmt.Errorf("%s must contain one JSON document", name)
	}

	canonical, hash, err := fp.CanonicalJSONSHA256(decoded)
	if err != nil {
		return "", "", fmt.Errorf("canonicalize %s: %w", name, err)
	}
	return string(canonical), hash, nil
}

func reportTimestamp(name string, value string, fallback time.Time) (string, error) {
	if value == "" {
		return fallback.UTC().Format(time.RFC3339), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed.UTC().Format(time.RFC3339), nil
}

func attemptFromCompletion(completion model.WorkCompletion) (ledger.Attempt, bool, error) {
	if completion.AttemptID == "" {
		return ledger.Attempt{}, false, nil
	}

	startedAt, err := time.Parse(time.RFC3339, completion.StartedAt)
	if err != nil {
		return ledger.Attempt{}, false, fmt.Errorf("parse started_at: %w", err)
	}

	completedAt, err := time.Parse(time.RFC3339, completion.CompletedAt)
	if err != nil {
		return ledger.Attempt{}, false, fmt.Errorf("parse completed_at: %w", err)
	}

	return ledger.Attempt{
		ID:                  completion.AttemptID,
		WorkflowInstanceID:  completion.WorkflowInstanceID,
		StepInstanceID:      completion.StepInstanceID,
		WorkItemID:          completion.ID,
		WorkItemFingerprint: completion.WorkItemFingerprint,
		InputFingerprint:    completion.InputFingerprint,
		OutputFingerprint:   completion.OutputFingerprint,
		CodeVersion:         completion.CodeVersion,
		Status:              ledger.AttemptStatusCompleted,
		StartedAt:           startedAt,
		CompletedAt:         completedAt,
		Variables:           runtimeVariablesFromCompletion(completion),
	}, true, nil
}

func runtimeVariablesFromCompletion(completion model.WorkCompletion) []ledger.AttemptVariable {
	variables := []ledger.AttemptVariable{
		runtimeStringVariable("workflow_definition_id", completion.WorkflowDefinitionID, "workflow"),
		runtimeStringVariable("workflow_fingerprint", completion.WorkflowFingerprint, "workflow"),
		runtimeStringVariable("workflow_instance_id", completion.WorkflowInstanceID, "workflow"),
		runtimeStringVariable("step_definition_id", completion.StepDefinitionID, "step"),
		runtimeStringVariable("step_fingerprint", completion.StepFingerprint, "step"),
		runtimeStringVariable("step_instance_id", completion.StepInstanceID, "step"),
		runtimeStringVariable("work_item_id", completion.ID, "work_item"),
		runtimeStringVariable("work_item_fingerprint", completion.WorkItemFingerprint, "work_item"),
		runtimeStringVariable("input_fingerprint", completion.InputFingerprint, "work_item"),
		runtimeStringVariable("output_fingerprint", completion.OutputFingerprint, "work_item"),
		runtimeStringVariable("code_version", completion.CodeVersion, "work_item"),
		runtimeStringVariable("attempt_id", completion.AttemptID, "attempt"),
		runtimeStringVariable("started_at", completion.StartedAt, "attempt"),
		runtimeStringVariable("completed_at", completion.CompletedAt, "attempt"),
	}

	for name, parameter := range completion.Parameters {
		variables = append(variables, ledger.AttemptVariable{
			Namespace: "work_item",
			Name:      name,
			Type:      parameter.Type,
			Value:     parameter.Value,
			Source:    "controller",
			Lifecycle: "work_item",
		})
	}

	return variables
}

func runtimeVariablesFromSkip(item model.WorkItem, skip model.WorkSkip, skippedAt time.Time) []ledger.AttemptVariable {
	timestamp := skippedAt.UTC().Format(time.RFC3339)
	return []ledger.AttemptVariable{
		runtimeStringVariable("workflow_definition_id", item.WorkflowDefinitionID, "workflow"),
		runtimeStringVariable("workflow_fingerprint", item.WorkflowFingerprint, "workflow"),
		runtimeStringVariable("workflow_instance_id", item.WorkflowInstanceID, "workflow"),
		runtimeStringVariable("step_definition_id", item.StepDefinitionID, "step"),
		runtimeStringVariable("step_fingerprint", item.StepFingerprint, "step"),
		runtimeStringVariable("step_instance_id", item.StepInstanceID, "step"),
		runtimeStringVariable("work_item_id", skip.ID, "work_item"),
		runtimeStringVariable("work_item_fingerprint", item.WorkItemFingerprint, "work_item"),
		runtimeStringVariable("input_fingerprint", item.InputFingerprint, "work_item"),
		runtimeStringVariable("output_fingerprint", item.OutputFingerprint, "work_item"),
		runtimeStringVariable("code_version", item.CodeVersion, "work_item"),
		runtimeStringVariable("prior_attempt_id", skip.PriorAttemptID, "attempt"),
		runtimeStringVariable("skip_reason", skip.Reason, "attempt"),
		runtimeStringVariable("started_at", timestamp, "attempt"),
		runtimeStringVariable("completed_at", timestamp, "attempt"),
	}
}

func runtimeStringVariable(name string, value string, lifecycle string) ledger.AttemptVariable {
	return ledger.AttemptVariable{
		Namespace: "runtime",
		Name:      name,
		Type:      "string",
		Value:     value,
		Source:    "worker",
		Lifecycle: lifecycle,
	}
}

func skippedAttemptFromWorkSkip(item model.WorkItem, skip model.WorkSkip, skippedAt time.Time) (ledger.Attempt, error) {
	if err := skip.Validate(); err != nil {
		return ledger.Attempt{}, err
	}
	if skippedAt.IsZero() {
		skippedAt = time.Now().UTC()
	}

	return ledger.Attempt{
		ID:                  skip.ID + "-skip-" + randomHex(8),
		WorkflowInstanceID:  item.WorkflowInstanceID,
		StepInstanceID:      item.StepInstanceID,
		WorkItemID:          skip.ID,
		WorkItemFingerprint: item.WorkItemFingerprint,
		InputFingerprint:    item.InputFingerprint,
		OutputFingerprint:   item.OutputFingerprint,
		CodeVersion:         item.CodeVersion,
		Status:              ledger.AttemptStatusSkipped,
		StartedAt:           skippedAt.UTC(),
		CompletedAt:         skippedAt.UTC(),
		Variables:           runtimeVariablesFromSkip(item, skip, skippedAt.UTC()),
	}, nil
}

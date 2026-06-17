package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"goetl/internal/model"
)

func reportWorkComplete(controllerURL string, item model.WorkItem, startedAt time.Time) error {
	url := strings.TrimRight(controllerURL, "/") + "/work/complete"

	body, err := json.Marshal(workCompletion(item, startedAt))
	if err != nil {
		return fmt.Errorf("encode work completion: %w", err)
	}

	response, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post work completion to %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("post work completion to %s: unexpected status %s", url, response.Status)
	}

	return nil
}

func workCompletion(item model.WorkItem, startedAt time.Time) model.WorkCompletion {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	completedAt := time.Now().UTC().Format(time.RFC3339)

	completion := model.WorkCompletion{
		ID:                   item.ID,
		AttemptID:            item.ID + "-attempt-" + randomHex(8),
		WorkflowDefinitionID: item.WorkflowDefinitionID,
		WorkflowFingerprint:  item.WorkflowFingerprint,
		WorkflowInstanceID:   item.WorkflowInstanceID,
		StepDefinitionID:     item.StepDefinitionID,
		StepFingerprint:      item.StepFingerprint,
		StepInstanceID:       item.StepInstanceID,
		WorkItemFingerprint:  item.WorkItemFingerprint,
		InputFingerprint:     item.InputFingerprint,
		OutputFingerprint:    item.OutputFingerprint,
		CodeVersion:          item.CodeVersion,
		StartedAt:            startedAt.UTC().Format(time.RFC3339),
		CompletedAt:          completedAt,
		Parameters:           item.Parameters,
	}

	if completion.WorkflowInstanceID == "" {
		completion.WorkflowInstanceID = "demo-workflow-instance"
	}
	if completion.WorkflowDefinitionID == "" {
		completion.WorkflowDefinitionID = "demo-workflow"
	}
	if completion.WorkflowFingerprint == "" {
		completion.WorkflowFingerprint = "demo-workflow-fingerprint"
	}
	if completion.StepInstanceID == "" {
		completion.StepInstanceID = "demo-step-instance"
	}
	if completion.StepDefinitionID == "" {
		completion.StepDefinitionID = "demo-step"
	}
	if completion.StepFingerprint == "" {
		completion.StepFingerprint = "demo-step-fingerprint"
	}
	if completion.WorkItemFingerprint == "" {
		completion.WorkItemFingerprint = "demo-work-item:" + item.ID
	}
	if completion.InputFingerprint == "" {
		completion.InputFingerprint = "demo-input:" + item.ID
	}
	if completion.OutputFingerprint == "" {
		completion.OutputFingerprint = "demo-output:" + item.ID
	}
	if completion.CodeVersion == "" {
		completion.CodeVersion = "demo"
	}

	return completion
}

func randomHex(byteCount int) string {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(data)
}

func reportWorkFailed(controllerURL string, itemID string, workErr error) error {
	url := strings.TrimRight(controllerURL, "/") + "/work/fail"

	body, err := json.Marshal(model.WorkFailure{ID: itemID, Error: workErr.Error()})
	if err != nil {
		return fmt.Errorf("encode work failure: %w", err)
	}

	response, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post work failure to %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("post work failure to %s: unexpected status %s", url, response.Status)
	}

	return nil
}

func fetchWorkItem(controllerURL string) (model.WorkItem, bool, error) {
	url := strings.TrimRight(controllerURL, "/") + "/work/next"

	response, err := http.Get(url)
	if err != nil {
		return model.WorkItem{}, false, fmt.Errorf("get work item from %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNoContent {
		return model.WorkItem{}, false, nil
	}

	if response.StatusCode != http.StatusOK {
		return model.WorkItem{}, false, fmt.Errorf("get work item from %s: unexpected status %s", url, response.Status)
	}

	var item model.WorkItem
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		return model.WorkItem{}, false, fmt.Errorf("decode work item from %s: %w", url, err)
	}

	if err := item.Validate(); err != nil {
		return model.WorkItem{}, false, fmt.Errorf("validate work item from %s: %w", url, err)
	}

	return item, true, nil
}

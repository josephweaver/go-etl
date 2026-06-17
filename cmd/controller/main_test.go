package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"goetl/internal/ledger"
	"goetl/internal/model"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

func TestNextWorkHandler(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	var item model.WorkItem
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if item.ID != "test-001" {
		t.Fatalf("unexpected id: %q", item.ID)
	}
}

func TestNextWorkHandlerReturnsNoContentWhenQueueIsEmpty(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestNextWorkHandlerRejectsPost(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandler(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{"id":"test-001"}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandlerRecordsAttemptWhenMetadataPresent(t *testing.T) {
	controller := newTestController()
	db, err := initConfiguredLedger(context.Background(), ControllerConfig{Variables: []variable.Variable{
		{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"},
			Type:       variable.TypePath,
			Expression: filepath.Join(t.TempDir(), "ledger.sqlite"),
		},
	}})
	if err != nil {
		t.Fatalf("initialize ledger: %v", err)
	}
	defer db.Close()
	controller.ledger = db
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"attempt_id":"attempt-001",
		"workflow_definition_id":"workflow-definition-001",
		"workflow_fingerprint":"workflow-fingerprint",
		"workflow_instance_id":"workflow-instance-001",
		"step_definition_id":"step-definition-001",
		"step_fingerprint":"step-fingerprint",
		"step_instance_id":"step-instance-001",
		"work_item_fingerprint":"work-item-fingerprint",
		"input_fingerprint":"input-fingerprint",
		"output_fingerprint":"output-fingerprint",
		"code_version":"code-version",
		"started_at":"2026-06-06T12:00:00Z",
		"completed_at":"2026-06-06T12:01:00Z",
		"parameters": {
			"input_path": {
				"type": "path",
				"value": "demo-summary-input.txt"
			}
		}
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM attempts`).Scan(&count); err != nil {
		t.Fatalf("query attempt count: %v", err)
	}
	if count != 1 {
		t.Fatalf("attempt count = %d, want 1", count)
	}

	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM attempt_variables WHERE namespace = 'runtime'`).Scan(&count); err != nil {
		t.Fatalf("query attempt variable count: %v", err)
	}
	if count != 14 {
		t.Fatalf("runtime attempt variable count = %d, want 14", count)
	}

	var valueJSON string
	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'runtime' AND name = 'workflow_definition_id'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query workflow definition variable: %v", err)
	}
	if valueJSON != `"workflow-definition-001"` {
		t.Fatalf("workflow_definition_id value_json = %q", valueJSON)
	}

	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'runtime' AND name = 'workflow_fingerprint'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query workflow fingerprint variable: %v", err)
	}
	if valueJSON != `"workflow-fingerprint"` {
		t.Fatalf("workflow_fingerprint value_json = %q", valueJSON)
	}

	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'runtime' AND name = 'workflow_instance_id'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query workflow instance variable: %v", err)
	}
	if valueJSON != `"workflow-instance-001"` {
		t.Fatalf("workflow_instance_id value_json = %q", valueJSON)
	}

	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'work_item' AND name = 'input_path'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query input path variable: %v", err)
	}
	if valueJSON != `"demo-summary-input.txt"` {
		t.Fatalf("input_path value_json = %q", valueJSON)
	}
}

func TestPriorCompletedAttemptFindsMatchingFingerprint(t *testing.T) {
	controller := newTestController()
	db, err := initConfiguredLedger(context.Background(), ControllerConfig{Variables: []variable.Variable{
		{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"},
			Type:       variable.TypePath,
			Expression: filepath.Join(t.TempDir(), "ledger.sqlite"),
		},
	}})
	if err != nil {
		t.Fatalf("initialize ledger: %v", err)
	}
	defer db.Close()
	controller.ledger = db

	completion := model.WorkCompletion{
		ID:                   "test-001",
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
	}
	attempt, _, err := attemptFromCompletion(completion)
	if err != nil {
		t.Fatalf("build attempt: %v", err)
	}
	if err := controller.recordAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("record attempt: %v", err)
	}

	found, ok, err := controller.priorCompletedAttempt(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
	})
	if err != nil {
		t.Fatalf("priorCompletedAttempt() error = %v", err)
	}
	if !ok {
		t.Fatal("expected a prior attempt")
	}
	if found.ID != "attempt-001" {
		t.Fatalf("attempt id = %q, want attempt-001", found.ID)
	}
}

func TestPriorCompletedAttemptReturnsMissingWithoutLedgerOrFingerprint(t *testing.T) {
	controller := newTestController()

	if attempt, ok, err := controller.priorCompletedAttempt(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
	}); err != nil || ok {
		t.Fatalf("priorCompletedAttempt() = %+v, %v, %v; want missing nil error", attempt, ok, err)
	}

	db, err := initConfiguredLedger(context.Background(), ControllerConfig{Variables: []variable.Variable{
		{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"},
			Type:       variable.TypePath,
			Expression: filepath.Join(t.TempDir(), "ledger.sqlite"),
		},
	}})
	if err != nil {
		t.Fatalf("initialize ledger: %v", err)
	}
	defer db.Close()
	controller.ledger = db

	if attempt, ok, err := controller.priorCompletedAttempt(context.Background(), model.WorkItem{}); err != nil || ok {
		t.Fatalf("priorCompletedAttempt() = %+v, %v, %v; want missing nil error", attempt, ok, err)
	}
}

func TestPriorCompletedAttemptMatchesWorkItem(t *testing.T) {
	item := model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	}
	attempt := ledger.Attempt{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
		Status:              ledger.AttemptStatusCompleted,
	}

	if !priorCompletedAttemptMatchesWorkItem(item, attempt) {
		t.Fatal("expected matching prior attempt")
	}
}

func TestPriorCompletedAttemptMatchesWorkItemRejectsMismatch(t *testing.T) {
	baseItem := model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	}
	baseAttempt := ledger.Attempt{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
		Status:              ledger.AttemptStatusCompleted,
	}

	tests := []struct {
		name    string
		item    model.WorkItem
		attempt ledger.Attempt
	}{
		{
			name:    "failed prior attempt",
			item:    baseItem,
			attempt: withAttemptStatus(baseAttempt, ledger.AttemptStatusFailed),
		},
		{
			name:    "work item fingerprint",
			item:    withWorkItemFingerprint(baseItem, "changed"),
			attempt: baseAttempt,
		},
		{
			name:    "input fingerprint",
			item:    withInputFingerprint(baseItem, "changed"),
			attempt: baseAttempt,
		},
		{
			name:    "output fingerprint",
			item:    withOutputFingerprint(baseItem, "changed"),
			attempt: baseAttempt,
		},
		{
			name:    "code version",
			item:    withCodeVersion(baseItem, "changed"),
			attempt: baseAttempt,
		},
		{
			name:    "missing current fingerprint",
			item:    withInputFingerprint(baseItem, ""),
			attempt: baseAttempt,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if priorCompletedAttemptMatchesWorkItem(test.item, test.attempt) {
				t.Fatal("expected prior attempt mismatch")
			}
		})
	}
}

func TestReusablePriorAttemptFindsMatchingAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
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
	})

	attempt, ok, err := controller.reusablePriorAttempt(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	})
	if err != nil {
		t.Fatalf("reusablePriorAttempt() error = %v", err)
	}
	if !ok {
		t.Fatal("expected reusable prior attempt")
	}
	if attempt.ID != "attempt-001" {
		t.Fatalf("attempt id = %q, want attempt-001", attempt.ID)
	}
}

func TestReusablePriorAttemptRejectsMismatchedAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
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
		CodeVersion:          "old-code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})

	attempt, ok, err := controller.reusablePriorAttempt(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "new-code-version",
	})
	if err != nil {
		t.Fatalf("reusablePriorAttempt() error = %v", err)
	}
	if ok {
		t.Fatalf("unexpected reusable attempt: %+v", attempt)
	}
}

func TestWorkReuseDecisionReportsReusableAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
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
	})

	decision, err := controller.workReuseDecision(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	})
	if err != nil {
		t.Fatalf("workReuseDecision() error = %v", err)
	}

	if !decision.Reusable || decision.Reason != "matched_prior_completed_attempt" || decision.PriorAttemptID != "attempt-001" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestWorkReuseDecisionReportsMismatchedAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
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
		CodeVersion:          "old-code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})

	decision, err := controller.workReuseDecision(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "new-code-version",
	})
	if err != nil {
		t.Fatalf("workReuseDecision() error = %v", err)
	}

	if decision.Reusable || decision.Reason != "prior_attempt_mismatch" || decision.PriorAttemptID != "attempt-001" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestWorkReuseDecisionReportsMissingAttempt(t *testing.T) {
	controller := newController(nil)

	decision, err := controller.workReuseDecision(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	})
	if err != nil {
		t.Fatalf("workReuseDecision() error = %v", err)
	}

	if decision.Reusable || decision.Reason != "no_prior_completed_attempt" || decision.PriorAttemptID != "" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestWorkSkipForReuseDecisionBuildsSkip(t *testing.T) {
	skip, ok, err := workSkipForReuseDecision(model.WorkItem{ID: "work-item-001"}, WorkReuseDecision{
		Reusable:       true,
		Reason:         "matched_prior_completed_attempt",
		PriorAttemptID: "attempt-001",
	})
	if err != nil {
		t.Fatalf("workSkipForReuseDecision() error = %v", err)
	}
	if !ok {
		t.Fatal("expected skip marker")
	}
	if skip.ID != "work-item-001" || skip.PriorAttemptID != "attempt-001" || skip.Reason != "matched_prior_completed_attempt" {
		t.Fatalf("unexpected skip marker: %+v", skip)
	}
}

func TestWorkSkipForReuseDecisionReturnsMissingForNonReusableDecision(t *testing.T) {
	skip, ok, err := workSkipForReuseDecision(model.WorkItem{ID: "work-item-001"}, WorkReuseDecision{
		Reason: "prior_attempt_mismatch",
	})
	if err != nil {
		t.Fatalf("workSkipForReuseDecision() error = %v", err)
	}
	if ok {
		t.Fatalf("unexpected skip marker: %+v", skip)
	}
}

func TestWorkSkipForReuseDecisionRejectsInvalidSkip(t *testing.T) {
	if _, _, err := workSkipForReuseDecision(model.WorkItem{}, WorkReuseDecision{
		Reusable:       true,
		Reason:         "matched_prior_completed_attempt",
		PriorAttemptID: "attempt-001",
	}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCompleteWorkHandlerRejectsInvalidAttemptMetadata(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"attempt_id":"attempt-001",
		"started_at":"not-a-time",
		"completed_at":"2026-06-06T12:01:00Z"
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandlerRejectsUnassignedItem(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{"id":"test-001"}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandlerRejectsGet(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodGet, "/work/complete", nil)
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandlerRejectsMissingID(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestFailWorkHandler(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001","error":"failed"}`))
	response := httptest.NewRecorder()

	controller.failWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	if controller.failed["test-001"].Error != "failed" {
		t.Fatalf("unexpected failure: %+v", controller.failed["test-001"])
	}
}

func TestFailWorkHandlerRejectsUnassignedItem(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001","error":"failed"}`))
	response := httptest.NewRecorder()

	controller.failWorkHandler(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestFailWorkHandlerRejectsMissingError(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001"}`))
	response := httptest.NewRecorder()

	controller.failWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestStatusHandler(t *testing.T) {
	controller := newTestController()

	status := getStatus(t, controller)

	if status.Pending != 1 || status.Assigned != 0 || status.Failed != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestStatusHandlerReportsAssignedWork(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	status := getStatus(t, controller)

	if status.Pending != 0 || status.Assigned != 1 || status.Failed != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestStatusHandlerReportsFailedWork(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001","error":"failed"}`))
	response := httptest.NewRecorder()
	controller.failWorkHandler(response, request)

	status := getStatus(t, controller)

	if status.Pending != 0 || status.Assigned != 0 || status.Failed != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestStatusHandlerReportsLedgerCounts(t *testing.T) {
	controller := newTestController()
	db, err := initConfiguredLedger(context.Background(), ControllerConfig{Variables: []variable.Variable{
		{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"},
			Type:       variable.TypePath,
			Expression: filepath.Join(t.TempDir(), "ledger.sqlite"),
		},
	}})
	if err != nil {
		t.Fatalf("initialize ledger: %v", err)
	}
	defer db.Close()
	controller.ledger = db
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"attempt_id":"attempt-001",
		"workflow_definition_id":"workflow-definition-001",
		"workflow_fingerprint":"workflow-fingerprint",
		"workflow_instance_id":"workflow-instance-001",
		"step_definition_id":"step-definition-001",
		"step_fingerprint":"step-fingerprint",
		"step_instance_id":"step-instance-001",
		"work_item_fingerprint":"work-item-fingerprint",
		"input_fingerprint":"input-fingerprint",
		"output_fingerprint":"output-fingerprint",
		"code_version":"code-version",
		"started_at":"2026-06-06T12:00:00Z",
		"completed_at":"2026-06-06T12:01:00Z"
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected completion status code: %d", response.Code)
	}

	status := getStatus(t, controller)

	if status.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", status.Attempts)
	}

	if status.AttemptVariables != 14 {
		t.Fatalf("attempt_variables = %d, want 14", status.AttemptVariables)
	}
}

func TestStatusHandlerReportsPendingReuseCandidates(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
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
	})
	controller.pending = append(controller.pending, model.WorkItem{
		ID:                  "test-001",
		Type:                model.WorkItemTypeWriteDemoOutput,
		OutputFilename:      "result.txt",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	})

	status := getStatus(t, controller)

	if status.PendingReuseCandidates != 1 {
		t.Fatalf("pending_reuse_candidates = %d, want 1", status.PendingReuseCandidates)
	}
}

func TestPendingReuseDecisionReasonsCountsReasons(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
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
	})
	items := []model.WorkItem{
		{
			WorkItemFingerprint: "work-item-fingerprint",
			InputFingerprint:    "input-fingerprint",
			OutputFingerprint:   "output-fingerprint",
			CodeVersion:         "code-version",
		},
		{
			WorkItemFingerprint: "work-item-fingerprint",
			InputFingerprint:    "input-fingerprint",
			OutputFingerprint:   "output-fingerprint",
			CodeVersion:         "new-code-version",
		},
		{
			WorkItemFingerprint: "missing-fingerprint",
			InputFingerprint:    "input-fingerprint",
			OutputFingerprint:   "output-fingerprint",
			CodeVersion:         "code-version",
		},
	}

	reasons, err := controller.pendingReuseDecisionReasons(context.Background(), items)
	if err != nil {
		t.Fatalf("pendingReuseDecisionReasons() error = %v", err)
	}

	if reasons["matched_prior_completed_attempt"] != 1 {
		t.Fatalf("matched count = %d, want 1", reasons["matched_prior_completed_attempt"])
	}
	if reasons["prior_attempt_mismatch"] != 1 {
		t.Fatalf("mismatch count = %d, want 1", reasons["prior_attempt_mismatch"])
	}
	if reasons["no_prior_completed_attempt"] != 1 {
		t.Fatalf("missing count = %d, want 1", reasons["no_prior_completed_attempt"])
	}
}

func TestStatusHandlerRejectsPost(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/status", nil)
	response := httptest.NewRecorder()

	controller.statusHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkHandler(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{
		"id":"test-001",
		"type":"write_demo_output",
		"output_filename":"result.txt"
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	status := getStatus(t, controller)
	if status.Pending != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestSubmitWorkHandlerRejectsInvalidItem(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{"id":"test-001"}`))
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkHandlerRejectsDuplicateID(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{
		"id":"test-001",
		"type":"write_demo_output",
		"output_filename":"duplicate.txt"
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkHandlerRejectsGet(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/work", nil)
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkflowHandler(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "int"}},
					"Expression": "[2024, 2025]"
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		}
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	status := getStatus(t, controller)
	if status.Pending != 2 {
		t.Fatalf("unexpected status: %+v", status)
	}

	nextRequest := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	nextResponse := httptest.NewRecorder()
	controller.nextWorkHandler(nextResponse, nextRequest)

	if nextResponse.Code != http.StatusOK {
		t.Fatalf("unexpected next work status code: %d", nextResponse.Code)
	}

	var item model.WorkItem
	if err := json.NewDecoder(nextResponse.Body).Decode(&item); err != nil {
		t.Fatalf("decode next work item: %v", err)
	}

	if !strings.HasPrefix(item.WorkflowInstanceID, "cdl-instance-") {
		t.Fatalf("unexpected workflow instance id: %q", item.WorkflowInstanceID)
	}

	if item.WorkflowDefinitionID != "cdl" {
		t.Fatalf("unexpected workflow definition id: %q", item.WorkflowDefinitionID)
	}

	if !strings.HasPrefix(item.WorkflowFingerprint, "workflow:sha256:") {
		t.Fatalf("unexpected workflow fingerprint: %q", item.WorkflowFingerprint)
	}

	if item.StepDefinitionID != "download" {
		t.Fatalf("unexpected step definition id: %q", item.StepDefinitionID)
	}

	if !strings.HasPrefix(item.StepFingerprint, "step:sha256:") {
		t.Fatalf("unexpected step fingerprint: %q", item.StepFingerprint)
	}

	if item.StepInstanceID != item.WorkflowInstanceID+"-step-download" {
		t.Fatalf("unexpected step instance id: %q", item.StepInstanceID)
	}

	if !strings.HasPrefix(item.WorkItemFingerprint, "work-item:sha256:") {
		t.Fatalf("unexpected work item fingerprint: %q", item.WorkItemFingerprint)
	}

	if !strings.HasPrefix(item.InputFingerprint, "input:sha256:") {
		t.Fatalf("unexpected input fingerprint: %q", item.InputFingerprint)
	}

	if !strings.HasPrefix(item.OutputFingerprint, "output:sha256:") {
		t.Fatalf("unexpected output fingerprint: %q", item.OutputFingerprint)
	}

	if item.CodeVersion == "" || item.CodeVersion == "demo" {
		t.Fatalf("unexpected code version: %q", item.CodeVersion)
	}
}

func TestSubmitWorkflowHandlerUsesConfiguredCodeVersion(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "int"}},
					"Expression": "[2024]"
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"Name": {"Namespace": "override", "Key": "code_version"},
				"Type": {"Kind": "string"},
				"Expression": "test-version"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	nextRequest := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	nextResponse := httptest.NewRecorder()
	controller.nextWorkHandler(nextResponse, nextRequest)

	if nextResponse.Code != http.StatusOK {
		t.Fatalf("unexpected next work status code: %d", nextResponse.Code)
	}

	var item model.WorkItem
	if err := json.NewDecoder(nextResponse.Body).Decode(&item); err != nil {
		t.Fatalf("decode next work item: %v", err)
	}

	if item.CodeVersion != "test-version" {
		t.Fatalf("code version = %q, want test-version", item.CodeVersion)
	}
}

func TestWorkItemsWithRuntimeMetadataFingerprintsParameters(t *testing.T) {
	items := workItemsWithRuntimeMetadata("summary", []workflow.CompiledWorkItem{
		{
			WorkflowID: "summary",
			StepID:     "summarize",
			WorkItem: model.WorkItem{
				ID:             "summary-a",
				Type:           model.WorkItemTypeSummarizeInputFile,
				OutputFilename: "summary-a.txt",
				Parameters: model.Parameters{
					"input_path": {Type: "path", Value: "a.txt"},
				},
			},
		},
		{
			WorkflowID: "summary",
			StepID:     "summarize",
			WorkItem: model.WorkItem{
				ID:             "summary-b",
				Type:           model.WorkItemTypeSummarizeInputFile,
				OutputFilename: "summary-b.txt",
				Parameters: model.Parameters{
					"input_path": {Type: "path", Value: "b.txt"},
				},
			},
		},
	}, "test-version")

	if items[0].InputFingerprint == items[1].InputFingerprint {
		t.Fatalf("input fingerprints should differ: %s", items[0].InputFingerprint)
	}

	if items[0].OutputFingerprint == items[1].OutputFingerprint {
		t.Fatalf("output fingerprints should differ: %s", items[0].OutputFingerprint)
	}

	if items[0].WorkflowDefinitionID != "summary" {
		t.Fatalf("workflow definition id = %q, want summary", items[0].WorkflowDefinitionID)
	}

	if !strings.HasPrefix(items[0].WorkflowFingerprint, "workflow:sha256:") {
		t.Fatalf("unexpected workflow fingerprint: %q", items[0].WorkflowFingerprint)
	}

	if items[0].StepDefinitionID != "summarize" {
		t.Fatalf("step definition id = %q, want summarize", items[0].StepDefinitionID)
	}

	if !strings.HasPrefix(items[0].StepFingerprint, "step:sha256:") {
		t.Fatalf("unexpected step fingerprint: %q", items[0].StepFingerprint)
	}

	if !strings.HasPrefix(items[0].WorkItemFingerprint, "work-item:sha256:") {
		t.Fatalf("unexpected work item fingerprint: %q", items[0].WorkItemFingerprint)
	}

	if items[0].CodeVersion != "test-version" {
		t.Fatalf("code version = %q, want test-version", items[0].CodeVersion)
	}
}

func TestBuildSetting(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
		},
	}

	if got := buildSetting(info, "vcs.revision"); got != "abc123" {
		t.Fatalf("build setting = %q, want abc123", got)
	}

	if got := buildSetting(info, "missing"); got != "" {
		t.Fatalf("missing build setting = %q, want empty", got)
	}
}

func TestSubmitWorkflowHandlerStartsConfiguredWorker(t *testing.T) {
	starter := &testWorkerStarter{}
	controller := newController(nil)
	controller.worker = starter
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "int"}},
					"Expression": "[2024]"
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_target_environment"},
				"Type": {"Kind": "string"},
				"Expression": "local"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	if starter.calls != 1 {
		t.Fatalf("unexpected worker starter calls: %d", starter.calls)
	}

	if starter.target != "local" {
		t.Fatalf("unexpected worker target: %s", starter.target)
	}
}

func TestSubmitWorkflowHandlerStartsPlannedWorkerCount(t *testing.T) {
	starter := &testWorkerStarter{}
	controller := newController(nil)
	controller.worker = starter
	controller.scaleCfg = WorkerScaleConfig{MinCount: 2, MaxCount: 2, CountPerStart: 2}
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "int"}},
					"Expression": "[2024, 2025]"
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_target_environment"},
				"Type": {"Kind": "string"},
				"Expression": "local"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	if starter.calls != 2 {
		t.Fatalf("unexpected worker starter calls: %d", starter.calls)
	}
}

func TestSubmitWorkflowHandlerUsesSubmittedWorkerScaleConfig(t *testing.T) {
	starter := &testWorkerStarter{}
	controller := newController(nil)
	controller.worker = starter
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "int"}},
					"Expression": "[2024, 2025]"
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_target_environment"},
				"Type": {"Kind": "string"},
				"Expression": "local"
			},
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_min_count"},
				"Type": {"Kind": "int"},
				"Expression": "2"
			},
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_max_count"},
				"Type": {"Kind": "int"},
				"Expression": "2"
			},
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_count_per_start"},
				"Type": {"Kind": "int"},
				"Expression": "2"
			},
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_min_elapsed_time_between_starts"},
				"Type": {"Kind": "string"},
				"Expression": "0s"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	if starter.calls != 2 {
		t.Fatalf("unexpected worker starter calls: %d", starter.calls)
	}
}

func TestSubmitWorkflowHandlerRejectsInvalidWorkerScaleConfig(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "int"}},
					"Expression": "[2024]"
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_target_environment"},
				"Type": {"Kind": "string"},
				"Expression": "local"
			},
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_max_count"},
				"Type": {"Kind": "string"},
				"Expression": "two"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkflowHandlerWaitsForWorkerClaimBeforeOrganicScaleUp(t *testing.T) {
	starter := &testWorkerStarter{}
	controller := newController(nil)
	controller.worker = starter
	controller.scaleCfg = WorkerScaleConfig{MaxCount: 2, CountPerStart: 1}

	submitWorkflowYears(t, controller, `[2024]`)
	submitWorkflowYears(t, controller, `[2025]`)

	if starter.calls != 1 {
		t.Fatalf("unexpected worker starter calls before claim: %d", starter.calls)
	}

	assignNextWork(t, controller)
	submitWorkflowYears(t, controller, `[2026]`)

	if starter.calls != 2 {
		t.Fatalf("unexpected worker starter calls after claim: %d", starter.calls)
	}
}

func TestSubmitWorkflowHandlerRejectsInvalidWorkerTargetType(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "int"}},
					"Expression": "[2024]"
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_target_environment"},
				"Type": {"Kind": "int"},
				"Expression": "1"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkflowHandlerRejectsDuplicateGeneratedID(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "string"}},
					"Expression": "[\"001\"]"
				}
			],
			"Steps": [
				{
					"ID": "test",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"IDPrefix": "test",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		}
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func submitWorkflowYears(t *testing.T, controller *Controller, years string) {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"Name": {"Namespace": "workflow", "Key": "years"},
					"Type": {"Kind": "list", "Element": {"Kind": "int"}},
					"Expression": `+strconv.Quote(years)+`
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"Name": {"Namespace": "worker_config", "Key": "worker_target_environment"},
				"Type": {"Kind": "string"},
				"Expression": "local"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

type testWorkerStarter struct {
	calls  int
	target string
}

func (s *testWorkerStarter) StartWorker(targetEnvironment string, resolver variable.Resolver) error {
	s.calls++
	s.target = targetEnvironment
	return nil
}

func TestSubmitWorkflowHandlerRejectsInvalidPayload(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{"workflow": {}}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkflowHandlerRejectsGet(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/workflow", nil)
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestShutdownHandler(t *testing.T) {
	called := make(chan struct{}, 1)
	controller := newController(nil)
	controller.shutdown = func(context.Context) error {
		called <- struct{}{}
		return nil
	}

	request := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	response := httptest.NewRecorder()

	controller.shutdownHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	<-called
}

func TestShutdownHandlerRejectsGet(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/shutdown", nil)
	response := httptest.NewRecorder()

	controller.shutdownHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestShutdownHandlerRejectsUnavailableShutdown(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	response := httptest.NewRecorder()

	controller.shutdownHandler(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func newTestController() *Controller {
	return newController([]model.WorkItem{
		{
			ID:             "test-001",
			Type:           model.WorkItemTypeWriteDemoOutput,
			OutputFilename: "result.txt",
		},
	})
}

func newControllerWithCompletedAttempt(t *testing.T, completion model.WorkCompletion) *Controller {
	t.Helper()

	controller := newController(nil)
	db, err := initConfiguredLedger(context.Background(), ControllerConfig{Variables: []variable.Variable{
		{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"},
			Type:       variable.TypePath,
			Expression: filepath.Join(t.TempDir(), "ledger.sqlite"),
		},
	}})
	if err != nil {
		t.Fatalf("initialize ledger: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	controller.ledger = db

	attempt, _, err := attemptFromCompletion(completion)
	if err != nil {
		t.Fatalf("build attempt: %v", err)
	}
	if err := controller.recordAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("record attempt: %v", err)
	}

	return controller
}

func getStatus(t *testing.T, controller *Controller) model.ControllerStatus {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/status", nil)
	response := httptest.NewRecorder()
	controller.statusHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	var status model.ControllerStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}

	return status
}

func withAttemptStatus(attempt ledger.Attempt, status ledger.AttemptStatus) ledger.Attempt {
	attempt.Status = status
	return attempt
}

func withWorkItemFingerprint(item model.WorkItem, fingerprint string) model.WorkItem {
	item.WorkItemFingerprint = fingerprint
	return item
}

func withInputFingerprint(item model.WorkItem, fingerprint string) model.WorkItem {
	item.InputFingerprint = fingerprint
	return item
}

func withOutputFingerprint(item model.WorkItem, fingerprint string) model.WorkItem {
	item.OutputFingerprint = fingerprint
	return item
}

func withCodeVersion(item model.WorkItem, codeVersion string) model.WorkItem {
	item.CodeVersion = codeVersion
	return item
}

func assignNextWork(t *testing.T, controller *Controller) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()
	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected assignment status code: %d", response.Code)
	}
}

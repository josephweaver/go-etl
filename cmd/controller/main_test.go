package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
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
		"workflow_instance_id":"workflow-instance-001",
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
	if count != 10 {
		t.Fatalf("attempt variable count = %d, want 10", count)
	}

	var valueJSON string
	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'runtime' AND name = 'workflow_instance_id'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query workflow instance variable: %v", err)
	}
	if valueJSON != `"workflow-instance-001"` {
		t.Fatalf("workflow_instance_id value_json = %q", valueJSON)
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
		"workflow_instance_id":"workflow-instance-001",
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

	if status.AttemptVariables != 10 {
		t.Fatalf("attempt_variables = %d, want 10", status.AttemptVariables)
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

	if item.StepInstanceID != item.WorkflowInstanceID+"-step-download" {
		t.Fatalf("unexpected step instance id: %q", item.StepInstanceID)
	}

	if item.WorkItemFingerprint != "work-item:"+item.ID {
		t.Fatalf("unexpected work item fingerprint: %q", item.WorkItemFingerprint)
	}

	if item.CodeVersion != "demo" {
		t.Fatalf("unexpected code version: %q", item.CodeVersion)
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

func assignNextWork(t *testing.T, controller *Controller) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()
	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected assignment status code: %d", response.Code)
	}
}

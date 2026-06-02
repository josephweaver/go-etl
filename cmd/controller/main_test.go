package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"goetl/internal/model"
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

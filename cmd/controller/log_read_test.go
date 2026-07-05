package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"goetl/internal/model"
)

func TestSubmissionLogsHandlerReturnsEmptyEntriesForKnownSubmissionWithoutLogs(t *testing.T) {
	t.Parallel()

	controller := newController()
	controller.logRootPath = t.TempDir()
	controller.logReadDefaultTail = 3
	controller.logReadMaxTail = 10
	controller.workflowStore = openTestWorkflowExecutionStore(t)
	defer controller.workflowStore.Close()

	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, controller.workflowStore)

	request := httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/logs", nil)
	response := httptest.NewRecorder()

	controller.submissionLogsHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", response.Code)
	}

	var got submissionLogResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SubmissionID != run.ID {
		t.Fatalf("submission_id = %q, want %q", got.SubmissionID, run.ID)
	}
	if got.Tail != 3 {
		t.Fatalf("tail = %d, want 3", got.Tail)
	}
	if got.Truncated {
		t.Fatalf("truncated = %t, want false", got.Truncated)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("entries = %d, want 0", len(got.Entries))
	}
}

func TestSubmissionLogsHandlerRejectsUnknownSubmission(t *testing.T) {
	t.Parallel()

	controller := newController()
	controller.logReadDefaultTail = 3
	controller.logReadMaxTail = 10
	controller.logRootPath = t.TempDir()
	controller.workflowStore = openTestWorkflowExecutionStore(t)
	defer controller.workflowStore.Close()

	request := httptest.NewRequest(http.MethodGet, "/submissions/missing/logs", nil)
	response := httptest.NewRecorder()
	controller.submissionLogsHandler(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", response.Code)
	}
}

func TestSubmissionLogsHandlerParsesQueryFiltersAndTail(t *testing.T) {
	t.Parallel()

	controller := newController()
	controller.logRootPath = t.TempDir()
	controller.logReadDefaultTail = 3
	controller.logReadMaxTail = 10
	controller.workflowStore = openTestWorkflowExecutionStore(t)
	defer controller.workflowStore.Close()

	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, controller.workflowStore)

	sink, err := newFilesystemLogSink(controller.logRootPath, string(model.LogLevelDebug))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}

	if err := sink.Write(model.LogObservation{
		SubmissionID: run.ID,
		AttemptID:    "attempt-1",
		Component:    "worker",
		Stream:       "stdout",
		Level:        model.LogLevelInfo,
		Timestamp:    "2026-07-05T12:00:00Z",
		Message:      "info stdout",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := sink.Write(model.LogObservation{
		SubmissionID: run.ID,
		AttemptID:    "attempt-1",
		Component:    "worker",
		Stream:       "stderr",
		Level:        model.LogLevelWarn,
		Timestamp:    "2026-07-05T12:00:01Z",
		Message:      "warn stderr",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := sink.Write(model.LogObservation{
		SubmissionID: run.ID,
		AttemptID:    "attempt-2",
		Component:    "worker",
		Stream:       "stdout",
		Level:        model.LogLevelError,
		Timestamp:    "2026-07-05T12:00:02Z",
		Message:      "error stdout",
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/logs?level=warn&stream=stderr", nil)
	response := httptest.NewRecorder()
	controller.submissionLogsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", response.Code)
	}
	var warnOnly submissionLogResponse
	if err := json.NewDecoder(response.Body).Decode(&warnOnly); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(warnOnly.Entries) != 1 {
		t.Fatalf("warn entries = %d, want 1", len(warnOnly.Entries))
	}
	if warnOnly.Entries[0].Message != "warn stderr" {
		t.Fatalf("entry message = %q, want %q", warnOnly.Entries[0].Message, "warn stderr")
	}

	request = httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/logs?attempt_id=attempt-1&tail=1", nil)
	response = httptest.NewRecorder()
	controller.submissionLogsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", response.Code)
	}
	var attempt1 submissionLogResponse
	if err := json.NewDecoder(response.Body).Decode(&attempt1); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(attempt1.Entries) != 1 {
		t.Fatalf("attempt entries = %d, want 1", len(attempt1.Entries))
	}
	if attempt1.Entries[0].AttemptID != "attempt-1" {
		t.Fatalf("entry attempt_id = %q, want attempt-1", attempt1.Entries[0].AttemptID)
	}
}

func TestSubmissionLogsHandlerRejectsInvalidQueries(t *testing.T) {
	t.Parallel()

	controller := newController()
	controller.logRootPath = t.TempDir()
	controller.logReadDefaultTail = 3
	controller.logReadMaxTail = 3
	controller.workflowStore = openTestWorkflowExecutionStore(t)
	defer controller.workflowStore.Close()

	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, controller.workflowStore)

	request := httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/logs?tail=10", nil)
	response := httptest.NewRecorder()
	controller.submissionLogsHandler(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/logs?level=critical", nil)
	response = httptest.NewRecorder()
	controller.submissionLogsHandler(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/logs?stream=panic", nil)
	response = httptest.NewRecorder()
	controller.submissionLogsHandler(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", response.Code)
	}
}

func TestSubmissionHandlerRoutesUnknownSuffixTo404(t *testing.T) {
	t.Parallel()

	controller := newController()
	controller.logRootPath = t.TempDir()
	controller.logReadDefaultTail = 3
	controller.logReadMaxTail = 10
	controller.workflowStore = openTestWorkflowExecutionStore(t)
	defer controller.workflowStore.Close()

	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, controller.workflowStore)

	request := httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/bad", nil)
	response := httptest.NewRecorder()
	controller.submissionHandler(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", response.Code)
	}
}

func TestSubmissionLogsHandlerReturnsTruncationMetadata(t *testing.T) {
	t.Parallel()

	controller := newController()
	controller.logRootPath = t.TempDir()
	controller.logReadDefaultTail = 2
	controller.logReadMaxTail = 10
	controller.workflowStore = openTestWorkflowExecutionStore(t)
	defer controller.workflowStore.Close()

	ctx := context.Background()
	run := insertTestPersistenceRunWithStage(t, ctx, controller.workflowStore)

	sink, err := newFilesystemLogSink(controller.logRootPath, string(model.LogLevelDebug))
	if err != nil {
		t.Fatalf("newFilesystemLogSink() error = %v", err)
	}
	for index := 0; index < 4; index++ {
		if err := sink.Write(model.LogObservation{
			SubmissionID: run.ID,
			Component:    "worker",
			Level:        model.LogLevelInfo,
			Timestamp:    "2026-07-05T12:00:0" + strconv.Itoa(index) + "Z",
			Message:      "m" + strconv.Itoa(index),
		}); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/submissions/"+run.ID+"/logs?tail=2", nil)
	response := httptest.NewRecorder()
	controller.submissionLogsHandler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", response.Code)
	}

	var got submissionLogResponse
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Tail != 2 {
		t.Fatalf("tail = %d, want 2", got.Tail)
	}
	if !got.Truncated {
		t.Fatalf("truncated = %t, want true", got.Truncated)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(got.Entries))
	}
	if got.Entries[0].Message != "m2" || got.Entries[1].Message != "m3" {
		t.Fatalf("entries = %#v, want m2,m3", got.Entries)
	}
}

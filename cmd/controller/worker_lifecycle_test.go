package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"goetl/internal/model"
	"goetl/internal/persistence"
)

func TestRegisterWorkerHandlerCreatesActiveSession(t *testing.T) {
	controller, store := testWorkerLifecycleController(t)
	controller.workerExecutor.reserveInflightStarts(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 1)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewBufferString(`{"execution_handle":"pid-123"}`))

	controller.registerWorkerHandler(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var registration model.WorkerRegistration
	if err := json.NewDecoder(response.Body).Decode(&registration); err != nil {
		t.Fatalf("decode registration response: %v", err)
	}
	if registration.WorkerID == "" || registration.WorkerSessionID == "" {
		t.Fatalf("registration missing ids: %+v", registration)
	}
	if registration.HeartbeatIntervalSeconds != 60 || registration.DeadAfterSeconds != 300 {
		t.Fatalf("heartbeat policy = %d/%d, want 60/300", registration.HeartbeatIntervalSeconds, registration.DeadAfterSeconds)
	}
	session, found, err := store.GetWorkerSession(context.Background(), registration.WorkerID, registration.WorkerSessionID)
	if err != nil {
		t.Fatalf("GetWorkerSession() error = %v", err)
	}
	if !found {
		t.Fatal("registered worker session not found")
	}
	if session.Status != persistence.WorkerSessionStatusActive || session.ExecutionHandle != "pid-123" {
		t.Fatalf("session = %+v, want active pid-123", session)
	}
	if got := len(controller.workerExecutor.Snapshot().InflightStarts); got != 0 {
		t.Fatalf("inflight starts = %d, want 0", got)
	}
}

func TestWorkerLifecycleSignalFiresForRegisterAndStopOnly(t *testing.T) {
	controller, store := testWorkerLifecycleController(t)
	var signals []string
	controller.workerStateChanged = func(reason string) {
		signals = append(signals, reason)
	}

	registerResponse := httptest.NewRecorder()
	controller.registerWorkerHandler(registerResponse, httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewBufferString(`{}`)))
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d; body=%s", registerResponse.Code, http.StatusCreated, registerResponse.Body.String())
	}
	var registration model.WorkerRegistration
	if err := json.NewDecoder(registerResponse.Body).Decode(&registration); err != nil {
		t.Fatalf("decode registration response: %v", err)
	}

	heartbeatBody := `{"worker_id":"` + registration.WorkerID + `","worker_session_id":"` + registration.WorkerSessionID + `"}`
	heartbeatResponse := httptest.NewRecorder()
	controller.heartbeatWorkerHandler(heartbeatResponse, httptest.NewRequest(http.MethodPost, "/workers/heartbeat", bytes.NewBufferString(heartbeatBody)))
	if heartbeatResponse.Code != http.StatusNoContent {
		t.Fatalf("heartbeat status = %d, want %d; body=%s", heartbeatResponse.Code, http.StatusNoContent, heartbeatResponse.Body.String())
	}

	stopBody := `{"worker_id":"` + registration.WorkerID + `","worker_session_id":"` + registration.WorkerSessionID + `","reason":"test_complete"}`
	stopResponse := httptest.NewRecorder()
	controller.stopWorkerHandler(stopResponse, httptest.NewRequest(http.MethodPost, "/workers/stop", bytes.NewBufferString(stopBody)))
	if stopResponse.Code != http.StatusNoContent {
		t.Fatalf("stop status = %d, want %d; body=%s", stopResponse.Code, http.StatusNoContent, stopResponse.Body.String())
	}

	session, found, err := store.GetWorkerSession(context.Background(), registration.WorkerID, registration.WorkerSessionID)
	if err != nil {
		t.Fatalf("GetWorkerSession() error = %v", err)
	}
	if !found || session.Status != persistence.WorkerSessionStatusStopped || session.EndReason != "test_complete" {
		t.Fatalf("session = %+v found=%v, want stopped test_complete", session, found)
	}
	want := []string{"worker_registered", "worker_stopped"}
	if len(signals) != len(want) {
		t.Fatalf("signals = %v, want %v", signals, want)
	}
	for i := range want {
		if signals[i] != want[i] {
			t.Fatalf("signals = %v, want %v", signals, want)
		}
	}
}

func TestHeartbeatWorkerHandlerRejectsUnknownSession(t *testing.T) {
	controller, _ := testWorkerLifecycleController(t)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/workers/heartbeat", bytes.NewBufferString(`{"worker_id":"worker-404","worker_session_id":"session-404"}`))

	controller.heartbeatWorkerHandler(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
}

func TestStopWorkerHandlerIsIdempotentForStoppedSession(t *testing.T) {
	controller, store := testWorkerLifecycleController(t)
	_, err := store.RegisterWorkerSession(context.Background(), persistence.RegisterWorkerSessionRequest{
		WorkerID:     "worker-001",
		SessionID:    "session-001",
		RegisteredAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("RegisterWorkerSession() error = %v", err)
	}
	body := `{"worker_id":"worker-001","worker_session_id":"session-001","reason":"process_exit"}`

	first := httptest.NewRecorder()
	controller.stopWorkerHandler(first, httptest.NewRequest(http.MethodPost, "/workers/stop", bytes.NewBufferString(body)))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first stop status = %d, want %d; body=%s", first.Code, http.StatusNoContent, first.Body.String())
	}
	second := httptest.NewRecorder()
	controller.stopWorkerHandler(second, httptest.NewRequest(http.MethodPost, "/workers/stop", bytes.NewBufferString(body)))
	if second.Code != http.StatusNoContent {
		t.Fatalf("second stop status = %d, want %d; body=%s", second.Code, http.StatusNoContent, second.Body.String())
	}
}

func TestWorkerLifecycleHandlersRejectBadMethodAndOversizedBody(t *testing.T) {
	controller, _ := testWorkerLifecycleController(t)
	controller.maxRequestBytes = 2

	wrongMethod := httptest.NewRecorder()
	controller.registerWorkerHandler(wrongMethod, httptest.NewRequest(http.MethodGet, "/workers/register", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d, want %d", wrongMethod.Code, http.StatusMethodNotAllowed)
	}

	tooLarge := httptest.NewRecorder()
	controller.registerWorkerHandler(tooLarge, httptest.NewRequest(http.MethodPost, "/workers/register", bytes.NewBufferString(`{"x":1}`)))
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d, want %d", tooLarge.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestNextWorkHandlerRequiresWorkerSessionHeaders(t *testing.T) {
	controller, _ := testWorkerLifecycleController(t)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestNextWorkHandlerRejectsExpiredWorkerSession(t *testing.T) {
	controller, store := testWorkerLifecycleController(t)
	if _, err := store.RegisterWorkerSession(context.Background(), persistence.RegisterWorkerSessionRequest{
		WorkerID:     "worker-001",
		SessionID:    "session-001",
		RegisteredAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("RegisterWorkerSession() error = %v", err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	setTestWorkerSessionHeaders(request, "worker-001", "session-001")

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusConflict, response.Body.String())
	}
}

func TestNextWorkHandlerReturnsNoContentForLiveSessionWithNoWork(t *testing.T) {
	controller, store := testWorkerLifecycleController(t)
	if _, err := store.RegisterWorkerSession(context.Background(), persistence.RegisterWorkerSessionRequest{
		WorkerID:     "worker-001",
		SessionID:    "session-001",
		RegisteredAt: "2999-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("RegisterWorkerSession() error = %v", err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	setTestWorkerSessionHeaders(request, "worker-001", "session-001")

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
}

func setTestWorkerSessionHeaders(request *http.Request, workerID string, sessionID string) {
	request.Header.Set(workerIDHeader, workerID)
	request.Header.Set(workerSessionIDHeader, sessionID)
}

func testWorkerLifecycleController(t *testing.T) (*Controller, *persistence.Store) {
	t.Helper()
	store, err := persistence.OpenStore(context.Background(), persistence.Config{
		Driver:           persistence.DriverSQLite,
		ConnectionString: filepath.Join(t.TempDir(), "workflow.sqlite"),
	})
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	controller := newController()
	controller.workflowStore = store
	controller.maxRequestBytes = 1024
	return controller, store
}

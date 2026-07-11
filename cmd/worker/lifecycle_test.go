package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"goetl/internal/model"
)

func TestRegisterWorkerPostsRequestAndReturnsSession(t *testing.T) {
	const token = "worker-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/workers/register" {
			t.Fatalf("request = %s %s, want POST /workers/register", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("authorization = %q, want bearer token", got)
		}
		var request model.WorkerRegistrationRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode registration request: %v", err)
		}
		if request.ExecutionHandle != "pid-123" || request.ExecutionEnvironment != "local" {
			t.Fatalf("request = %+v, want execution handle and environment", request)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(model.WorkerRegistration{
			WorkerID:                 "worker-001",
			WorkerSessionID:          "session-001",
			HeartbeatIntervalSeconds: 30,
			DeadAfterSeconds:         120,
		})
	}))
	defer server.Close()

	client := testLifecycleWorkerClient(t, server.URL, token)
	session, err := client.RegisterWorker(context.Background(), model.WorkerRegistrationRequest{
		ExecutionHandle:      "pid-123",
		ExecutionEnvironment: "local",
	})
	if err != nil {
		t.Fatalf("RegisterWorker() error = %v", err)
	}
	if session.WorkerID != "worker-001" || session.WorkerSessionID != "session-001" {
		t.Fatalf("session ids = %+v, want worker-001/session-001", session)
	}
	if session.HeartbeatInterval != 30*time.Second || session.DeadAfter != 120*time.Second {
		t.Fatalf("session timing = %s/%s, want 30s/120s", session.HeartbeatInterval, session.DeadAfter)
	}
}

func TestRegisterWorkerRejectsInvalidControllerResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(model.WorkerRegistration{
			WorkerID:                 "worker-001",
			WorkerSessionID:          "session-001",
			HeartbeatIntervalSeconds: 60,
			DeadAfterSeconds:         60,
		})
	}))
	defer server.Close()

	client := testLifecycleWorkerClient(t, server.URL, "worker-token")
	if _, err := client.RegisterWorker(context.Background(), model.WorkerRegistrationRequest{}); err == nil {
		t.Fatal("RegisterWorker() error = nil, want invalid timing error")
	}
}

func TestHeartbeatWorkerPostsSessionAndMapsConflict(t *testing.T) {
	var heartbeatSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workers/heartbeat" {
			t.Fatalf("path = %s, want /workers/heartbeat", r.URL.Path)
		}
		var request model.WorkerHeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode heartbeat request: %v", err)
		}
		if request.WorkerID != "worker-001" || request.WorkerSessionID != "session-001" {
			t.Fatalf("request = %+v, want worker/session ids", request)
		}
		heartbeatSeen = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := testLifecycleWorkerClient(t, server.URL, "worker-token")
	if err := client.HeartbeatWorker(context.Background(), testWorkerSession()); err != nil {
		t.Fatalf("HeartbeatWorker() error = %v", err)
	}
	if !heartbeatSeen {
		t.Fatal("heartbeat request was not observed")
	}

	conflict := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "worker session is not active", http.StatusConflict)
	}))
	defer conflict.Close()
	client = testLifecycleWorkerClient(t, conflict.URL, "worker-token")
	if err := client.HeartbeatWorker(context.Background(), testWorkerSession()); !errors.Is(err, ErrWorkerSessionNotActive) {
		t.Fatalf("HeartbeatWorker() error = %v, want ErrWorkerSessionNotActive", err)
	}
}

func TestStopWorkerPostsReasonAndMapsConflict(t *testing.T) {
	var stopSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/workers/stop" {
			t.Fatalf("request = %s %s, want POST /workers/stop", r.Method, r.URL.Path)
		}
		var request model.WorkerStopRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode stop request: %v", err)
		}
		if request.WorkerID != "worker-001" || request.WorkerSessionID != "session-001" || request.Reason != "no_work" {
			t.Fatalf("request = %+v, want worker/session ids and no_work", request)
		}
		stopSeen = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := testLifecycleWorkerClient(t, server.URL, "worker-token")
	if err := client.StopWorker(context.Background(), testWorkerSession(), "no_work"); err != nil {
		t.Fatalf("StopWorker() error = %v", err)
	}
	if !stopSeen {
		t.Fatal("stop request was not observed")
	}

	conflict := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "worker session is not active", http.StatusConflict)
	}))
	defer conflict.Close()
	client = testLifecycleWorkerClient(t, conflict.URL, "worker-token")
	if err := client.StopWorker(context.Background(), testWorkerSession(), "no_work"); !errors.Is(err, ErrWorkerSessionNotActive) {
		t.Fatalf("StopWorker() error = %v, want ErrWorkerSessionNotActive", err)
	}
}

func TestWorkerLifecycleMethodsValidateInputs(t *testing.T) {
	client := WorkerControllerClient{}
	if err := client.HeartbeatWorker(context.Background(), WorkerSession{}); err == nil {
		t.Fatal("HeartbeatWorker() error = nil, want missing identity error")
	}
	if err := client.StopWorker(context.Background(), testWorkerSession(), ""); err == nil {
		t.Fatal("StopWorker() error = nil, want missing reason error")
	}
}

func testWorkerSession() WorkerSession {
	return WorkerSession{
		WorkerID:          "worker-001",
		WorkerSessionID:   "session-001",
		HeartbeatInterval: time.Minute,
		DeadAfter:         5 * time.Minute,
	}
}

func testLifecycleWorkerClient(t *testing.T, controllerURL string, token string) WorkerControllerClient {
	t.Helper()
	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	client, err := NewWorkerControllerClient(Config{
		ControllerURL:       controllerURL,
		ControllerTokenFile: tokenFile,
	})
	if err != nil {
		t.Fatalf("NewWorkerControllerClient() error = %v", err)
	}
	return client
}

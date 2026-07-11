package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"goetl/internal/model"
)

func TestRunWorkerLoop(t *testing.T) {
	var mu sync.Mutex
	pending := []model.WorkItem{
		{ID: "test-001", Type: model.WorkItemTypeWriteDemoOutput, OutputFilename: "first.txt"},
		{ID: "test-002", Type: model.WorkItemTypeWriteDemoOutput, OutputFilename: "second.txt"},
	}
	var completed []string
	var stops []string
	var order []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			order = append(order, "register")
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			assertTestWorkerSessionHeaders(t, r)
			order = append(order, "next")
			if len(pending) == 0 {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			item := pending[0]
			pending = pending[1:]
			json.NewEncoder(w).Encode(item)
		case r.Method == http.MethodPost && r.URL.Path == "/work/complete":
			var completion model.WorkCompletion
			if err := json.NewDecoder(r.Body).Decode(&completion); err != nil {
				t.Fatalf("decode completion: %v", err)
			}
			assertTestWorkerSessionHeaders(t, r)

			completed = append(completed, completion.ID)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/workers/stop":
			var stop model.WorkerStopRequest
			if err := json.NewDecoder(r.Body).Decode(&stop); err != nil {
				t.Fatalf("decode stop: %v", err)
			}
			stops = append(stops, stop.Reason)
			order = append(order, "stop")
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := newTestWorker(t)
	worker.Config.ControllerURL = server.URL

	if err := runWorkerLoop(worker); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(completed) != 2 {
		t.Fatalf("unexpected completed count: %d", len(completed))
	}

	if completed[0] != "test-001" || completed[1] != "test-002" {
		t.Fatalf("unexpected completed items: %v", completed)
	}
	if len(stops) != 1 || stops[0] != "no_work" {
		t.Fatalf("stops = %v, want [no_work]", stops)
	}
	if len(order) < 2 || order[0] != "register" || order[1] != "next" {
		t.Fatalf("order = %v, want registration before first claim", order)
	}
}

func TestWorkerConfigPath(t *testing.T) {
	if got := workerConfigPath([]string{"worker"}); got != "demo-config.json" {
		t.Fatalf("unexpected default config path: %s", got)
	}

	if got := workerConfigPath([]string{"worker", "custom.json"}); got != "custom.json" {
		t.Fatalf("unexpected custom config path: %s", got)
	}
}

func TestRunWorkerLoopReportsFailure(t *testing.T) {
	var failure model.WorkFailure
	var stops []string
	nextCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			assertTestWorkerSessionHeaders(t, r)
			nextCalls++
			json.NewEncoder(w).Encode(model.WorkItem{
				ID:             "test-001",
				Type:           "unknown",
				OutputFilename: "result.txt",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/work/fail":
			if err := json.NewDecoder(r.Body).Decode(&failure); err != nil {
				t.Fatalf("decode failure: %v", err)
			}
			assertTestWorkerSessionHeaders(t, r)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/workers/stop":
			var stop model.WorkerStopRequest
			if err := json.NewDecoder(r.Body).Decode(&stop); err != nil {
				t.Fatalf("decode stop: %v", err)
			}
			stops = append(stops, stop.Reason)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := newTestWorker(t)
	worker.Config.ControllerURL = server.URL

	if err := runWorkerLoop(worker); err == nil {
		t.Fatal("expected an error")
	}

	if nextCalls != 1 {
		t.Fatalf("unexpected next work calls: %d", nextCalls)
	}

	if failure.ID != "test-001" {
		t.Fatalf("unexpected failure id: %q", failure.ID)
	}

	if failure.Error == "" {
		t.Fatal("expected failure error")
	}
	if len(stops) != 1 || stops[0] != "worker_error" {
		t.Fatalf("stops = %v, want [worker_error]", stops)
	}
}

func TestRunWorkerLoopStopsBeforeNoWorkExit(t *testing.T) {
	var sawStop bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			assertTestWorkerSessionHeaders(t, r)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/workers/stop":
			var stop model.WorkerStopRequest
			if err := json.NewDecoder(r.Body).Decode(&stop); err != nil {
				t.Fatalf("decode stop: %v", err)
			}
			if stop.Reason != "no_work" {
				t.Fatalf("stop reason = %q, want no_work", stop.Reason)
			}
			sawStop = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := newTestWorker(t)
	worker.Config.ControllerURL = server.URL

	if err := runWorkerLoop(worker); err != nil {
		t.Fatalf("runWorkerLoop() error = %v", err)
	}
	if !sawStop {
		t.Fatal("worker did not send no_work stop")
	}
}

func TestRunWorkerLoopStopsClaimingAfterHeartbeatRejected(t *testing.T) {
	nextStarted := make(chan struct{})
	allowNext := make(chan struct{})
	var nextStartedOnce sync.Once
	var completed int
	var failed int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodPost && r.URL.Path == "/workers/heartbeat":
			http.Error(w, "worker session is not active", http.StatusConflict)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			assertTestWorkerSessionHeaders(t, r)
			nextStartedOnce.Do(func() {
				close(nextStarted)
			})
			<-allowNext
			_ = json.NewEncoder(w).Encode(model.WorkItem{
				ID:             "test-001",
				Type:           model.WorkItemTypeWriteDemoOutput,
				OutputFilename: "result.txt",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/work/complete":
			completed++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/work/fail":
			failed++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	worker := newTestWorker(t)
	worker.Config.ControllerURL = server.URL
	clock := newFakeWorkerLifecycleClock()
	worker.LifecycleClock = clock
	result := make(chan error, 1)
	go func() {
		result <- runWorkerLoop(worker)
	}()

	clock.waitForTicker()
	<-nextStarted
	clock.tick()
	close(allowNext)

	err := <-result
	if !errors.Is(err, ErrWorkerSessionNotActive) {
		t.Fatalf("runWorkerLoop() error = %v, want ErrWorkerSessionNotActive", err)
	}
	if completed != 0 || failed != 0 {
		t.Fatalf("terminal reports completed=%d failed=%d, want none", completed, failed)
	}
}

func assertTestWorkerSessionHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get(workerIDHeader); got != "worker-001" {
		t.Fatalf("%s = %q, want worker-001", workerIDHeader, got)
	}
	if got := r.Header.Get(workerSessionIDHeader); got != "session-001" {
		t.Fatalf("%s = %q, want session-001", workerSessionIDHeader, got)
	}
}

func writeTestWorkerRegistration(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(model.WorkerRegistration{
		WorkerID:                 "worker-001",
		WorkerSessionID:          "session-001",
		HeartbeatIntervalSeconds: 3600,
		DeadAfterSeconds:         7200,
	}); err != nil {
		t.Fatalf("encode registration: %v", err)
	}
}

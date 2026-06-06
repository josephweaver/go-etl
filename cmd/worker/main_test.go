package main

import (
	"encoding/json"
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
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

			completed = append(completed, completion.ID)
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
	nextCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
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
}

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"goetl/internal/model"
)

func TestReportWorkFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/work/fail" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var failure model.WorkFailure
		if err := json.NewDecoder(r.Body).Decode(&failure); err != nil {
			t.Fatalf("decode failure: %v", err)
		}

		if failure.ID != "test-001" || failure.Error != "failed" {
			t.Fatalf("unexpected failure: %+v", failure)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := reportWorkFailed(server.URL, "test-001", errors.New("failed")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportWorkComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/work/complete" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		var completion model.WorkCompletion
		if err := json.NewDecoder(r.Body).Decode(&completion); err != nil {
			t.Fatalf("decode completion: %v", err)
		}

		if completion.ID != "test-001" {
			t.Fatalf("unexpected id: %q", completion.ID)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := reportWorkComplete(server.URL, "test-001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportWorkCompleteRejectsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := reportWorkComplete(server.URL, "test-001"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchWorkItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/work/next" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "test-001",
			"type": "write_demo_output",
			"output_filename": "result.txt"
		}`))
	}))
	defer server.Close()

	item, hasWork, err := fetchWorkItem(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasWork {
		t.Fatal("expected work item")
	}

	if item.ID != "test-001" {
		t.Fatalf("unexpected id: %q", item.ID)
	}

	if item.Type != model.WorkItemTypeWriteDemoOutput {
		t.Fatalf("unexpected type: %q", item.Type)
	}

	if item.OutputFilename != "result.txt" {
		t.Fatalf("unexpected output filename: %q", item.OutputFilename)
	}
}

func TestFetchWorkItemRejectsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, _, err := fetchWorkItem(server.URL); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchWorkItemRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":`))
	}))
	defer server.Close()

	if _, _, err := fetchWorkItem(server.URL); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchWorkItemRejectsInvalidItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"id": "test-001",
			"type": "write_demo_output",
			"output_filename": "../outside.txt"
		}`))
	}))
	defer server.Close()

	if _, _, err := fetchWorkItem(server.URL); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFetchWorkItemReturnsNoWork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	item, hasWork, err := fetchWorkItem(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hasWork {
		t.Fatalf("unexpected work item: %+v", item)
	}
}

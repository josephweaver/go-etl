package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestSendLogObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/observations/logs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := LogClient{ControllerURL: server.URL}
	err := client.SendLogObservation(validLogObservation(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendLogObservationUsesControllerClientAuth(t *testing.T) {
	const sentinel = "goetl-worker-controller-token-sentinel-006"
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") == "Bearer "+sentinel
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	controller, err := NewWorkerControllerClient(Config{
		ControllerURL:       server.URL,
		ControllerTokenFile: tokenFile,
	})
	if err != nil {
		t.Fatalf("NewWorkerControllerClient() error = %v", err)
	}

	client := LogClient{Controller: controller}
	if err := client.SendLogObservation(validLogObservation(t)); err != nil {
		t.Fatalf("SendLogObservation() error = %v", err)
	}
	if !sawAuth {
		t.Fatal("expected log observation request to include bearer token")
	}
}

func TestSendLogObservationRejectsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := LogClient{ControllerURL: server.URL}
	err := client.SendLogObservation(validLogObservation(t))
	if err == nil {
		t.Fatal("expected an error")
	}

	var deliveryErr *LogDeliveryError
	if !errors.As(err, &deliveryErr) {
		t.Fatalf("expected log delivery error, got %T: %v", err, err)
	}
}

func TestSendLogObservationRejectsTransportError(t *testing.T) {
	client := LogClient{ControllerURL: "http://127.0.0.1:0"}
	err := client.SendLogObservation(validLogObservation(t))
	if err == nil {
		t.Fatal("expected an error")
	}

	var deliveryErr *LogDeliveryError
	if !errors.As(err, &deliveryErr) {
		t.Fatalf("expected log delivery error, got %T: %v", err, err)
	}
}

func TestSendLogObservationRejectsInvalidObservation(t *testing.T) {
	client := LogClient{ControllerURL: "http://127.0.0.1:0"}
	err := client.SendLogObservation(model.LogObservation{
		Component: "",
		Level:     model.LogLevelInfo,
		Timestamp: "2026-07-05T11:00:00Z",
		Message:   "test",
	})
	if err == nil {
		t.Fatal("expected an error")
	}

	var deliveryErr *LogDeliveryError
	if !errors.As(err, &deliveryErr) {
		t.Fatalf("expected log delivery error, got %T: %v", err, err)
	}
}

func TestSendLogObservationWithFallbackDoesNotWriteAfterSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	root := t.TempDir()
	fallbackPath := filepath.Join(root, fallbackObservationsFileName)

	client := LogClient{ControllerURL: server.URL}
	err := client.SendLogObservationWithFallback(validLogObservation(t), root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(fallbackPath); err == nil {
		t.Fatalf("did not expect fallback log file %q", fallbackPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("fallback path check failed: %v", err)
	}
}

func TestSendLogObservationWithFallbackWritesOnDeliveryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	root := t.TempDir()
	fallbackPath := filepath.Join(root, fallbackObservationsFileName)

	client := LogClient{ControllerURL: server.URL}
	err := client.SendLogObservationWithFallback(validLogObservation(t), root)
	if err == nil {
		t.Fatal("expected an error")
	}

	var deliveryErr *LogDeliveryError
	if !errors.As(err, &deliveryErr) {
		t.Fatalf("expected log delivery error, got %T: %v", err, err)
	}

	data, err := os.ReadFile(fallbackPath)
	if err != nil {
		t.Fatalf("expected fallback file %q: %v", fallbackPath, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || len(lines[0]) == 0 {
		t.Fatal("expected one jsonl line in fallback file")
	}

	var observed model.LogObservation
	if err := json.Unmarshal([]byte(lines[0]), &observed); err != nil {
		t.Fatalf("expected structured fallback entry: %v", err)
	}

	want := validLogObservation(t)
	if observed.Message != want.Message {
		t.Fatalf("unexpected message in fallback entry: %q", observed.Message)
	}
}

func TestSendLogObservationWithFallbackReturnsLoggingErrorWhenFallbackAlsoFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	root := t.TempDir()
	conflictingPath := filepath.Join(root, "log-file")
	if err := os.WriteFile(conflictingPath, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := LogClient{ControllerURL: server.URL}
	err := client.SendLogObservationWithFallback(validLogObservation(t), conflictingPath)
	if err == nil {
		t.Fatal("expected an error")
	}

	var deliveryErr *LogDeliveryError
	if !errors.As(err, &deliveryErr) {
		t.Fatalf("expected log delivery error, got %T: %v", err, err)
	}
}

func validLogObservation(t *testing.T) model.LogObservation {
	t.Helper()
	return model.LogObservation{
		Component: "worker",
		Level:     model.LogLevelInfo,
		Timestamp: "2026-07-05T11:00:00Z",
		Message:   "hello",
	}
}

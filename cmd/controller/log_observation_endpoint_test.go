package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterControllerRoutesRegistersLogObservationEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	controller := newController()
	controller.maxRequestBytes = 1024
	registerControllerRoutes(mux, controller)

	request := httptest.NewRequest(http.MethodGet, "/observations/logs", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("response code = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestLogObservationsHandlerAcceptsValidObservation(t *testing.T) {
	controller := newController()
	controller.maxRequestBytes = 1024

	request := httptest.NewRequest(http.MethodPost, "/observations/logs", strings.NewReader(`{
  "component": "worker",
  "level": "info",
  "timestamp": "2026-07-05T12:00:00Z",
  "message": "hello"
}`))
	response := httptest.NewRecorder()

	controller.logObservationsHandler(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("response code = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestLogObservationsHandlerRejectsInvalidJSON(t *testing.T) {
	controller := newController()
	controller.maxRequestBytes = 1024

	request := httptest.NewRequest(http.MethodPost, "/observations/logs", strings.NewReader(`{
  "component": "worker"
  "level": "info"
`))
	response := httptest.NewRecorder()

	controller.logObservationsHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("response code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestLogObservationsHandlerRejectsInvalidObservation(t *testing.T) {
	controller := newController()
	controller.maxRequestBytes = 1024

	request := httptest.NewRequest(http.MethodPost, "/observations/logs", strings.NewReader(`{
  "level": "info",
  "timestamp": "2026-07-05T12:00:00Z",
  "message": "missing component"
}`))
	response := httptest.NewRecorder()

	controller.logObservationsHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("response code = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestLogObservationsHandlerRejectsOversizedBody(t *testing.T) {
	controller := newController()
	controller.maxRequestBytes = 32

	request := httptest.NewRequest(http.MethodPost, "/observations/logs", strings.NewReader(strings.Repeat("x", 128)))
	response := httptest.NewRecorder()

	controller.logObservationsHandler(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("response code = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestLogObservationsHandlerWorksDuringRecoveryMode(t *testing.T) {
	controller := newController()
	controller.maxRequestBytes = 1024
	controller.enterRecoveryMode()

	request := httptest.NewRequest(http.MethodPost, "/observations/logs", strings.NewReader(`{
  "component": "controller",
  "level": "warn",
  "timestamp": "2026-07-05T12:00:00Z",
  "message": "controller starting"
}`))
	response := httptest.NewRecorder()

	controller.logObservationsHandler(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("response code = %d, want %d", response.Code, http.StatusNoContent)
	}
}

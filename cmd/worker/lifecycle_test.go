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

func TestRunHeartbeatStopsOnContextCancellation(t *testing.T) {
	clock := newFakeWorkerLifecycleClock()
	ctx, cancel := context.WithCancel(context.Background())
	result := runHeartbeatAsync(ctx, testWorkerSession(), func(context.Context, WorkerSession) error {
		t.Fatal("heartbeat should not run before a tick")
		return nil
	}, clock)

	cancel()

	if err := receiveHeartbeatResult(t, result); err != nil {
		t.Fatalf("RunHeartbeat() error = %v, want nil", err)
	}
	if !clock.ticker.stopped {
		t.Fatal("ticker was not stopped")
	}
}

func TestRunHeartbeatRefreshesLastAcceptedHeartbeat(t *testing.T) {
	clock := newFakeWorkerLifecycleClock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls int
	result := runHeartbeatAsync(ctx, testWorkerSession(), func(context.Context, WorkerSession) error {
		calls++
		if calls == 1 {
			return nil
		}
		return errors.New("controller unavailable")
	}, clock)

	clock.tick()
	clock.advance(4 * time.Minute)
	clock.tick()
	cancel()

	if err := receiveHeartbeatResult(t, result); err != nil {
		t.Fatalf("RunHeartbeat() error = %v, want nil", err)
	}
	if calls != 2 {
		t.Fatalf("heartbeat calls = %d, want 2", calls)
	}
}

func TestRunHeartbeatRejectedSessionSelfFences(t *testing.T) {
	clock := newFakeWorkerLifecycleClock()
	result := runHeartbeatAsync(context.Background(), testWorkerSession(), func(context.Context, WorkerSession) error {
		return ErrWorkerSessionNotActive
	}, clock)

	clock.tick()

	if err := receiveHeartbeatResult(t, result); !errors.Is(err, ErrWorkerSessionNotActive) {
		t.Fatalf("RunHeartbeat() error = %v, want ErrWorkerSessionNotActive", err)
	}
}

func TestRunHeartbeatTransportFailureBeforeDeadAfterDoesNotFence(t *testing.T) {
	clock := newFakeWorkerLifecycleClock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls int
	result := runHeartbeatAsync(ctx, testWorkerSession(), func(context.Context, WorkerSession) error {
		calls++
		if calls == 1 {
			return errors.New("controller unavailable")
		}
		return nil
	}, clock)

	clock.advance(4 * time.Minute)
	clock.tick()
	clock.tick()
	cancel()

	if err := receiveHeartbeatResult(t, result); err != nil {
		t.Fatalf("RunHeartbeat() error = %v, want nil", err)
	}
	if calls != 2 {
		t.Fatalf("heartbeat calls = %d, want 2", calls)
	}
}

func TestRunHeartbeatTransportFailureAfterDeadAfterSelfFences(t *testing.T) {
	clock := newFakeWorkerLifecycleClock()
	result := runHeartbeatAsync(context.Background(), testWorkerSession(), func(context.Context, WorkerSession) error {
		return errors.New("controller unavailable")
	}, clock)

	clock.advance(5 * time.Minute)
	clock.tick()

	if err := receiveHeartbeatResult(t, result); !errors.Is(err, ErrWorkerSelfFenced) {
		t.Fatalf("RunHeartbeat() error = %v, want ErrWorkerSelfFenced", err)
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

func runHeartbeatAsync(ctx context.Context, session WorkerSession, heartbeat WorkerHeartbeatFunc, clock *fakeWorkerLifecycleClock) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- RunHeartbeat(ctx, session, heartbeat, clock)
	}()
	clock.waitForTicker()
	return result
}

func receiveHeartbeatResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for heartbeat result")
		return nil
	}
}

type fakeWorkerLifecycleClock struct {
	now          time.Time
	ticker       *fakeWorkerLifecycleTicker
	tickerReady  chan struct{}
	tickerOpened bool
}

type fakeWorkerLifecycleTicker struct {
	ch      chan time.Time
	stopped bool
}

func newFakeWorkerLifecycleClock() *fakeWorkerLifecycleClock {
	return &fakeWorkerLifecycleClock{
		now:         time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		tickerReady: make(chan struct{}),
	}
}

func (c *fakeWorkerLifecycleClock) Now() time.Time {
	return c.now
}

func (c *fakeWorkerLifecycleClock) NewTicker(time.Duration) WorkerLifecycleTicker {
	c.ticker = &fakeWorkerLifecycleTicker{ch: make(chan time.Time)}
	if !c.tickerOpened {
		close(c.tickerReady)
		c.tickerOpened = true
	}
	return c.ticker
}

func (c *fakeWorkerLifecycleClock) advance(duration time.Duration) {
	c.now = c.now.Add(duration)
}

func (c *fakeWorkerLifecycleClock) tick() {
	c.ticker.ch <- c.now
}

func (c *fakeWorkerLifecycleClock) waitForTicker() {
	<-c.tickerReady
}

func (t *fakeWorkerLifecycleTicker) C() <-chan time.Time {
	return t.ch
}

func (t *fakeWorkerLifecycleTicker) Stop() {
	t.stopped = true
}

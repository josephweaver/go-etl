package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
		case r.Method == http.MethodPost && r.URL.Path == "/workers/heartbeat":
			w.WriteHeader(http.StatusNoContent)
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
		case r.Method == http.MethodPost && r.URL.Path == "/workers/heartbeat":
			w.WriteHeader(http.StatusNoContent)
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

func TestRunWorkerLoopPollsUntilIdleTimeout(t *testing.T) {
	var nextCalls int
	var sawStop bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			assertTestWorkerSessionHeaders(t, r)
			nextCalls++
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
	worker.Config.IdlePollIntervalSeconds = 1
	worker.Config.IdleTimeoutSeconds = 1

	startedAt := time.Now()
	if err := runWorkerLoop(worker); err != nil {
		t.Fatalf("runWorkerLoop() error = %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed < time.Second {
		t.Fatalf("worker stopped after %s, want at least one idle poll interval", elapsed)
	}
	if nextCalls < 2 {
		t.Fatalf("next calls = %d, want at least 2", nextCalls)
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

func TestRunWorkerLoopIdleDrainStopsWithoutClaim(t *testing.T) {
	var nextCalls int
	var stops []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			nextCalls++
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
	drains := NewWorkerDrainRequests()
	if _, err := drains.Request(WorkerDrainRequest{Reason: WorkerDrainReasonSlurmSignal, RequestedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := runWorkerLoopWithDrain(worker, drains); err != nil {
		t.Fatalf("runWorkerLoopWithDrain() error = %v", err)
	}
	if nextCalls != 0 {
		t.Fatalf("next calls = %d, want 0", nextCalls)
	}
	if len(stops) != 1 || stops[0] != "slurm_drain_idle" {
		t.Fatalf("stops = %v, want [slurm_drain_idle]", stops)
	}
}

func TestRunWorkerLoopActiveDrainReportsCompletionThenStops(t *testing.T) {
	var mu sync.Mutex
	var nextCalls int
	var completed int
	var stops []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			nextCalls++
			_ = json.NewEncoder(w).Encode(model.WorkItem{ID: "work-001", AttemptID: "attempt-001", Type: model.WorkItemTypeWriteDemoOutput, OutputFilename: "output.json"})
		case r.Method == http.MethodPost && r.URL.Path == "/workers/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/work/complete":
			completed++
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

	execution := newFakeSupervisedExecution()
	started := make(chan struct{})
	adapter := &mainLoopPauseAdapter{execution: execution, started: started}
	worker := checkpointMainLoopWorker(t, server.URL, CheckpointModeShutdown, adapter)
	drains := NewWorkerDrainRequests()
	result := make(chan error, 1)
	go func() { result <- runWorkerLoopWithDrain(worker, drains) }()
	<-started
	if _, err := drains.Request(WorkerDrainRequest{Reason: WorkerDrainReasonSlurmSignal, RequestedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	execution.result <- ExecutionResult{Evidence: WorkEvidence{OutputSHA256: strings.Repeat("a", 64)}}
	if err := <-result; err != nil {
		t.Fatalf("runWorkerLoopWithDrain() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if nextCalls != 1 || completed != 1 {
		t.Fatalf("next/completed = %d/%d, want 1/1", nextCalls, completed)
	}
	if len(stops) != 1 || stops[0] != "slurm_drain_finished" {
		t.Fatalf("stops = %v, want [slurm_drain_finished]", stops)
	}
}

func TestRunWorkerLoopDrainDuringClaimFinishesClaimedWorkThenStops(t *testing.T) {
	claimStarted := make(chan struct{})
	releaseClaim := make(chan struct{})
	var nextCalls int
	var completed int
	var stops []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			nextCalls++
			close(claimStarted)
			<-releaseClaim
			_ = json.NewEncoder(w).Encode(model.WorkItem{ID: "work-001", AttemptID: "attempt-001", Type: model.WorkItemTypeWriteDemoOutput, OutputFilename: "output.json"})
		case r.Method == http.MethodPost && r.URL.Path == "/workers/heartbeat":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/work/complete":
			completed++
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
	drains := NewWorkerDrainRequests()
	result := make(chan error, 1)
	go func() { result <- runWorkerLoopWithDrain(worker, drains) }()
	<-claimStarted
	if _, err := drains.Request(WorkerDrainRequest{Reason: WorkerDrainReasonSlurmSignal, RequestedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	close(releaseClaim)
	if err := <-result; err != nil {
		t.Fatalf("runWorkerLoopWithDrain() error = %v", err)
	}

	if nextCalls != 1 || completed != 1 {
		t.Fatalf("next/completed = %d/%d, want 1/1", nextCalls, completed)
	}
	if len(stops) != 1 || stops[0] != "slurm_drain_finished" {
		t.Fatalf("stops = %v, want [slurm_drain_finished]", stops)
	}
}

func TestRunWorkerLoopResumeLaunchRejectionStopsWithoutWorkFailure(t *testing.T) {
	var completed int
	var failed int
	var stops []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			_ = json.NewEncoder(w).Encode(model.WorkItem{
				ID: "work-001", AttemptID: "attempt-002", Type: model.WorkItemTypeWriteDemoOutput,
				OutputFilename: "output.json", Resume: workerResumeAssignment(t),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/work/complete":
			completed++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/work/fail":
			failed++
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

	adapter := &fakePauseAdapter{execution: newFakeSupervisedExecution()}
	worker := checkpointMainLoopWorker(t, server.URL, CheckpointModeShutdown, adapter)
	registration := validPauseAdapterRegistration(adapter)
	registration.WorkItemType = model.WorkItemTypeAssetMaterialize
	registry, err := NewPauseAdapterRegistry(registration)
	if err != nil {
		t.Fatal(err)
	}
	worker.PauseAdapters = registry

	err = runWorkerLoop(worker)
	if err == nil || !strings.Contains(err.Error(), "no pause adapter") {
		t.Fatalf("runWorkerLoop() error = %v", err)
	}
	if completed != 0 || failed != 0 {
		t.Fatalf("terminal reports completed=%d failed=%d, want none", completed, failed)
	}
	if len(stops) != 1 || stops[0] != "resume_launch_rejected" {
		t.Fatalf("stops = %v, want [resume_launch_rejected]", stops)
	}
}

func TestRunWorkerLoopMapsSupervisorSuspensionAndAbandonment(t *testing.T) {
	for _, test := range []struct {
		name       string
		captureErr error
		wantStop   string
		wantErr    bool
	}{
		{name: "suspended", wantStop: "checkpoint_suspended"},
		{name: "abandoned", captureErr: errors.New("capture failed"), wantStop: "checkpoint_abandoned", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var terminalReports int
			var stops []string
			var registration PauseAdapterRegistration
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
					writeTestWorkerRegistration(t, w)
				case r.Method == http.MethodGet && r.URL.Path == "/work/next":
					_ = json.NewEncoder(w).Encode(model.WorkItem{ID: "work-001", AttemptID: "attempt-001", Type: model.WorkItemTypeWriteDemoOutput, OutputFilename: "output.json"})
				case r.Method == http.MethodPost && r.URL.Path == "/work/checkpoint/confirm":
					var confirmation model.WorkCheckpointConfirmation
					if err := json.NewDecoder(r.Body).Decode(&confirmation); err != nil {
						t.Fatalf("decode confirmation: %v", err)
					}
					_ = json.NewEncoder(w).Encode(supervisorAcknowledgement(t, confirmation))
				case r.Method == http.MethodPost && (r.URL.Path == "/work/complete" || r.URL.Path == "/work/fail"):
					terminalReports++
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

			execution := newSupervisorTestExecution()
			execution.suspend = func(_ context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
				if test.captureErr != nil {
					return PreparedCheckpoint{}, test.captureErr
				}
				return preparedSupervisorCheckpoint(t, request, registration), nil
			}
			adapter := &fakePauseAdapter{execution: execution}
			worker := checkpointMainLoopWorker(t, server.URL, CheckpointModeYield, adapter)
			worker.Config.WorkItemExecutionQuantumSeconds = 1
			registration, _ = worker.PauseAdapters.AdapterFor(model.WorkItemTypeWriteDemoOutput)

			err := runWorkerLoop(worker)
			if (err != nil) != test.wantErr {
				t.Fatalf("runWorkerLoop() error = %v, wantErr %t", err, test.wantErr)
			}
			if terminalReports != 0 {
				t.Fatalf("terminal reports = %d, want 0", terminalReports)
			}
			if len(stops) != 1 || stops[0] != test.wantStop {
				t.Fatalf("stops = %v, want [%s]", stops, test.wantStop)
			}
		})
	}
}

func TestRunWorkerLoopHeartbeatRejectionCancelsSupervisor(t *testing.T) {
	started := make(chan struct{})
	var terminalReports int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/workers/register":
			writeTestWorkerRegistration(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/work/next":
			_ = json.NewEncoder(w).Encode(model.WorkItem{ID: "work-001", AttemptID: "attempt-001", Type: model.WorkItemTypeWriteDemoOutput, OutputFilename: "output.json"})
		case r.Method == http.MethodPost && r.URL.Path == "/workers/heartbeat":
			http.Error(w, "worker session is not active", http.StatusConflict)
		case r.Method == http.MethodPost && (r.URL.Path == "/work/complete" || r.URL.Path == "/work/fail"):
			terminalReports++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	execution := newFakeSupervisedExecution()
	adapter := &mainLoopPauseAdapter{execution: execution, started: started}
	worker := checkpointMainLoopWorker(t, server.URL, CheckpointModeShutdown, adapter)
	clock := newFakeWorkerLifecycleClock()
	worker.LifecycleClock = clock
	result := make(chan error, 1)
	go func() { result <- runWorkerLoop(worker) }()
	clock.waitForTicker()
	<-started
	clock.tick()

	err := <-result
	if !errors.Is(err, ErrWorkerSessionNotActive) {
		t.Fatalf("runWorkerLoop() error = %v, want ErrWorkerSessionNotActive", err)
	}
	if terminalReports != 0 {
		t.Fatalf("terminal reports = %d, want 0", terminalReports)
	}
	if execution.terminations != 1 {
		t.Fatalf("execution terminations = %d, want 1", execution.terminations)
	}
}

type mainLoopPauseAdapter struct {
	execution SupervisedExecution
	started   chan struct{}
	once      sync.Once
}

func (adapter *mainLoopPauseAdapter) StartFresh(context.Context, model.WorkItem) (SupervisedExecution, error) {
	adapter.once.Do(func() { close(adapter.started) })
	return adapter.execution, nil
}

func (adapter *mainLoopPauseAdapter) StartResume(context.Context, model.WorkItem, model.WorkItemResumeAssignment) (SupervisedExecution, error) {
	adapter.once.Do(func() { close(adapter.started) })
	return adapter.execution, nil
}

func checkpointMainLoopWorker(t *testing.T, controllerURL string, mode CheckpointMode, adapter PauseAdapter) Worker {
	t.Helper()
	worker := newTestWorker(t)
	worker.Config.ControllerURL = controllerURL
	configureWorkerCheckpointMode(&worker.Config, mode)
	registration := validPauseAdapterRegistration(adapter)
	registry, err := NewPauseAdapterRegistry(registration)
	if err != nil {
		t.Fatal(err)
	}
	worker.PauseAdapters = registry
	return worker
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

package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"goetl/internal/persistence"
	"goetl/internal/variable"
)

func TestCareTakerSignalDoesNotBlock(t *testing.T) {
	caretaker := NewCareTaker(fakeCareTakerClock{})
	caretaker.Signal("first")

	done := make(chan struct{})
	go func() {
		caretaker.Signal("second")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Signal blocked with a full wake channel")
	}
}

func TestCareTakerSignalsCoalesce(t *testing.T) {
	caretaker := NewCareTaker(fakeCareTakerClock{})

	caretaker.Signal("first")
	caretaker.Signal("second")
	caretaker.Signal("third")

	if got := len(caretaker.wakeCh); got != 1 {
		t.Fatalf("wake channel length = %d, want 1 coalesced signal", got)
	}
	count, reason := caretaker.signalSnapshot()
	if count != 3 || reason != "third" {
		t.Fatalf("signal snapshot = %d/%q, want 3/third", count, reason)
	}
}

func TestCareTakerRunPerformsInitialReconcile(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	source := &fakeCareTakerStateSource{
		recoverCh: make(chan int, 2),
	}
	clock := &controllableCareTakerClock{now: now}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{}, source, &fakeCareTakerLauncher{}, NewWorkerCapacityManager(nil), clock)
	ctx, cancel := context.WithCancel(context.Background())
	done := runCareTakerForTest(caretaker, ctx)

	waitForCareTakerCall(t, source.recoverCh)
	cancel()
	waitForCareTakerStop(t, done)
}

func TestCareTakerWakesOnStateSignal(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	source := &fakeCareTakerStateSource{
		recoverCh: make(chan int, 4),
	}
	clock := &controllableCareTakerClock{now: now}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{}, source, &fakeCareTakerLauncher{}, NewWorkerCapacityManager(nil), clock)
	ctx, cancel := context.WithCancel(context.Background())
	done := runCareTakerForTest(caretaker, ctx)

	waitForCareTakerCall(t, source.recoverCh)
	caretaker.Signal("state_changed")
	waitForCareTakerCall(t, source.recoverCh)

	cancel()
	waitForCareTakerStop(t, done)
}

func TestCareTakerWakesAtEarliestHeartbeatExpiry(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	source := &fakeCareTakerStateSource{
		snapshot:  WorkerCapacitySnapshot{EarliestHeartbeat: now},
		recoverCh: make(chan int, 4),
	}
	clock := &controllableCareTakerClock{
		now:     now,
		timerCh: make(chan *controllableCareTakerTimer, 2),
	}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		DeadAfter: 5 * time.Minute,
	}, source, &fakeCareTakerLauncher{}, NewWorkerCapacityManager(nil), clock)
	ctx, cancel := context.WithCancel(context.Background())
	done := runCareTakerForTest(caretaker, ctx)

	waitForCareTakerCall(t, source.recoverCh)
	timer := waitForCareTakerTimer(t, clock.timerCh)
	if timer.duration != 5*time.Minute {
		t.Fatalf("timer duration = %s, want 5m", timer.duration)
	}

	timer.fire(now.Add(5 * time.Minute))
	waitForCareTakerCall(t, source.recoverCh)

	cancel()
	waitForCareTakerStop(t, done)
}

func TestCareTakerWakesAtInflightReservationExpiry(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	source := &fakeCareTakerStateSource{
		recoverCh: make(chan int, 4),
	}
	clock := &controllableCareTakerClock{
		now:     now,
		timerCh: make(chan *controllableCareTakerTimer, 2),
	}
	manager := NewWorkerCapacityManager(nil)
	manager.reserveInflightStarts(now, 1)
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		InflightStartTimeout: 5 * time.Minute,
	}, source, &fakeCareTakerLauncher{}, manager, clock)
	ctx, cancel := context.WithCancel(context.Background())
	done := runCareTakerForTest(caretaker, ctx)

	waitForCareTakerCall(t, source.recoverCh)
	timer := waitForCareTakerTimer(t, clock.timerCh)
	if timer.duration != 5*time.Minute {
		t.Fatalf("timer duration = %s, want 5m", timer.duration)
	}

	timer.fire(now.Add(5 * time.Minute))
	waitForCareTakerCall(t, source.recoverCh)

	cancel()
	waitForCareTakerStop(t, done)
}

func TestCareTakerUsesFallbackSafetySweep(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	source := &fakeCareTakerStateSource{
		recoverCh: make(chan int, 4),
	}
	clock := &controllableCareTakerClock{
		now:     now,
		timerCh: make(chan *controllableCareTakerTimer, 2),
	}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		FallbackInterval: 10 * time.Minute,
	}, source, &fakeCareTakerLauncher{}, NewWorkerCapacityManager(nil), clock)
	ctx, cancel := context.WithCancel(context.Background())
	done := runCareTakerForTest(caretaker, ctx)

	waitForCareTakerCall(t, source.recoverCh)
	timer := waitForCareTakerTimer(t, clock.timerCh)
	if timer.duration != 10*time.Minute {
		t.Fatalf("timer duration = %s, want 10m", timer.duration)
	}

	cancel()
	waitForCareTakerStop(t, done)
}

func TestCareTakerShutdownDoesNotRequireClosingWakeChannel(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	source := &fakeCareTakerStateSource{
		recoverCh: make(chan int, 2),
	}
	clock := &controllableCareTakerClock{now: now}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{}, source, &fakeCareTakerLauncher{}, NewWorkerCapacityManager(nil), clock)
	ctx, cancel := context.WithCancel(context.Background())
	done := runCareTakerForTest(caretaker, ctx)

	waitForCareTakerCall(t, source.recoverCh)
	cancel()
	waitForCareTakerStop(t, done)

	signalDone := make(chan struct{})
	go func() {
		caretaker.Signal("after_shutdown")
		close(signalDone)
	}()
	select {
	case <-signalDone:
	case <-time.After(time.Second):
		t.Fatal("Signal blocked after CareTaker shutdown")
	}
}

func TestControllerCareTakerConfigUsesStartupPolicy(t *testing.T) {
	resolver := variable.NewResolver(variable.NewSet(testStartupScope(t,
		testStartupVariable(variable.NamespaceControllerConfig, "worker_heartbeat_interval", variable.TypeString, "2m"),
		testStartupVariable(variable.NamespaceControllerConfig, "worker_dead_after", variable.TypeString, "7m"),
		testStartupVariable(variable.NamespaceControllerConfig, "worker_execution_pattern", variable.TypeString, workerExecutionPatternOneByOneUntilSaturated),
		testStartupVariable(variable.NamespaceControllerConfig, "worker_max_active", variable.TypeInt, 4),
		testStartupVariable(variable.NamespaceControllerConfig, "worker_inflight_start_timeout", variable.TypeString, "45s"),
	)), variable.ResolverConfig{})

	cfg, err := controllerCareTakerConfig(resolver, controllerOperationalPolicy{
		CaretakerIntervalScheduleMillis: 2000,
		CaretakerMissedIntervalLimit:    3,
	})
	if err != nil {
		t.Fatalf("controllerCareTakerConfig() error = %v", err)
	}
	if cfg.DeadAfter != 7*time.Minute {
		t.Fatalf("DeadAfter = %s, want 7m", cfg.DeadAfter)
	}
	if cfg.InflightStartTimeout != 45*time.Second {
		t.Fatalf("InflightStartTimeout = %s, want 45s", cfg.InflightStartTimeout)
	}
	if cfg.FallbackInterval != 2*time.Second || cfg.RetryInitial != 2*time.Second || cfg.RetryMaximum != 6*time.Second {
		t.Fatalf("timer policy = fallback %s retry %s max %s, want 2s/2s/6s", cfg.FallbackInterval, cfg.RetryInitial, cfg.RetryMaximum)
	}
	if cfg.WorkerExecution.MaxActiveWorkers != 4 {
		t.Fatalf("MaxActiveWorkers = %d, want 4", cfg.WorkerExecution.MaxActiveWorkers)
	}
}

func TestControllerStartsOneCareTaker(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store

	controller.startCareTaker(context.Background(), CareTakerConfig{})
	first := controller.caretaker
	if first == nil || controller.caretakerCancel == nil || controller.caretakerDone == nil {
		t.Fatal("controller did not start CareTaker lifecycle")
	}
	if controller.workerStateChanged == nil {
		t.Fatal("controller did not route worker state changes to CareTaker")
	}
	waitForCareTakerSignalCount(t, first, 1)

	controller.startCareTaker(context.Background(), CareTakerConfig{FallbackInterval: time.Minute})
	if controller.caretaker != first {
		t.Fatal("controller started more than one CareTaker")
	}
	controller.workerStateChanged("test_signal")
	_, reason := first.signalSnapshot()
	if reason != "test_signal" {
		t.Fatalf("last CareTaker signal reason = %q, want test_signal", reason)
	}
	if err := controller.stopCareTaker(); err != nil {
		t.Fatalf("stopCareTaker() error = %v", err)
	}
}

func TestControllerShutdownCancelsAndJoinsCareTaker(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	controller.startCareTaker(context.Background(), CareTakerConfig{})

	if err := controller.stopCareTaker(); err != nil {
		t.Fatalf("stopCareTaker() error = %v", err)
	}
	if controller.caretakerCancel != nil || controller.caretakerDone != nil {
		t.Fatal("CareTaker lifecycle handles were not cleared after shutdown")
	}

	done := make(chan struct{})
	go func() {
		controller.caretaker.Signal("after_shutdown")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CareTaker signal blocked after controller shutdown")
	}
}

func TestCalculateCareTakerNextWakeUsesEarliestHeartbeatExpiry(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	next := calculateCareTakerNextWake(now, CareTakerConfig{
		DeadAfter:        5 * time.Minute,
		FallbackInterval: 30 * time.Minute,
	}, WorkerCapacitySnapshot{
		EarliestHeartbeat: now.Add(-time.Minute),
	}, WorkerExecutionState{}, 0)

	want := now.Add(4 * time.Minute)
	if !next.At.Equal(want) || next.Reason != "worker_expiry" {
		t.Fatalf("next wake = %+v, want %s worker_expiry", next, want)
	}
}

func TestCalculateCareTakerNextWakeUsesEarliestInflightExpiry(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	state := WorkerExecutionState{
		InflightStarts: []WorkerStartReservation{
			{ID: "newer", CreatedAt: now.Add(-time.Minute)},
			{ID: "older", CreatedAt: now.Add(-4 * time.Minute)},
		},
	}

	next := calculateCareTakerNextWake(now, CareTakerConfig{
		InflightStartTimeout: 5 * time.Minute,
		FallbackInterval:     30 * time.Minute,
	}, WorkerCapacitySnapshot{}, state, 0)

	want := now.Add(time.Minute)
	if !next.At.Equal(want) || next.Reason != "inflight_start_expiry" {
		t.Fatalf("next wake = %+v, want %s inflight_start_expiry", next, want)
	}
}

func TestCalculateCareTakerNextWakeUsesFallbackWhenNoEarlierDeadline(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	next := calculateCareTakerNextWake(now, CareTakerConfig{
		FallbackInterval: 10 * time.Minute,
	}, WorkerCapacitySnapshot{}, WorkerExecutionState{}, 0)

	want := now.Add(10 * time.Minute)
	if !next.At.Equal(want) || next.Reason != "fallback_sweep" {
		t.Fatalf("next wake = %+v, want %s fallback_sweep", next, want)
	}
}

func TestCalculateCareTakerNextWakeReturnsZeroWhenNoDeadline(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	next := calculateCareTakerNextWake(now, CareTakerConfig{}, WorkerCapacitySnapshot{}, WorkerExecutionState{}, 0)

	if !next.At.IsZero() || next.Reason != "" {
		t.Fatalf("next wake = %+v, want zero", next)
	}
}

func TestCalculateCareTakerNextWakeClampsPastDeadlineToNow(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	next := calculateCareTakerNextWake(now, CareTakerConfig{
		DeadAfter: 5 * time.Minute,
	}, WorkerCapacitySnapshot{
		EarliestHeartbeat: now.Add(-10 * time.Minute),
	}, WorkerExecutionState{}, 0)

	if !next.At.Equal(now) || next.Reason != "worker_expiry" {
		t.Fatalf("next wake = %+v, want immediate worker_expiry", next)
	}
}

func TestCalculateCareTakerNextWakeBoundsRetryDelay(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	next := calculateCareTakerNextWake(now, CareTakerConfig{
		RetryInitial:     time.Minute,
		RetryMaximum:     5 * time.Minute,
		FallbackInterval: 30 * time.Minute,
	}, WorkerCapacitySnapshot{}, WorkerExecutionState{}, 30*time.Second)

	want := now.Add(time.Minute)
	if !next.At.Equal(want) || next.Reason != "retry" {
		t.Fatalf("next wake = %+v, want %s retry", next, want)
	}

	next = calculateCareTakerNextWake(now, CareTakerConfig{
		RetryInitial:     time.Minute,
		RetryMaximum:     5 * time.Minute,
		FallbackInterval: 30 * time.Minute,
	}, WorkerCapacitySnapshot{}, WorkerExecutionState{}, 10*time.Minute)
	want = now.Add(5 * time.Minute)
	if !next.At.Equal(want) || next.Reason != "retry" {
		t.Fatalf("next wake = %+v, want %s retry", next, want)
	}
}

func TestCareTakerRecoversExpiredSessionsBeforeCapacityPlan(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	var events []string
	source := &fakeCareTakerStateSource{
		snapshot: WorkerCapacitySnapshot{PendingQueued: 1, PendingClaimable: 1},
		events:   &events,
	}
	launcher := &fakeCareTakerLauncher{events: &events}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		DeadAfter: 5 * time.Minute,
		WorkerExecution: WorkerExecutionConfig{
			Pattern:              workerExecutionPatternOneByOneUntilSaturated,
			MaxActiveWorkers:     2,
			InflightStartTimeout: 5 * time.Minute,
		},
	}, source, launcher, NewWorkerCapacityManager(nil), fakeCareTakerClock{now: now})

	if _, err := caretaker.reconcile(context.Background(), now); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	want := []string{"recover", "snapshot", "launch"}
	if !equalStrings(events, want) {
		t.Fatalf("events = %+v, want %+v", events, want)
	}
	if launcher.calls != 1 || launcher.counts[0] != 1 {
		t.Fatalf("launcher calls = %d counts=%+v, want one launch", launcher.calls, launcher.counts)
	}
}

func TestRecoveredWorkParticipatesInSameCapacityPlan(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	source := &fakeCareTakerStateSource{
		recovery: WorkerRecoverySummary{ExpiredSessions: 1, AbandonedAttempts: 1, RequeuedWorkItems: 1},
		snapshot: WorkerCapacitySnapshot{
			PendingQueued:    1,
			PendingClaimable: 1,
		},
	}
	launcher := &fakeCareTakerLauncher{}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		DeadAfter: 5 * time.Minute,
		WorkerExecution: WorkerExecutionConfig{
			Pattern:              workerExecutionPatternOneByOneUntilSaturated,
			MaxActiveWorkers:     2,
			InflightStartTimeout: 5 * time.Minute,
		},
	}, source, launcher, NewWorkerCapacityManager(nil), fakeCareTakerClock{now: now})

	if _, err := caretaker.reconcile(context.Background(), now); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if launcher.calls != 1 {
		t.Fatalf("launcher calls = %d, want recovered work to launch in same reconcile", launcher.calls)
	}
}

func TestCareTakerPrunesExpiredInflightBeforePlan(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	manager := NewWorkerCapacityManager(nil)
	manager.reserveInflightStarts(now.Add(-10*time.Minute), 1)
	source := &fakeCareTakerStateSource{
		snapshot: WorkerCapacitySnapshot{PendingQueued: 1, PendingClaimable: 1},
	}
	launcher := &fakeCareTakerLauncher{}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		DeadAfter: 5 * time.Minute,
		WorkerExecution: WorkerExecutionConfig{
			Pattern:              workerExecutionPatternOneByOneUntilSaturated,
			MaxActiveWorkers:     2,
			InflightStartTimeout: 5 * time.Minute,
		},
		InflightStartTimeout: 5 * time.Minute,
	}, source, launcher, manager, fakeCareTakerClock{now: now})

	if _, err := caretaker.reconcile(context.Background(), now); err != nil {
		t.Fatalf("reconcile() error = %v", err)
	}
	if launcher.calls != 1 {
		t.Fatalf("launcher calls = %d, want launch after expired inflight prune", launcher.calls)
	}
}

func TestCareTakerLaunchFailureRemovesReservationAndSchedulesRetry(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	manager := NewWorkerCapacityManager(nil)
	source := &fakeCareTakerStateSource{
		snapshot: WorkerCapacitySnapshot{PendingQueued: 1, PendingClaimable: 1},
	}
	launchErr := errors.New("launch failed")
	launcher := &fakeCareTakerLauncher{err: launchErr}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		RetryInitial: 30 * time.Second,
		RetryMaximum: 5 * time.Minute,
		WorkerExecution: WorkerExecutionConfig{
			Pattern:              workerExecutionPatternOneByOneUntilSaturated,
			MaxActiveWorkers:     2,
			InflightStartTimeout: 5 * time.Minute,
		},
	}, source, launcher, manager, fakeCareTakerClock{now: now})

	next, err := caretaker.reconcile(context.Background(), now)
	if !errors.Is(err, launchErr) {
		t.Fatalf("reconcile() error = %v, want launch error", err)
	}
	if got := len(manager.Snapshot().InflightStarts); got != 0 {
		t.Fatalf("inflight starts = %d, want removed on launch failure", got)
	}
	want := now.Add(30 * time.Second)
	if !next.At.Equal(want) || next.Reason != "retry" {
		t.Fatalf("next wake = %+v, want %s retry", next, want)
	}
}

func TestCareTakerPersistenceFailureDoesNotLaunch(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	sourceErr := errors.New("store unavailable")
	source := &fakeCareTakerStateSource{err: sourceErr}
	launcher := &fakeCareTakerLauncher{}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		RetryInitial: 30 * time.Second,
		RetryMaximum: 5 * time.Minute,
	}, source, launcher, NewWorkerCapacityManager(nil), fakeCareTakerClock{now: now})

	next, err := caretaker.reconcile(context.Background(), now)
	if !errors.Is(err, sourceErr) {
		t.Fatalf("reconcile() error = %v, want source error", err)
	}
	if launcher.calls != 0 {
		t.Fatalf("launcher calls = %d, want no launch on persistence failure", launcher.calls)
	}
	want := now.Add(30 * time.Second)
	if !next.At.Equal(want) || next.Reason != "retry" {
		t.Fatalf("next wake = %+v, want %s retry", next, want)
	}
}

func TestSuccessfulReconcileResetsBackoff(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	sourceErr := errors.New("store unavailable")
	source := &fakeCareTakerStateSource{err: sourceErr}
	caretaker := NewConfiguredCareTaker(CareTakerConfig{
		RetryInitial:     30 * time.Second,
		RetryMaximum:     5 * time.Minute,
		FallbackInterval: 10 * time.Minute,
	}, source, &fakeCareTakerLauncher{}, NewWorkerCapacityManager(nil), fakeCareTakerClock{now: now})

	if _, err := caretaker.reconcile(context.Background(), now); !errors.Is(err, sourceErr) {
		t.Fatalf("first reconcile() error = %v, want source error", err)
	}
	if caretaker.retryDelay != 30*time.Second {
		t.Fatalf("retryDelay = %s, want 30s after failure", caretaker.retryDelay)
	}

	source.err = nil
	next, err := caretaker.reconcile(context.Background(), now.Add(time.Second))
	if err != nil {
		t.Fatalf("second reconcile() error = %v", err)
	}
	if caretaker.retryDelay != 0 {
		t.Fatalf("retryDelay = %s, want reset after success", caretaker.retryDelay)
	}
	if next.Reason == "retry" {
		t.Fatalf("next wake = %+v, want non-retry after successful reconcile", next)
	}
}

func TestControllerRecoverExpiredWorkerSessionsAdapter(t *testing.T) {
	ctx := context.Background()
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	work := testPersistenceWorkItem("work-001", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []persistence.WorkItemRecord{work}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := store.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{{WorkItemRecord: work, QueuedAt: "2026-07-11T12:00:00Z"}}); err != nil {
		t.Fatalf("EnqueueWorkItems() error = %v", err)
	}
	if _, err := store.RegisterWorkerSession(ctx, persistence.RegisterWorkerSessionRequest{
		WorkerID:     "worker-expired",
		SessionID:    "session-expired",
		RegisteredAt: "2026-07-11T12:00:00Z",
	}); err != nil {
		t.Fatalf("RegisterWorkerSession() error = %v", err)
	}
	if _, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{
		AttemptID:       "attempt-001",
		WorkerID:        "worker-expired",
		WorkerSessionID: "session-expired",
		ExecutorType:    persistence.ExecutorTypeWorker,
		StartedAt:       "2026-07-11T12:00:01Z",
	}); err != nil || !found {
		t.Fatalf("ClaimNextWork() found=%v error=%v, want success", found, err)
	}

	summary, err := controller.RecoverExpiredWorkerSessions(ctx, time.Date(2026, 7, 11, 12, 6, 0, 0, time.UTC), 5*time.Minute)
	if err != nil {
		t.Fatalf("RecoverExpiredWorkerSessions() error = %v", err)
	}
	want := WorkerRecoverySummary{ExpiredSessions: 1, AbandonedAttempts: 1, RequeuedWorkItems: 1}
	if summary != want {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}
}

func TestControllerWorkerCapacitySnapshotIncludesEarliestHeartbeat(t *testing.T) {
	ctx := context.Background()
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	work := testPersistenceWorkItem("work-001", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []persistence.WorkItemRecord{work}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := store.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{{WorkItemRecord: work, QueuedAt: "2026-07-11T12:00:00Z"}}); err != nil {
		t.Fatalf("EnqueueWorkItems() error = %v", err)
	}
	if _, err := store.RegisterWorkerSession(ctx, persistence.RegisterWorkerSessionRequest{
		WorkerID:     "worker-a",
		SessionID:    "session-a",
		RegisteredAt: "2026-07-11T11:58:00Z",
	}); err != nil {
		t.Fatalf("RegisterWorkerSession(a) error = %v", err)
	}
	if _, err := store.RegisterWorkerSession(ctx, persistence.RegisterWorkerSessionRequest{
		WorkerID:     "worker-b",
		SessionID:    "session-b",
		RegisteredAt: "2026-07-11T11:59:00Z",
	}); err != nil {
		t.Fatalf("RegisterWorkerSession(b) error = %v", err)
	}

	snapshot, err := controller.WorkerCapacitySnapshot(ctx, time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC), 5*time.Minute)
	if err != nil {
		t.Fatalf("WorkerCapacitySnapshot() error = %v", err)
	}
	if snapshot.PendingQueued != 1 || snapshot.PendingClaimable != 1 || snapshot.LiveWorkerSessions != 2 {
		t.Fatalf("snapshot = %+v, want queued/claimable/live 1/1/2", snapshot)
	}
	wantHeartbeat := time.Date(2026, 7, 11, 11, 58, 0, 0, time.UTC)
	if !snapshot.EarliestHeartbeat.Equal(wantHeartbeat) {
		t.Fatalf("EarliestHeartbeat = %s, want %s", snapshot.EarliestHeartbeat, wantHeartbeat)
	}
}

type fakeCareTakerClock struct {
	now time.Time
}

func (c fakeCareTakerClock) Now() time.Time {
	return c.now
}

func (c fakeCareTakerClock) NewTimer(d time.Duration) CareTakerTimer {
	return fakeCareTakerTimer{ch: make(chan time.Time)}
}

type fakeCareTakerTimer struct {
	ch chan time.Time
}

func (t fakeCareTakerTimer) C() <-chan time.Time {
	return t.ch
}

func (t fakeCareTakerTimer) Stop() bool {
	return true
}

func (t fakeCareTakerTimer) Reset(d time.Duration) bool {
	return true
}

type fakeCareTakerStateSource struct {
	mu        sync.Mutex
	recovery  WorkerRecoverySummary
	snapshot  WorkerCapacitySnapshot
	err       error
	calls     []string
	events    *[]string
	recoverCh chan int
}

func (s *fakeCareTakerStateSource) RecoverExpiredWorkerSessions(ctx context.Context, now time.Time, deadAfter time.Duration) (WorkerRecoverySummary, error) {
	s.mu.Lock()
	s.calls = append(s.calls, "recover")
	if s.events != nil {
		*s.events = append(*s.events, "recover")
	}
	recoverCh := s.recoverCh
	recoverCalls := 0
	for _, call := range s.calls {
		if call == "recover" {
			recoverCalls++
		}
	}
	s.mu.Unlock()
	if recoverCh != nil {
		recoverCh <- recoverCalls
	}
	if s.err != nil {
		return WorkerRecoverySummary{}, s.err
	}
	return s.recovery, nil
}

func (s *fakeCareTakerStateSource) WorkerCapacitySnapshot(ctx context.Context, now time.Time, deadAfter time.Duration) (WorkerCapacitySnapshot, error) {
	s.mu.Lock()
	s.calls = append(s.calls, "snapshot")
	if s.events != nil {
		*s.events = append(*s.events, "snapshot")
	}
	s.mu.Unlock()
	if s.err != nil {
		return WorkerCapacitySnapshot{}, s.err
	}
	return s.snapshot, nil
}

type fakeCareTakerLauncher struct {
	calls  int
	counts []int
	err    error
	events *[]string
}

func (l *fakeCareTakerLauncher) StartWorkers(ctx context.Context, count int) error {
	l.calls++
	l.counts = append(l.counts, count)
	if l.events != nil {
		*l.events = append(*l.events, "launch")
	}
	return l.err
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type controllableCareTakerClock struct {
	mu      sync.Mutex
	now     time.Time
	timerCh chan *controllableCareTakerTimer
}

func (c *controllableCareTakerClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *controllableCareTakerClock) NewTimer(d time.Duration) CareTakerTimer {
	timer := &controllableCareTakerTimer{ch: make(chan time.Time, 1), duration: d}
	if c.timerCh != nil {
		c.timerCh <- timer
	}
	return timer
}

type controllableCareTakerTimer struct {
	ch       chan time.Time
	duration time.Duration
	stopped  bool
}

func (t *controllableCareTakerTimer) C() <-chan time.Time {
	return t.ch
}

func (t *controllableCareTakerTimer) Stop() bool {
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}

func (t *controllableCareTakerTimer) Reset(d time.Duration) bool {
	wasActive := !t.stopped
	t.duration = d
	t.stopped = false
	return wasActive
}

func (t *controllableCareTakerTimer) fire(at time.Time) {
	t.ch <- at
}

func runCareTakerForTest(caretaker *CareTaker, ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- caretaker.Run(ctx)
	}()
	return done
}

func waitForCareTakerCall(t *testing.T, ch <-chan int) int {
	t.Helper()
	select {
	case call := <-ch:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CareTaker call")
		return 0
	}
}

func waitForCareTakerTimer(t *testing.T, ch <-chan *controllableCareTakerTimer) *controllableCareTakerTimer {
	t.Helper()
	select {
	case timer := <-ch:
		return timer
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CareTaker timer")
		return nil
	}
}

func waitForCareTakerStop(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CareTaker Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CareTaker shutdown")
	}
}

func waitForCareTakerSignalCount(t *testing.T, caretaker *CareTaker, wantMin int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		count, _ := caretaker.signalSnapshot()
		if count >= wantMin {
			return
		}
		time.Sleep(time.Millisecond)
	}
	count, reason := caretaker.signalSnapshot()
	t.Fatalf("CareTaker signal count = %d/%q, want at least %d", count, reason, wantMin)
}

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"goetl/internal/persistence"
)

func TestOneByOneStartsWhenActiveBelowClaimable(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	pattern := OneByOneUntilSaturatedPattern{}

	plan := pattern.Plan(now, WorkerDemand{PendingClaimable: 5}, WorkerExecutionState{}, WorkerExecutionConfig{MaxActiveWorkers: 10})
	if plan.StartCount != 1 {
		t.Fatalf("StartCount = %d, want 1", plan.StartCount)
	}
	if plan.Reason != "active_capacity_below_claimable_work" {
		t.Fatalf("Reason = %q, want active_capacity_below_claimable_work", plan.Reason)
	}
}

func TestOneByOneDoesNotStartWhenNoClaimableWork(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	pattern := OneByOneUntilSaturatedPattern{}

	plan := pattern.Plan(now, WorkerDemand{PendingQueued: 5, PendingClaimable: 0}, WorkerExecutionState{}, WorkerExecutionConfig{MaxActiveWorkers: 10})
	if plan.StartCount != 0 {
		t.Fatalf("StartCount = %d, want 0", plan.StartCount)
	}
	if plan.Reason != "no_claimable_work" {
		t.Fatalf("Reason = %q, want no_claimable_work", plan.Reason)
	}
}

func TestOneByOneDoesNotStartWhenActiveEqualsClaimable(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	pattern := OneByOneUntilSaturatedPattern{}

	plan := pattern.Plan(now, WorkerDemand{PendingClaimable: 2, RunningAttempts: 2}, WorkerExecutionState{}, WorkerExecutionConfig{MaxActiveWorkers: 10})
	if plan.StartCount != 0 {
		t.Fatalf("StartCount = %d, want 0", plan.StartCount)
	}
	if plan.Reason != "active_capacity_satisfies_claimable_work" {
		t.Fatalf("Reason = %q, want active_capacity_satisfies_claimable_work", plan.Reason)
	}
}

func TestOneByOneDoesNotStartWhenMaxActiveReached(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	pattern := OneByOneUntilSaturatedPattern{}

	plan := pattern.Plan(now, WorkerDemand{PendingClaimable: 5, RunningAttempts: 2}, WorkerExecutionState{}, WorkerExecutionConfig{MaxActiveWorkers: 2})
	if plan.StartCount != 0 {
		t.Fatalf("StartCount = %d, want 0", plan.StartCount)
	}
	if plan.Reason != "max_active_workers_reached" {
		t.Fatalf("Reason = %q, want max_active_workers_reached", plan.Reason)
	}
}

func TestOneByOneCountsInflightStartAsActive(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	pattern := OneByOneUntilSaturatedPattern{}
	state := WorkerExecutionState{
		InflightStarts: []WorkerStartReservation{{ID: "start-1", CreatedAt: now.Add(-time.Minute)}},
	}

	plan := pattern.Plan(now, WorkerDemand{PendingClaimable: 1}, state, WorkerExecutionConfig{
		MaxActiveWorkers:     10,
		InflightStartTimeout: 5 * time.Minute,
	})
	if plan.StartCount != 0 {
		t.Fatalf("StartCount = %d, want 0", plan.StartCount)
	}
	if plan.Reason != "waiting_for_inflight_start_claim" {
		t.Fatalf("Reason = %q, want waiting_for_inflight_start_claim", plan.Reason)
	}
}

func TestOneByOneWaitsForInflightStartEvenWhenMoreWorkIsClaimable(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	pattern := OneByOneUntilSaturatedPattern{}
	state := WorkerExecutionState{
		InflightStarts: []WorkerStartReservation{{ID: "start-1", CreatedAt: now.Add(-time.Minute)}},
	}

	plan := pattern.Plan(now, WorkerDemand{PendingClaimable: 5}, state, WorkerExecutionConfig{
		MaxActiveWorkers:     10,
		InflightStartTimeout: 5 * time.Minute,
	})
	if plan.StartCount != 0 {
		t.Fatalf("StartCount = %d, want 0", plan.StartCount)
	}
	if plan.Reason != "waiting_for_inflight_start_claim" {
		t.Fatalf("Reason = %q, want waiting_for_inflight_start_claim", plan.Reason)
	}
}

func TestOneByOneIgnoresExpiredInflightStart(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	pattern := OneByOneUntilSaturatedPattern{}
	state := WorkerExecutionState{
		InflightStarts: []WorkerStartReservation{{ID: "start-1", CreatedAt: now.Add(-10 * time.Minute)}},
	}

	plan := pattern.Plan(now, WorkerDemand{PendingClaimable: 1}, state, WorkerExecutionConfig{
		MaxActiveWorkers:     10,
		InflightStartTimeout: 5 * time.Minute,
	})
	if plan.StartCount != 1 {
		t.Fatalf("StartCount = %d, want 1", plan.StartCount)
	}
}

func TestNullWorkerExecutionPatternDoesNotStart(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	pattern := NullWorkerExecutionPattern{}

	plan := pattern.Plan(now, WorkerDemand{PendingClaimable: 5}, WorkerExecutionState{}, WorkerExecutionConfig{})
	if plan.StartCount != 0 {
		t.Fatalf("StartCount = %d, want 0", plan.StartCount)
	}
	if plan.Reason != "worker_scheduling_disabled" {
		t.Fatalf("Reason = %q, want worker_scheduling_disabled", plan.Reason)
	}
}

func TestEvaluateWorkerCapacityStartsOneWorker(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	manager := NewWorkerCapacityManager(nil)
	starts := 0

	plan, err := manager.Evaluate(context.Background(), now, defaultWorkerExecutionConfig(), fixedDemand(WorkerDemand{
		PendingQueued:    5,
		PendingClaimable: 5,
	}), func(ctx context.Context, count int) error {
		starts += count
		return nil
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if plan.StartCount != 1 {
		t.Fatalf("StartCount = %d, want 1", plan.StartCount)
	}
	if starts != 1 {
		t.Fatalf("starts = %d, want 1", starts)
	}
}

func TestEvaluateWorkerCapacityNullPatternDoesNotLaunchOrReserve(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	manager := NewWorkerCapacityManager(nil)
	cfg := defaultWorkerExecutionConfig()
	cfg.Pattern = workerExecutionPatternNull

	plan, err := manager.Evaluate(context.Background(), now, cfg, fixedDemand(WorkerDemand{
		PendingQueued:    5,
		PendingClaimable: 5,
	}), func(ctx context.Context, count int) error {
		t.Fatalf("launchFn called with count %d", count)
		return nil
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if plan.StartCount != 0 {
		t.Fatalf("StartCount = %d, want 0", plan.StartCount)
	}
	if plan.Reason != "worker_scheduling_disabled" {
		t.Fatalf("Reason = %q, want worker_scheduling_disabled", plan.Reason)
	}
	state := manager.Snapshot()
	if len(state.InflightStarts) != 0 {
		t.Fatalf("inflight starts = %d, want 0", len(state.InflightStarts))
	}
}

func TestEvaluateWorkerCapacityRecordsInflightBeforeLaunch(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	manager := NewWorkerCapacityManager(nil)

	_, err := manager.Evaluate(context.Background(), now, defaultWorkerExecutionConfig(), fixedDemand(WorkerDemand{
		PendingQueued:    5,
		PendingClaimable: 5,
	}), func(ctx context.Context, count int) error {
		if len(manager.state.InflightStarts) != 1 {
			t.Fatalf("inflight starts during launch = %d, want 1", len(manager.state.InflightStarts))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
}

func TestEvaluateWorkerCapacityRemovesInflightOnLaunchFailure(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	manager := NewWorkerCapacityManager(nil)
	launchErr := errors.New("launch failed")

	_, err := manager.Evaluate(context.Background(), now, defaultWorkerExecutionConfig(), fixedDemand(WorkerDemand{
		PendingQueued:    5,
		PendingClaimable: 5,
	}), func(ctx context.Context, count int) error {
		return launchErr
	})
	if !errors.Is(err, launchErr) {
		t.Fatalf("Evaluate() error = %v, want %v", err, launchErr)
	}
	state := manager.Snapshot()
	if len(state.InflightStarts) != 0 {
		t.Fatalf("inflight starts after failure = %d, want 0", len(state.InflightStarts))
	}
}

func TestWorkerClaimConfirmsInflightStart(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	manager := NewWorkerCapacityManager(nil)

	_, err := manager.Evaluate(context.Background(), now, defaultWorkerExecutionConfig(), fixedDemand(WorkerDemand{
		PendingQueued:    2,
		PendingClaimable: 2,
	}), func(ctx context.Context, count int) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if !manager.ConfirmInflightStartClaimed() {
		t.Fatal("ConfirmInflightStartClaimed() = false, want true")
	}
	state := manager.Snapshot()
	if len(state.InflightStarts) != 0 {
		t.Fatalf("inflight starts after claim = %d, want 0", len(state.InflightStarts))
	}
}

func TestWorkerClaimCanTriggerNextOneByOneStart(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	manager := NewWorkerCapacityManager(nil)
	starts := 0
	launch := func(ctx context.Context, count int) error {
		starts += count
		return nil
	}

	_, err := manager.Evaluate(context.Background(), now, defaultWorkerExecutionConfig(), fixedDemand(WorkerDemand{
		PendingQueued:    3,
		PendingClaimable: 3,
	}), launch)
	if err != nil {
		t.Fatalf("first Evaluate() error = %v", err)
	}
	manager.ConfirmInflightStartClaimed()
	_, err = manager.Evaluate(context.Background(), now.Add(time.Second), defaultWorkerExecutionConfig(), fixedDemand(WorkerDemand{
		PendingQueued:    2,
		PendingClaimable: 2,
		RunningAttempts:  1,
	}), launch)
	if err != nil {
		t.Fatalf("second Evaluate() error = %v", err)
	}
	if starts != 2 {
		t.Fatalf("starts = %d, want 2", starts)
	}
}

func TestEvaluateWorkerCapacityDoesNotStartForResourceBlockedQueuedWork(t *testing.T) {
	ctx := context.Background()
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	run := insertTestPersistenceRunWithStage(t, ctx, store)

	resourceKey := "target:local/memory-mib"
	running := testPersistenceWorkItem("running-work", run.ID, 0, 0)
	blocked := testPersistenceWorkItem("blocked-work", run.ID, 0, 1)
	queuedAt := "2026-07-09T12:00:00Z"

	if err := store.QueueWorkItems(ctx, persistence.QueueWorkItemsRequest{
		WorkItems: []persistence.WorkItemRecord{running},
		ResourceConstraints: []persistence.WorkItemResourceConstraintRecord{
			testWorkerExecutionResourceConstraint(running.ID, resourceKey),
		},
		QueuedWork: []persistence.QueuedWorkRecord{{WorkItemRecord: running, QueuedAt: queuedAt}},
	}); err != nil {
		t.Fatalf("QueueWorkItems(running) error = %v", err)
	}
	if _, found, err := store.ClaimNextWork(ctx, testWorkerClaimRequest(t, store, "attempt-running", queuedAt)); err != nil || !found {
		t.Fatalf("ClaimNextWork(running) found = %v error = %v, want success", found, err)
	}
	if err := store.QueueWorkItems(ctx, persistence.QueueWorkItemsRequest{
		WorkItems: []persistence.WorkItemRecord{blocked},
		ResourceConstraints: []persistence.WorkItemResourceConstraintRecord{
			testWorkerExecutionResourceConstraint(blocked.ID, resourceKey),
		},
		QueuedWork: []persistence.QueuedWorkRecord{{WorkItemRecord: blocked, QueuedAt: queuedAt}},
	}); err != nil {
		t.Fatalf("QueueWorkItems(blocked) error = %v", err)
	}

	starter := &testWorkerStarter{}
	controller := newController()
	controller.workflowStore = store
	controller.workerStarter = starter
	controller.launchResolver = testLocalWorkerLaunchResolver()

	if err := controller.EvaluateWorkerCapacity(ctx, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("EvaluateWorkerCapacity() error = %v", err)
	}
	if starter.calls != 0 {
		t.Fatalf("worker starter calls = %d, want 0", starter.calls)
	}
}

func TestEvaluateWorkerCapacitySkipsMissingWorkerTargetWithoutInflightReservation(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	ctx := context.Background()
	queuedAt := "2026-07-09T12:00:00Z"
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	work := testPersistenceWorkItem("queued-no-worker-target", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []persistence.WorkItemRecord{work}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := store.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{{WorkItemRecord: work, QueuedAt: queuedAt}}); err != nil {
		t.Fatalf("EnqueueWorkItems() error = %v", err)
	}
	controller := newController()
	controller.workflowStore = store

	if err := controller.EvaluateWorkerCapacity(ctx, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("EvaluateWorkerCapacity() error = %v", err)
	}
	state := controller.workerExecutor.Snapshot()
	if len(state.InflightStarts) != 0 {
		t.Fatalf("inflight starts = %d, want 0", len(state.InflightStarts))
	}
}

func fixedDemand(demand WorkerDemand) func(context.Context) (WorkerDemand, error) {
	return func(context.Context) (WorkerDemand, error) {
		return demand, nil
	}
}

func testWorkerExecutionResourceConstraint(workItemID string, resourceKey string) persistence.WorkItemResourceConstraintRecord {
	return persistence.WorkItemResourceConstraintRecord{
		WorkItemID:      workItemID,
		ConstraintIndex: 0,
		ResourceKey:     resourceKey,
		RequestedUnits:  1,
		Operator:        "<=",
		TargetUnits:     1,
		CreatedAt:       "2026-07-09T12:00:00Z",
	}
}

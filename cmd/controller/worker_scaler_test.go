package main

import (
	"testing"
	"time"
)

func TestWorkerScaleStateStartsMinCountImmediately(t *testing.T) {
	now := time.Now()
	state := WorkerScaleState{}
	cfg := WorkerScaleConfig{
		MinCount:                10,
		MaxCount:                10,
		CountPerStart:           10,
		MinElapsedBetweenStarts: 30 * time.Second,
	}

	starts := state.PlanStarts(now, 10, 0, cfg)
	if starts != 10 {
		t.Fatalf("unexpected start count: %d", starts)
	}
}

func TestWorkerScaleStateCapsStartsByPendingWork(t *testing.T) {
	now := time.Now()
	state := WorkerScaleState{}
	cfg := WorkerScaleConfig{
		MinCount:      10,
		MaxCount:      10,
		CountPerStart: 10,
	}

	starts := state.PlanStarts(now, 3, 0, cfg)
	if starts != 3 {
		t.Fatalf("unexpected start count: %d", starts)
	}
}

func TestWorkerScaleStateBlocksOrganicStartUntilPriorWorkerClaimsWork(t *testing.T) {
	now := time.Now()
	state := WorkerScaleState{}
	cfg := WorkerScaleConfig{
		MaxCount:                2,
		CountPerStart:           1,
		MinElapsedBetweenStarts: 30 * time.Second,
	}

	starts := state.PlanStarts(now, 10, 0, cfg)
	if starts != 1 {
		t.Fatalf("unexpected first start count: %d", starts)
	}

	state.RecordStart(now, starts, 0)

	starts = state.PlanStarts(now.Add(time.Minute), 9, 0, cfg)
	if starts != 0 {
		t.Fatalf("unexpected unconfirmed start count: %d", starts)
	}

	starts = state.PlanStarts(now.Add(time.Minute), 9, 1, cfg)
	if starts != 1 {
		t.Fatalf("unexpected confirmed start count: %d", starts)
	}
}

func TestWorkerScaleStateHonorsElapsedTimeBetweenOrganicStarts(t *testing.T) {
	now := time.Now()
	state := WorkerScaleState{}
	cfg := WorkerScaleConfig{
		MaxCount:                2,
		CountPerStart:           1,
		MinElapsedBetweenStarts: 30 * time.Second,
	}

	starts := state.PlanStarts(now, 10, 0, cfg)
	state.RecordStart(now, starts, 0)

	starts = state.PlanStarts(now.Add(10*time.Second), 9, 1, cfg)
	if starts != 0 {
		t.Fatalf("unexpected throttled start count: %d", starts)
	}

	starts = state.PlanStarts(now.Add(30*time.Second), 9, 1, cfg)
	if starts != 1 {
		t.Fatalf("unexpected unthrottled start count: %d", starts)
	}
}

func TestWorkerScaleStateHonorsMaxWorkers(t *testing.T) {
	now := time.Now()
	state := WorkerScaleState{StartedWorkers: 2}
	cfg := WorkerScaleConfig{
		MaxCount:      2,
		CountPerStart: 1,
	}

	starts := state.PlanStarts(now, 10, 2, cfg)
	if starts != 0 {
		t.Fatalf("unexpected start count: %d", starts)
	}
}

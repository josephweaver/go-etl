package main

import (
	"testing"
	"time"
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

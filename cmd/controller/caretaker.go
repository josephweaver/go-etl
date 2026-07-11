package main

import (
	"sync"
	"time"
)

type CareTakerClock interface {
	Now() time.Time
	NewTimer(time.Duration) CareTakerTimer
}

type CareTakerTimer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(time.Duration) bool
}

type realCareTakerClock struct{}

func (realCareTakerClock) Now() time.Time {
	return time.Now().UTC()
}

func (realCareTakerClock) NewTimer(d time.Duration) CareTakerTimer {
	return realCareTakerTimer{timer: time.NewTimer(d)}
}

type realCareTakerTimer struct {
	timer *time.Timer
}

func (t realCareTakerTimer) C() <-chan time.Time {
	return t.timer.C
}

func (t realCareTakerTimer) Stop() bool {
	return t.timer.Stop()
}

func (t realCareTakerTimer) Reset(d time.Duration) bool {
	return t.timer.Reset(d)
}

type WorkerCapacitySnapshot struct {
	PendingQueued      int
	PendingClaimable   int
	RunningAttempts    int
	LiveWorkerSessions int
	EarliestHeartbeat  time.Time
}

type WorkerRecoverySummary struct {
	ExpiredSessions   int
	AbandonedAttempts int
	RequeuedWorkItems int
}

type CareTakerConfig struct {
	DeadAfter            time.Duration
	InflightStartTimeout time.Duration
	FallbackInterval     time.Duration
	RetryInitial         time.Duration
	RetryMaximum         time.Duration
	WorkerExecution      WorkerExecutionConfig
}

type CareTakerNextWake struct {
	At     time.Time
	Reason string
}

type CareTaker struct {
	wakeCh chan struct{}
	clock  CareTakerClock

	mu          sync.Mutex
	signalCount int
	lastSignal  string
}

func NewCareTaker(clock CareTakerClock) *CareTaker {
	if clock == nil {
		clock = realCareTakerClock{}
	}
	return &CareTaker{
		wakeCh: make(chan struct{}, 1),
		clock:  clock,
	}
}

func (c *CareTaker) Signal(reason string) {
	if c == nil {
		return
	}
	c.recordSignal(reason)
	select {
	case c.wakeCh <- struct{}{}:
	default:
	}
}

func (c *CareTaker) recordSignal(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.signalCount++
	c.lastSignal = reason
}

func (c *CareTaker) signalSnapshot() (int, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.signalCount, c.lastSignal
}

func calculateCareTakerNextWake(now time.Time, cfg CareTakerConfig, snapshot WorkerCapacitySnapshot, state WorkerExecutionState, retryDelay time.Duration) CareTakerNextWake {
	var next CareTakerNextWake
	consider := func(at time.Time, reason string) {
		if at.IsZero() {
			return
		}
		if at.Before(now) {
			at = now
		}
		if next.At.IsZero() || at.Before(next.At) {
			next = CareTakerNextWake{At: at, Reason: reason}
		}
	}

	if cfg.DeadAfter > 0 && !snapshot.EarliestHeartbeat.IsZero() {
		consider(snapshot.EarliestHeartbeat.Add(cfg.DeadAfter), "worker_expiry")
	}
	if cfg.InflightStartTimeout > 0 {
		for _, reservation := range state.InflightStarts {
			consider(reservation.CreatedAt.Add(cfg.InflightStartTimeout), "inflight_start_expiry")
		}
	}
	if retryDelay > 0 {
		if cfg.RetryMaximum > 0 && retryDelay > cfg.RetryMaximum {
			retryDelay = cfg.RetryMaximum
		}
		if cfg.RetryInitial > 0 && retryDelay < cfg.RetryInitial {
			retryDelay = cfg.RetryInitial
		}
		consider(now.Add(retryDelay), "retry")
	}
	if cfg.FallbackInterval > 0 {
		consider(now.Add(cfg.FallbackInterval), "fallback_sweep")
	}

	return next
}

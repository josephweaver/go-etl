package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"goetl/internal/persistence"
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

type CareTakerStateSource interface {
	RecoverExpiredWorkerSessions(ctx context.Context, now time.Time, deadAfter time.Duration) (WorkerRecoverySummary, error)
	WorkerCapacitySnapshot(ctx context.Context, now time.Time, deadAfter time.Duration) (WorkerCapacitySnapshot, error)
}

type CareTakerWorkerLauncher interface {
	StartWorkers(ctx context.Context, count int) error
}

type CareTaker struct {
	wakeCh chan struct{}
	clock  CareTakerClock
	cfg    CareTakerConfig
	source CareTakerStateSource
	launch CareTakerWorkerLauncher
	exec   *WorkerCapacityManager

	mu          sync.Mutex
	signalCount int
	lastSignal  string
	retryDelay  time.Duration
}

func NewCareTaker(clock CareTakerClock) *CareTaker {
	return NewConfiguredCareTaker(CareTakerConfig{}, nil, nil, nil, clock)
}

func NewConfiguredCareTaker(cfg CareTakerConfig, source CareTakerStateSource, launcher CareTakerWorkerLauncher, exec *WorkerCapacityManager, clock CareTakerClock) *CareTaker {
	if clock == nil {
		clock = realCareTakerClock{}
	}
	if exec == nil {
		exec = NewWorkerCapacityManager(nil)
	}
	return &CareTaker{
		wakeCh: make(chan struct{}, 1),
		clock:  clock,
		cfg:    cfg,
		source: source,
		launch: launcher,
		exec:   exec,
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

func (c *CareTaker) Run(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if c.clock == nil {
		c.clock = realCareTakerClock{}
	}

	c.Signal("startup")

	var timer CareTakerTimer
	var timerC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			stopAndDrainCareTakerTimer(timer)
			return nil
		case <-c.wakeCh:
			drainCareTakerSignals(c.wakeCh)
			next, err := c.reconcile(ctx, c.clock.Now())
			if ctx.Err() != nil {
				stopAndDrainCareTakerTimer(timer)
				return nil
			}
			if err != nil {
				fmt.Printf("caretaker reconcile failed: %v\n", err)
			}
			timer, timerC = resetCareTakerTimer(c.clock, timer, next)
		case <-timerC:
			next, err := c.reconcile(ctx, c.clock.Now())
			if ctx.Err() != nil {
				stopAndDrainCareTakerTimer(timer)
				return nil
			}
			if err != nil {
				fmt.Printf("caretaker reconcile failed: %v\n", err)
			}
			timer, timerC = resetCareTakerTimer(c.clock, timer, next)
		}
	}
}

func drainCareTakerSignals(ch <-chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func stopAndDrainCareTakerTimer(timer CareTakerTimer) {
	if timer == nil {
		return
	}
	if timer.Stop() {
		return
	}
	select {
	case <-timer.C():
	default:
	}
}

func resetCareTakerTimer(clock CareTakerClock, timer CareTakerTimer, next CareTakerNextWake) (CareTakerTimer, <-chan time.Time) {
	if next.At.IsZero() {
		stopAndDrainCareTakerTimer(timer)
		return nil, nil
	}
	if clock == nil {
		clock = realCareTakerClock{}
	}
	delay := next.At.Sub(clock.Now())
	if delay < 0 {
		delay = 0
	}
	if timer == nil {
		timer = clock.NewTimer(delay)
		return timer, timer.C()
	}
	stopAndDrainCareTakerTimer(timer)
	timer.Reset(delay)
	return timer, timer.C()
}

func (c *CareTaker) reconcile(ctx context.Context, now time.Time) (CareTakerNextWake, error) {
	if c.source == nil {
		return CareTakerNextWake{}, nil
	}
	if c.launch == nil {
		return CareTakerNextWake{}, nil
	}
	if c.exec == nil {
		c.exec = NewWorkerCapacityManager(nil)
	}

	if _, err := c.source.RecoverExpiredWorkerSessions(ctx, now, c.cfg.DeadAfter); err != nil {
		c.noteReconcileError()
		return calculateCareTakerNextWake(now, c.cfg, WorkerCapacitySnapshot{}, c.exec.Snapshot(), c.retryDelay), err
	}
	c.exec.PruneExpiredInflightStarts(now, c.cfg.InflightStartTimeout)
	snapshot, err := c.source.WorkerCapacitySnapshot(ctx, now, c.cfg.DeadAfter)
	if err != nil {
		c.noteReconcileError()
		return calculateCareTakerNextWake(now, c.cfg, WorkerCapacitySnapshot{}, c.exec.Snapshot(), c.retryDelay), err
	}

	_, err = c.exec.EvaluateSnapshot(ctx, now, c.cfg.WorkerExecution, snapshot, c.launch.StartWorkers)
	if err != nil {
		c.noteReconcileError()
		return calculateCareTakerNextWake(now, c.cfg, snapshot, c.exec.Snapshot(), c.retryDelay), err
	}
	c.clearReconcileError()
	return calculateCareTakerNextWake(now, c.cfg, snapshot, c.exec.Snapshot(), 0), nil
}

func (c *CareTaker) noteReconcileError() {
	if c.cfg.RetryInitial <= 0 {
		c.retryDelay = 0
		return
	}
	if c.retryDelay <= 0 {
		c.retryDelay = c.cfg.RetryInitial
		return
	}
	c.retryDelay *= 2
	if c.cfg.RetryMaximum > 0 && c.retryDelay > c.cfg.RetryMaximum {
		c.retryDelay = c.cfg.RetryMaximum
	}
}

func (c *CareTaker) clearReconcileError() {
	c.retryDelay = 0
}

func (c *Controller) RecoverExpiredWorkerSessions(ctx context.Context, now time.Time, deadAfter time.Duration) (WorkerRecoverySummary, error) {
	if c.workflowStore == nil {
		return WorkerRecoverySummary{}, fmt.Errorf("workflow store required")
	}
	result, err := c.workflowStore.RecoverExpiredWorkerSessions(ctx, persistence.RecoverExpiredWorkerSessionsRequest{
		Cutoff:      now.Add(-deadAfter).Format(time.RFC3339Nano),
		RecoveredAt: now.UTC().Format(time.RFC3339Nano),
		Reason:      "heartbeat_expired",
	})
	if err != nil {
		return WorkerRecoverySummary{}, err
	}
	return WorkerRecoverySummary{
		ExpiredSessions:   result.ExpiredSessions,
		AbandonedAttempts: result.AbandonedAttempts,
		RequeuedWorkItems: result.RequeuedWorkItems,
	}, nil
}

func (c *Controller) WorkerCapacitySnapshot(ctx context.Context, now time.Time, deadAfter time.Duration) (WorkerCapacitySnapshot, error) {
	demand, err := c.workerDemand(ctx, now, deadAfter)
	if err != nil {
		return WorkerCapacitySnapshot{}, err
	}
	live, err := c.workflowStore.ListLiveWorkerSessions(ctx, now.Add(-deadAfter))
	if err != nil {
		return WorkerCapacitySnapshot{}, fmt.Errorf("list live worker sessions: %w", err)
	}
	var earliest time.Time
	for _, session := range live {
		heartbeat, err := time.Parse(time.RFC3339Nano, session.LastHeartbeatAt)
		if err != nil {
			return WorkerCapacitySnapshot{}, fmt.Errorf("parse worker session heartbeat %s: %w", session.ID, err)
		}
		if earliest.IsZero() || heartbeat.Before(earliest) {
			earliest = heartbeat
		}
	}
	return WorkerCapacitySnapshot{
		PendingQueued:      demand.PendingQueued,
		PendingClaimable:   demand.PendingClaimable,
		RunningAttempts:    demand.RunningAttempts,
		LiveWorkerSessions: demand.LiveWorkerSessions,
		EarliestHeartbeat:  earliest,
	}, nil
}

func (c *Controller) StartWorkers(ctx context.Context, count int) error {
	return c.startWorkers(ctx, c.launchResolver, count)
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

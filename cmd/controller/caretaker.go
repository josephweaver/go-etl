package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"goetl/internal/persistence"
	"goetl/internal/variable"
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

type CareTakerReconcileResult struct {
	Recovery WorkerRecoverySummary
	Snapshot WorkerCapacitySnapshot
	Plan     WorkerStartPlan
	State    WorkerExecutionState
	NextWake CareTakerNextWake
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

func controllerCareTakerConfig(resolver variable.Resolver, policy controllerOperationalPolicy) (CareTakerConfig, error) {
	heartbeat, err := workerHeartbeatPolicyConfig(resolver, defaultWorkerHeartbeatPolicy())
	if err != nil {
		return CareTakerConfig{}, err
	}
	workerExecution, err := workerExecutionConfig(resolver, defaultWorkerExecutionConfig())
	if err != nil {
		return CareTakerConfig{}, err
	}
	fallback := time.Duration(policy.CaretakerIntervalScheduleMillis) * time.Millisecond
	retryMaximum := fallback
	if policy.CaretakerMissedIntervalLimit > 1 {
		retryMaximum = fallback * time.Duration(policy.CaretakerMissedIntervalLimit)
	}
	return CareTakerConfig{
		DeadAfter:            heartbeat.DeadAfter,
		InflightStartTimeout: workerExecution.InflightStartTimeout,
		FallbackInterval:     fallback,
		RetryInitial:         fallback,
		RetryMaximum:         retryMaximum,
		WorkerExecution:      workerExecution,
	}, nil
}

func (c *Controller) startCareTaker(parent context.Context, cfg CareTakerConfig) {
	if c == nil || c.caretaker != nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	caretaker := NewConfiguredCareTaker(cfg, c, c, c.workerExecutor, realCareTakerClock{})
	done := make(chan error, 1)
	c.caretaker = caretaker
	c.caretakerCancel = cancel
	c.caretakerDone = done
	c.workerStateChanged = caretaker.Signal
	go func() {
		done <- caretaker.Run(ctx)
	}()
}

func (c *Controller) stopCareTaker() error {
	if c == nil || c.caretakerCancel == nil || c.caretakerDone == nil {
		return nil
	}
	cancel := c.caretakerCancel
	done := c.caretakerDone
	cancel()
	err := <-done
	c.caretakerCancel = nil
	c.caretakerDone = nil
	return err
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
	fmt.Println("caretaker_started")
	defer fmt.Println("caretaker_stopped")

	var timer CareTakerTimer
	var timerC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			stopAndDrainCareTakerTimer(timer)
			return nil
		case <-c.wakeCh:
			drainCareTakerSignals(c.wakeCh)
			count, reason := c.signalSnapshot()
			next, ok := c.runReconcile(ctx, "signal", count, reason)
			if !ok {
				stopAndDrainCareTakerTimer(timer)
				return nil
			}
			timer, timerC = resetCareTakerTimer(c.clock, timer, next)
			logCareTakerSleeping(next)
		case <-timerC:
			next, ok := c.runReconcile(ctx, "timer", 0, "")
			if !ok {
				stopAndDrainCareTakerTimer(timer)
				return nil
			}
			timer, timerC = resetCareTakerTimer(c.clock, timer, next)
			logCareTakerSleeping(next)
		}
	}
}

func (c *CareTaker) runReconcile(ctx context.Context, trigger string, signalCount int, signalReason string) (CareTakerNextWake, bool) {
	started := c.clock.Now()
	fmt.Printf("caretaker_reconcile_started trigger=%s signal_count=%d signal_reason=%q\n", trigger, signalCount, signalReason)
	result, err := c.reconcileDetailed(ctx, started)
	if ctx.Err() != nil {
		return CareTakerNextWake{}, false
	}
	duration := c.clock.Now().Sub(started)
	if err != nil {
		fmt.Printf("caretaker_reconcile_failed trigger=%s error=%q next_wake_at=%s next_wake_reason=%s duration=%s\n",
			trigger,
			err.Error(),
			formatCareTakerWake(result.NextWake.At),
			result.NextWake.Reason,
			duration,
		)
		return result.NextWake, true
	}
	fmt.Printf("caretaker_reconcile_completed trigger=%s pending_queued=%d pending_claimable=%d running_attempts=%d live_worker_sessions=%d inflight_starts=%d start_count=%d plan_reason=%s expired_sessions=%d abandoned_attempts=%d requeued_work_items=%d next_wake_at=%s next_wake_reason=%s duration=%s\n",
		trigger,
		result.Snapshot.PendingQueued,
		result.Snapshot.PendingClaimable,
		result.Snapshot.RunningAttempts,
		result.Snapshot.LiveWorkerSessions,
		len(result.State.InflightStarts),
		result.Plan.StartCount,
		result.Plan.Reason,
		result.Recovery.ExpiredSessions,
		result.Recovery.AbandonedAttempts,
		result.Recovery.RequeuedWorkItems,
		formatCareTakerWake(result.NextWake.At),
		result.NextWake.Reason,
		duration,
	)
	return result.NextWake, true
}

func logCareTakerSleeping(next CareTakerNextWake) {
	fmt.Printf("caretaker_sleeping next_wake_at=%s next_wake_reason=%s\n", formatCareTakerWake(next.At), next.Reason)
}

func formatCareTakerWake(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339Nano)
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
	result, err := c.reconcileDetailed(ctx, now)
	return result.NextWake, err
}

func (c *CareTaker) reconcileDetailed(ctx context.Context, now time.Time) (CareTakerReconcileResult, error) {
	var result CareTakerReconcileResult
	if c.source == nil {
		return result, nil
	}
	if c.launch == nil {
		return result, nil
	}
	if c.exec == nil {
		c.exec = NewWorkerCapacityManager(nil)
	}

	recovery, err := c.source.RecoverExpiredWorkerSessions(ctx, now, c.cfg.DeadAfter)
	result.Recovery = recovery
	if err != nil {
		c.noteReconcileError()
		result.State = c.exec.Snapshot()
		result.NextWake = calculateCareTakerNextWake(now, c.cfg, WorkerCapacitySnapshot{}, result.State, c.retryDelay)
		return result, err
	}
	c.exec.PruneExpiredInflightStarts(now, c.cfg.InflightStartTimeout)
	snapshot, err := c.source.WorkerCapacitySnapshot(ctx, now, c.cfg.DeadAfter)
	result.Snapshot = snapshot
	if err != nil {
		c.noteReconcileError()
		result.State = c.exec.Snapshot()
		result.NextWake = calculateCareTakerNextWake(now, c.cfg, WorkerCapacitySnapshot{}, result.State, c.retryDelay)
		return result, err
	}

	plan, err := c.exec.EvaluateSnapshot(ctx, now, c.cfg.WorkerExecution, snapshot, c.launch.StartWorkers)
	result.Plan = plan
	result.State = c.exec.Snapshot()
	if err != nil {
		if errors.Is(err, errWorkerTargetEnvironmentNotConfigured) {
			fmt.Println("worker_start_skipped reason=worker_target_environment_not_configured")
			c.clearReconcileError()
			result.NextWake = calculateCareTakerNextWake(now, c.cfg, snapshot, result.State, 0)
			return result, nil
		}
		c.noteReconcileError()
		result.State = c.exec.Snapshot()
		result.NextWake = calculateCareTakerNextWake(now, c.cfg, snapshot, result.State, c.retryDelay)
		return result, err
	}
	c.clearReconcileError()
	result.State = c.exec.Snapshot()
	result.NextWake = calculateCareTakerNextWake(now, c.cfg, snapshot, result.State, 0)
	return result, nil
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

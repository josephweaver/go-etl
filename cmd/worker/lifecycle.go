package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"goetl/internal/controllerhttp"
	"goetl/internal/model"
)

var ErrWorkerSessionNotActive = errors.New("worker session is not active")
var ErrWorkerSelfFenced = errors.New("worker self-fenced after missed heartbeat")

type WorkerSession struct {
	WorkerID          string
	WorkerSessionID   string
	HeartbeatInterval time.Duration
	DeadAfter         time.Duration
}

type WorkerHeartbeatFunc func(context.Context, WorkerSession) error

type WorkerLifecycleClock interface {
	Now() time.Time
	NewTicker(time.Duration) WorkerLifecycleTicker
}

type WorkerLifecycleTicker interface {
	C() <-chan time.Time
	Stop()
}

type realWorkerLifecycleClock struct{}

type realWorkerLifecycleTicker struct {
	ticker *time.Ticker
}

func (realWorkerLifecycleClock) Now() time.Time {
	return time.Now().UTC()
}

func (realWorkerLifecycleClock) NewTicker(interval time.Duration) WorkerLifecycleTicker {
	return realWorkerLifecycleTicker{ticker: time.NewTicker(interval)}
}

func (t realWorkerLifecycleTicker) C() <-chan time.Time {
	return t.ticker.C
}

func (t realWorkerLifecycleTicker) Stop() {
	t.ticker.Stop()
}

func RunHeartbeat(ctx context.Context, session WorkerSession, heartbeat WorkerHeartbeatFunc, clock WorkerLifecycleClock) error {
	if err := session.Validate(); err != nil {
		return err
	}
	if heartbeat == nil {
		return fmt.Errorf("worker heartbeat function is required")
	}
	if clock == nil {
		clock = realWorkerLifecycleClock{}
	}

	ticker := clock.NewTicker(session.HeartbeatInterval)
	defer ticker.Stop()

	lastAccepted := clock.Now()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C():
			if err := heartbeat(ctx, session); err != nil {
				if errors.Is(err, ErrWorkerSessionNotActive) {
					return ErrWorkerSessionNotActive
				}
				if clock.Now().Sub(lastAccepted) >= session.DeadAfter {
					return ErrWorkerSelfFenced
				}
				continue
			}
			lastAccepted = clock.Now()
		}
	}
}

func (c WorkerControllerClient) RegisterWorker(ctx context.Context, request model.WorkerRegistrationRequest) (WorkerSession, error) {
	httpRequest, err := c.newJSONRequest(ctx, http.MethodPost, "/workers/register", request)
	if err != nil {
		return WorkerSession{}, fmt.Errorf("create worker registration request: %w", err)
	}
	response, err := c.client.Do(httpRequest, http.StatusCreated)
	if err != nil {
		return WorkerSession{}, fmt.Errorf("post worker registration: %w", err)
	}
	defer response.Body.Close()

	var registration model.WorkerRegistration
	if err := json.NewDecoder(response.Body).Decode(&registration); err != nil {
		return WorkerSession{}, fmt.Errorf("decode worker registration: %w", err)
	}
	session := WorkerSession{
		WorkerID:          registration.WorkerID,
		WorkerSessionID:   registration.WorkerSessionID,
		HeartbeatInterval: time.Duration(registration.HeartbeatIntervalSeconds) * time.Second,
		DeadAfter:         time.Duration(registration.DeadAfterSeconds) * time.Second,
	}
	if err := session.Validate(); err != nil {
		return WorkerSession{}, fmt.Errorf("validate worker registration: %w", err)
	}
	return session, nil
}

func (c WorkerControllerClient) HeartbeatWorker(ctx context.Context, session WorkerSession) error {
	if err := session.ValidateIdentity(); err != nil {
		return err
	}
	httpRequest, err := c.newJSONRequest(ctx, http.MethodPost, "/workers/heartbeat", model.WorkerHeartbeatRequest{
		WorkerID:        session.WorkerID,
		WorkerSessionID: session.WorkerSessionID,
	})
	if err != nil {
		return fmt.Errorf("create worker heartbeat request: %w", err)
	}
	response, err := c.client.Do(httpRequest, http.StatusNoContent)
	if err != nil {
		if isControllerConflict(err) {
			return ErrWorkerSessionNotActive
		}
		return fmt.Errorf("post worker heartbeat: %w", err)
	}
	defer response.Body.Close()
	return nil
}

func (c WorkerControllerClient) StopWorker(ctx context.Context, session WorkerSession, reason string) error {
	if err := session.ValidateIdentity(); err != nil {
		return err
	}
	if reason == "" {
		return fmt.Errorf("worker stop reason is required")
	}
	httpRequest, err := c.newJSONRequest(ctx, http.MethodPost, "/workers/stop", model.WorkerStopRequest{
		WorkerID:        session.WorkerID,
		WorkerSessionID: session.WorkerSessionID,
		Reason:          reason,
	})
	if err != nil {
		return fmt.Errorf("create worker stop request: %w", err)
	}
	response, err := c.client.Do(httpRequest, http.StatusNoContent)
	if err != nil {
		if isControllerConflict(err) {
			return ErrWorkerSessionNotActive
		}
		return fmt.Errorf("post worker stop: %w", err)
	}
	defer response.Body.Close()
	return nil
}

func (s WorkerSession) Validate() error {
	if err := s.ValidateIdentity(); err != nil {
		return err
	}
	if s.HeartbeatInterval <= 0 {
		return fmt.Errorf("worker heartbeat interval must be greater than zero")
	}
	if s.DeadAfter <= 0 {
		return fmt.Errorf("worker dead-after duration must be greater than zero")
	}
	if s.DeadAfter <= s.HeartbeatInterval {
		return fmt.Errorf("worker dead-after duration must be greater than heartbeat interval")
	}
	return nil
}

func (s WorkerSession) ValidateIdentity() error {
	if s.WorkerID == "" {
		return fmt.Errorf("worker id is required")
	}
	if s.WorkerSessionID == "" {
		return fmt.Errorf("worker session id is required")
	}
	return nil
}

func isControllerConflict(err error) bool {
	var statusErr controllerhttp.StatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusConflict
}

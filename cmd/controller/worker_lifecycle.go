package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"goetl/internal/model"
	"goetl/internal/persistence"
)

func (c *Controller) registerWorkerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	var request model.WorkerRegistrationRequest
	if !c.decodeWorkerLifecycleRequest(w, r, &request, "decode worker registration") {
		return
	}

	policy, err := workerHeartbeatPolicyConfig(c.launchResolver, defaultWorkerHeartbeatPolicy())
	if err != nil {
		http.Error(w, "worker heartbeat policy: "+err.Error(), http.StatusInternalServerError)
		return
	}

	registeredAt := time.Now().UTC()
	session, err := c.workflowStore.RegisterWorkerSession(r.Context(), persistence.RegisterWorkerSessionRequest{
		WorkerID:        "worker-" + randomHex(16),
		SessionID:       "worker-session-" + randomHex(16),
		RegisteredAt:    registeredAt.Format(time.RFC3339Nano),
		ExecutionHandle: request.ExecutionHandle,
	})
	if err != nil {
		http.Error(w, "register worker session", http.StatusInternalServerError)
		return
	}

	c.ConfirmWorkerStartRegistered(registeredAt)
	c.signalWorkerStateChanged("worker_registered")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(model.WorkerRegistration{
		WorkerID:                 session.WorkerID,
		WorkerSessionID:          session.ID,
		HeartbeatIntervalSeconds: int(policy.HeartbeatInterval / time.Second),
		DeadAfterSeconds:         int(policy.DeadAfter / time.Second),
	}); err != nil {
		http.Error(w, "encode worker registration", http.StatusInternalServerError)
	}
}

func (c *Controller) heartbeatWorkerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	var request model.WorkerHeartbeatRequest
	if !c.decodeWorkerLifecycleRequest(w, r, &request, "decode worker heartbeat") {
		return
	}
	if request.WorkerID == "" || request.WorkerSessionID == "" {
		http.Error(w, "worker id and worker session id are required", http.StatusBadRequest)
		return
	}

	updated, err := c.workflowStore.HeartbeatWorkerSession(r.Context(), persistence.HeartbeatWorkerSessionRequest{
		WorkerID:    request.WorkerID,
		SessionID:   request.WorkerSessionID,
		HeartbeatAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		http.Error(w, "heartbeat worker session", http.StatusInternalServerError)
		return
	}
	if !updated {
		http.Error(w, "worker session is not active", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) stopWorkerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	var request model.WorkerStopRequest
	if !c.decodeWorkerLifecycleRequest(w, r, &request, "decode worker stop") {
		return
	}
	if request.WorkerID == "" || request.WorkerSessionID == "" || request.Reason == "" {
		http.Error(w, "worker id, worker session id, and reason are required", http.StatusBadRequest)
		return
	}

	session, found, err := c.workflowStore.GetWorkerSession(r.Context(), request.WorkerID, request.WorkerSessionID)
	if err != nil {
		http.Error(w, "get worker session", http.StatusInternalServerError)
		return
	}
	if !found || session.Status == persistence.WorkerSessionStatusDead {
		http.Error(w, "worker session is not active", http.StatusConflict)
		return
	}
	if session.Status == persistence.WorkerSessionStatusStopped {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	result, err := c.workflowStore.StopWorkerSessionAndRecoverWork(r.Context(), persistence.StopWorkerSessionAndRecoverWorkRequest{
		WorkerID:  request.WorkerID,
		SessionID: request.WorkerSessionID,
		StoppedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Reason:    request.Reason,
	})
	if err != nil {
		http.Error(w, "stop worker session", http.StatusInternalServerError)
		return
	}
	if !result.Changed {
		http.Error(w, "worker session is not active", http.StatusConflict)
		return
	}

	c.signalWorkerStateChanged("worker_stopped")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) decodeWorkerLifecycleRequest(w http.ResponseWriter, r *http.Request, target any, label string) bool {
	body := r.Body
	if c.maxRequestBytes > 0 {
		if r.ContentLength > int64(c.maxRequestBytes) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		body = http.MaxBytesReader(w, r.Body, int64(c.maxRequestBytes))
	}

	decoder := json.NewDecoder(body)
	if err := decoder.Decode(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, label, http.StatusBadRequest)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "request body must be one JSON document", http.StatusBadRequest)
		return false
	}
	return true
}

func (c *Controller) signalWorkerStateChanged(reason string) {
	if c.workerStateChanged != nil {
		c.workerStateChanged(reason)
	}
}

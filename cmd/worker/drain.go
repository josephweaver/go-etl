package main

import (
	"fmt"
	"sync"
	"time"
)

type WorkerDrainReason string

const (
	WorkerDrainReasonSlurmSignal    WorkerDrainReason = "slurm_sigusr1"
	WorkerDrainReasonAdministrative WorkerDrainReason = "administrative"
)

type WorkerDrainRequest struct {
	Reason      WorkerDrainReason
	RequestedAt time.Time
}

func (request WorkerDrainRequest) Validate() error {
	switch request.Reason {
	case WorkerDrainReasonSlurmSignal, WorkerDrainReasonAdministrative:
	default:
		return fmt.Errorf("unsupported worker drain reason %q", request.Reason)
	}
	if request.RequestedAt.IsZero() {
		return fmt.Errorf("worker drain request time is required")
	}
	return nil
}

type WorkerDrainSource interface {
	C() <-chan WorkerDrainRequest
}

// WorkerDrainRequests latches and publishes only the first valid drain request.
type WorkerDrainRequests struct {
	mu        sync.Mutex
	requests  chan WorkerDrainRequest
	current   WorkerDrainRequest
	requested bool
}

func NewWorkerDrainRequests() *WorkerDrainRequests {
	return &WorkerDrainRequests{
		requests: make(chan WorkerDrainRequest, 1),
	}
}

func (requests *WorkerDrainRequests) C() <-chan WorkerDrainRequest {
	requests.mu.Lock()
	defer requests.mu.Unlock()
	return requests.channelLocked()
}

func (requests *WorkerDrainRequests) Request(request WorkerDrainRequest) (bool, error) {
	if err := request.Validate(); err != nil {
		return false, err
	}
	request.RequestedAt = request.RequestedAt.UTC()

	requests.mu.Lock()
	defer requests.mu.Unlock()
	if requests.requested {
		return false, nil
	}

	requests.current = request
	requests.requested = true
	requests.channelLocked() <- request
	return true, nil
}

func (requests *WorkerDrainRequests) Current() (WorkerDrainRequest, bool) {
	requests.mu.Lock()
	defer requests.mu.Unlock()
	return requests.current, requests.requested
}

func (requests *WorkerDrainRequests) channelLocked() chan WorkerDrainRequest {
	if requests.requests == nil {
		requests.requests = make(chan WorkerDrainRequest, 1)
	}
	return requests.requests
}

var _ WorkerDrainSource = (*WorkerDrainRequests)(nil)

package main

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"sync"
	"time"

	"goetl/internal/controllerauth"
	"goetl/internal/persistence"
	"goetl/internal/reposource"
	"goetl/internal/variable"
)

type Controller struct {
	mu                  sync.Mutex
	claimMu             sync.Mutex
	ledger              *sql.DB
	workflowStore       *persistence.Store
	repoSourceProviders map[string]reposource.Provider
	repoCacheLayout     reposource.CacheLayout
	workerStarter       WorkerStarter
	launchResolver      variable.Resolver
	workerExecutor      *WorkerCapacityManager
	logSink             logObservationSink
	shutdown            func(context.Context) error
	env                 *ExecutionEnvironment
	recoveryStartedAt   time.Time
	normalAdmission     bool
	maxRequestBytes     int
	logRootPath         string
	logReadDefaultTail  int
	logReadMaxTail      int
	authMode            controllerauth.Mode
	authStore           controllerauth.Store
	authPolicy          controllerauth.Policy
	workerStateChanged  func(string)
	caretaker           *CareTaker
	caretakerCancel     context.CancelFunc
	caretakerDone       chan error
}

func newController() *Controller {
	return &Controller{
		workerStarter:   LocalWorkerStarter{},
		workerExecutor:  NewWorkerCapacityManager(nil),
		normalAdmission: true,
	}
}

func (c *Controller) enterRecoveryMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recoveryStartedAt = time.Now().UTC()
	c.normalAdmission = false
}

func (c *Controller) allowNormalAdmission() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalAdmission = true
}

func (c *Controller) completeStartupRecovery(ctx context.Context) error {
	if err := c.verifyActiveRunSources(ctx); err != nil {
		return err
	}
	c.allowNormalAdmission()
	return nil
}

func (c *Controller) recoveryAdmissionClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.normalAdmission
}

func (c *Controller) requireNormalAdmission(w http.ResponseWriter) bool {
	if c.recoveryAdmissionClosed() {
		http.Error(w, "controller is in recovery mode", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (c *Controller) signalCareTaker(reason string) {
	if c == nil || c.workerStateChanged == nil {
		return
	}
	c.workerStateChanged(reason)
}

func (c *Controller) submissionHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := strings.CutSuffix(r.URL.Path, "/status"); ok {
		c.submissionStatusHandler(w, r)
		return
	}
	if _, ok := strings.CutSuffix(r.URL.Path, "/logs"); ok {
		c.submissionLogsHandler(w, r)
		return
	}
	http.NotFound(w, r)
}

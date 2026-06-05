package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"goetl/internal/model"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

type Controller struct {
	mu       sync.Mutex
	pending  []model.WorkItem
	assigned map[string]model.WorkItem
	failed   map[string]model.WorkFailure
	shutdown func(context.Context) error
	worker   WorkerStarter
	scaler   WorkerScaleState
	scaleCfg WorkerScaleConfig
}

type WorkflowSubmission struct {
	Workflow  workflow.Workflow   `json:"workflow"`
	Variables []variable.Variable `json:"variables"`
}

type WorkerStarter interface {
	StartWorker(targetEnvironment string, resolver variable.Resolver) error
}

func newController(items []model.WorkItem) *Controller {
	return &Controller{
		pending:  items,
		assigned: make(map[string]model.WorkItem),
		failed:   make(map[string]model.WorkFailure),
		scaleCfg: WorkerScaleConfig{
			MaxCount:                2,
			CountPerStart:           1,
			MinElapsedBetweenStarts: 30 * time.Second,
		},
	}
}

func main() {
	controller := newController([]model.WorkItem{
		{
			ID:             "local-demo-001",
			Type:           model.WorkItemTypeWriteDemoOutput,
			OutputFilename: "local-demo-001.txt",
		},
	})
	controller.worker = LocalWorkerStarter{}

	mux := http.NewServeMux()
	server := &http.Server{Addr: ":8080", Handler: mux}
	controller.shutdown = server.Shutdown

	mux.HandleFunc("/work/next", controller.nextWorkHandler)
	mux.HandleFunc("/work/complete", controller.completeWorkHandler)
	mux.HandleFunc("/work/fail", controller.failWorkHandler)
	mux.HandleFunc("/workflow", controller.submitWorkflowHandler)
	mux.HandleFunc("/work", controller.submitWorkHandler)
	mux.HandleFunc("/shutdown", controller.shutdownHandler)
	mux.HandleFunc("/status", controller.statusHandler)

	fmt.Println("controller listening on :8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("controller failed:", err)
	}
}

func (c *Controller) submitWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var item model.WorkItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, "decode work item", http.StatusBadRequest)
		return
	}

	if err := item.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasWorkItemID(item.ID) {
		http.Error(w, "work item id already exists", http.StatusConflict)
		return
	}

	c.pending = append(c.pending, item)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) submitWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var submission WorkflowSubmission
	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		http.Error(w, "decode workflow submission", http.StatusBadRequest)
		return
	}

	workflowScope, err := variable.NewScope(submission.Workflow.Variables...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	submissionScope, err := variable.NewScope(submission.Variables...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resolver := variable.NewResolver(variable.NewSet(workflowScope, submissionScope), variable.ResolverConfig{})
	items, err := workflow.CompileWorkflow(resolver, submission.Workflow)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	workerTarget, err := workerTargetEnvironment(resolver)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	scaleCfg, err := workerScaleConfig(resolver, c.scaleCfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.mu.Lock()

	for _, item := range items {
		if c.hasWorkItemID(item.ID) {
			c.mu.Unlock()
			http.Error(w, "work item id already exists", http.StatusConflict)
			return
		}
	}

	startCount := 0
	assignedCount := len(c.assigned)
	c.pending = append(c.pending, items...)
	if workerTarget != "" && c.worker != nil {
		now := time.Now()
		startCount = c.scaler.PlanStarts(now, len(c.pending), assignedCount, scaleCfg)
		c.scaler.RecordStart(now, startCount, assignedCount)
	}
	c.mu.Unlock()

	if workerTarget != "" && c.worker != nil {
		for range startCount {
			if err := c.worker.StartWorker(workerTarget, resolver); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func workerTargetEnvironment(resolver variable.Resolver) (string, error) {
	reference, err := variable.ParseReference("worker_target_environment")
	if err != nil {
		return "", err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return "", nil
	}

	if value.Type != variable.TypeString {
		return "", fmt.Errorf("worker_target_environment has type %s, want string", value.Type)
	}

	workerTarget, ok := value.Value.(string)
	if !ok {
		return "", fmt.Errorf("worker_target_environment is required")
	}

	return workerTarget, nil
}

func workerScaleConfig(resolver variable.Resolver, defaults WorkerScaleConfig) (WorkerScaleConfig, error) {
	cfg := defaults

	var err error
	if cfg.MinCount, err = optionalIntVariable(resolver, "worker_min_count", cfg.MinCount); err != nil {
		return WorkerScaleConfig{}, err
	}
	if cfg.MaxCount, err = optionalIntVariable(resolver, "worker_max_count", cfg.MaxCount); err != nil {
		return WorkerScaleConfig{}, err
	}
	if cfg.CountPerStart, err = optionalIntVariable(resolver, "worker_count_per_start", cfg.CountPerStart); err != nil {
		return WorkerScaleConfig{}, err
	}
	if cfg.MinElapsedBetweenStarts, err = optionalDurationVariable(resolver, "worker_min_elapsed_time_between_starts", cfg.MinElapsedBetweenStarts); err != nil {
		return WorkerScaleConfig{}, err
	}

	return cfg, nil
}

func optionalIntVariable(resolver variable.Resolver, name string, fallback int) (int, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return 0, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return fallback, nil
	}

	if value.Type != variable.TypeInt {
		return 0, fmt.Errorf("%s has type %s, want int", name, value.Type)
	}

	number, ok := value.Value.(int)
	if !ok {
		return 0, fmt.Errorf("%s must be an int", name)
	}

	return number, nil
}

func optionalDurationVariable(resolver variable.Resolver, name string, fallback time.Duration) (time.Duration, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return 0, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return fallback, nil
	}

	if value.Type != variable.TypeString {
		return 0, fmt.Errorf("%s has type %s, want string", name, value.Type)
	}

	text, ok := value.Value.(string)
	if !ok || text == "" {
		return 0, fmt.Errorf("%s is required", name)
	}

	duration, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}

	return duration, nil
}

func (c *Controller) shutdownHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if c.shutdown == nil {
		http.Error(w, "shutdown unavailable", http.StatusServiceUnavailable)
		return
	}

	go func() {
		if err := c.shutdown(context.Background()); err != nil {
			fmt.Println("controller shutdown failed:", err)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) hasWorkItemID(id string) bool {
	for _, item := range c.pending {
		if item.ID == id {
			return true
		}
	}

	if _, ok := c.assigned[id]; ok {
		return true
	}

	if _, ok := c.failed[id]; ok {
		return true
	}

	return false
}

func (c *Controller) statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	c.mu.Lock()
	status := model.ControllerStatus{
		Pending:  len(c.pending),
		Assigned: len(c.assigned),
		Failed:   len(c.failed),
	}
	c.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "encode status", http.StatusInternalServerError)
	}
}

func (c *Controller) failWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var failure model.WorkFailure
	if err := json.NewDecoder(r.Body).Decode(&failure); err != nil {
		http.Error(w, "decode failure", http.StatusBadRequest)
		return
	}

	if failure.ID == "" || failure.Error == "" {
		http.Error(w, "work item id and error are required", http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.assigned[failure.ID]; !ok {
		http.Error(w, "work item not assigned", http.StatusNotFound)
		return
	}

	delete(c.assigned, failure.ID)
	c.failed[failure.ID] = failure
	fmt.Println("work item failed:", failure.ID, failure.Error)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) completeWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var completion model.WorkCompletion
	if err := json.NewDecoder(r.Body).Decode(&completion); err != nil {
		http.Error(w, "decode completion", http.StatusBadRequest)
		return
	}

	if completion.ID == "" {
		http.Error(w, "work item id is required", http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.assigned[completion.ID]; !ok {
		http.Error(w, "work item not assigned", http.StatusNotFound)
		return
	}

	delete(c.assigned, completion.ID)
	fmt.Println("work item completed:", completion.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) nextWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.pending) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	item := c.pending[0]
	c.pending = c.pending[1:]
	c.assigned[item.ID] = item

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(item); err != nil {
		http.Error(w, "encode work item", http.StatusInternalServerError)
	}
}

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"goetl/internal/ledger"
	"goetl/internal/model"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

type Controller struct {
	mu       sync.Mutex
	pending  []model.WorkItem
	assigned map[string]model.WorkItem
	failed   map[string]model.WorkFailure
	ledger   *sql.DB
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

type WorkReuseDecision struct {
	Reusable       bool
	Reason         string
	PriorAttemptID string
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
	config, err := controllerConfigFromArgs(os.Args)
	if err != nil {
		fmt.Println("controller config failed:", err)
		return
	}

	ledgerDB, err := initConfiguredLedger(context.Background(), config)
	if err != nil {
		fmt.Println("controller ledger failed:", err)
		return
	}
	if ledgerDB != nil {
		defer ledgerDB.Close()
	}

	controller := newController(nil)
	controller.ledger = ledgerDB
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

func controllerConfigFromArgs(args []string) (ControllerConfig, error) {
	if len(args) < 2 {
		return ControllerConfig{}, nil
	}

	return loadControllerConfig(args[1])
}

func initConfiguredLedger(ctx context.Context, config ControllerConfig) (*sql.DB, error) {
	if len(config.Variables) == 0 {
		return nil, nil
	}

	scope, err := variable.NewScope(config.Variables...)
	if err != nil {
		return nil, err
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	path, err := optionalPathVariable(resolver, "ledger_db_path")
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}

	db, err := ledger.OpenSQLite(path)
	if err != nil {
		return nil, err
	}

	if err := ledger.InitSQLiteSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func optionalPathVariable(resolver variable.Resolver, name string) (string, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return "", nil
	}

	if value.Type != variable.TypePath {
		return "", fmt.Errorf("%s has type %s, want path", name, value.Type)
	}

	path, ok := value.Value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a path", name)
	}

	return path, nil
}

func (c *Controller) recordAttempt(ctx context.Context, attempt ledger.Attempt) error {
	if c.ledger == nil {
		return nil
	}

	return ledger.InsertAttempt(ctx, c.ledger, attempt)
}

func (c *Controller) recordSkippedAttempt(ctx context.Context, item model.WorkItem, skippedAt time.Time) (model.WorkSkip, bool, error) {
	decision, err := c.workReuseDecision(ctx, item)
	if err != nil {
		return model.WorkSkip{}, false, err
	}

	skip, ok, err := workSkipForReuseDecision(item, decision)
	if err != nil || !ok {
		return model.WorkSkip{}, false, err
	}

	attempt, err := skippedAttemptFromWorkSkip(item, skip, skippedAt)
	if err != nil {
		return model.WorkSkip{}, false, err
	}
	if err := c.recordAttempt(ctx, attempt); err != nil {
		return model.WorkSkip{}, false, err
	}

	return skip, true, nil
}

func (c *Controller) priorCompletedAttempt(ctx context.Context, item model.WorkItem) (ledger.Attempt, bool, error) {
	if c.ledger == nil || item.WorkItemFingerprint == "" {
		return ledger.Attempt{}, false, nil
	}

	return ledger.FindLatestCompletedAttemptByWorkItemFingerprint(ctx, c.ledger, item.WorkItemFingerprint)
}

func priorCompletedAttemptMatchesWorkItem(item model.WorkItem, attempt ledger.Attempt) bool {
	if attempt.Status != ledger.AttemptStatusCompleted {
		return false
	}
	if item.WorkItemFingerprint == "" || item.InputFingerprint == "" || item.OutputFingerprint == "" || item.CodeVersion == "" {
		return false
	}

	return item.WorkItemFingerprint == attempt.WorkItemFingerprint &&
		item.InputFingerprint == attempt.InputFingerprint &&
		item.OutputFingerprint == attempt.OutputFingerprint &&
		item.CodeVersion == attempt.CodeVersion
}

func (c *Controller) reusablePriorAttempt(ctx context.Context, item model.WorkItem) (ledger.Attempt, bool, error) {
	attempt, ok, err := c.priorCompletedAttempt(ctx, item)
	if err != nil || !ok {
		return ledger.Attempt{}, false, err
	}
	if !priorCompletedAttemptMatchesWorkItem(item, attempt) {
		return ledger.Attempt{}, false, nil
	}

	return attempt, true, nil
}

func (c *Controller) workReuseDecision(ctx context.Context, item model.WorkItem) (WorkReuseDecision, error) {
	attempt, ok, err := c.priorCompletedAttempt(ctx, item)
	if err != nil {
		return WorkReuseDecision{}, err
	}
	if !ok {
		return WorkReuseDecision{Reason: "no_prior_completed_attempt"}, nil
	}
	if !priorCompletedAttemptMatchesWorkItem(item, attempt) {
		return WorkReuseDecision{
			Reason:         "prior_attempt_mismatch",
			PriorAttemptID: attempt.ID,
		}, nil
	}

	return WorkReuseDecision{
		Reusable:       true,
		Reason:         "matched_prior_completed_attempt",
		PriorAttemptID: attempt.ID,
	}, nil
}

func workSkipForReuseDecision(item model.WorkItem, decision WorkReuseDecision) (model.WorkSkip, bool, error) {
	if !decision.Reusable {
		return model.WorkSkip{}, false, nil
	}

	skip := model.WorkSkip{
		ID:             item.ID,
		PriorAttemptID: decision.PriorAttemptID,
		Reason:         decision.Reason,
	}
	if err := skip.Validate(); err != nil {
		return model.WorkSkip{}, false, err
	}

	return skip, true, nil
}

func skippedAttemptFromWorkSkip(item model.WorkItem, skip model.WorkSkip, skippedAt time.Time) (ledger.Attempt, error) {
	if err := skip.Validate(); err != nil {
		return ledger.Attempt{}, err
	}
	if skippedAt.IsZero() {
		skippedAt = time.Now().UTC()
	}

	return ledger.Attempt{
		ID:                  skip.ID + "-skip-" + randomHex(8),
		WorkflowInstanceID:  item.WorkflowInstanceID,
		StepInstanceID:      item.StepInstanceID,
		WorkItemID:          skip.ID,
		WorkItemFingerprint: item.WorkItemFingerprint,
		InputFingerprint:    item.InputFingerprint,
		OutputFingerprint:   item.OutputFingerprint,
		CodeVersion:         item.CodeVersion,
		Status:              ledger.AttemptStatusSkipped,
		StartedAt:           skippedAt.UTC(),
		CompletedAt:         skippedAt.UTC(),
		Variables:           runtimeVariablesFromSkip(item, skip, skippedAt.UTC()),
	}, nil
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
	codeVersion, err := controllerCodeVersion(resolver)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	compiledItems, err := workflow.CompileWorkflowItems(resolver, submission.Workflow)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	items := workItemsWithRuntimeMetadata(submission.Workflow.ID, compiledItems, codeVersion)

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

func workItemsWithRuntimeMetadata(workflowID string, compiledItems []workflow.CompiledWorkItem, codeVersion string) []model.WorkItem {
	workflowInstanceID := workflowID + "-instance-" + randomHex(8)
	workflowFingerprint := fingerprint("workflow", map[string]any{
		"id": workflowID,
	})
	items := make([]model.WorkItem, 0, len(compiledItems))

	for _, compiled := range compiledItems {
		item := compiled.WorkItem
		item.WorkflowDefinitionID = workflowID
		item.WorkflowFingerprint = workflowFingerprint
		item.WorkflowInstanceID = workflowInstanceID
		item.StepDefinitionID = compiled.StepID
		item.StepFingerprint = fingerprint("step", map[string]any{
			"workflow_fingerprint": workflowFingerprint,
			"id":                   compiled.StepID,
		})
		item.StepInstanceID = workflowInstanceID + "-step-" + compiled.StepID
		item.WorkItemFingerprint = fingerprint("work-item", map[string]any{
			"id":              item.ID,
			"type":            item.Type,
			"output_filename": item.OutputFilename,
			"parameters":      item.Parameters,
		})
		item.InputFingerprint = fingerprint("input", item.Parameters)
		item.OutputFingerprint = fingerprint("output", map[string]any{
			"output_filename": item.OutputFilename,
		})
		item.CodeVersion = codeVersion
		items = append(items, item)
	}

	return items
}

func controllerCodeVersion(resolver variable.Resolver) (string, error) {
	configured, ok, err := optionalStringVariable(resolver, "code_version")
	if err != nil {
		return "", err
	}
	if ok {
		return configured, nil
	}

	return buildInfoCodeVersion(), nil
}

func optionalStringVariable(resolver variable.Resolver, name string) (string, bool, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", false, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return "", false, nil
	}

	if value.Type != variable.TypeString {
		return "", false, fmt.Errorf("%s has type %s, want string", name, value.Type)
	}

	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", false, fmt.Errorf("%s is required", name)
	}

	return text, true, nil
}

func buildInfoCodeVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	revision := buildSetting(info, "vcs.revision")
	if revision == "" {
		return "unknown"
	}

	modified := buildSetting(info, "vcs.modified")
	if modified == "true" {
		return revision + "-modified"
	}

	return revision
}

func buildSetting(info *debug.BuildInfo, key string) string {
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

func fingerprint(label string, value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(fmt.Sprint(value))
	}
	sum := sha256.Sum256(data)
	return label + ":sha256:" + hex.EncodeToString(sum[:])
}

func randomHex(byteCount int) string {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(data)
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
	pendingItems := append([]model.WorkItem(nil), c.pending...)
	status := model.ControllerStatus{
		Pending:  len(c.pending),
		Assigned: len(c.assigned),
		Failed:   len(c.failed),
	}
	c.mu.Unlock()

	reuseReasons, err := c.pendingReuseDecisionReasons(r.Context(), pendingItems)
	if err != nil {
		http.Error(w, "query reuse candidates", http.StatusInternalServerError)
		return
	}
	status.PendingReuseCandidates = reuseReasons["matched_prior_completed_attempt"]

	attempts, attemptVariables, err := c.ledgerStatusCounts(r.Context())
	if err != nil {
		http.Error(w, "query ledger status", http.StatusInternalServerError)
		return
	}
	status.Attempts = attempts
	status.AttemptVariables = attemptVariables

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "encode status", http.StatusInternalServerError)
	}
}

func (c *Controller) ledgerStatusCounts(ctx context.Context) (int, int, error) {
	if c.ledger == nil {
		return 0, 0, nil
	}

	var attempts int
	if err := c.ledger.QueryRowContext(ctx, `SELECT COUNT(*) FROM attempts`).Scan(&attempts); err != nil {
		return 0, 0, fmt.Errorf("query attempts count: %w", err)
	}

	var attemptVariables int
	if err := c.ledger.QueryRowContext(ctx, `SELECT COUNT(*) FROM attempt_variables`).Scan(&attemptVariables); err != nil {
		return 0, 0, fmt.Errorf("query attempt variables count: %w", err)
	}

	return attempts, attemptVariables, nil
}

func (c *Controller) pendingReuseDecisionReasons(ctx context.Context, items []model.WorkItem) (map[string]int, error) {
	counts := make(map[string]int)
	for _, item := range items {
		decision, err := c.workReuseDecision(ctx, item)
		if err != nil {
			return nil, err
		}
		counts[decision.Reason]++
	}
	return counts, nil
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

	attempt, hasAttempt, err := attemptFromCompletion(completion)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if hasAttempt {
		if err := c.recordAttempt(r.Context(), attempt); err != nil {
			http.Error(w, "record completion", http.StatusInternalServerError)
			return
		}
	}

	delete(c.assigned, completion.ID)
	fmt.Println("work item completed:", completion.ID)
	w.WriteHeader(http.StatusNoContent)
}

func attemptFromCompletion(completion model.WorkCompletion) (ledger.Attempt, bool, error) {
	if completion.AttemptID == "" {
		return ledger.Attempt{}, false, nil
	}

	startedAt, err := time.Parse(time.RFC3339, completion.StartedAt)
	if err != nil {
		return ledger.Attempt{}, false, fmt.Errorf("parse started_at: %w", err)
	}

	completedAt, err := time.Parse(time.RFC3339, completion.CompletedAt)
	if err != nil {
		return ledger.Attempt{}, false, fmt.Errorf("parse completed_at: %w", err)
	}

	return ledger.Attempt{
		ID:                  completion.AttemptID,
		WorkflowInstanceID:  completion.WorkflowInstanceID,
		StepInstanceID:      completion.StepInstanceID,
		WorkItemID:          completion.ID,
		WorkItemFingerprint: completion.WorkItemFingerprint,
		InputFingerprint:    completion.InputFingerprint,
		OutputFingerprint:   completion.OutputFingerprint,
		CodeVersion:         completion.CodeVersion,
		Status:              ledger.AttemptStatusCompleted,
		StartedAt:           startedAt,
		CompletedAt:         completedAt,
		Variables:           runtimeVariablesFromCompletion(completion),
	}, true, nil
}

func runtimeVariablesFromCompletion(completion model.WorkCompletion) []ledger.AttemptVariable {
	variables := []ledger.AttemptVariable{
		runtimeStringVariable("workflow_definition_id", completion.WorkflowDefinitionID, "workflow"),
		runtimeStringVariable("workflow_fingerprint", completion.WorkflowFingerprint, "workflow"),
		runtimeStringVariable("workflow_instance_id", completion.WorkflowInstanceID, "workflow"),
		runtimeStringVariable("step_definition_id", completion.StepDefinitionID, "step"),
		runtimeStringVariable("step_fingerprint", completion.StepFingerprint, "step"),
		runtimeStringVariable("step_instance_id", completion.StepInstanceID, "step"),
		runtimeStringVariable("work_item_id", completion.ID, "work_item"),
		runtimeStringVariable("work_item_fingerprint", completion.WorkItemFingerprint, "work_item"),
		runtimeStringVariable("input_fingerprint", completion.InputFingerprint, "work_item"),
		runtimeStringVariable("output_fingerprint", completion.OutputFingerprint, "work_item"),
		runtimeStringVariable("code_version", completion.CodeVersion, "work_item"),
		runtimeStringVariable("attempt_id", completion.AttemptID, "attempt"),
		runtimeStringVariable("started_at", completion.StartedAt, "attempt"),
		runtimeStringVariable("completed_at", completion.CompletedAt, "attempt"),
	}

	for name, parameter := range completion.Parameters {
		variables = append(variables, ledger.AttemptVariable{
			Namespace: "work_item",
			Name:      name,
			Type:      parameter.Type,
			Value:     parameter.Value,
			Source:    "controller",
			Lifecycle: "work_item",
		})
	}

	return variables
}

func runtimeVariablesFromSkip(item model.WorkItem, skip model.WorkSkip, skippedAt time.Time) []ledger.AttemptVariable {
	timestamp := skippedAt.UTC().Format(time.RFC3339)
	return []ledger.AttemptVariable{
		runtimeStringVariable("workflow_definition_id", item.WorkflowDefinitionID, "workflow"),
		runtimeStringVariable("workflow_fingerprint", item.WorkflowFingerprint, "workflow"),
		runtimeStringVariable("workflow_instance_id", item.WorkflowInstanceID, "workflow"),
		runtimeStringVariable("step_definition_id", item.StepDefinitionID, "step"),
		runtimeStringVariable("step_fingerprint", item.StepFingerprint, "step"),
		runtimeStringVariable("step_instance_id", item.StepInstanceID, "step"),
		runtimeStringVariable("work_item_id", skip.ID, "work_item"),
		runtimeStringVariable("work_item_fingerprint", item.WorkItemFingerprint, "work_item"),
		runtimeStringVariable("input_fingerprint", item.InputFingerprint, "work_item"),
		runtimeStringVariable("output_fingerprint", item.OutputFingerprint, "work_item"),
		runtimeStringVariable("code_version", item.CodeVersion, "work_item"),
		runtimeStringVariable("prior_attempt_id", skip.PriorAttemptID, "attempt"),
		runtimeStringVariable("skip_reason", skip.Reason, "attempt"),
		runtimeStringVariable("started_at", timestamp, "attempt"),
		runtimeStringVariable("completed_at", timestamp, "attempt"),
	}
}

func runtimeStringVariable(name string, value string, lifecycle string) ledger.AttemptVariable {
	return ledger.AttemptVariable{
		Namespace: "runtime",
		Name:      name,
		Type:      "string",
		Value:     value,
		Source:    "worker",
		Lifecycle: lifecycle,
	}
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

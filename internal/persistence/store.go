package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const (
	DriverSQLite           = "sqlite"
	SupportedSchemaVersion = 1

	ExecutorTypeWorker     = "worker"
	ExecutorTypeController = "controller"
)

type Config struct {
	Driver           string
	ConnectionString string
}

type Store struct {
	db *sql.DB
}

type ProjectRecord struct {
	ID                 string
	Name               string
	RepositoryIdentity string
	SourceCommit       string
	ConfigPath         string
	SourceObjectID     string
	ConfigSHA256       string
	CreatedAt          string
}

type WorkflowRecord struct {
	ID                 string
	ProjectID          string
	Name               string
	RepositoryIdentity string
	SourceCommit       string
	WorkflowPath       string
	SourceObjectID     string
	WorkflowSHA256     string
	CreatedAt          string
}

type WorkflowRunRecord struct {
	ID                    string
	ProjectID             string
	WorkflowID            string
	SubmissionContextJSON string
	CreatedAt             string
}

type WorkflowStageRecord struct {
	RunID                string
	StageIndex           int
	StepID               string
	StageSourceReference string
	State                string
	CreatedAt            string
	ReadyAt              string
	StartedAt            string
	CompletedAt          string
	FailedAt             string
	OutputJSON           string
	OutputJSONSHA256     string
}

type WorkItemRecord struct {
	ID                   string
	RunID                string
	StageIndex           int
	WorkItemIndex        int
	WorkerPayloadJSON    string
	ResolvedInputsSHA256 string
	CreatedAt            string
}

type QueuedWorkRecord struct {
	WorkItemRecord
	QueuedAt string
}

type WorkItemStatusCounts struct {
	Queued    int
	Running   int
	Completed int
	Failed    int
}

type CompleteStageRequest struct {
	RunID            string
	StageIndex       int
	OutputJSON       string
	OutputJSONSHA256 string
	CompletedAt      string
	ReadyWorkItems   []WorkItemRecord
	ReadyQueuedWork  []QueuedWorkRecord
}

type CompleteStageResult struct {
	Stage     WorkflowStageRecord
	Found     bool
	Completed bool
}

type ClaimWorkRequest struct {
	AttemptID    string
	WorkerID     string
	ExecutorType string
	StartedAt    string
}

type ClaimedWorkRecord struct {
	AttemptID    string
	WorkItem     WorkItemRecord
	WorkerID     string
	ExecutorType string
	QueuedAt     string
	StartedAt    string
}

type CompleteAttemptRequest struct {
	AttemptID        string
	SkippedParentID  string
	OutputJSON       string
	OutputJSONSHA256 string
	PreStateSHA256   string
	PostStateSHA256  string
	CompletedAt      string
}

type CompletedWorkRecord struct {
	AttemptID        string
	WorkItemID       string
	SkippedParentID  string
	OutputJSON       string
	OutputJSONSHA256 string
	PreStateSHA256   string
	PostStateSHA256  string
	QueuedAt         string
	StartedAt        string
	CompletedAt      string
}

type FailAttemptRequest struct {
	AttemptID string
	Error     string
	FailedAt  string
}

type FailedWorkRecord struct {
	AttemptID  string
	WorkItemID string
	Error      string
	QueuedAt   string
	StartedAt  string
	FailedAt   string
}

func OpenStore(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Driver == "" {
		return nil, fmt.Errorf("database driver is required")
	}
	if cfg.ConnectionString == "" {
		return nil, fmt.Errorf("database connection string is required")
	}

	switch cfg.Driver {
	case DriverSQLite:
		return openSQLiteStore(ctx, cfg.ConnectionString)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) CurrentSchemaVersion(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not open")
	}

	var version int
	if err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func (s *Store) UpsertProject(ctx context.Context, project ProjectRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := project.validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project upsert: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getProject(ctx, tx, project.ID)
	if err != nil {
		return err
	}
	if found {
		if existing != project {
			return fmt.Errorf("project %s already exists with different values", project.ID)
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO projects (
		project_id,
		project_name,
		repository_identity,
		source_commit,
		config_path,
		source_object_id,
		config_sha256,
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		project.ID,
		project.Name,
		project.RepositoryIdentity,
		project.SourceCommit,
		project.ConfigPath,
		project.SourceObjectID,
		project.ConfigSHA256,
		project.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert project %s: %w", project.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project upsert: %w", err)
	}
	return nil
}

func (s *Store) GetProject(ctx context.Context, projectID string) (ProjectRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return ProjectRecord{}, false, err
	}
	return getProject(ctx, s.db, projectID)
}

func (s *Store) DeleteProjectIfUnused(ctx context.Context, projectID string) (bool, error) {
	if err := s.requireOpen(); err != nil {
		return false, err
	}
	if projectID == "" {
		return false, fmt.Errorf("project id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin project delete: %w", err)
	}
	defer tx.Rollback()

	var workflowCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflows WHERE project_id = ?`, projectID).Scan(&workflowCount); err != nil {
		return false, fmt.Errorf("count workflows for project %s: %w", projectID, err)
	}
	if workflowCount != 0 {
		return false, nil
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE project_id = ?`, projectID)
	if err != nil {
		return false, fmt.Errorf("delete project %s: %w", projectID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("delete project %s: %w", projectID, err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit project delete: %w", err)
	}
	return deleted, nil
}

func (s *Store) UpsertWorkflow(ctx context.Context, workflow WorkflowRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := workflow.validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow upsert: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getWorkflow(ctx, tx, workflow.ID)
	if err != nil {
		return err
	}
	if found {
		if existing != workflow {
			return fmt.Errorf("workflow %s already exists with different values", workflow.ID)
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO workflows (
		workflow_id,
		project_id,
		workflow_name,
		repository_identity,
		source_commit,
		workflow_path,
		source_object_id,
		workflow_sha256,
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		workflow.ID,
		workflow.ProjectID,
		workflow.Name,
		workflow.RepositoryIdentity,
		workflow.SourceCommit,
		workflow.WorkflowPath,
		workflow.SourceObjectID,
		workflow.WorkflowSHA256,
		workflow.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert workflow %s: %w", workflow.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow upsert: %w", err)
	}
	return nil
}

func (s *Store) GetWorkflow(ctx context.Context, workflowID string) (WorkflowRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkflowRecord{}, false, err
	}
	return getWorkflow(ctx, s.db, workflowID)
}

func (s *Store) DeleteWorkflowIfUnused(ctx context.Context, workflowID string) (bool, error) {
	if err := s.requireOpen(); err != nil {
		return false, err
	}
	if workflowID == "" {
		return false, fmt.Errorf("workflow id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin workflow delete: %w", err)
	}
	defer tx.Rollback()

	var instanceCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_instances WHERE workflow_id = ?`, workflowID).Scan(&instanceCount); err != nil {
		return false, fmt.Errorf("count workflow instances for workflow %s: %w", workflowID, err)
	}
	if instanceCount != 0 {
		return false, nil
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM workflows WHERE workflow_id = ?`, workflowID)
	if err != nil {
		return false, fmt.Errorf("delete workflow %s: %w", workflowID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("delete workflow %s: %w", workflowID, err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit workflow delete: %w", err)
	}
	return deleted, nil
}

func (s *Store) CreateWorkflowRun(ctx context.Context, run WorkflowRunRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := run.validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow run create: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getWorkflowRun(ctx, tx, run.ID)
	if err != nil {
		return err
	}
	if found {
		if existing != run {
			return fmt.Errorf("workflow run %s already exists with different values", run.ID)
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_instances (
		run_id,
		project_id,
		workflow_id,
		submission_context_json,
		created_at
	) VALUES (?, ?, ?, ?, ?)`,
		run.ID,
		run.ProjectID,
		run.WorkflowID,
		run.SubmissionContextJSON,
		run.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert workflow run %s: %w", run.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow run create: %w", err)
	}
	return nil
}

func (s *Store) GetWorkflowRun(ctx context.Context, runID string) (WorkflowRunRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkflowRunRecord{}, false, err
	}
	return getWorkflowRun(ctx, s.db, runID)
}

func (s *Store) InsertStagePlan(ctx context.Context, runID string, stages []WorkflowStageRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	if len(stages) == 0 {
		return fmt.Errorf("stage plan is required")
	}
	for index, stage := range stages {
		if err := stage.validate(); err != nil {
			return fmt.Errorf("stage %d: %w", index, err)
		}
		if stage.RunID != runID {
			return fmt.Errorf("stage %d run id %s does not match %s", index, stage.RunID, runID)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin stage plan insert: %w", err)
	}
	defer tx.Rollback()

	existing, err := listStagesForRun(ctx, tx, runID)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		if !sameStagePlan(existing, stages) {
			return fmt.Errorf("stage plan for run %s already exists with different values", runID)
		}
		return tx.Commit()
	}

	for _, stage := range stages {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_stages (
			run_id,
			stage_index,
			step_id,
			stage_source_reference,
			state,
			created_at,
			ready_at,
			started_at,
			completed_at,
			failed_at,
			output_json,
			output_json_sha256
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			stage.RunID,
			stage.StageIndex,
			stage.StepID,
			stage.StageSourceReference,
			stage.State,
			stage.CreatedAt,
			nullString(stage.ReadyAt),
			nullString(stage.StartedAt),
			nullString(stage.CompletedAt),
			nullString(stage.FailedAt),
			nullString(stage.OutputJSON),
			nullString(stage.OutputJSONSHA256),
		); err != nil {
			return fmt.Errorf("insert workflow stage %s/%d: %w", stage.RunID, stage.StageIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit stage plan insert: %w", err)
	}
	return nil
}

func (s *Store) GetWorkflowStage(ctx context.Context, runID string, stageIndex int) (WorkflowStageRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkflowStageRecord{}, false, err
	}
	return getWorkflowStage(ctx, s.db, runID, stageIndex)
}

func (s *Store) ListActiveWorkflowRuns(ctx context.Context) ([]WorkflowRunRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		run_id,
		project_id,
		workflow_id,
		submission_context_json,
		created_at
	FROM workflow_instances
	WHERE NOT EXISTS (
		SELECT 1
		FROM workflow_stages
		WHERE workflow_stages.run_id = workflow_instances.run_id
	)
	OR EXISTS (
		SELECT 1
		FROM workflow_stages
		WHERE workflow_stages.run_id = workflow_instances.run_id
		AND workflow_stages.state NOT IN ('completed', 'failed', 'skipped')
	)
	ORDER BY created_at, run_id`)
	if err != nil {
		return nil, fmt.Errorf("list active workflow runs: %w", err)
	}
	defer rows.Close()

	runs := []WorkflowRunRecord{}
	for rows.Next() {
		run, err := scanWorkflowRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list active workflow runs: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active workflow runs: %w", err)
	}
	return runs, nil
}

func (s *Store) InsertWorkItems(ctx context.Context, items []WorkItemRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := validateWorkItems(items); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin work item insert: %w", err)
	}
	defer tx.Rollback()

	if err := insertWorkItems(ctx, tx, items); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit work item insert: %w", err)
	}
	return nil
}

func (s *Store) EnqueueWorkItems(ctx context.Context, items []QueuedWorkRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := validateQueuedWorkItems(items); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queued work insert: %w", err)
	}
	defer tx.Rollback()

	if err := enqueueWorkItems(ctx, tx, items); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit queued work insert: %w", err)
	}
	return nil
}

func (s *Store) GetWorkItem(ctx context.Context, workItemID string) (WorkItemRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkItemRecord{}, false, err
	}
	return getWorkItem(ctx, s.db, workItemID)
}

func (s *Store) ListQueuedWorkItems(ctx context.Context) ([]QueuedWorkRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		work_items.work_item_id,
		work_items.run_id,
		work_items.stage_index,
		work_items.work_item_index,
		work_items.worker_payload_json,
		work_items.resolved_inputs_sha256,
		work_items.created_at,
		queued_work.queued_at
	FROM queued_work
	JOIN work_items ON work_items.work_item_id = queued_work.work_item_id
	ORDER BY queued_work.queued_at, queued_work.work_item_id`)
	if err != nil {
		return nil, fmt.Errorf("list queued work items: %w", err)
	}
	defer rows.Close()

	items := []QueuedWorkRecord{}
	for rows.Next() {
		item, err := scanQueuedWork(rows)
		if err != nil {
			return nil, fmt.Errorf("list queued work items: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list queued work items: %w", err)
	}
	return items, nil
}

func (s *Store) CountWorkItemsForStage(ctx context.Context, runID string, stageIndex int) (WorkItemStatusCounts, error) {
	if err := s.requireOpen(); err != nil {
		return WorkItemStatusCounts{}, err
	}
	if runID == "" {
		return WorkItemStatusCounts{}, fmt.Errorf("run id is required")
	}
	if stageIndex < 0 {
		return WorkItemStatusCounts{}, fmt.Errorf("stage index must be non-negative")
	}

	var counts WorkItemStatusCounts
	queries := []struct {
		name string
		sql  string
		dest *int
	}{
		{name: "queued", sql: `SELECT COUNT(*) FROM queued_work JOIN work_items ON work_items.work_item_id = queued_work.work_item_id WHERE work_items.run_id = ? AND work_items.stage_index = ?`, dest: &counts.Queued},
		{name: "running", sql: `SELECT COUNT(*) FROM running_work JOIN work_items ON work_items.work_item_id = running_work.work_item_id WHERE work_items.run_id = ? AND work_items.stage_index = ?`, dest: &counts.Running},
		{name: "completed", sql: `SELECT COUNT(*) FROM completed_work JOIN work_items ON work_items.work_item_id = completed_work.work_item_id WHERE work_items.run_id = ? AND work_items.stage_index = ?`, dest: &counts.Completed},
		{name: "failed", sql: `SELECT COUNT(*) FROM failed_work JOIN work_items ON work_items.work_item_id = failed_work.work_item_id WHERE work_items.run_id = ? AND work_items.stage_index = ?`, dest: &counts.Failed},
	}
	for _, query := range queries {
		if err := s.db.QueryRowContext(ctx, query.sql, runID, stageIndex).Scan(query.dest); err != nil {
			return WorkItemStatusCounts{}, fmt.Errorf("count %s work items for stage %s/%d: %w", query.name, runID, stageIndex, err)
		}
	}
	return counts, nil
}

func (s *Store) CompleteStageIfReady(ctx context.Context, request CompleteStageRequest) (CompleteStageResult, error) {
	if err := s.requireOpen(); err != nil {
		return CompleteStageResult{}, err
	}
	if err := request.validate(); err != nil {
		return CompleteStageResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompleteStageResult{}, fmt.Errorf("begin complete stage: %w", err)
	}
	defer tx.Rollback()

	stage, found, err := getWorkflowStage(ctx, tx, request.RunID, request.StageIndex)
	if err != nil {
		return CompleteStageResult{}, err
	}
	if !found {
		return CompleteStageResult{Found: false}, nil
	}
	if stage.State == "completed" {
		if !stageMatchesCompletionRequest(stage, request) {
			return CompleteStageResult{}, fmt.Errorf("stage %s/%d already completed with different values", request.RunID, request.StageIndex)
		}
		return CompleteStageResult{Stage: stage, Found: true, Completed: true}, nil
	}
	if stage.State == "failed" || stage.State == "skipped" || stage.State == "blocked" {
		return CompleteStageResult{}, fmt.Errorf("stage %s/%d is %s and cannot be completed", request.RunID, request.StageIndex, stage.State)
	}

	ready, err := stageWorkComplete(ctx, tx, request.RunID, request.StageIndex)
	if err != nil {
		return CompleteStageResult{}, err
	}
	if !ready {
		return CompleteStageResult{Stage: stage, Found: true, Completed: false}, nil
	}

	if _, err := tx.ExecContext(ctx, `UPDATE workflow_stages
	SET state = 'completed',
		completed_at = ?,
		output_json = ?,
		output_json_sha256 = ?
	WHERE run_id = ? AND stage_index = ?`,
		request.CompletedAt,
		request.OutputJSON,
		request.OutputJSONSHA256,
		request.RunID,
		request.StageIndex,
	); err != nil {
		return CompleteStageResult{}, fmt.Errorf("complete stage %s/%d: %w", request.RunID, request.StageIndex, err)
	}

	if len(request.ReadyWorkItems) != 0 {
		if err := insertWorkItems(ctx, tx, request.ReadyWorkItems); err != nil {
			return CompleteStageResult{}, err
		}
	}
	if len(request.ReadyQueuedWork) != 0 {
		if err := enqueueWorkItems(ctx, tx, request.ReadyQueuedWork); err != nil {
			return CompleteStageResult{}, err
		}
	}

	stage.State = "completed"
	stage.CompletedAt = request.CompletedAt
	stage.OutputJSON = request.OutputJSON
	stage.OutputJSONSHA256 = request.OutputJSONSHA256
	if err := tx.Commit(); err != nil {
		return CompleteStageResult{}, fmt.Errorf("commit complete stage: %w", err)
	}
	return CompleteStageResult{Stage: stage, Found: true, Completed: true}, nil
}

func (s *Store) ClaimNextWork(ctx context.Context, request ClaimWorkRequest) (ClaimedWorkRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return ClaimedWorkRecord{}, false, err
	}
	if err := request.validate(); err != nil {
		return ClaimedWorkRecord{}, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("begin claim next work: %w", err)
	}
	defer tx.Rollback()

	queued, err := scanQueuedWork(tx.QueryRowContext(ctx, `SELECT
		work_items.work_item_id,
		work_items.run_id,
		work_items.stage_index,
		work_items.work_item_index,
		work_items.worker_payload_json,
		work_items.resolved_inputs_sha256,
		work_items.created_at,
		queued_work.queued_at
	FROM queued_work
	JOIN work_items ON work_items.work_item_id = queued_work.work_item_id
	ORDER BY queued_work.queued_at, queued_work.work_item_id
	LIMIT 1`))
	if err == sql.ErrNoRows {
		return ClaimedWorkRecord{}, false, nil
	}
	if err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("claim next work: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		worker_id,
		executor_type,
		started_at
	) VALUES (?, ?, ?, ?, ?)`,
		request.AttemptID,
		queued.ID,
		nullString(request.WorkerID),
		request.ExecutorType,
		request.StartedAt,
	); err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("insert work item attempt %s: %w", request.AttemptID, err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO running_work (
		attempt_id,
		work_item_id,
		worker_id,
		queued_at,
		started_at
	) VALUES (?, ?, ?, ?, ?)`,
		request.AttemptID,
		queued.ID,
		nullString(request.WorkerID),
		queued.QueuedAt,
		request.StartedAt,
	); err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("insert running work %s: %w", request.AttemptID, err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM queued_work WHERE work_item_id = ?`, queued.ID)
	if err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("delete queued work %s: %w", queued.ID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("delete queued work %s: %w", queued.ID, err)
	}
	if !deleted {
		return ClaimedWorkRecord{}, false, fmt.Errorf("delete queued work %s: no row deleted", queued.ID)
	}

	claimed := ClaimedWorkRecord{
		AttemptID:    request.AttemptID,
		WorkItem:     queued.WorkItemRecord,
		WorkerID:     request.WorkerID,
		ExecutorType: request.ExecutorType,
		QueuedAt:     queued.QueuedAt,
		StartedAt:    request.StartedAt,
	}
	if err := tx.Commit(); err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("commit claim next work: %w", err)
	}
	return claimed, true, nil
}

func (s *Store) CompleteAttempt(ctx context.Context, request CompleteAttemptRequest) (CompletedWorkRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return CompletedWorkRecord{}, false, err
	}
	if err := request.validate(); err != nil {
		return CompletedWorkRecord{}, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompletedWorkRecord{}, false, fmt.Errorf("begin complete attempt: %w", err)
	}
	defer tx.Rollback()

	running, found, err := getRunningWork(ctx, tx, request.AttemptID)
	if err != nil {
		return CompletedWorkRecord{}, false, fmt.Errorf("get running work %s: %w", request.AttemptID, err)
	}
	if !found {
		existing, completed, err := getCompletedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return CompletedWorkRecord{}, false, fmt.Errorf("get completed work %s: %w", request.AttemptID, err)
		}
		if completed {
			if !completedWorkMatchesRequest(existing, request) {
				return CompletedWorkRecord{}, false, fmt.Errorf("complete attempt %s conflicts with existing completed work", request.AttemptID)
			}
			return existing, true, nil
		}
		_, failed, err := getFailedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return CompletedWorkRecord{}, false, fmt.Errorf("get failed work %s: %w", request.AttemptID, err)
		}
		if failed {
			return CompletedWorkRecord{}, false, fmt.Errorf("complete attempt %s conflicts with existing failed work", request.AttemptID)
		}
		return CompletedWorkRecord{}, false, nil
	}

	completed := CompletedWorkRecord{
		AttemptID:        request.AttemptID,
		WorkItemID:       running.workItemID,
		SkippedParentID:  request.SkippedParentID,
		OutputJSON:       request.OutputJSON,
		OutputJSONSHA256: request.OutputJSONSHA256,
		PreStateSHA256:   request.PreStateSHA256,
		PostStateSHA256:  request.PostStateSHA256,
		QueuedAt:         running.queuedAt,
		StartedAt:        running.startedAt,
		CompletedAt:      request.CompletedAt,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO completed_work (
		attempt_id,
		work_item_id,
		skipped_parent_id,
		output_json,
		output_json_sha256,
		pre_state_sha256,
		post_state_sha256,
		queued_at,
		started_at,
		completed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		completed.AttemptID,
		completed.WorkItemID,
		nullString(completed.SkippedParentID),
		completed.OutputJSON,
		completed.OutputJSONSHA256,
		completed.PreStateSHA256,
		completed.PostStateSHA256,
		completed.QueuedAt,
		completed.StartedAt,
		completed.CompletedAt,
	); err != nil {
		return CompletedWorkRecord{}, false, fmt.Errorf("insert completed work %s: %w", request.AttemptID, err)
	}
	if err := deleteRunningWork(ctx, tx, request.AttemptID); err != nil {
		return CompletedWorkRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return CompletedWorkRecord{}, false, fmt.Errorf("commit complete attempt: %w", err)
	}
	return completed, true, nil
}

func (s *Store) FailAttempt(ctx context.Context, request FailAttemptRequest) (FailedWorkRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return FailedWorkRecord{}, false, err
	}
	if err := request.validate(); err != nil {
		return FailedWorkRecord{}, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FailedWorkRecord{}, false, fmt.Errorf("begin fail attempt: %w", err)
	}
	defer tx.Rollback()

	running, found, err := getRunningWork(ctx, tx, request.AttemptID)
	if err != nil {
		return FailedWorkRecord{}, false, fmt.Errorf("get running work %s: %w", request.AttemptID, err)
	}
	if !found {
		existing, failed, err := getFailedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return FailedWorkRecord{}, false, fmt.Errorf("get failed work %s: %w", request.AttemptID, err)
		}
		if failed {
			if !failedWorkMatchesRequest(existing, request) {
				return FailedWorkRecord{}, false, fmt.Errorf("fail attempt %s conflicts with existing failed work", request.AttemptID)
			}
			return existing, true, nil
		}
		_, completed, err := getCompletedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return FailedWorkRecord{}, false, fmt.Errorf("get completed work %s: %w", request.AttemptID, err)
		}
		if completed {
			return FailedWorkRecord{}, false, fmt.Errorf("fail attempt %s conflicts with existing completed work", request.AttemptID)
		}
		return FailedWorkRecord{}, false, nil
	}

	failed := FailedWorkRecord{
		AttemptID:  request.AttemptID,
		WorkItemID: running.workItemID,
		Error:      request.Error,
		QueuedAt:   running.queuedAt,
		StartedAt:  running.startedAt,
		FailedAt:   request.FailedAt,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO failed_work (
		attempt_id,
		work_item_id,
		error,
		queued_at,
		started_at,
		failed_at
	) VALUES (?, ?, ?, ?, ?, ?)`,
		failed.AttemptID,
		failed.WorkItemID,
		failed.Error,
		failed.QueuedAt,
		failed.StartedAt,
		failed.FailedAt,
	); err != nil {
		return FailedWorkRecord{}, false, fmt.Errorf("insert failed work %s: %w", request.AttemptID, err)
	}
	if err := deleteRunningWork(ctx, tx, request.AttemptID); err != nil {
		return FailedWorkRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return FailedWorkRecord{}, false, fmt.Errorf("commit fail attempt: %w", err)
	}
	return failed, true, nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type scanner interface {
	Scan(...any) error
}

type runningWorkRecord struct {
	attemptID  string
	workItemID string
	queuedAt   string
	startedAt  string
}

func getRunningWork(ctx context.Context, q queryer, attemptID string) (runningWorkRecord, bool, error) {
	var running runningWorkRecord
	err := q.QueryRowContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		queued_at,
		started_at
	FROM running_work
	WHERE attempt_id = ?`, attemptID).Scan(
		&running.attemptID,
		&running.workItemID,
		&running.queuedAt,
		&running.startedAt,
	)
	if err == sql.ErrNoRows {
		return runningWorkRecord{}, false, nil
	}
	if err != nil {
		return runningWorkRecord{}, false, err
	}
	return running, true, nil
}

func getCompletedWork(ctx context.Context, q queryer, attemptID string) (CompletedWorkRecord, bool, error) {
	completed, err := scanCompletedWork(q.QueryRowContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		skipped_parent_id,
		output_json,
		output_json_sha256,
		pre_state_sha256,
		post_state_sha256,
		queued_at,
		started_at,
		completed_at
	FROM completed_work
	WHERE attempt_id = ?`, attemptID))
	if err == sql.ErrNoRows {
		return CompletedWorkRecord{}, false, nil
	}
	if err != nil {
		return CompletedWorkRecord{}, false, err
	}
	return completed, true, nil
}

func getFailedWork(ctx context.Context, q queryer, attemptID string) (FailedWorkRecord, bool, error) {
	failed, err := scanFailedWork(q.QueryRowContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		error,
		queued_at,
		started_at,
		failed_at
	FROM failed_work
	WHERE attempt_id = ?`, attemptID))
	if err == sql.ErrNoRows {
		return FailedWorkRecord{}, false, nil
	}
	if err != nil {
		return FailedWorkRecord{}, false, err
	}
	return failed, true, nil
}

func deleteRunningWork(ctx context.Context, tx *sql.Tx, attemptID string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM running_work WHERE attempt_id = ?`, attemptID)
	if err != nil {
		return fmt.Errorf("delete running work %s: %w", attemptID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return fmt.Errorf("delete running work %s: %w", attemptID, err)
	}
	if !deleted {
		return fmt.Errorf("delete running work %s: no row deleted", attemptID)
	}
	return nil
}

func stageWorkComplete(ctx context.Context, q queryer, runID string, stageIndex int) (bool, error) {
	var total int
	var completed int
	var queued int
	var running int
	var failed int
	err := q.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM work_items WHERE run_id = ? AND stage_index = ?),
		(SELECT COUNT(DISTINCT completed_work.work_item_id)
			FROM completed_work
			JOIN work_items ON work_items.work_item_id = completed_work.work_item_id
			WHERE work_items.run_id = ? AND work_items.stage_index = ?),
		(SELECT COUNT(*)
			FROM queued_work
			JOIN work_items ON work_items.work_item_id = queued_work.work_item_id
			WHERE work_items.run_id = ? AND work_items.stage_index = ?),
		(SELECT COUNT(*)
			FROM running_work
			JOIN work_items ON work_items.work_item_id = running_work.work_item_id
			WHERE work_items.run_id = ? AND work_items.stage_index = ?),
		(SELECT COUNT(*)
			FROM failed_work
			JOIN work_items ON work_items.work_item_id = failed_work.work_item_id
			WHERE work_items.run_id = ? AND work_items.stage_index = ?)`,
		runID, stageIndex,
		runID, stageIndex,
		runID, stageIndex,
		runID, stageIndex,
		runID, stageIndex,
	).Scan(&total, &completed, &queued, &running, &failed)
	if err != nil {
		return false, fmt.Errorf("check stage work completion %s/%d: %w", runID, stageIndex, err)
	}
	return total > 0 && completed == total && queued == 0 && running == 0 && failed == 0, nil
}

func insertWorkItems(ctx context.Context, tx *sql.Tx, items []WorkItemRecord) error {
	for _, item := range items {
		existing, found, err := getWorkItem(ctx, tx, item.ID)
		if err != nil {
			return err
		}
		if found {
			if existing != item {
				return fmt.Errorf("work item %s already exists with different values", item.ID)
			}
			continue
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO work_items (
			work_item_id,
			run_id,
			stage_index,
			work_item_index,
			worker_payload_json,
			resolved_inputs_sha256,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.ID,
			item.RunID,
			item.StageIndex,
			item.WorkItemIndex,
			item.WorkerPayloadJSON,
			item.ResolvedInputsSHA256,
			item.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert work item %s: %w", item.ID, err)
		}
	}
	return nil
}

func enqueueWorkItems(ctx context.Context, tx *sql.Tx, items []QueuedWorkRecord) error {
	for _, item := range items {
		existingQueuedAt, found, err := getQueuedWork(ctx, tx, item.ID)
		if err != nil {
			return err
		}
		if found {
			if existingQueuedAt != item.QueuedAt {
				return fmt.Errorf("queued work item %s already exists with different queued_at", item.ID)
			}
			continue
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO queued_work (
			work_item_id,
			queued_at
		) VALUES (?, ?)`,
			item.ID,
			item.QueuedAt,
		); err != nil {
			return fmt.Errorf("enqueue work item %s: %w", item.ID, err)
		}
	}
	return nil
}

func getProject(ctx context.Context, q queryer, projectID string) (ProjectRecord, bool, error) {
	if projectID == "" {
		return ProjectRecord{}, false, fmt.Errorf("project id is required")
	}

	var project ProjectRecord
	err := q.QueryRowContext(ctx, `SELECT
		project_id,
		project_name,
		repository_identity,
		source_commit,
		config_path,
		source_object_id,
		config_sha256,
		created_at
	FROM projects
	WHERE project_id = ?`, projectID).Scan(
		&project.ID,
		&project.Name,
		&project.RepositoryIdentity,
		&project.SourceCommit,
		&project.ConfigPath,
		&project.SourceObjectID,
		&project.ConfigSHA256,
		&project.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return ProjectRecord{}, false, nil
	}
	if err != nil {
		return ProjectRecord{}, false, fmt.Errorf("get project %s: %w", projectID, err)
	}
	return project, true, nil
}

func getWorkflow(ctx context.Context, q queryer, workflowID string) (WorkflowRecord, bool, error) {
	if workflowID == "" {
		return WorkflowRecord{}, false, fmt.Errorf("workflow id is required")
	}

	var workflow WorkflowRecord
	err := q.QueryRowContext(ctx, `SELECT
		workflow_id,
		project_id,
		workflow_name,
		repository_identity,
		source_commit,
		workflow_path,
		source_object_id,
		workflow_sha256,
		created_at
	FROM workflows
	WHERE workflow_id = ?`, workflowID).Scan(
		&workflow.ID,
		&workflow.ProjectID,
		&workflow.Name,
		&workflow.RepositoryIdentity,
		&workflow.SourceCommit,
		&workflow.WorkflowPath,
		&workflow.SourceObjectID,
		&workflow.WorkflowSHA256,
		&workflow.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return WorkflowRecord{}, false, nil
	}
	if err != nil {
		return WorkflowRecord{}, false, fmt.Errorf("get workflow %s: %w", workflowID, err)
	}
	return workflow, true, nil
}

func getWorkflowRun(ctx context.Context, q queryer, runID string) (WorkflowRunRecord, bool, error) {
	if runID == "" {
		return WorkflowRunRecord{}, false, fmt.Errorf("run id is required")
	}

	run, err := scanWorkflowRun(q.QueryRowContext(ctx, `SELECT
		run_id,
		project_id,
		workflow_id,
		submission_context_json,
		created_at
	FROM workflow_instances
	WHERE run_id = ?`, runID))
	if err == sql.ErrNoRows {
		return WorkflowRunRecord{}, false, nil
	}
	if err != nil {
		return WorkflowRunRecord{}, false, fmt.Errorf("get workflow run %s: %w", runID, err)
	}
	return run, true, nil
}

func getWorkflowStage(ctx context.Context, q queryer, runID string, stageIndex int) (WorkflowStageRecord, bool, error) {
	if runID == "" {
		return WorkflowStageRecord{}, false, fmt.Errorf("run id is required")
	}
	if stageIndex < 0 {
		return WorkflowStageRecord{}, false, fmt.Errorf("stage index must be non-negative")
	}

	stage, err := scanWorkflowStage(q.QueryRowContext(ctx, workflowStageSelectSQL()+` WHERE run_id = ? AND stage_index = ?`, runID, stageIndex))
	if err == sql.ErrNoRows {
		return WorkflowStageRecord{}, false, nil
	}
	if err != nil {
		return WorkflowStageRecord{}, false, fmt.Errorf("get workflow stage %s/%d: %w", runID, stageIndex, err)
	}
	return stage, true, nil
}

func getWorkItem(ctx context.Context, q queryer, workItemID string) (WorkItemRecord, bool, error) {
	if workItemID == "" {
		return WorkItemRecord{}, false, fmt.Errorf("work item id is required")
	}

	item, err := scanWorkItem(q.QueryRowContext(ctx, `SELECT
		work_item_id,
		run_id,
		stage_index,
		work_item_index,
		worker_payload_json,
		resolved_inputs_sha256,
		created_at
	FROM work_items
	WHERE work_item_id = ?`, workItemID))
	if err == sql.ErrNoRows {
		return WorkItemRecord{}, false, nil
	}
	if err != nil {
		return WorkItemRecord{}, false, fmt.Errorf("get work item %s: %w", workItemID, err)
	}
	return item, true, nil
}

func getQueuedWork(ctx context.Context, q queryer, workItemID string) (string, bool, error) {
	if workItemID == "" {
		return "", false, fmt.Errorf("work item id is required")
	}

	var queuedAt string
	err := q.QueryRowContext(ctx, `SELECT queued_at FROM queued_work WHERE work_item_id = ?`, workItemID).Scan(&queuedAt)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get queued work item %s: %w", workItemID, err)
	}
	return queuedAt, true, nil
}

func listStagesForRun(ctx context.Context, tx *sql.Tx, runID string) ([]WorkflowStageRecord, error) {
	rows, err := tx.QueryContext(ctx, workflowStageSelectSQL()+` WHERE run_id = ? ORDER BY stage_index`, runID)
	if err != nil {
		return nil, fmt.Errorf("list workflow stages for run %s: %w", runID, err)
	}
	defer rows.Close()

	stages := []WorkflowStageRecord{}
	for rows.Next() {
		stage, err := scanWorkflowStage(rows)
		if err != nil {
			return nil, fmt.Errorf("list workflow stages for run %s: %w", runID, err)
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow stages for run %s: %w", runID, err)
	}
	return stages, nil
}

func workflowStageSelectSQL() string {
	return `SELECT
		run_id,
		stage_index,
		step_id,
		stage_source_reference,
		state,
		created_at,
		ready_at,
		started_at,
		completed_at,
		failed_at,
		output_json,
		output_json_sha256
	FROM workflow_stages`
}

func scanWorkflowRun(row scanner) (WorkflowRunRecord, error) {
	var run WorkflowRunRecord
	err := row.Scan(
		&run.ID,
		&run.ProjectID,
		&run.WorkflowID,
		&run.SubmissionContextJSON,
		&run.CreatedAt,
	)
	return run, err
}

func scanWorkflowStage(row scanner) (WorkflowStageRecord, error) {
	var stage WorkflowStageRecord
	var readyAt sql.NullString
	var startedAt sql.NullString
	var completedAt sql.NullString
	var failedAt sql.NullString
	var outputJSON sql.NullString
	var outputJSONSHA256 sql.NullString

	err := row.Scan(
		&stage.RunID,
		&stage.StageIndex,
		&stage.StepID,
		&stage.StageSourceReference,
		&stage.State,
		&stage.CreatedAt,
		&readyAt,
		&startedAt,
		&completedAt,
		&failedAt,
		&outputJSON,
		&outputJSONSHA256,
	)
	if err != nil {
		return WorkflowStageRecord{}, err
	}

	stage.ReadyAt = readyAt.String
	stage.StartedAt = startedAt.String
	stage.CompletedAt = completedAt.String
	stage.FailedAt = failedAt.String
	stage.OutputJSON = outputJSON.String
	stage.OutputJSONSHA256 = outputJSONSHA256.String
	return stage, nil
}

func scanWorkItem(row scanner) (WorkItemRecord, error) {
	var item WorkItemRecord
	err := row.Scan(
		&item.ID,
		&item.RunID,
		&item.StageIndex,
		&item.WorkItemIndex,
		&item.WorkerPayloadJSON,
		&item.ResolvedInputsSHA256,
		&item.CreatedAt,
	)
	return item, err
}

func scanQueuedWork(row scanner) (QueuedWorkRecord, error) {
	var item QueuedWorkRecord
	err := row.Scan(
		&item.ID,
		&item.RunID,
		&item.StageIndex,
		&item.WorkItemIndex,
		&item.WorkerPayloadJSON,
		&item.ResolvedInputsSHA256,
		&item.CreatedAt,
		&item.QueuedAt,
	)
	return item, err
}

func scanCompletedWork(row scanner) (CompletedWorkRecord, error) {
	var completed CompletedWorkRecord
	var skippedParentID sql.NullString
	err := row.Scan(
		&completed.AttemptID,
		&completed.WorkItemID,
		&skippedParentID,
		&completed.OutputJSON,
		&completed.OutputJSONSHA256,
		&completed.PreStateSHA256,
		&completed.PostStateSHA256,
		&completed.QueuedAt,
		&completed.StartedAt,
		&completed.CompletedAt,
	)
	if err != nil {
		return CompletedWorkRecord{}, err
	}
	completed.SkippedParentID = skippedParentID.String
	return completed, nil
}

func scanFailedWork(row scanner) (FailedWorkRecord, error) {
	var failed FailedWorkRecord
	err := row.Scan(
		&failed.AttemptID,
		&failed.WorkItemID,
		&failed.Error,
		&failed.QueuedAt,
		&failed.StartedAt,
		&failed.FailedAt,
	)
	return failed, err
}

func (s *Store) requireOpen() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	return nil
}

func (r WorkflowRunRecord) validate() error {
	if r.ID == "" {
		return fmt.Errorf("run id is required")
	}
	if r.ProjectID == "" {
		return fmt.Errorf("run project id is required")
	}
	if r.WorkflowID == "" {
		return fmt.Errorf("run workflow id is required")
	}
	if r.SubmissionContextJSON == "" {
		return fmt.Errorf("run submission context json is required")
	}
	if !json.Valid([]byte(r.SubmissionContextJSON)) {
		return fmt.Errorf("run submission context json must be valid JSON")
	}
	if r.CreatedAt == "" {
		return fmt.Errorf("run created at is required")
	}
	return nil
}

func (s WorkflowStageRecord) validate() error {
	if s.RunID == "" {
		return fmt.Errorf("stage run id is required")
	}
	if s.StageIndex < 0 {
		return fmt.Errorf("stage index must be non-negative")
	}
	if s.StepID == "" {
		return fmt.Errorf("stage step id is required")
	}
	if s.StageSourceReference == "" {
		return fmt.Errorf("stage source reference is required")
	}
	if !validStageState(s.State) {
		return fmt.Errorf("unsupported stage state: %s", s.State)
	}
	if s.CreatedAt == "" {
		return fmt.Errorf("stage created at is required")
	}
	if s.OutputJSON != "" && !json.Valid([]byte(s.OutputJSON)) {
		return fmt.Errorf("stage output json must be valid JSON")
	}
	return nil
}

func (r CompleteStageRequest) validate() error {
	if r.RunID == "" {
		return fmt.Errorf("stage run id is required")
	}
	if r.StageIndex < 0 {
		return fmt.Errorf("stage index must be non-negative")
	}
	if r.OutputJSON == "" {
		return fmt.Errorf("stage output json is required")
	}
	if !json.Valid([]byte(r.OutputJSON)) {
		return fmt.Errorf("stage output json must be valid JSON")
	}
	if r.OutputJSONSHA256 == "" {
		return fmt.Errorf("stage output json sha256 is required")
	}
	if r.CompletedAt == "" {
		return fmt.Errorf("stage completed at is required")
	}
	if len(r.ReadyWorkItems) != 0 {
		if err := validateWorkItems(r.ReadyWorkItems); err != nil {
			return fmt.Errorf("ready work: %w", err)
		}
	}
	if len(r.ReadyQueuedWork) != 0 {
		if err := validateQueuedWorkItems(r.ReadyQueuedWork); err != nil {
			return fmt.Errorf("ready queue: %w", err)
		}
	}
	return nil
}

func validStageState(state string) bool {
	switch state {
	case "ready", "running", "completed", "failed", "skipped", "blocked":
		return true
	default:
		return false
	}
}

func (w WorkItemRecord) validate() error {
	if w.ID == "" {
		return fmt.Errorf("work item id is required")
	}
	if w.RunID == "" {
		return fmt.Errorf("work item run id is required")
	}
	if w.StageIndex < 0 {
		return fmt.Errorf("work item stage index must be non-negative")
	}
	if w.WorkItemIndex < 0 {
		return fmt.Errorf("work item index must be non-negative")
	}
	if w.WorkerPayloadJSON == "" {
		return fmt.Errorf("work item worker payload json is required")
	}
	if !json.Valid([]byte(w.WorkerPayloadJSON)) {
		return fmt.Errorf("work item worker payload json must be valid JSON")
	}
	if w.ResolvedInputsSHA256 == "" {
		return fmt.Errorf("work item resolved inputs sha256 is required")
	}
	if w.CreatedAt == "" {
		return fmt.Errorf("work item created at is required")
	}
	return nil
}

func validateWorkItems(items []WorkItemRecord) error {
	if len(items) == 0 {
		return fmt.Errorf("work items are required")
	}
	for index, item := range items {
		if err := item.validate(); err != nil {
			return fmt.Errorf("work item %d: %w", index, err)
		}
	}
	return nil
}

func validateQueuedWorkItems(items []QueuedWorkRecord) error {
	if len(items) == 0 {
		return fmt.Errorf("queued work items are required")
	}
	for index, item := range items {
		if item.ID == "" {
			return fmt.Errorf("queued work item %d id is required", index)
		}
		if item.QueuedAt == "" {
			return fmt.Errorf("queued work item %d queued at is required", index)
		}
	}
	return nil
}

func (r ClaimWorkRequest) validate() error {
	if r.AttemptID == "" {
		return fmt.Errorf("claim attempt id is required")
	}
	if !validExecutorType(r.ExecutorType) {
		return fmt.Errorf("unsupported claim executor type: %s", r.ExecutorType)
	}
	if r.StartedAt == "" {
		return fmt.Errorf("claim started at is required")
	}
	return nil
}

func (r CompleteAttemptRequest) validate() error {
	if r.AttemptID == "" {
		return fmt.Errorf("complete attempt id is required")
	}
	if r.OutputJSON == "" {
		return fmt.Errorf("complete output json is required")
	}
	if !json.Valid([]byte(r.OutputJSON)) {
		return fmt.Errorf("complete output json must be valid JSON")
	}
	if r.OutputJSONSHA256 == "" {
		return fmt.Errorf("complete output json sha256 is required")
	}
	if r.PreStateSHA256 == "" {
		return fmt.Errorf("complete pre state sha256 is required")
	}
	if r.PostStateSHA256 == "" {
		return fmt.Errorf("complete post state sha256 is required")
	}
	if r.CompletedAt == "" {
		return fmt.Errorf("complete completed at is required")
	}
	return nil
}

func (r FailAttemptRequest) validate() error {
	if r.AttemptID == "" {
		return fmt.Errorf("fail attempt id is required")
	}
	if r.Error == "" {
		return fmt.Errorf("fail error is required")
	}
	if r.FailedAt == "" {
		return fmt.Errorf("fail failed at is required")
	}
	return nil
}

func completedWorkMatchesRequest(completed CompletedWorkRecord, request CompleteAttemptRequest) bool {
	return completed.AttemptID == request.AttemptID &&
		completed.SkippedParentID == request.SkippedParentID &&
		completed.OutputJSON == request.OutputJSON &&
		completed.OutputJSONSHA256 == request.OutputJSONSHA256 &&
		completed.PreStateSHA256 == request.PreStateSHA256 &&
		completed.PostStateSHA256 == request.PostStateSHA256 &&
		completed.CompletedAt == request.CompletedAt
}

func failedWorkMatchesRequest(failed FailedWorkRecord, request FailAttemptRequest) bool {
	return failed.AttemptID == request.AttemptID &&
		failed.Error == request.Error &&
		failed.FailedAt == request.FailedAt
}

func stageMatchesCompletionRequest(stage WorkflowStageRecord, request CompleteStageRequest) bool {
	return stage.RunID == request.RunID &&
		stage.StageIndex == request.StageIndex &&
		stage.CompletedAt == request.CompletedAt &&
		stage.OutputJSON == request.OutputJSON &&
		stage.OutputJSONSHA256 == request.OutputJSONSHA256
}

func validExecutorType(executorType string) bool {
	switch executorType {
	case ExecutorTypeWorker, ExecutorTypeController:
		return true
	default:
		return false
	}
}

func (p ProjectRecord) validate() error {
	if p.ID == "" {
		return fmt.Errorf("project id is required")
	}
	if p.RepositoryIdentity == "" {
		return fmt.Errorf("project repository identity is required")
	}
	if p.SourceCommit == "" {
		return fmt.Errorf("project source commit is required")
	}
	if p.ConfigPath == "" {
		return fmt.Errorf("project config path is required")
	}
	if p.ConfigSHA256 == "" {
		return fmt.Errorf("project config sha256 is required")
	}
	if p.CreatedAt == "" {
		return fmt.Errorf("project created at is required")
	}
	return nil
}

func (w WorkflowRecord) validate() error {
	if w.ID == "" {
		return fmt.Errorf("workflow id is required")
	}
	if w.ProjectID == "" {
		return fmt.Errorf("workflow project id is required")
	}
	if w.RepositoryIdentity == "" {
		return fmt.Errorf("workflow repository identity is required")
	}
	if w.SourceCommit == "" {
		return fmt.Errorf("workflow source commit is required")
	}
	if w.WorkflowPath == "" {
		return fmt.Errorf("workflow path is required")
	}
	if w.WorkflowSHA256 == "" {
		return fmt.Errorf("workflow sha256 is required")
	}
	if w.CreatedAt == "" {
		return fmt.Errorf("workflow created at is required")
	}
	return nil
}

func rowsAffected(result sql.Result) (bool, error) {
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count != 0, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func sameStagePlan(left []WorkflowStageRecord, right []WorkflowStageRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

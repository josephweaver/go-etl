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
	if len(items) == 0 {
		return fmt.Errorf("work items are required")
	}
	for index, item := range items {
		if err := item.validate(); err != nil {
			return fmt.Errorf("work item %d: %w", index, err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin work item insert: %w", err)
	}
	defer tx.Rollback()

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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit work item insert: %w", err)
	}
	return nil
}

func (s *Store) EnqueueWorkItems(ctx context.Context, items []QueuedWorkRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queued work insert: %w", err)
	}
	defer tx.Rollback()

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

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type scanner interface {
	Scan(...any) error
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

package persistence

import (
	"context"
	"database/sql"
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

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
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

func (s *Store) requireOpen() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
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

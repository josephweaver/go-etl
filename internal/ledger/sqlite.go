package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS attempts (
		attempt_id TEXT PRIMARY KEY,
		workflow_instance_id TEXT NOT NULL,
		step_instance_id TEXT NOT NULL,
		work_item_id TEXT NOT NULL,
		work_item_fingerprint TEXT NOT NULL,
		input_fingerprint TEXT NOT NULL,
		output_fingerprint TEXT NOT NULL,
		code_version TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at TEXT NOT NULL,
		completed_at TEXT
	);`,
	`CREATE TABLE IF NOT EXISTS attempt_variables (
		attempt_id TEXT NOT NULL,
		namespace TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		value_json TEXT NOT NULL,
		source TEXT NOT NULL,
		lifecycle TEXT NOT NULL,

		PRIMARY KEY (attempt_id, namespace, name),
		FOREIGN KEY (attempt_id) REFERENCES attempts(attempt_id)
	);`,
}

type AttemptStatus string

const (
	AttemptStatusCompleted AttemptStatus = "completed"
	AttemptStatusFailed    AttemptStatus = "failed"
)

type Attempt struct {
	ID                  string
	WorkflowInstanceID  string
	StepInstanceID      string
	WorkItemID          string
	WorkItemFingerprint string
	InputFingerprint    string
	OutputFingerprint   string
	CodeVersion         string
	Status              AttemptStatus
	StartedAt           time.Time
	CompletedAt         time.Time
	Variables           []AttemptVariable
}

type AttemptVariable struct {
	Namespace string
	Name      string
	Type      string
	Value     any
	Source    string
	Lifecycle string
}

func OpenSQLite(path string) (*sql.DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("create sqlite ledger directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite ledger: %w", err)
	}

	return db, nil
}

func InitSQLiteSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	for _, statement := range schemaStatements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize sqlite schema: %w", err)
		}
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO schema_version (version)
		SELECT ? WHERE NOT EXISTS (SELECT 1 FROM schema_version);`, schemaVersion); err != nil {
		return fmt.Errorf("record sqlite schema version: %w", err)
	}

	return nil
}

func InsertAttempt(ctx context.Context, db *sql.DB, attempt Attempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin attempt insert: %w", err)
	}
	defer tx.Rollback()

	var completedAt any
	if !attempt.CompletedAt.IsZero() {
		completedAt = attempt.CompletedAt.Format(time.RFC3339Nano)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO attempts (
		attempt_id,
		workflow_instance_id,
		step_instance_id,
		work_item_id,
		work_item_fingerprint,
		input_fingerprint,
		output_fingerprint,
		code_version,
		status,
		started_at,
		completed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attempt.ID,
		attempt.WorkflowInstanceID,
		attempt.StepInstanceID,
		attempt.WorkItemID,
		attempt.WorkItemFingerprint,
		attempt.InputFingerprint,
		attempt.OutputFingerprint,
		attempt.CodeVersion,
		string(attempt.Status),
		attempt.StartedAt.Format(time.RFC3339Nano),
		completedAt,
	); err != nil {
		return fmt.Errorf("insert attempt: %w", err)
	}

	for _, item := range attempt.Variables {
		valueJSON, err := json.Marshal(item.Value)
		if err != nil {
			return fmt.Errorf("encode attempt variable %s.%s: %w", item.Namespace, item.Name, err)
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO attempt_variables (
			attempt_id,
			namespace,
			name,
			type,
			value_json,
			source,
			lifecycle
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			attempt.ID,
			item.Namespace,
			item.Name,
			item.Type,
			string(valueJSON),
			item.Source,
			item.Lifecycle,
		); err != nil {
			return fmt.Errorf("insert attempt variable %s.%s: %w", item.Namespace, item.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attempt insert: %w", err)
	}

	return nil
}

func FindLatestCompletedAttemptByWorkItemFingerprint(ctx context.Context, db *sql.DB, fingerprint string) (Attempt, bool, error) {
	if fingerprint == "" {
		return Attempt{}, false, fmt.Errorf("work item fingerprint is required")
	}

	var attempt Attempt
	var status string
	var startedAt string
	var completedAt sql.NullString
	err := db.QueryRowContext(ctx, `SELECT
		attempt_id,
		workflow_instance_id,
		step_instance_id,
		work_item_id,
		work_item_fingerprint,
		input_fingerprint,
		output_fingerprint,
		code_version,
		status,
		started_at,
		completed_at
	FROM attempts
	WHERE work_item_fingerprint = ? AND status = ?
	ORDER BY completed_at DESC, started_at DESC, attempt_id DESC
	LIMIT 1`,
		fingerprint,
		string(AttemptStatusCompleted),
	).Scan(
		&attempt.ID,
		&attempt.WorkflowInstanceID,
		&attempt.StepInstanceID,
		&attempt.WorkItemID,
		&attempt.WorkItemFingerprint,
		&attempt.InputFingerprint,
		&attempt.OutputFingerprint,
		&attempt.CodeVersion,
		&status,
		&startedAt,
		&completedAt,
	)
	if err == sql.ErrNoRows {
		return Attempt{}, false, nil
	}
	if err != nil {
		return Attempt{}, false, fmt.Errorf("query completed attempt: %w", err)
	}

	parsedStartedAt, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return Attempt{}, false, fmt.Errorf("parse started_at: %w", err)
	}
	attempt.StartedAt = parsedStartedAt

	if completedAt.Valid {
		parsedCompletedAt, err := time.Parse(time.RFC3339Nano, completedAt.String)
		if err != nil {
			return Attempt{}, false, fmt.Errorf("parse completed_at: %w", err)
		}
		attempt.CompletedAt = parsedCompletedAt
	}

	attempt.Status = AttemptStatus(status)
	return attempt, true, nil
}

func (a Attempt) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("attempt id is required")
	}
	if a.WorkflowInstanceID == "" {
		return fmt.Errorf("workflow instance id is required")
	}
	if a.StepInstanceID == "" {
		return fmt.Errorf("step instance id is required")
	}
	if a.WorkItemID == "" {
		return fmt.Errorf("work item id is required")
	}
	if a.WorkItemFingerprint == "" {
		return fmt.Errorf("work item fingerprint is required")
	}
	if a.InputFingerprint == "" {
		return fmt.Errorf("input fingerprint is required")
	}
	if a.OutputFingerprint == "" {
		return fmt.Errorf("output fingerprint is required")
	}
	if a.CodeVersion == "" {
		return fmt.Errorf("code version is required")
	}
	if a.Status != AttemptStatusCompleted && a.Status != AttemptStatusFailed {
		return fmt.Errorf("unsupported attempt status: %s", a.Status)
	}
	if a.StartedAt.IsZero() {
		return fmt.Errorf("started at is required")
	}

	for index, item := range a.Variables {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("attempt variable %d: %w", index, err)
		}
	}

	return nil
}

func (v AttemptVariable) Validate() error {
	if v.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if v.Name == "" {
		return fmt.Errorf("name is required")
	}
	if v.Type == "" {
		return fmt.Errorf("type is required")
	}
	if v.Source == "" {
		return fmt.Errorf("source is required")
	}
	if v.Lifecycle == "" {
		return fmt.Errorf("lifecycle is required")
	}

	return nil
}

package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var sqliteSchemaStatements = []string{
	`CREATE TABLE schema_version (
		version INTEGER NOT NULL
	);`,
	`CREATE TABLE projects (
		project_id TEXT PRIMARY KEY,
		project_name TEXT,
		repository_identity TEXT NOT NULL,
		source_commit TEXT NOT NULL,
		config_path TEXT NOT NULL,
		source_object_id TEXT,
		config_sha256 TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE workflows (
		workflow_id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		workflow_name TEXT,
		repository_identity TEXT NOT NULL,
		source_commit TEXT NOT NULL,
		workflow_path TEXT NOT NULL,
		source_object_id TEXT,
		workflow_sha256 TEXT NOT NULL,
		created_at TEXT NOT NULL,

		FOREIGN KEY (project_id) REFERENCES projects(project_id)
	);`,
	`CREATE TABLE workflow_instances (
		run_id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		submission_context_json TEXT NOT NULL CHECK (json_valid(submission_context_json)),
		created_at TEXT NOT NULL,

		FOREIGN KEY (project_id) REFERENCES projects(project_id),
		FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
	);`,
	`CREATE TABLE workflow_stages (
		run_id TEXT NOT NULL,
		stage_index INTEGER NOT NULL,
		step_id TEXT NOT NULL,
		stage_source_reference TEXT NOT NULL,
		state TEXT NOT NULL CHECK (state IN ('ready', 'running', 'completed', 'failed', 'skipped', 'blocked')),
		created_at TEXT NOT NULL,
		ready_at TEXT,
		started_at TEXT,
		completed_at TEXT,
		failed_at TEXT,
		output_json TEXT CHECK (output_json IS NULL OR json_valid(output_json)),
		output_json_sha256 TEXT,

		PRIMARY KEY (run_id, stage_index),
		FOREIGN KEY (run_id) REFERENCES workflow_instances(run_id)
	);`,
	`CREATE TABLE work_items (
		work_item_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		stage_index INTEGER NOT NULL,
		work_item_index INTEGER NOT NULL,
		worker_payload_json TEXT NOT NULL CHECK (json_valid(worker_payload_json)),
		resolved_inputs_sha256 TEXT NOT NULL,
		created_at TEXT NOT NULL,

		UNIQUE (run_id, stage_index, work_item_index),
		FOREIGN KEY (run_id, stage_index) REFERENCES workflow_stages(run_id, stage_index)
	);`,
	`CREATE TABLE workers (
		worker_id TEXT PRIMARY KEY,
		run_id TEXT,
		execution_handle TEXT,
		created_at TEXT NOT NULL,

		FOREIGN KEY (run_id) REFERENCES workflow_instances(run_id)
	);`,
	`CREATE TABLE work_item_attempts (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		worker_id TEXT,
		executor_type TEXT NOT NULL CHECK (executor_type IN ('worker', 'controller')),
		started_at TEXT NOT NULL,

		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (worker_id) REFERENCES workers(worker_id)
	);`,
	`CREATE TABLE queued_work (
		work_item_id TEXT PRIMARY KEY,
		queued_at TEXT NOT NULL,

		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id)
	);`,
	`CREATE TABLE running_work (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL UNIQUE,
		worker_id TEXT,
		started_at TEXT NOT NULL,

		FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (worker_id) REFERENCES workers(worker_id)
	);`,
	`CREATE TABLE completed_work (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		skipped_parent_id TEXT,
		output_json TEXT NOT NULL CHECK (json_valid(output_json)),
		output_json_sha256 TEXT NOT NULL,
		pre_state_sha256 TEXT NOT NULL,
		post_state_sha256 TEXT NOT NULL,
		completed_at TEXT NOT NULL,

		FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (skipped_parent_id) REFERENCES completed_work(attempt_id)
	);`,
	`CREATE TABLE failed_work (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		error TEXT NOT NULL,
		failed_at TEXT NOT NULL,

		FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id)
	);`,
}

func openSQLiteStore(ctx context.Context, connectionString string) (*Store, error) {
	if connectionString != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(connectionString), 0755); err != nil {
			return nil, fmt.Errorf("create sqlite database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", connectionString)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := initSQLiteStoreSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func initSQLiteStoreSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite schema initialization: %w", err)
	}
	defer tx.Rollback()

	var tableCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tableCount); err != nil {
		return fmt.Errorf("inspect sqlite schema: %w", err)
	}

	if tableCount == 0 {
		if err := createSQLiteStoreSchema(ctx, tx); err != nil {
			return err
		}
	} else {
		if err := validateSQLiteStoreSchemaVersion(ctx, tx); err != nil {
			return err
		}
		hasCoreSchema, err := sqliteCoreSchemaExists(ctx, tx)
		if err != nil {
			return err
		}
		if !hasCoreSchema {
			metadataOnly, err := sqliteMetadataOnlySchema(ctx, tx)
			if err != nil {
				return err
			}
			if !metadataOnly {
				return fmt.Errorf("sqlite schema version %d is incomplete and cannot be replaced automatically", SupportedSchemaVersion)
			}
			if err := replaceSQLiteDevelopmentSchema(ctx, tx); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite schema initialization: %w", err)
	}
	return nil
}

func createSQLiteStoreSchema(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range sqliteSchemaStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize sqlite schema: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, SupportedSchemaVersion); err != nil {
		return fmt.Errorf("record sqlite schema version: %w", err)
	}
	return nil
}

func validateSQLiteStoreSchemaVersion(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT version FROM schema_version`)
	if err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	defer rows.Close()

	versions := make([]int, 0, 2)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("read sqlite schema version: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if len(versions) != 1 {
		return fmt.Errorf("sqlite schema_version must contain exactly one row, got %d", len(versions))
	}
	if versions[0] != SupportedSchemaVersion {
		return fmt.Errorf("sqlite schema version %d is unsupported; controller supports version %d", versions[0], SupportedSchemaVersion)
	}

	return nil
}

func sqliteCoreSchemaExists(ctx context.Context, tx *sql.Tx) (bool, error) {
	var tableCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table'
		AND name IN (
			'projects',
			'workflows',
			'workflow_instances',
			'workflow_stages',
			'work_items',
			'workers',
			'work_item_attempts',
			'queued_work',
			'running_work',
			'completed_work',
			'failed_work'
		)`).Scan(&tableCount); err != nil {
		return false, fmt.Errorf("inspect sqlite core schema: %w", err)
	}
	return tableCount == len(sqliteSchemaStatements)-1, nil
}

func sqliteMetadataOnlySchema(ctx context.Context, tx *sql.Tx) (bool, error) {
	var tableCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table'
		AND name NOT LIKE 'sqlite_%'
		AND name != 'schema_version'`).Scan(&tableCount); err != nil {
		return false, fmt.Errorf("inspect sqlite metadata-only schema: %w", err)
	}
	return tableCount == 0, nil
}

func replaceSQLiteDevelopmentSchema(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return fmt.Errorf("list sqlite development schema tables: %w", err)
	}
	defer rows.Close()

	tables := []string{}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return fmt.Errorf("list sqlite development schema tables: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list sqlite development schema tables: %w", err)
	}

	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DROP TABLE %s`, table)); err != nil {
			return fmt.Errorf("drop sqlite development schema table %s: %w", table, err)
		}
	}
	return createSQLiteStoreSchema(ctx, tx)
}

package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const sqlitePreviousSchemaVersion = 6

var sqliteVersionSixTableColumns = map[string][]string{
	"projects": {
		"project_id",
		"project_name",
		"repository_identity",
		"source_revision_id",
		"config_path",
		"source_object_id",
		"config_sha256",
		"created_at",
	},
	"workflows": {
		"workflow_id",
		"project_id",
		"workflow_name",
		"repository_identity",
		"source_revision_id",
		"workflow_path",
		"source_object_id",
		"workflow_sha256",
		"created_at",
	},
	"workflow_instances": {
		"run_id",
		"project_id",
		"workflow_id",
		"submission_context_json",
		"created_at",
	},
	"workflow_stages": {
		"run_id",
		"stage_index",
		"step_id",
		"stage_source_reference",
		"state",
		"created_at",
		"ready_at",
		"started_at",
		"completed_at",
		"failed_at",
		"output_json",
		"output_json_sha256",
	},
	"workflow_dependency_steps": {
		"run_id",
		"stage_index",
		"step_index",
		"step_id",
		"parallel_with",
		"created_at",
	},
	"workflow_dependency_work_items": {
		"run_id",
		"stage_index",
		"step_index",
		"work_item_id",
		"work_item_index",
		"created_at",
	},
	"workflow_step_output_facts": {
		"run_id",
		"step_index",
		"output_json",
		"output_json_sha256",
		"output_json_bytes",
		"output_json_pruned",
		"output_kind",
		"created_at",
		"updated_at",
	},
	"work_items": {
		"work_item_id",
		"run_id",
		"stage_index",
		"work_item_index",
		"worker_payload_json",
		"resolved_inputs_sha256",
		"created_at",
	},
	"workers": {
		"worker_id",
		"run_id",
		"execution_handle",
		"created_at",
	},
	"worker_sessions": {
		"worker_session_id",
		"worker_id",
		"status",
		"registered_at",
		"last_heartbeat_at",
		"ended_at",
		"end_reason",
		"execution_handle",
	},
	"work_item_attempts": {
		"attempt_id",
		"work_item_id",
		"worker_id",
		"worker_session_id",
		"executor_type",
		"started_at",
	},
	"queued_work": {
		"work_item_id",
		"queued_at",
	},
	"work_item_resource_constraints": {
		"work_item_id",
		"constraint_index",
		"resource_key",
		"requested_units",
		"operator",
		"target_units",
		"created_at",
	},
	"running_work": {
		"attempt_id",
		"work_item_id",
		"worker_id",
		"worker_session_id",
		"queued_at",
		"started_at",
	},
	"abandoned_work": {
		"attempt_id",
		"work_item_id",
		"worker_id",
		"worker_session_id",
		"queued_at",
		"started_at",
		"abandoned_at",
		"reason",
	},
	"completed_work": {
		"attempt_id",
		"work_item_id",
		"skipped_parent_id",
		"output_json",
		"output_json_sha256",
		"pre_state_sha256",
		"post_state_sha256",
		"queued_at",
		"started_at",
		"completed_at",
	},
	"failed_work": {
		"attempt_id",
		"work_item_id",
		"error",
		"queued_at",
		"started_at",
		"failed_at",
	},
}

var sqliteSchemaStatements = []string{
	`CREATE TABLE schema_version (
		version INTEGER NOT NULL
	);`,
	`CREATE TABLE projects (
		project_id TEXT PRIMARY KEY,
		project_name TEXT,
		repository_identity TEXT NOT NULL,
		source_revision_id TEXT,
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
		source_revision_id TEXT,
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
	`CREATE TABLE workflow_dependency_steps (
		run_id TEXT NOT NULL,
		stage_index INTEGER NOT NULL,
		step_index INTEGER NOT NULL,
		step_id TEXT NOT NULL,
		parallel_with TEXT NOT NULL,
		created_at TEXT NOT NULL,

		PRIMARY KEY (run_id, step_index),
		UNIQUE (run_id, stage_index, step_id),
		FOREIGN KEY (run_id) REFERENCES workflow_instances(run_id)
	);`,
	`CREATE TABLE workflow_dependency_work_items (
		run_id TEXT NOT NULL,
		stage_index INTEGER NOT NULL,
		step_index INTEGER NOT NULL,
		work_item_id TEXT NOT NULL,
		work_item_index INTEGER NOT NULL,
		created_at TEXT NOT NULL,

		PRIMARY KEY (run_id, work_item_id),
		UNIQUE (run_id, step_index, work_item_index),
		FOREIGN KEY (run_id, step_index) REFERENCES workflow_dependency_steps(run_id, step_index)
	);`,
	`CREATE TABLE workflow_step_output_facts (
		run_id TEXT NOT NULL,
		step_index INTEGER NOT NULL,
		output_json TEXT CHECK (output_json IS NULL OR json_valid(output_json)),
		output_json_sha256 TEXT NOT NULL,
		output_json_bytes INTEGER NOT NULL,
		output_json_pruned INTEGER NOT NULL CHECK (output_json_pruned IN (0, 1)),
		output_kind TEXT NOT NULL CHECK (output_kind IN ('aggregate', 'empty_fanout', 'skipped')),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,

		PRIMARY KEY (run_id, step_index),
		FOREIGN KEY (run_id, step_index) REFERENCES workflow_dependency_steps(run_id, step_index)
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
	`CREATE TABLE worker_sessions (
		worker_session_id TEXT PRIMARY KEY,
		worker_id TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('active', 'stopped', 'dead')),
		registered_at TEXT NOT NULL,
		last_heartbeat_at TEXT NOT NULL,
		ended_at TEXT,
		end_reason TEXT,
		execution_handle TEXT,

		UNIQUE (worker_id, worker_session_id),
		FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
		CHECK (
			(status = 'active' AND ended_at IS NULL)
			OR
			(status IN ('stopped', 'dead') AND ended_at IS NOT NULL)
		)
	);`,
	`CREATE TABLE work_item_attempts (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		worker_id TEXT,
		worker_session_id TEXT,
		executor_type TEXT NOT NULL CHECK (executor_type IN ('worker', 'controller')),
		started_at TEXT NOT NULL,
		resumed_from_attempt_id TEXT,
		resume_artifact_id TEXT,
		execution_lineage_id TEXT,
		resume_attempt_number INTEGER CHECK (resume_attempt_number IS NULL OR resume_attempt_number > 0),

		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
		FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id),
		FOREIGN KEY (worker_id, worker_session_id) REFERENCES worker_sessions(worker_id, worker_session_id),
		FOREIGN KEY (resumed_from_attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (resume_artifact_id) REFERENCES resume_artifacts(resume_artifact_id),
		CHECK (
			(executor_type = 'worker' AND worker_id IS NOT NULL AND worker_session_id IS NOT NULL)
			OR
			(executor_type = 'controller' AND worker_id IS NULL AND worker_session_id IS NULL)
		),
		CHECK (
			(
				resumed_from_attempt_id IS NULL
				AND resume_artifact_id IS NULL
				AND resume_attempt_number IS NULL
			)
			OR
			(
				resumed_from_attempt_id IS NOT NULL
				AND resume_artifact_id IS NOT NULL
				AND execution_lineage_id IS NOT NULL
				AND resume_attempt_number IS NOT NULL
			)
		)
	);`,
	`CREATE TABLE queued_work (
		work_item_id TEXT PRIMARY KEY,
		queued_at TEXT NOT NULL,
		resume_artifact_id TEXT,

		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (resume_artifact_id) REFERENCES resume_artifacts(resume_artifact_id)
	);`,
	`CREATE TABLE work_item_resource_constraints (
		work_item_id TEXT NOT NULL,
		constraint_index INTEGER NOT NULL,
		resource_key TEXT NOT NULL,
		requested_units INTEGER NOT NULL,
		operator TEXT NOT NULL CHECK (operator IN ('=', '!=', '<', '>', '<=', '>=')),
		target_units INTEGER NOT NULL,
		created_at TEXT NOT NULL,

		PRIMARY KEY (work_item_id, constraint_index),
		UNIQUE (work_item_id, resource_key),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),

		CHECK (constraint_index >= 0),
		CHECK (resource_key <> ''),
		CHECK (requested_units > 0),
		CHECK (target_units >= 0)
	);`,
	`CREATE TABLE running_work (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL UNIQUE,
		worker_id TEXT,
		worker_session_id TEXT,
		queued_at TEXT NOT NULL,
		started_at TEXT NOT NULL,

		FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
		FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id),
		FOREIGN KEY (worker_id, worker_session_id) REFERENCES worker_sessions(worker_id, worker_session_id)
	);`,
	`CREATE TABLE resume_artifacts (
		resume_artifact_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		producing_attempt_id TEXT NOT NULL,
		execution_lineage_id TEXT NOT NULL,
		resume_generation INTEGER NOT NULL CHECK (resume_generation > 0),
		capture_kind TEXT NOT NULL CHECK (capture_kind IN ('periodic', 'quantum', 'final')),
		pause_strategy TEXT NOT NULL CHECK (pause_strategy IN ('dmtcp', 'native', 'manual')),
		manifest_json TEXT NOT NULL CHECK (json_valid(manifest_json)),
		manifest_sha256 TEXT NOT NULL,
		storage_scope TEXT NOT NULL CHECK (storage_scope = 'shared_tmp'),
		manifest_relative_path TEXT NOT NULL,
		created_at TEXT NOT NULL,
		accepted_at TEXT NOT NULL,

		UNIQUE (execution_lineage_id, resume_generation),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (producing_attempt_id) REFERENCES work_item_attempts(attempt_id)
	);`,
	`CREATE TABLE suspended_work (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		resume_artifact_id TEXT NOT NULL,
		worker_id TEXT NOT NULL,
		worker_session_id TEXT NOT NULL,
		queued_at TEXT NOT NULL,
		started_at TEXT NOT NULL,
		suspended_at TEXT NOT NULL,
		suspend_reason TEXT NOT NULL CHECK (suspend_reason IN ('quantum', 'shutdown')),

		FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (resume_artifact_id) REFERENCES resume_artifacts(resume_artifact_id),
		FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
		FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id),
		FOREIGN KEY (worker_id, worker_session_id) REFERENCES worker_sessions(worker_id, worker_session_id)
	);`,
	`CREATE TABLE abandoned_work (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		worker_id TEXT NOT NULL,
		worker_session_id TEXT NOT NULL,
		queued_at TEXT NOT NULL,
		started_at TEXT NOT NULL,
		abandoned_at TEXT NOT NULL,
		reason TEXT NOT NULL,

		FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
		FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id)
	);`,
	`CREATE TABLE completed_work (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		skipped_parent_id TEXT,
		output_json TEXT NOT NULL CHECK (json_valid(output_json)),
		output_json_sha256 TEXT NOT NULL,
		pre_state_sha256 TEXT NOT NULL,
		post_state_sha256 TEXT NOT NULL,
		queued_at TEXT NOT NULL,
		started_at TEXT NOT NULL,
		completed_at TEXT NOT NULL,

		FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
		FOREIGN KEY (skipped_parent_id) REFERENCES completed_work(attempt_id)
	);`,
	`CREATE TABLE failed_work (
		attempt_id TEXT PRIMARY KEY,
		work_item_id TEXT NOT NULL,
		error TEXT NOT NULL,
		queued_at TEXT NOT NULL,
		started_at TEXT NOT NULL,
		failed_at TEXT NOT NULL,

		FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
		FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id)
	);`,
}

var sqliteIndexStatements = []string{
	`CREATE INDEX IF NOT EXISTS idx_work_items_run_stage_work_item
	ON work_items(run_id, stage_index, work_item_id);`,
	`CREATE INDEX IF NOT EXISTS idx_workflow_dependency_steps_run_stage_step
	ON workflow_dependency_steps(run_id, stage_index, step_index);`,
	`CREATE INDEX IF NOT EXISTS idx_workflow_dependency_work_items_run_stage_step_order
	ON workflow_dependency_work_items(run_id, stage_index, step_index, work_item_index, work_item_id);`,
	`CREATE INDEX IF NOT EXISTS idx_queued_work_queued_at_work_item
	ON queued_work(queued_at, work_item_id);`,
	`CREATE INDEX IF NOT EXISTS idx_running_work_started_at_attempt
	ON running_work(started_at, attempt_id);`,
	`CREATE INDEX IF NOT EXISTS idx_resume_artifacts_producing_attempt_generation
	ON resume_artifacts(producing_attempt_id, resume_generation);`,
	`CREATE INDEX IF NOT EXISTS idx_resume_artifacts_lineage_generation
	ON resume_artifacts(execution_lineage_id, resume_generation DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_suspended_work_item_time
	ON suspended_work(work_item_id, suspended_at, attempt_id);`,
	`CREATE INDEX IF NOT EXISTS idx_work_item_attempts_resume_artifact_attempt
	ON work_item_attempts(resume_artifact_id, resume_attempt_number);`,
	`CREATE INDEX IF NOT EXISTS idx_work_item_attempts_lineage_started
	ON work_item_attempts(execution_lineage_id, started_at, attempt_id);`,
	`CREATE INDEX IF NOT EXISTS idx_queued_work_resume_artifact
	ON queued_work(resume_artifact_id);`,
	`CREATE INDEX IF NOT EXISTS idx_worker_sessions_status_heartbeat
	ON worker_sessions(status, last_heartbeat_at);`,
	`CREATE INDEX IF NOT EXISTS idx_worker_sessions_worker_registered
	ON worker_sessions(worker_id, registered_at);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_running_work_one_per_worker_session
	ON running_work(worker_session_id)
	WHERE worker_session_id IS NOT NULL;`,
	`CREATE INDEX IF NOT EXISTS idx_abandoned_work_item_time
	ON abandoned_work(work_item_id, abandoned_at, attempt_id);`,
	`CREATE INDEX IF NOT EXISTS idx_completed_work_item_completed_at
	ON completed_work(work_item_id, completed_at, attempt_id);`,
	`CREATE INDEX IF NOT EXISTS idx_failed_work_item_failed_at
	ON failed_work(work_item_id, failed_at, attempt_id);`,
	`CREATE INDEX IF NOT EXISTS idx_work_item_resource_constraints_resource_key
	ON work_item_resource_constraints(resource_key);`,
	`CREATE INDEX IF NOT EXISTS idx_work_item_resource_constraints_work_item_id
	ON work_item_resource_constraints(work_item_id);`,
}

var sqliteViewStatements = []string{
	`CREATE VIEW IF NOT EXISTS queued_resource_constraint_checks AS
	SELECT
		q.work_item_id,
		q.queued_at,
		c.constraint_index,
		c.resource_key,
		COALESCE((
			SELECT SUM(r.requested_units)
			FROM running_work rw
			JOIN work_item_resource_constraints r
				ON r.work_item_id = rw.work_item_id
			WHERE r.resource_key = c.resource_key
		), 0) AS total_units,
		c.requested_units,
		c.operator,
		c.target_units
	FROM queued_work q
	JOIN work_item_resource_constraints c
		ON c.work_item_id = q.work_item_id;`,
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
		version, err := readSQLiteStoreSchemaVersion(ctx, tx)
		if err != nil {
			return err
		}
		switch version {
		case sqlitePreviousSchemaVersion:
			if err := validateSQLiteVersionSixMigrationSource(ctx, tx); err != nil {
				return err
			}
			if err := migrateSQLiteVersionSixToSeven(ctx, tx); err != nil {
				return err
			}
		case SupportedSchemaVersion:
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
		default:
			return unsupportedSQLiteSchemaVersion(version)
		}
	}

	if err := ensureSQLiteStoreIndexes(ctx, tx); err != nil {
		return err
	}
	if err := ensureSQLiteStoreViews(ctx, tx); err != nil {
		return err
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
	if err := createSQLiteStoreViews(ctx, tx); err != nil {
		return err
	}
	return nil
}

func ensureSQLiteStoreIndexes(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range sqliteIndexStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize sqlite indexes: %w", err)
		}
	}
	return nil
}

func ensureSQLiteStoreViews(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range sqliteViewStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize sqlite views: %w", err)
		}
	}
	return nil
}

func createSQLiteStoreViews(ctx context.Context, tx *sql.Tx) error {
	for _, statement := range sqliteViewStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create sqlite views: %w", err)
		}
	}
	return nil
}

func readSQLiteStoreSchemaVersion(ctx context.Context, tx *sql.Tx) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT version FROM schema_version`)
	if err != nil {
		return 0, fmt.Errorf("read sqlite schema version: %w", err)
	}
	defer rows.Close()

	versions := make([]int, 0, 2)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return 0, fmt.Errorf("read sqlite schema version: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read sqlite schema version: %w", err)
	}
	if len(versions) != 1 {
		return 0, fmt.Errorf("sqlite schema_version must contain exactly one row, got %d", len(versions))
	}
	return versions[0], nil
}

func unsupportedSQLiteSchemaVersion(version int) error {
	return fmt.Errorf(
		"sqlite schema version %d is unsupported; controller supports version %d; rebuild the development database or add an explicit migration",
		version,
		SupportedSchemaVersion,
	)
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
			'workflow_dependency_steps',
			'workflow_dependency_work_items',
			'workflow_step_output_facts',
			'work_items',
			'workers',
			'worker_sessions',
			'work_item_resource_constraints',
			'work_item_attempts',
			'queued_work',
			'running_work',
			'resume_artifacts',
			'suspended_work',
			'abandoned_work',
			'completed_work',
			'failed_work'
		)`).Scan(&tableCount); err != nil {
		return false, fmt.Errorf("inspect sqlite core schema: %w", err)
	}
	return tableCount == len(sqliteSchemaStatements)-1, nil
}

func validateSQLiteVersionSixMigrationSource(ctx context.Context, tx *sql.Tx) error {
	var tableCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table'
		AND name IN (
			'projects',
			'workflows',
			'workflow_instances',
			'workflow_stages',
			'workflow_dependency_steps',
			'workflow_dependency_work_items',
			'workflow_step_output_facts',
			'work_items',
			'workers',
			'worker_sessions',
			'work_item_resource_constraints',
			'work_item_attempts',
			'queued_work',
			'running_work',
			'abandoned_work',
			'completed_work',
			'failed_work'
		)`).Scan(&tableCount); err != nil {
		return fmt.Errorf("inspect sqlite version 6 schema: %w", err)
	}
	if tableCount != len(sqliteSchemaStatements)-3 {
		return fmt.Errorf("sqlite schema version 6 is incomplete and cannot be migrated automatically")
	}

	var applicationTableCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&applicationTableCount); err != nil {
		return fmt.Errorf("count sqlite version 6 tables: %w", err)
	}
	if applicationTableCount != len(sqliteVersionSixTableColumns)+1 {
		return fmt.Errorf("sqlite schema version 6 has unexpected tables and cannot be migrated automatically")
	}

	for table, expectedColumns := range sqliteVersionSixTableColumns {
		columns, err := sqliteTableColumns(ctx, tx, table)
		if err != nil {
			return err
		}
		if !equalStrings(columns, expectedColumns) {
			return fmt.Errorf("sqlite schema version 6 table %s has unsupported columns", table)
		}
	}

	var violations int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&violations); err != nil {
		return fmt.Errorf("check sqlite version 6 foreign keys: %w", err)
	}
	if violations != 0 {
		return fmt.Errorf("sqlite schema version 6 has %d foreign key violations and cannot be migrated", violations)
	}
	return nil
}

func migrateSQLiteVersionSixToSeven(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE resume_artifacts (
			resume_artifact_id TEXT PRIMARY KEY,
			work_item_id TEXT NOT NULL,
			producing_attempt_id TEXT NOT NULL,
			execution_lineage_id TEXT NOT NULL,
			resume_generation INTEGER NOT NULL CHECK (resume_generation > 0),
			capture_kind TEXT NOT NULL CHECK (capture_kind IN ('periodic', 'quantum', 'final')),
			pause_strategy TEXT NOT NULL CHECK (pause_strategy IN ('dmtcp', 'native', 'manual')),
			manifest_json TEXT NOT NULL CHECK (json_valid(manifest_json)),
			manifest_sha256 TEXT NOT NULL,
			storage_scope TEXT NOT NULL CHECK (storage_scope = 'shared_tmp'),
			manifest_relative_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			accepted_at TEXT NOT NULL,

			UNIQUE (execution_lineage_id, resume_generation),
			FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
			FOREIGN KEY (producing_attempt_id) REFERENCES work_item_attempts(attempt_id)
		);`,
		`ALTER TABLE work_item_attempts
			ADD COLUMN resumed_from_attempt_id TEXT REFERENCES work_item_attempts(attempt_id);`,
		`ALTER TABLE work_item_attempts
			ADD COLUMN resume_artifact_id TEXT REFERENCES resume_artifacts(resume_artifact_id);`,
		`ALTER TABLE work_item_attempts
			ADD COLUMN execution_lineage_id TEXT;`,
		`ALTER TABLE work_item_attempts
			ADD COLUMN resume_attempt_number INTEGER CHECK (resume_attempt_number IS NULL OR resume_attempt_number > 0);`,
		`ALTER TABLE queued_work
			ADD COLUMN resume_artifact_id TEXT REFERENCES resume_artifacts(resume_artifact_id);`,
		`CREATE TABLE suspended_work (
			attempt_id TEXT PRIMARY KEY,
			work_item_id TEXT NOT NULL,
			resume_artifact_id TEXT NOT NULL,
			worker_id TEXT NOT NULL,
			worker_session_id TEXT NOT NULL,
			queued_at TEXT NOT NULL,
			started_at TEXT NOT NULL,
			suspended_at TEXT NOT NULL,
			suspend_reason TEXT NOT NULL CHECK (suspend_reason IN ('quantum', 'shutdown')),

			FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
			FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
			FOREIGN KEY (resume_artifact_id) REFERENCES resume_artifacts(resume_artifact_id),
			FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
			FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id),
			FOREIGN KEY (worker_id, worker_session_id) REFERENCES worker_sessions(worker_id, worker_session_id)
		);`,
		`UPDATE schema_version SET version = 7 WHERE version = 6;`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sqlite schema version 6 to 7: %w", err)
		}
	}
	return nil
}

func sqliteTableColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return nil, fmt.Errorf("inspect sqlite table %s columns: %w", table, err)
	}
	defer rows.Close()

	columns := []string{}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("inspect sqlite table %s columns: %w", table, err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect sqlite table %s columns: %w", table, err)
	}
	return columns, nil
}

func equalStrings(left, right []string) bool {
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

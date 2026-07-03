package persistence

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpenStoreCreatesSQLiteDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "missing", "store.sqlite")

	store := openTestStore(t, ctx, path)
	defer store.Close()

	var version int
	if err := store.db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("query schema version: %v", err)
	}
	if version != SupportedSchemaVersion {
		t.Fatalf("version = %d, want %d", version, SupportedSchemaVersion)
	}

	for _, table := range []string{
		"projects",
		"workflows",
		"workflow_instances",
		"workflow_stages",
		"work_items",
		"workers",
		"work_item_attempts",
		"queued_work",
		"running_work",
		"completed_work",
		"failed_work",
	} {
		assertSQLiteTableExists(t, ctx, store.db, table)
	}
}

func TestOpenStoreAcceptsExistingSupportedSQLiteDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.sqlite")

	store := openTestStore(t, ctx, path)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store = openTestStore(t, ctx, path)
	defer store.Close()
}

func TestOpenStoreReplacesMetadataOnlySQLiteDevelopmentSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.sqlite")
	db := openRawSQLite(t, path)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_version (version INTEGER NOT NULL); INSERT INTO schema_version VALUES (?)`, SupportedSchemaVersion); err != nil {
		t.Fatalf("setup metadata-only schema: %v", err)
	}
	db.Close()

	store := openTestStore(t, ctx, path)
	defer store.Close()

	assertSQLiteTableExists(t, ctx, store.db, "projects")
	assertSQLiteTableExists(t, ctx, store.db, "completed_work")
}

func TestOpenStoreRejectsIncompleteSameVersionSQLiteSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.sqlite")
	db := openRawSQLite(t, path)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_version (version INTEGER NOT NULL); INSERT INTO schema_version VALUES (?); CREATE TABLE unexpected_table (id TEXT)`, SupportedSchemaVersion); err != nil {
		t.Fatalf("setup incomplete schema: %v", err)
	}
	db.Close()

	store, err := OpenStore(ctx, Config{Driver: DriverSQLite, ConnectionString: path})
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("OpenStore() error = %v, want incomplete schema", err)
	}
	if store != nil {
		t.Fatalf("OpenStore() store = %#v, want nil", store)
	}
}

func TestOpenStoreRejectsUnsupportedSQLiteSchemaVersion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.sqlite")
	db := openRawSQLite(t, path)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_version (version INTEGER NOT NULL); INSERT INTO schema_version VALUES (?)`, SupportedSchemaVersion+1); err != nil {
		t.Fatalf("setup schema: %v", err)
	}
	db.Close()

	store, err := OpenStore(ctx, Config{Driver: DriverSQLite, ConnectionString: path})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("OpenStore() error = %v, want unsupported schema", err)
	}
	if store != nil {
		t.Fatalf("OpenStore() store = %#v, want nil", store)
	}
}

func TestOpenStoreRejectsExistingSQLiteSchemaWithoutMetadata(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.sqlite")
	db := openRawSQLite(t, path)
	if _, err := db.ExecContext(ctx, `CREATE TABLE existing_table (id TEXT)`); err != nil {
		t.Fatalf("setup schema: %v", err)
	}
	db.Close()

	store, err := OpenStore(ctx, Config{Driver: DriverSQLite, ConnectionString: path})
	if err == nil || !strings.Contains(err.Error(), "read sqlite schema version") {
		t.Fatalf("OpenStore() error = %v, want missing schema metadata", err)
	}
	if store != nil {
		t.Fatalf("OpenStore() store = %#v, want nil", store)
	}
}

func TestOpenStoreEnablesSQLiteForeignKeys(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	var enabled int
	if err := store.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d, want 1", enabled)
	}
}

func TestOpenStoreCreatesSQLiteCoreConstraints(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	if _, err := store.db.ExecContext(ctx, `INSERT INTO workflows (
		workflow_id,
		project_id,
		repository_identity,
		source_commit,
		workflow_path,
		workflow_sha256,
		created_at
	) VALUES ('workflow-missing-project', 'missing-project', 'repo', 'commit', 'workflow.json', ?, '2026-07-03T00:00:00Z')`, strings.Repeat("a", 64)); err == nil {
		t.Fatal("expected workflow insert with missing project to fail")
	}

	insertMinimalStage(t, ctx, store.db)

	if _, err := store.db.ExecContext(ctx, `INSERT INTO workflow_instances (
		run_id,
		project_id,
		workflow_id,
		submission_context_json,
		created_at
	) VALUES ('run-invalid-json', 'project-001', 'workflow-001', 'not-json', '2026-07-03T00:00:00Z')`); err == nil {
		t.Fatal("expected invalid submission_context_json to fail")
	}

	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_items (
		work_item_id,
		run_id,
		stage_index,
		work_item_index,
		worker_payload_json,
		resolved_inputs_sha256,
		created_at
	) VALUES ('work-item-001', 'run-001', 0, 0, '{"plugin":"demo","parameters":{}}', ?, '2026-07-03T00:00:00Z')`, strings.Repeat("b", 64)); err != nil {
		t.Fatalf("insert work item: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_items (
		work_item_id,
		run_id,
		stage_index,
		work_item_index,
		worker_payload_json,
		resolved_inputs_sha256,
		created_at
	) VALUES ('work-item-duplicate', 'run-001', 0, 0, '{"plugin":"demo","parameters":{}}', ?, '2026-07-03T00:00:00Z')`, strings.Repeat("c", 64)); err == nil {
		t.Fatal("expected duplicate work item index to fail")
	}
}

func TestOpenStoreCreatesSQLiteSkippedParentColumn(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	insertMinimalStage(t, ctx, store.db)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_items (
		work_item_id,
		run_id,
		stage_index,
		work_item_index,
		worker_payload_json,
		resolved_inputs_sha256,
		created_at
	) VALUES ('work-item-001', 'run-001', 0, 0, '{"plugin":"demo","parameters":{}}', ?, '2026-07-03T00:00:00Z')`, strings.Repeat("b", 64)); err != nil {
		t.Fatalf("insert work item 001: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_items (
		work_item_id,
		run_id,
		stage_index,
		work_item_index,
		worker_payload_json,
		resolved_inputs_sha256,
		created_at
	) VALUES ('work-item-002', 'run-001', 0, 1, '{"plugin":"demo","parameters":{}}', ?, '2026-07-03T00:00:00Z')`, strings.Repeat("c", 64)); err != nil {
		t.Fatalf("insert work item 002: %v", err)
	}
	insertAttempt(t, ctx, store.db, "attempt-001", "work-item-001", "worker")
	insertAttempt(t, ctx, store.db, "attempt-002", "work-item-002", "controller")

	for _, statement := range []string{
		`INSERT INTO completed_work (
			attempt_id,
			work_item_id,
			output_json,
			output_json_sha256,
			pre_state_sha256,
			post_state_sha256,
			queued_at,
			started_at,
			completed_at
		) VALUES ('attempt-001', 'work-item-001', '[]', ?, ?, ?, '2026-07-03T00:00:00Z', '2026-07-03T00:00:01Z', '2026-07-03T00:00:02Z')`,
		`INSERT INTO completed_work (
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
		) VALUES ('attempt-002', 'work-item-002', 'attempt-001', '[]', ?, ?, ?, '2026-07-03T00:00:00Z', '2026-07-03T00:00:01Z', '2026-07-03T00:00:02Z')`,
	} {
		if _, err := store.db.ExecContext(ctx, statement, strings.Repeat("d", 64), strings.Repeat("e", 64), strings.Repeat("f", 64)); err != nil {
			t.Fatalf("insert completed work: %v", err)
		}
	}

	var skippedParentID string
	if err := store.db.QueryRowContext(ctx, `SELECT skipped_parent_id FROM completed_work WHERE attempt_id = 'attempt-002'`).Scan(&skippedParentID); err != nil {
		t.Fatalf("query skipped parent: %v", err)
	}
	if skippedParentID != "attempt-001" {
		t.Fatalf("skipped_parent_id = %q, want attempt-001", skippedParentID)
	}
}

func TestOpenStoreCreatesSQLiteRunningWorkQueuedAtColumn(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	insertMinimalStage(t, ctx, store.db)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_items (
		work_item_id,
		run_id,
		stage_index,
		work_item_index,
		worker_payload_json,
		resolved_inputs_sha256,
		created_at
	) VALUES ('work-item-001', 'run-001', 0, 0, '{"plugin":"demo","parameters":{}}', ?, '2026-07-03T00:00:00Z')`, strings.Repeat("b", 64)); err != nil {
		t.Fatalf("insert work item: %v", err)
	}
	insertAttempt(t, ctx, store.db, "attempt-001", "work-item-001", "worker")

	if _, err := store.db.ExecContext(ctx, `INSERT INTO running_work (
		attempt_id,
		work_item_id,
		queued_at,
		started_at
	) VALUES ('attempt-001', 'work-item-001', '2026-07-03T00:00:01Z', '2026-07-03T00:00:02Z')`); err != nil {
		t.Fatalf("insert running work: %v", err)
	}

	var queuedAt string
	if err := store.db.QueryRowContext(ctx, `SELECT queued_at FROM running_work WHERE attempt_id = 'attempt-001'`).Scan(&queuedAt); err != nil {
		t.Fatalf("query running queued_at: %v", err)
	}
	if queuedAt != "2026-07-03T00:00:01Z" {
		t.Fatalf("queued_at = %q, want 2026-07-03T00:00:01Z", queuedAt)
	}
}

func TestOpenStoreDoesNotCreateSupportedSchemaAfterFailedValidation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.sqlite")
	db := openRawSQLite(t, path)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_version (version INTEGER NOT NULL); INSERT INTO schema_version VALUES (?), (?)`, SupportedSchemaVersion, SupportedSchemaVersion); err != nil {
		t.Fatalf("setup schema: %v", err)
	}
	db.Close()

	store, err := OpenStore(ctx, Config{Driver: DriverSQLite, ConnectionString: path})
	if err == nil || !strings.Contains(err.Error(), "exactly one row") {
		t.Fatalf("OpenStore() error = %v, want invalid schema version state", err)
	}
	if store != nil {
		t.Fatalf("OpenStore() store = %#v, want nil", store)
	}

	db = openRawSQLite(t, path)
	defer db.Close()
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_version`).Scan(&rows); err != nil {
		t.Fatalf("query schema_version count: %v", err)
	}
	if rows != 2 {
		t.Fatalf("schema_version rows = %d, want original invalid state", rows)
	}
}

func openTestStore(t *testing.T, ctx context.Context, path string) *Store {
	t.Helper()

	store, err := OpenStore(ctx, Config{Driver: DriverSQLite, ConnectionString: path})
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return store
}

func openRawSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	return db
}

func assertSQLiteTableExists(t *testing.T, ctx context.Context, db *sql.DB, table string) {
	t.Helper()

	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	if name != table {
		t.Fatalf("table name = %q, want %q", name, table)
	}
}

func insertMinimalStage(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	statements := []struct {
		query string
		args  []any
	}{
		{
			query: `INSERT INTO projects (
				project_id,
				repository_identity,
				source_commit,
				config_path,
				config_sha256,
				created_at
			) VALUES ('project-001', 'repo', 'commit', 'project.json', ?, '2026-07-03T00:00:00Z')`,
			args: []any{strings.Repeat("a", 64)},
		},
		{
			query: `INSERT INTO workflows (
				workflow_id,
				project_id,
				repository_identity,
				source_commit,
				workflow_path,
				workflow_sha256,
				created_at
			) VALUES ('workflow-001', 'project-001', 'repo', 'commit', 'workflow.json', ?, '2026-07-03T00:00:00Z')`,
			args: []any{strings.Repeat("b", 64)},
		},
		{
			query: `INSERT INTO workflow_instances (
				run_id,
				project_id,
				workflow_id,
				submission_context_json,
				created_at
			) VALUES ('run-001', 'project-001', 'workflow-001', '[]', '2026-07-03T00:00:00Z')`,
		},
		{
			query: `INSERT INTO workflow_stages (
				run_id,
				stage_index,
				step_id,
				stage_source_reference,
				state,
				created_at
			) VALUES ('run-001', 0, 'step-001', 'workflow.json#/steps/0', 'ready', '2026-07-03T00:00:00Z')`,
		},
	}

	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("execute setup statement: %v", err)
		}
	}
}

func insertAttempt(t *testing.T, ctx context.Context, db *sql.DB, attemptID, workItemID, executorType string) {
	t.Helper()

	if _, err := db.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		executor_type,
		started_at
	) VALUES (?, ?, ?, '2026-07-03T00:00:00Z')`, attemptID, workItemID, executorType); err != nil {
		t.Fatalf("insert attempt %s: %v", attemptID, err)
	}
}

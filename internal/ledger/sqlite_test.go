package ledger

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestInitSQLiteSchemaCreatesVersionOneLedger(t *testing.T) {
	ctx := context.Background()

	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()

	if err := InitSQLiteSchema(ctx, db); err != nil {
		t.Fatalf("InitSQLiteSchema() error = %v", err)
	}

	var version int
	if err := db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("query schema version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("schema version = %d, want %d", version, schemaVersion)
	}

	for _, table := range []string{"attempts", "attempt_variables"} {
		var name string
		err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("query table %s: %v", table, err)
		}
		if name != table {
			t.Fatalf("table name = %q, want %q", name, table)
		}
	}
}

func TestInsertAttemptStoresAttemptAndVariables(t *testing.T) {
	ctx := context.Background()
	db := testLedgerDB(t, ctx)
	defer db.Close()

	startedAt := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	attempt := Attempt{
		ID:                  "attempt-001",
		WorkflowInstanceID:  "workflow-instance-001",
		StepInstanceID:      "step-instance-001",
		WorkItemID:          "work-item-001",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
		Status:              AttemptStatusCompleted,
		StartedAt:           startedAt,
		CompletedAt:         completedAt,
		Variables: []AttemptVariable{
			{
				Namespace: "runtime",
				Name:      "work_item_id",
				Type:      "string",
				Value:     "work-item-001",
				Source:    "controller",
				Lifecycle: "work_item",
			},
		},
	}

	if err := InsertAttempt(ctx, db, attempt); err != nil {
		t.Fatalf("InsertAttempt() error = %v", err)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM attempts WHERE attempt_id = ?`, attempt.ID).Scan(&status); err != nil {
		t.Fatalf("query attempt: %v", err)
	}
	if status != string(AttemptStatusCompleted) {
		t.Fatalf("status = %q, want %q", status, AttemptStatusCompleted)
	}

	var valueJSON string
	if err := db.QueryRowContext(ctx, `SELECT value_json FROM attempt_variables WHERE attempt_id = ? AND namespace = ? AND name = ?`,
		attempt.ID, "runtime", "work_item_id").Scan(&valueJSON); err != nil {
		t.Fatalf("query attempt variable: %v", err)
	}
	if valueJSON != `"work-item-001"` {
		t.Fatalf("value_json = %q, want %q", valueJSON, `"work-item-001"`)
	}
}

func TestInsertAttemptRejectsInvalidAttempt(t *testing.T) {
	ctx := context.Background()
	db := testLedgerDB(t, ctx)
	defer db.Close()

	if err := InsertAttempt(ctx, db, Attempt{}); err == nil {
		t.Fatal("expected an error")
	}
}

func testLedgerDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()

	db, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}

	if err := InitSQLiteSchema(ctx, db); err != nil {
		db.Close()
		t.Fatalf("InitSQLiteSchema() error = %v", err)
	}

	return db
}

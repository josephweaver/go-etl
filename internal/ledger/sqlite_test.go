package ledger

import (
	"context"
	"database/sql"
	"path/filepath"
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

func TestOpenSQLiteCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "ledger.sqlite")

	db, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("ping sqlite: %v", err)
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

func TestInsertAttemptStoresSkippedAttempt(t *testing.T) {
	ctx := context.Background()
	db := testLedgerDB(t, ctx)
	defer db.Close()

	attempt := testAttempt(
		"attempt-skip-001",
		"work-item-fingerprint",
		AttemptStatusSkipped,
		time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
	)
	attempt.Variables = []AttemptVariable{
		{
			Namespace: "runtime",
			Name:      "prior_attempt_id",
			Type:      "string",
			Value:     "attempt-001",
			Source:    "controller",
			Lifecycle: "attempt",
		},
		{
			Namespace: "runtime",
			Name:      "skip_reason",
			Type:      "string",
			Value:     "matched_prior_completed_attempt",
			Source:    "controller",
			Lifecycle: "attempt",
		},
	}

	if err := InsertAttempt(ctx, db, attempt); err != nil {
		t.Fatalf("InsertAttempt() error = %v", err)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM attempts WHERE attempt_id = ?`, attempt.ID).Scan(&status); err != nil {
		t.Fatalf("query skipped attempt: %v", err)
	}
	if status != string(AttemptStatusSkipped) {
		t.Fatalf("status = %q, want %q", status, AttemptStatusSkipped)
	}

	var valueJSON string
	if err := db.QueryRowContext(ctx, `SELECT value_json FROM attempt_variables WHERE attempt_id = ? AND namespace = ? AND name = ?`,
		attempt.ID, "runtime", "prior_attempt_id").Scan(&valueJSON); err != nil {
		t.Fatalf("query prior attempt variable: %v", err)
	}
	if valueJSON != `"attempt-001"` {
		t.Fatalf("prior_attempt_id value_json = %q, want %q", valueJSON, `"attempt-001"`)
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

func TestFindLatestCompletedAttemptByWorkItemFingerprint(t *testing.T) {
	ctx := context.Background()
	db := testLedgerDB(t, ctx)
	defer db.Close()

	startedAt := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	attempts := []Attempt{
		testAttempt("attempt-old", "work-item-fingerprint", AttemptStatusCompleted, startedAt, startedAt.Add(time.Minute)),
		testAttempt("attempt-failed", "work-item-fingerprint", AttemptStatusFailed, startedAt.Add(time.Minute), startedAt.Add(2*time.Minute)),
		testAttempt("attempt-new", "work-item-fingerprint", AttemptStatusCompleted, startedAt.Add(2*time.Minute), startedAt.Add(3*time.Minute)),
		testAttempt("attempt-other", "other-fingerprint", AttemptStatusCompleted, startedAt.Add(3*time.Minute), startedAt.Add(4*time.Minute)),
	}
	for _, attempt := range attempts {
		if err := InsertAttempt(ctx, db, attempt); err != nil {
			t.Fatalf("InsertAttempt(%s) error = %v", attempt.ID, err)
		}
	}

	attempt, ok, err := FindLatestCompletedAttemptByWorkItemFingerprint(ctx, db, "work-item-fingerprint")
	if err != nil {
		t.Fatalf("FindLatestCompletedAttemptByWorkItemFingerprint() error = %v", err)
	}
	if !ok {
		t.Fatal("expected a matching attempt")
	}
	if attempt.ID != "attempt-new" {
		t.Fatalf("attempt id = %q, want attempt-new", attempt.ID)
	}
	if attempt.Status != AttemptStatusCompleted {
		t.Fatalf("status = %q, want completed", attempt.Status)
	}
}

func TestFindLatestCompletedAttemptByWorkItemFingerprintReturnsMissing(t *testing.T) {
	ctx := context.Background()
	db := testLedgerDB(t, ctx)
	defer db.Close()

	attempt, ok, err := FindLatestCompletedAttemptByWorkItemFingerprint(ctx, db, "missing")
	if err != nil {
		t.Fatalf("FindLatestCompletedAttemptByWorkItemFingerprint() error = %v", err)
	}
	if ok {
		t.Fatalf("unexpected attempt: %+v", attempt)
	}
}

func TestFindLatestCompletedAttemptByWorkItemFingerprintRejectsMissingFingerprint(t *testing.T) {
	ctx := context.Background()
	db := testLedgerDB(t, ctx)
	defer db.Close()

	if _, _, err := FindLatestCompletedAttemptByWorkItemFingerprint(ctx, db, ""); err == nil {
		t.Fatal("expected an error")
	}
}

func testAttempt(id string, fingerprint string, status AttemptStatus, startedAt time.Time, completedAt time.Time) Attempt {
	return Attempt{
		ID:                  id,
		WorkflowInstanceID:  "workflow-instance-001",
		StepInstanceID:      "step-instance-001",
		WorkItemID:          "work-item-001",
		WorkItemFingerprint: fingerprint,
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
		Status:              status,
		StartedAt:           startedAt,
		CompletedAt:         completedAt,
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

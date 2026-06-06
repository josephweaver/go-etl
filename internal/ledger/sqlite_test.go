package ledger

import (
	"context"
	"testing"
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

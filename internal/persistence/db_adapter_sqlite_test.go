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

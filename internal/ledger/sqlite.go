package ledger

import (
	"context"
	"database/sql"
	"fmt"

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

func OpenSQLite(path string) (*sql.DB, error) {
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

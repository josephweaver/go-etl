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
		for _, statement := range sqliteSchemaStatements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("initialize sqlite schema: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, SupportedSchemaVersion); err != nil {
			return fmt.Errorf("record sqlite schema version: %w", err)
		}
	} else if err := validateSQLiteStoreSchemaVersion(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite schema initialization: %w", err)
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

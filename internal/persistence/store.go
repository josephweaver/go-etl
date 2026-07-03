package persistence

import (
	"context"
	"database/sql"
	"fmt"
)

const (
	DriverSQLite           = "sqlite"
	SupportedSchemaVersion = 1
)

type Config struct {
	Driver           string
	ConnectionString string
}

type Store struct {
	db *sql.DB
}

func OpenStore(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Driver == "" {
		return nil, fmt.Errorf("database driver is required")
	}
	if cfg.ConnectionString == "" {
		return nil, fmt.Errorf("database connection string is required")
	}

	switch cfg.Driver {
	case DriverSQLite:
		return openSQLiteStore(ctx, cfg.ConnectionString)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) CurrentSchemaVersion(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not open")
	}

	var version int
	if err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

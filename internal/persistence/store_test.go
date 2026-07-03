package persistence

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenStoreRejectsInvalidConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "missing driver", cfg: Config{ConnectionString: ":memory:"}, want: "database driver is required"},
		{name: "missing connection string", cfg: Config{Driver: DriverSQLite}, want: "database connection string is required"},
		{name: "unsupported driver", cfg: Config{Driver: "postgres", ConnectionString: "dsn"}, want: "unsupported database driver"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := OpenStore(ctx, tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("OpenStore() error = %v, want %q", err, tt.want)
			}
			if store != nil {
				t.Fatalf("OpenStore() store = %#v, want nil", store)
			}
		})
	}
}

func TestStoreCurrentSchemaVersion(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	version, err := store.CurrentSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion() error = %v", err)
	}
	if version != SupportedSchemaVersion {
		t.Fatalf("version = %d, want %d", version, SupportedSchemaVersion)
	}
}

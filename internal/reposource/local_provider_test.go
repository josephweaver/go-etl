package reposource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalProviderReadsOnlyRequestedValidatedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "project.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.py"), []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	provider := NewLocalProvider(RepositoryIdentity{Value: "local:demo", DisplayName: "Demo"}, root)
	resolved, err := provider.Resolve(context.Background(), "working-tree")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.RevisionID != nil {
		t.Fatalf("RevisionID = %q, want nil", *resolved.RevisionID)
	}

	reads, err := provider.ReadFiles(context.Background(), resolved, []string{"project.json", "scripts/run.py"})
	if err != nil {
		t.Fatalf("ReadFiles() error = %v", err)
	}
	if len(reads) != 2 {
		t.Fatalf("len(reads) = %d, want 2", len(reads))
	}
	if got := string(reads[0].Content.Data); got != `{"name":"demo"}` {
		t.Fatalf("project data = %q", got)
	}
	if reads[0].Content.ObjectID != nil {
		t.Fatal("local object id is not nil")
	}
	if reads[0].RawSHA256 == "" {
		t.Fatal("raw sha256 is empty")
	}
	if reads[0].Request.SourcePath != "project.json" {
		t.Fatalf("source path = %q, want project.json", reads[0].Request.SourcePath)
	}
	if got := provider.ProvenanceWarning(); got != LocalProvenanceWarning {
		t.Fatalf("ProvenanceWarning() = %q", got)
	}
}

func TestLocalProviderRejectsUnsafePathBeforeRead(t *testing.T) {
	provider := NewLocalProvider(RepositoryIdentity{Value: "local:demo"}, t.TempDir())
	resolved, err := provider.Resolve(context.Background(), "working-tree")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if _, err := provider.ReadFiles(context.Background(), resolved, []string{"../secret.json"}); err == nil {
		t.Fatal("expected unsafe path error")
	}
}

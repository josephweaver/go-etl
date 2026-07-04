package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalSourceControlAdapterResolvesDemoProject(t *testing.T) {
	root := filepath.Join("..", "..", "..", "go-etl-demo-project")
	adapter := NewLocalSourceControlAdapter(map[string]string{
		"local:demo": root,
	})

	resolved, err := adapter.Resolve(context.Background(), SourceDocumentReference{
		Repository: "local:demo",
		Ref:        "main",
		Path:       "project.json",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resolved.RepositoryIdentity != "local:demo" {
		t.Fatalf("repository identity = %q, want local:demo", resolved.RepositoryIdentity)
	}
	if resolved.RequestedRef != "main" {
		t.Fatalf("requested ref = %q, want main", resolved.RequestedRef)
	}
	if resolved.ResolvedCommit == "" {
		t.Fatal("resolved commit is empty")
	}
	if resolved.Path != "project.json" {
		t.Fatalf("path = %q, want project.json", resolved.Path)
	}
	if resolved.SourceObjectID == "" {
		t.Fatal("source object id is empty")
	}
	if len(resolved.Data) == 0 {
		t.Fatal("data is empty")
	}
}

func TestLocalSourceControlAdapterRejectsUnknownRepository(t *testing.T) {
	adapter := NewLocalSourceControlAdapter(map[string]string{})

	_, err := adapter.Resolve(context.Background(), SourceDocumentReference{
		Repository: "local:missing",
		Ref:        "main",
		Path:       "project.json",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestLocalSourceControlAdapterRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "project.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewLocalSourceControlAdapter(map[string]string{"local:test": root})

	tests := []string{
		"",
		"../project.json",
		filepath.Join(root, "project.json"),
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			_, err := adapter.Resolve(context.Background(), SourceDocumentReference{
				Repository: "local:test",
				Ref:        "main",
				Path:       path,
			})
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLocalSourceControlAdapterUsesUnversionedIdentityOutsideGit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "project.json"), []byte(`{"id":"demo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewLocalSourceControlAdapter(map[string]string{"local:test": root})

	resolved, err := adapter.Resolve(context.Background(), SourceDocumentReference{
		Repository: "local:test",
		Ref:        "main",
		Path:       "project.json",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if resolved.ResolvedCommit != localUnversionedCommit {
		t.Fatalf("resolved commit = %q, want %q", resolved.ResolvedCommit, localUnversionedCommit)
	}
	if resolved.SourceObjectID == "" {
		t.Fatal("source object id is empty")
	}
}

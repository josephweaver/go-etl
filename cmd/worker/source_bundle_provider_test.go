package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestFileSourceBundleProviderReadsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source-bundle.zip")
	want := []byte("zip fixture bytes")
	if err := os.WriteFile(path, want, 0644); err != nil {
		t.Fatal(err)
	}

	provider := FileSourceBundleProvider{Path: path}
	got, err := provider.SourceBundle(model.WorkItem{})
	if err != nil {
		t.Fatalf("SourceBundle() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("SourceBundle() = %q, want %q", got, want)
	}
}

func TestFileSourceBundleProviderRejectsMissingPath(t *testing.T) {
	provider := FileSourceBundleProvider{}
	if _, err := provider.SourceBundle(model.WorkItem{}); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("SourceBundle() error = %v, want missing path", err)
	}
}

func TestFileSourceBundleProviderRejectsDirectory(t *testing.T) {
	provider := FileSourceBundleProvider{Path: t.TempDir()}
	if _, err := provider.SourceBundle(model.WorkItem{}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("SourceBundle() error = %v, want regular-file error", err)
	}
}

func TestControllerSourceBundleProviderUsesRunID(t *testing.T) {
	want := []byte("controller zip bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/workflow-runs/run-123/source-bundle.zip" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write(want)
	}))
	defer server.Close()

	controller, err := NewWorkerControllerClient(Config{ControllerURL: server.URL})
	if err != nil {
		t.Fatalf("NewWorkerControllerClient() error = %v", err)
	}
	provider := ControllerSourceBundleProvider{Controller: controller}
	got, err := provider.SourceBundle(model.WorkItem{
		Source: &model.WorkItemSource{RunID: "run-123"},
	})
	if err != nil {
		t.Fatalf("SourceBundle() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("SourceBundle() = %q, want %q", got, want)
	}
}

func TestControllerSourceBundleProviderRequiresSource(t *testing.T) {
	tests := []struct {
		name string
		item model.WorkItem
		want string
	}{
		{name: "missing source", item: model.WorkItem{}, want: "source is required"},
		{name: "missing run id", item: model.WorkItem{Source: &model.WorkItemSource{}}, want: "run id is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := ControllerSourceBundleProvider{}
			if _, err := provider.SourceBundle(test.item); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("SourceBundle() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestControllerSourceBundleProviderRequiresInitializedClient(t *testing.T) {
	provider := ControllerSourceBundleProvider{}
	_, err := provider.SourceBundle(model.WorkItem{Source: &model.WorkItemSource{RunID: "run-123"}})
	if err == nil || !strings.Contains(err.Error(), "initialized controller client") {
		t.Fatalf("SourceBundle() error = %v, want initialized-client error", err)
	}
}

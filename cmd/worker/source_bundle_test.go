package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestStageWorkItemSourceBundleSuccess(t *testing.T) {
	zipData := mustZipData(t,
		zipTestEntry{name: "nested/dir/file.txt", body: "hello world\n"},
		zipTestEntry{name: ".goet/source-manifest.json", body: "{}\n"},
	)

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		requestedPath = r.URL.Path
		if r.URL.Path != "/workflow-runs/run-123/source-bundle.zip" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipData)
	}))
	defer server.Close()

	worker := newSourceBundleTestWorker(t, server.URL)
	item := baseSourceBundleTestItem()

	staging, err := worker.stageWorkItemSourceBundle(item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestedPath != "/workflow-runs/run-123/source-bundle.zip" {
		t.Fatalf("unexpected request path: %s", requestedPath)
	}

	for _, dir := range []string{staging.AttemptDir, staging.SourceDir, staging.WorkDir, staging.LogDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected directory %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
	}

	assertFileContents(t, filepath.Join(staging.SourceDir, "nested", "dir", "file.txt"), "hello world\n")
	assertFileContents(t, filepath.Join(staging.SourceDir, ".goet", "source-manifest.json"), "{}\n")
}

func TestStageWorkItemSourceBundleUsesConfiguredProvider(t *testing.T) {
	zipData := mustZipData(t, zipTestEntry{name: "main.py", body: "print('direct')\n"})
	provider := &recordingSourceBundleProvider{body: zipData}
	worker := newSourceBundleTestWorker(t, "http://controller.invalid")
	worker.SourceBundles = provider
	item := baseSourceBundleTestItem()

	staging, err := worker.stageWorkItemSourceBundle(item)
	if err != nil {
		t.Fatalf("stageWorkItemSourceBundle() error = %v", err)
	}
	if provider.calls != 1 || provider.item.ID != item.ID {
		t.Fatalf("provider calls = %d, item = %+v", provider.calls, provider.item)
	}
	assertFileContents(t, filepath.Join(staging.SourceDir, "main.py"), "print('direct')\n")
}

func TestStageWorkItemSourceBundleRequiresProviderForLocalOnlyWorker(t *testing.T) {
	worker := newSourceBundleTestWorker(t, "http://controller.invalid")
	worker.LocalOnly = true

	_, err := worker.stageWorkItemSourceBundle(baseSourceBundleTestItem())
	if err == nil || !strings.Contains(err.Error(), "provider is required for local-only") {
		t.Fatalf("stageWorkItemSourceBundle() error = %v, want provider-required error", err)
	}
}

func TestStageWorkItemSourceBundleUsesControllerClientAuth(t *testing.T) {
	const sentinel = "goetl-worker-controller-token-sentinel-006"
	zipData := mustZipData(t, zipTestEntry{name: "main.py", body: "print('ok')\n"})
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") == "Bearer "+sentinel
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipData)
	}))
	defer server.Close()

	root := t.TempDir()
	tokenFile := filepath.Join(root, "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	cfg := Config{
		LogDir:              filepath.Join(root, "logs"),
		TmpDir:              filepath.Join(root, "tmp"),
		DataDir:             filepath.Join(root, "data"),
		ControllerURL:       server.URL,
		ControllerTokenFile: tokenFile,
	}
	for _, dir := range []string{cfg.LogDir, cfg.TmpDir, cfg.DataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create directory %s: %v", dir, err)
		}
	}
	controller, err := NewWorkerControllerClient(cfg)
	if err != nil {
		t.Fatalf("NewWorkerControllerClient() error = %v", err)
	}

	worker := Worker{Config: cfg, Controller: controller}
	if _, err := worker.stageWorkItemSourceBundle(baseSourceBundleTestItem()); err != nil {
		t.Fatalf("stage source bundle: %v", err)
	}
	if !sawAuth {
		t.Fatal("expected source bundle request to include bearer token")
	}
}

func TestStageWorkItemSourceBundleRejectsMissingInputs(t *testing.T) {
	tests := []struct {
		name string
		item model.WorkItem
	}{
		{
			name: "missing source",
			item: model.WorkItem{
				ID:             "work-1",
				AttemptID:      "attempt-abc",
				Type:           model.WorkItemTypePythonScript,
				OutputFilename: "result.txt",
			},
		},
		{
			name: "missing run id",
			item: model.WorkItem{
				ID:             "work-1",
				AttemptID:      "attempt-abc",
				Type:           model.WorkItemTypePythonScript,
				OutputFilename: "result.txt",
				Source:         &model.WorkItemSource{ManifestPath: "manifest.json"},
			},
		},
		{
			name: "missing attempt id",
			item: model.WorkItem{
				ID:             "work-1",
				Type:           model.WorkItemTypePythonScript,
				OutputFilename: "result.txt",
				Source:         &model.WorkItemSource{RunID: "run-123", ManifestPath: "manifest.json"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worker := newSourceBundleTestWorker(t, "http://example.invalid")
			_, err := worker.stageWorkItemSourceBundle(test.item)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestStageWorkItemSourceBundleRejectsControllerErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    string
	}{
		{
			name:       "non-2xx",
			statusCode: http.StatusServiceUnavailable,
			body:       "nope",
			wantErr:    "unexpected status",
		},
		{
			name:       "invalid zip",
			statusCode: http.StatusOK,
			body:       "not a zip file",
			wantErr:    "decode source bundle",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()

			worker := newSourceBundleTestWorker(t, server.URL)
			_, err := worker.stageWorkItemSourceBundle(baseSourceBundleTestItem())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

func TestStageWorkItemSourceBundleRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name      string
		entries   []zipTestEntry
		wantError string
	}{
		{
			name:      "absolute path",
			entries:   []zipTestEntry{{name: "/abs.txt", body: "x"}},
			wantError: "absolute",
		},
		{
			name:      ".. traversal",
			entries:   []zipTestEntry{{name: "../escape.txt", body: "x"}},
			wantError: ".. traversal",
		},
		{
			name:      "backslash",
			entries:   []zipTestEntry{{name: `dir\file.txt`, body: "x"}},
			wantError: "backslashes",
		},
		{
			name:      "drive path",
			entries:   []zipTestEntry{{name: "C:drive.txt", body: "x"}},
			wantError: "drive-qualified",
		},
		{
			name: "duplicate entry",
			entries: []zipTestEntry{
				{name: "dir/", isDir: true},
				{name: "dir/", isDir: true},
			},
			wantError: "duplicate normalized path",
		},
		{
			name:      "symlink-like",
			entries:   []zipTestEntry{{name: "link", body: "target", mode: os.ModeSymlink | 0777}},
			wantError: "symlink-like",
		},
		{
			name: "source root escape via ancestor file",
			entries: []zipTestEntry{
				{name: "bundle.txt", body: "file"},
				{name: "bundle.txt/nested.txt", body: "nested"},
			},
			wantError: "parent path",
		},
		{
			name: "directory/file collision",
			entries: []zipTestEntry{
				{name: "dir/", isDir: true},
				{name: "dir", body: "file"},
			},
			wantError: "duplicate normalized path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(mustZipData(t, test.entries...))
			}))
			defer server.Close()

			worker := newSourceBundleTestWorker(t, server.URL)
			_, err := worker.stageWorkItemSourceBundle(baseSourceBundleTestItem())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("expected error containing %q, got %v", test.wantError, err)
			}
		})
	}
}

func baseSourceBundleTestItem() model.WorkItem {
	return model.WorkItem{
		ID:             "work-1",
		AttemptID:      "attempt-abc",
		Type:           model.WorkItemTypePythonScript,
		Source:         &model.WorkItemSource{RunID: "run-123", ManifestPath: "manifest.json"},
		OutputFilename: "result.txt",
	}
}

func newSourceBundleTestWorker(t *testing.T, controllerURL string) Worker {
	t.Helper()

	root := t.TempDir()
	cfg := Config{
		LogDir:        filepath.Join(root, "logs"),
		TmpDir:        filepath.Join(root, "tmp"),
		DataDir:       filepath.Join(root, "data"),
		ControllerURL: controllerURL,
	}
	for _, dir := range []string{cfg.LogDir, cfg.TmpDir, cfg.DataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create directory %s: %v", dir, err)
		}
	}
	return Worker{Config: cfg}
}

type zipTestEntry struct {
	name  string
	body  string
	mode  os.FileMode
	isDir bool
}

type recordingSourceBundleProvider struct {
	body  []byte
	err   error
	item  model.WorkItem
	calls int
}

func (p *recordingSourceBundleProvider) SourceBundle(item model.WorkItem) ([]byte, error) {
	p.calls++
	p.item = item
	return p.body, p.err
}

func mustZipData(t *testing.T, entries ...zipTestEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		} else if entry.isDir {
			header.SetMode(os.ModeDir | 0755)
		} else {
			header.SetMode(0644)
		}

		writer, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", entry.name, err)
		}
		if entry.isDir {
			continue
		}
		if _, err := io.WriteString(writer, entry.body); err != nil {
			t.Fatalf("write zip entry %s: %v", entry.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

func assertFileContents(t *testing.T, path string, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("unexpected contents for %s: got %q want %q", path, string(data), want)
	}
}

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/persistence"
	"goetl/internal/reposource"
)

func TestSourceBundleHandlerReturnsZipOfAdmittedPythonFiles(t *testing.T) {
	controller, runID, submissionContext := setupSourceBundleRun(t)

	response := serveSourceBundleRequest(t, controller, runID)
	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type = %q, want application/zip", got)
	}

	entries := unzipSourceBundleEntries(t, response.Body.Bytes())
	if len(entries) != 4 {
		t.Fatalf("zip entry count = %d, want 4", len(entries))
	}
	if _, ok := entries["project.json"]; ok {
		t.Fatal("zip unexpectedly contains project.json")
	}
	if _, ok := entries["workflows/demo-workflow.json"]; ok {
		t.Fatal("zip unexpectedly contains workflow source")
	}
	if got := entries["scripts/train.py"]; got != "print('train')\n" {
		t.Fatalf("scripts/train.py = %q, want python entrypoint", got)
	}
	if got := entries["requirements.txt"]; got != "numpy==2.0.0\n" {
		t.Fatalf("requirements.txt = %q, want environment file", got)
	}
	if got := entries["scripts/lib/helpers.py"]; got != "def helper():\n    return 'ok'\n" {
		t.Fatalf("scripts/lib/helpers.py = %q, want support file", got)
	}

	manifestJSON := entries[sourceBundleManifestZipPath]
	if strings.Contains(manifestJSON, submissionContext.SourceAdmission.ManifestRef) {
		t.Fatalf("metadata unexpectedly contains manifest filesystem path %q", submissionContext.SourceAdmission.ManifestRef)
	}
	if strings.Contains(manifestJSON, "cache_path") || strings.Contains(manifestJSON, "CachePath") {
		t.Fatalf("metadata unexpectedly contains cache path fields: %s", manifestJSON)
	}
	var manifest bundleManifestFixture
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		t.Fatalf("decode source bundle manifest: %v", err)
	}
	if manifest.Schema == "" || manifest.RunID != runID {
		t.Fatalf("manifest header = %+v, want schema and run id", manifest)
	}
	if len(manifest.Files) != 3 {
		t.Fatalf("manifest file count = %d, want 3", len(manifest.Files))
	}
	for _, file := range manifest.Files {
		if file.SourcePath == "" || file.Role == "" {
			t.Fatalf("manifest file missing safe metadata: %+v", file)
		}
	}
}

func TestSourceBundleHandlerReturnsNotFoundForMissingRun(t *testing.T) {
	controller := newController()
	controller.workflowStore = openTestWorkflowExecutionStore(t)
	defer controller.workflowStore.Close()

	response := serveSourceBundleRequest(t, controller, "run-missing")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "workflow run not found") {
		t.Fatalf("body = %q, want missing run error", body)
	}
}

func TestSourceBundleHandlerRejectsRunWithoutSourceAdmissionContext(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()

	runID := createWorkflowRunWithoutSourceAdmissionContext(t, store)
	controller := newController()
	controller.workflowStore = store

	response := serveSourceBundleRequest(t, controller, runID)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want 500", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "missing source-admission context") {
		t.Fatalf("body = %q, want missing source-admission context error", body)
	}
}

func TestSourceBundleHandlerRejectsUnsafeAdmittedPath(t *testing.T) {
	controller, runID, submissionContext := setupSourceBundleRun(t)

	manifest := readSourceBundleManifest(t, submissionContext)
	for index := range manifest.Files {
		if manifest.Files[index].Role == reposource.FileRoleSupportFile {
			manifest.Files[index].SourcePath = "scripts/./lib/helpers.py"
			break
		}
	}
	writeSourceBundleManifest(t, submissionContext, manifest)

	response := serveSourceBundleRequest(t, controller, runID)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want 500", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "unsafe admitted source path") {
		t.Fatalf("body = %q, want unsafe path error", body)
	}
}

func TestSourceBundleHandlerReportsMissingCachedFile(t *testing.T) {
	controller, runID, submissionContext := setupSourceBundleRun(t)
	manifest := readSourceBundleManifest(t, submissionContext)
	removeCachedManifestFileByRole(t, controller, manifest, reposource.FileRoleSupportFile)

	response := serveSourceBundleRequest(t, controller, runID)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want 500", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "cached admitted source file is missing") {
		t.Fatalf("body = %q, want cache miss error", body)
	}
}

func TestSourceBundleHandlerReportsCorruptedCachedFile(t *testing.T) {
	controller, runID, submissionContext := setupSourceBundleRun(t)
	manifest := readSourceBundleManifest(t, submissionContext)
	corruptCachedManifestFileByRole(t, controller, manifest, reposource.FileRolePythonEntrypoint)

	response := serveSourceBundleRequest(t, controller, runID)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want 500", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "cached admitted source file is corrupted") {
		t.Fatalf("body = %q, want cache corruption error", body)
	}
}

func setupSourceBundleRun(t *testing.T) (*Controller, string, workflowRunSubmissionContext) {
	t.Helper()

	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		store.Close()
	})
	controller := newController()
	controller.workflowStore = store

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "project.json"), []byte(`{"id":"python-demo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "workflows"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts", "lib"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "train.py"), []byte("print('train')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "requirements.txt"), []byte("numpy==2.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "lib", "helpers.py"), []byte("def helper():\n    return 'ok'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configureTestLocalRepoSource(t, controller, "local:test", root)

	workflowJSON := `{
		"workflow": {
			"ID": "python-demo",
			"Steps": []
		},
		"source_manifest": {
			"files": [
				{"role": "python_entrypoint", "path": "scripts/train.py", "content_type": "text/x-python"},
				{"role": "python_environment", "path": "requirements.txt", "content_type": "text/plain"},
				{"role": "support_file", "path": "scripts/lib/helpers.py", "content_type": "text/x-python"}
			]
		},
		"variables": []
	}`
	if err := os.WriteFile(filepath.Join(root, "workflows", "demo-workflow.json"), []byte(workflowJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"project": {
			"repository": "local:test",
			"ref": "main",
			"path": "project.json"
		},
		"workflow": {
			"repository": "local:test",
			"ref": "main",
			"path": "workflows/demo-workflow.json"
		}
	}`))
	response := httptest.NewRecorder()
	controller.submitWorkflowHandler(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("submit workflow status code = %d, want 204", response.Code)
	}

	runs, err := store.ListActiveWorkflowRuns(context.Background())
	if err != nil {
		t.Fatalf("ListActiveWorkflowRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("active run count = %d, want 1", len(runs))
	}

	var submissionContext workflowRunSubmissionContext
	if err := json.Unmarshal([]byte(runs[0].SubmissionContextJSON), &submissionContext); err != nil {
		t.Fatalf("decode submission context: %v", err)
	}
	return controller, runs[0].ID, submissionContext
}

func serveSourceBundleRequest(t *testing.T, controller *Controller, runID string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	registerControllerRoutes(mux, controller)
	request := httptest.NewRequest(http.MethodGet, sourceBundleRoutePrefix+runID+sourceBundleRouteSuffix, nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func unzipSourceBundleEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	entries := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		entries[file.Name] = string(body)
	}
	return entries
}

func readSourceBundleManifest(t *testing.T, submissionContext workflowRunSubmissionContext) reposource.AdmittedSourceManifest {
	t.Helper()

	manifestData, err := os.ReadFile(filepath.FromSlash(submissionContext.SourceAdmission.ManifestRef))
	if err != nil {
		t.Fatalf("read admitted manifest: %v", err)
	}
	var manifest reposource.AdmittedSourceManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode admitted manifest: %v", err)
	}
	return manifest
}

func writeSourceBundleManifest(t *testing.T, submissionContext workflowRunSubmissionContext, manifest reposource.AdmittedSourceManifest) {
	t.Helper()

	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal admitted manifest: %v", err)
	}
	if err := os.WriteFile(filepath.FromSlash(submissionContext.SourceAdmission.ManifestRef), manifestData, 0o600); err != nil {
		t.Fatalf("write admitted manifest: %v", err)
	}
}

func removeCachedManifestFileByRole(t *testing.T, controller *Controller, manifest reposource.AdmittedSourceManifest, role reposource.FileRole) {
	t.Helper()

	cachePath := cachedManifestFilePathByRole(t, controller, manifest, role)
	if err := os.Remove(cachePath); err != nil {
		t.Fatalf("remove cached file %s: %v", cachePath, err)
	}
}

func corruptCachedManifestFileByRole(t *testing.T, controller *Controller, manifest reposource.AdmittedSourceManifest, role reposource.FileRole) {
	t.Helper()

	cachePath := cachedManifestFilePathByRole(t, controller, manifest, role)
	if err := os.WriteFile(cachePath, []byte("corrupted\n"), 0o600); err != nil {
		t.Fatalf("corrupt cached file %s: %v", cachePath, err)
	}
}

func cachedManifestFilePathByRole(t *testing.T, controller *Controller, manifest reposource.AdmittedSourceManifest, role reposource.FileRole) string {
	t.Helper()

	access, err := reposource.NewCacheAccess(controller.repoCacheLayout, manifest)
	if err != nil {
		t.Fatalf("NewCacheAccess() error = %v", err)
	}
	for _, file := range manifest.Files {
		if file.Role != role {
			continue
		}
		path, err := access.FilePathForManifestFile(file)
		if err != nil {
			t.Fatalf("FilePathForManifestFile(%s) error = %v", role, err)
		}
		return path
	}
	t.Fatalf("manifest missing role %s", role)
	return ""
}

func createWorkflowRunWithoutSourceAdmissionContext(t *testing.T, store *persistence.Store) string {
	t.Helper()

	ctx := context.Background()
	project := persistence.ProjectRecord{
		ID:                 "project-source-bundle",
		Name:               "Project",
		RepositoryIdentity: "repo",
		ConfigPath:         "project.json",
		ConfigSHA256:       strings.Repeat("a", 64),
		CreatedAt:          "2026-07-04T00:00:00Z",
	}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	workflow := persistence.WorkflowRecord{
		ID:                 "workflow-source-bundle",
		ProjectID:          project.ID,
		Name:               "Workflow",
		RepositoryIdentity: "repo",
		WorkflowPath:       "workflow.json",
		WorkflowSHA256:     strings.Repeat("b", 64),
		CreatedAt:          "2026-07-04T00:00:00Z",
	}
	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}
	run := persistence.WorkflowRunRecord{
		ID:                    "run-without-source-context",
		ProjectID:             project.ID,
		WorkflowID:            workflow.ID,
		SubmissionContextJSON: `{"variables":[]}`,
		CreatedAt:             "2026-07-04T00:00:00Z",
	}
	if err := store.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	return run.ID
}

type bundleManifestFixture struct {
	Schema string `json:"schema"`
	RunID  string `json:"run_id"`
	Files  []struct {
		Role                string  `json:"role"`
		SourcePath          string  `json:"source_path"`
		ContentType         string  `json:"content_type,omitempty"`
		SizeBytes           int64   `json:"size_bytes"`
		RawSHA256           *string `json:"raw_sha256,omitempty"`
		CanonicalJSONSHA256 *string `json:"canonical_json_sha256,omitempty"`
	} `json:"files"`
}

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	fptest "goetl/internal/fingerprint"
	"goetl/internal/persistence"
	"goetl/internal/reposource"
)

type reloadSourceFixture struct {
	controller *Controller
	store      *persistence.Store
	manifest   reposource.AdmittedSourceManifest
	provider   *reloadFakeProvider
}

type reloadFakeProvider struct {
	repository reposource.RepositoryIdentity
	revisionID *string
	files      map[string][]byte
	readCalls  int
	readErr    error
}

func (p *reloadFakeProvider) Resolve(ctx context.Context, requestedRef string) (reposource.ResolvedSourceReference, error) {
	return reposource.ResolvedSourceReference{
		Repository:   p.repository,
		RequestedRef: requestedRef,
		RevisionID:   p.revisionID,
	}, nil
}

func (p *reloadFakeProvider) ReadFiles(ctx context.Context, resolved reposource.ResolvedSourceReference, paths []string) ([]reposource.ReadFileResult, error) {
	p.readCalls++
	if p.readErr != nil {
		return nil, p.readErr
	}
	reads := make([]reposource.ReadFileResult, 0, len(paths))
	for _, path := range paths {
		data, ok := p.files[path]
		if !ok {
			return nil, fmt.Errorf("missing fake source file %s", path)
		}
		objectID := "object-" + strings.ReplaceAll(path, "/", "-")
		reads = append(reads, reposource.ReadFileResult{
			Request: reposource.SourceFileRequest{
				Repository: resolved.Repository,
				RevisionID: resolved.RevisionID,
				SourcePath: path,
			},
			Content: reposource.SourceFileContent{
				Data:     append([]byte(nil), data...),
				ObjectID: &objectID,
			},
			RawSHA256: fptest.SHA256Hex(data),
			SizeBytes: int64(len(data)),
		})
	}
	return reads, nil
}

func TestCompleteStartupRecoveryVerifiesCachedRunSources(t *testing.T) {
	fixture := newReloadSourceFixture(t, "github.com/acme/demo", stringPtr("abc123"))
	fixture.controller.enterRecoveryMode()

	if err := fixture.controller.completeStartupRecovery(context.Background()); err != nil {
		t.Fatalf("completeStartupRecovery() error = %v", err)
	}
	if fixture.provider.readCalls != 0 {
		t.Fatalf("provider read calls = %d, want 0", fixture.provider.readCalls)
	}
	if fixture.controller.recoveryAdmissionClosed() {
		t.Fatal("normal admission should be open")
	}
}

func TestCompleteStartupRecoveryRepairsMissingGitHubCache(t *testing.T) {
	fixture := newReloadSourceFixture(t, "github.com/acme/demo", stringPtr("abc123"))
	removeCachedFile(t, fixture.controller.repoCacheLayout, fixture.manifest, "project.json")
	fixture.controller.enterRecoveryMode()

	if err := fixture.controller.completeStartupRecovery(context.Background()); err != nil {
		t.Fatalf("completeStartupRecovery() error = %v", err)
	}
	if fixture.provider.readCalls != 1 {
		t.Fatalf("provider read calls = %d, want 1", fixture.provider.readCalls)
	}
}

func TestCompleteStartupRecoveryRepairsCorruptedGitHubCache(t *testing.T) {
	fixture := newReloadSourceFixture(t, "github.com/acme/demo", stringPtr("abc123"))
	writeCachedFile(t, fixture.controller.repoCacheLayout, fixture.manifest, "project.json", []byte(`{"id":"corrupt"}`))
	fixture.controller.enterRecoveryMode()

	if err := fixture.controller.completeStartupRecovery(context.Background()); err != nil {
		t.Fatalf("completeStartupRecovery() error = %v", err)
	}
	if fixture.provider.readCalls != 1 {
		t.Fatalf("provider read calls = %d, want 1", fixture.provider.readCalls)
	}
}

func TestCompleteStartupRecoveryReportsGitHubRepairFailure(t *testing.T) {
	fixture := newReloadSourceFixture(t, "github.com/acme/demo", stringPtr("abc123"))
	fixture.provider.readErr = fmt.Errorf("github unavailable")
	removeCachedFile(t, fixture.controller.repoCacheLayout, fixture.manifest, "workflow.json")
	fixture.controller.enterRecoveryMode()

	err := fixture.controller.completeStartupRecovery(context.Background())
	if err == nil || !strings.Contains(err.Error(), "repair") || !strings.Contains(err.Error(), "github unavailable") {
		t.Fatalf("completeStartupRecovery() error = %v, want repair failure", err)
	}
	if !fixture.controller.recoveryAdmissionClosed() {
		t.Fatal("normal admission should remain closed")
	}
}

func TestCompleteStartupRecoveryRejectsMissingLocalCache(t *testing.T) {
	fixture := newReloadSourceFixture(t, "local:demo", nil)
	removeCachedFile(t, fixture.controller.repoCacheLayout, fixture.manifest, "project.json")
	fixture.controller.enterRecoveryMode()

	err := fixture.controller.completeStartupRecovery(context.Background())
	if err == nil || !strings.Contains(err.Error(), "local source cache cannot be repaired") {
		t.Fatalf("completeStartupRecovery() error = %v, want local repair rejection", err)
	}
	if fixture.provider.readCalls != 0 {
		t.Fatalf("provider read calls = %d, want 0", fixture.provider.readCalls)
	}
}

func TestCompleteStartupRecoveryRejectsCorruptedLocalCache(t *testing.T) {
	fixture := newReloadSourceFixture(t, "local:demo", nil)
	writeCachedFile(t, fixture.controller.repoCacheLayout, fixture.manifest, "workflow.json", []byte(`{"workflow":{"ID":"changed","Steps":[]}}`))
	fixture.controller.enterRecoveryMode()

	err := fixture.controller.completeStartupRecovery(context.Background())
	if err == nil || !strings.Contains(err.Error(), "local source cache cannot be repaired") {
		t.Fatalf("completeStartupRecovery() error = %v, want local repair rejection", err)
	}
	if fixture.provider.readCalls != 0 {
		t.Fatalf("provider read calls = %d, want 0", fixture.provider.readCalls)
	}
}

func newReloadSourceFixture(t *testing.T, repository string, revisionID *string) reloadSourceFixture {
	t.Helper()

	store := openTestWorkflowExecutionStore(t)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	controller := newController()
	controller.workflowStore = store
	layout, err := reposource.NewCacheLayout(t.TempDir())
	if err != nil {
		t.Fatalf("NewCacheLayout() error = %v", err)
	}
	controller.repoCacheLayout = layout

	projectData := []byte(`{"id":"demo"}`)
	workflowData := []byte(`{"workflow":{"ID":"reload","Steps":[]},"variables":[]}`)
	_, projectHash, err := canonicalSourceDocument(projectData)
	if err != nil {
		t.Fatal(err)
	}
	_, workflowHash, err := canonicalSourceDocument(workflowData)
	if err != nil {
		t.Fatal(err)
	}

	source := reposource.ResolvedSourceReference{
		Repository:   reposource.RepositoryIdentity{Value: repository},
		RequestedRef: "main",
		RevisionID:   revisionID,
	}
	provider := &reloadFakeProvider{
		repository: source.Repository,
		revisionID: revisionID,
		files: map[string][]byte{
			"project.json":  projectData,
			"workflow.json": workflowData,
		},
	}
	controller.repoSourceProviders = map[string]reposource.Provider{repository: provider}
	reads, err := provider.ReadFiles(context.Background(), source, []string{"project.json", "workflow.json"})
	if err != nil {
		t.Fatal(err)
	}
	provider.readCalls = 0
	declared := []reposource.DeclaredSourceFile{
		{
			Role:                reposource.FileRoleProject,
			SourcePath:          "project.json",
			CachePath:           "project.json",
			ContentType:         "application/json",
			CanonicalJSONSHA256: &projectHash,
		},
		{
			Role:                reposource.FileRoleWorkflow,
			SourcePath:          "workflow.json",
			CachePath:           "workflow.json",
			ContentType:         "application/json",
			CanonicalJSONSHA256: &workflowHash,
		},
	}
	manifest, err := reposource.BuildAdmittedSourceManifest("run-001", source, declared, reads)
	if err != nil {
		t.Fatalf("BuildAdmittedSourceManifest() error = %v", err)
	}
	if err := reposource.PublishAdmittedSource(layout, manifest, reads); err != nil {
		t.Fatalf("PublishAdmittedSource() error = %v", err)
	}

	projectFile, err := manifestFileByRole(manifest, reposource.FileRoleProject)
	if err != nil {
		t.Fatal(err)
	}
	workflowFile, err := manifestFileByRole(manifest, reposource.FileRoleWorkflow)
	if err != nil {
		t.Fatal(err)
	}
	project := projectRecordFromAdmittedSource(source, projectFile, projectHash)
	workflow := workflowRecordFromAdmittedSource(project.ID, source, workflowFile, workflowHash)
	if err := store.UpsertProject(context.Background(), project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.UpsertWorkflow(context.Background(), workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}
	run, err := workflowRunRecordFromAdmittedManifest("run-001", project.ID, workflow.ID, manifest, nil, time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC), layout)
	if err != nil {
		t.Fatalf("workflowRunRecordFromAdmittedManifest() error = %v", err)
	}
	if err := store.CreateWorkflowRun(context.Background(), run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	return reloadSourceFixture{
		controller: controller,
		store:      store,
		manifest:   manifest,
		provider:   provider,
	}
}

func removeCachedFile(t *testing.T, layout reposource.CacheLayout, manifest reposource.AdmittedSourceManifest, cachePath string) {
	t.Helper()
	path, err := layout.FilePath(manifest, cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func writeCachedFile(t *testing.T, layout reposource.CacheLayout, manifest reposource.AdmittedSourceManifest, cachePath string, data []byte) {
	t.Helper()
	path, err := layout.FilePath(manifest, cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

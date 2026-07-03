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

func TestStoreUpsertAndGetProject(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project := testProjectRecord("project-001")

	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("second UpsertProject() error = %v", err)
	}

	got, found, err := store.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !found {
		t.Fatal("GetProject() found = false, want true")
	}
	if got != project {
		t.Fatalf("project = %+v, want %+v", got, project)
	}
}

func TestStoreUpsertProjectRejectsConflict(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project := testProjectRecord("project-001")
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}

	conflict := project
	conflict.ConfigSHA256 = strings.Repeat("f", 64)
	err := store.UpsertProject(ctx, conflict)
	if err == nil || !strings.Contains(err.Error(), "different values") {
		t.Fatalf("UpsertProject(conflict) error = %v, want conflict", err)
	}
}

func TestStoreGetProjectReturnsMissing(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	project, found, err := store.GetProject(ctx, "missing")
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if found {
		t.Fatalf("GetProject() found = true with project %+v, want false", project)
	}
}

func TestStoreDeleteProjectIfUnused(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project := testProjectRecord("project-001")
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}

	deleted, err := store.DeleteProjectIfUnused(ctx, project.ID)
	if err != nil {
		t.Fatalf("DeleteProjectIfUnused() error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteProjectIfUnused() deleted = false, want true")
	}

	_, found, err := store.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if found {
		t.Fatal("project still exists after delete")
	}

	deleted, err = store.DeleteProjectIfUnused(ctx, project.ID)
	if err != nil {
		t.Fatalf("DeleteProjectIfUnused(missing) error = %v", err)
	}
	if deleted {
		t.Fatal("DeleteProjectIfUnused(missing) deleted = true, want false")
	}
}

func TestStoreDeleteProjectIfUnusedKeepsReferencedProject(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project := testProjectRecord("project-001")
	workflow := testWorkflowRecord("workflow-001", project.ID)
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}

	deleted, err := store.DeleteProjectIfUnused(ctx, project.ID)
	if err != nil {
		t.Fatalf("DeleteProjectIfUnused() error = %v", err)
	}
	if deleted {
		t.Fatal("DeleteProjectIfUnused() deleted = true, want false")
	}
}

func TestStoreUpsertAndGetWorkflow(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project := testProjectRecord("project-001")
	workflow := testWorkflowRecord("workflow-001", project.ID)
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}

	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}
	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("second UpsertWorkflow() error = %v", err)
	}

	got, found, err := store.GetWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if !found {
		t.Fatal("GetWorkflow() found = false, want true")
	}
	if got != workflow {
		t.Fatalf("workflow = %+v, want %+v", got, workflow)
	}
}

func TestStoreUpsertWorkflowRejectsConflict(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project := testProjectRecord("project-001")
	workflow := testWorkflowRecord("workflow-001", project.ID)
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}

	conflict := workflow
	conflict.WorkflowSHA256 = strings.Repeat("e", 64)
	err := store.UpsertWorkflow(ctx, conflict)
	if err == nil || !strings.Contains(err.Error(), "different values") {
		t.Fatalf("UpsertWorkflow(conflict) error = %v, want conflict", err)
	}
}

func TestStoreGetWorkflowReturnsMissing(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	workflow, found, err := store.GetWorkflow(ctx, "missing")
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if found {
		t.Fatalf("GetWorkflow() found = true with workflow %+v, want false", workflow)
	}
}

func TestStoreDeleteWorkflowIfUnused(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project := testProjectRecord("project-001")
	workflow := testWorkflowRecord("workflow-001", project.ID)
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}

	deleted, err := store.DeleteWorkflowIfUnused(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("DeleteWorkflowIfUnused() error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteWorkflowIfUnused() deleted = false, want true")
	}

	_, found, err := store.GetWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if found {
		t.Fatal("workflow still exists after delete")
	}
}

func TestStoreDeleteWorkflowIfUnusedKeepsReferencedWorkflow(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project := testProjectRecord("project-001")
	workflow := testWorkflowRecord("workflow-001", project.ID)
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO workflow_instances (
		run_id,
		project_id,
		workflow_id,
		submission_context_json,
		created_at
	) VALUES ('run-001', ?, ?, '[]', '2026-07-03T00:00:00Z')`, project.ID, workflow.ID); err != nil {
		t.Fatalf("insert workflow instance: %v", err)
	}

	deleted, err := store.DeleteWorkflowIfUnused(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("DeleteWorkflowIfUnused() error = %v", err)
	}
	if deleted {
		t.Fatal("DeleteWorkflowIfUnused() deleted = true, want false")
	}
}

func TestStoreUpsertWorkflowRejectsMissingProject(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	err := store.UpsertWorkflow(ctx, testWorkflowRecord("workflow-001", "missing-project"))
	if err == nil || !strings.Contains(err.Error(), "insert workflow") {
		t.Fatalf("UpsertWorkflow() error = %v, want insert failure", err)
	}
}

func testProjectRecord(id string) ProjectRecord {
	return ProjectRecord{
		ID:                 id,
		Name:               "Project",
		RepositoryIdentity: "repo",
		SourceCommit:       "commit",
		ConfigPath:         "project.json",
		SourceObjectID:     "object",
		ConfigSHA256:       strings.Repeat("a", 64),
		CreatedAt:          "2026-07-03T00:00:00Z",
	}
}

func testWorkflowRecord(id string, projectID string) WorkflowRecord {
	return WorkflowRecord{
		ID:                 id,
		ProjectID:          projectID,
		Name:               "Workflow",
		RepositoryIdentity: "repo",
		SourceCommit:       "commit",
		WorkflowPath:       "workflow.json",
		SourceObjectID:     "object",
		WorkflowSHA256:     strings.Repeat("b", 64),
		CreatedAt:          "2026-07-03T00:00:00Z",
	}
}

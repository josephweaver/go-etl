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

func TestStoreCreateAndGetWorkflowRun(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project, workflow := insertTestProjectAndWorkflow(t, ctx, store)
	run := testWorkflowRunRecord("run-001", project.ID, workflow.ID)

	if err := store.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	if err := store.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("second CreateWorkflowRun() error = %v", err)
	}

	got, found, err := store.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if !found {
		t.Fatal("GetWorkflowRun() found = false, want true")
	}
	if got != run {
		t.Fatalf("run = %+v, want %+v", got, run)
	}
}

func TestStoreCreateWorkflowRunRejectsConflict(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project, workflow := insertTestProjectAndWorkflow(t, ctx, store)
	run := testWorkflowRunRecord("run-001", project.ID, workflow.ID)
	if err := store.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}

	conflict := run
	conflict.SubmissionContextJSON = `[{"name":"changed"}]`
	err := store.CreateWorkflowRun(ctx, conflict)
	if err == nil || !strings.Contains(err.Error(), "different values") {
		t.Fatalf("CreateWorkflowRun(conflict) error = %v, want conflict", err)
	}
}

func TestStoreCreateWorkflowRunRejectsMissingParents(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	err := store.CreateWorkflowRun(ctx, testWorkflowRunRecord("run-001", "missing-project", "missing-workflow"))
	if err == nil || !strings.Contains(err.Error(), "insert workflow run") {
		t.Fatalf("CreateWorkflowRun() error = %v, want insert failure", err)
	}
}

func TestStoreGetWorkflowRunReturnsMissing(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	run, found, err := store.GetWorkflowRun(ctx, "missing")
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}
	if found {
		t.Fatalf("GetWorkflowRun() found = true with run %+v, want false", run)
	}
}

func TestStoreInsertStagePlanAndGetStage(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project, workflow := insertTestProjectAndWorkflow(t, ctx, store)
	run := insertTestWorkflowRun(t, ctx, store, "run-001", project.ID, workflow.ID)
	stages := []WorkflowStageRecord{
		testWorkflowStageRecord(run.ID, 0, "ready"),
		testWorkflowStageRecord(run.ID, 1, "blocked"),
	}

	if err := store.InsertStagePlan(ctx, run.ID, stages); err != nil {
		t.Fatalf("InsertStagePlan() error = %v", err)
	}
	if err := store.InsertStagePlan(ctx, run.ID, stages); err != nil {
		t.Fatalf("second InsertStagePlan() error = %v", err)
	}

	stage, found, err := store.GetWorkflowStage(ctx, run.ID, 1)
	if err != nil {
		t.Fatalf("GetWorkflowStage() error = %v", err)
	}
	if !found {
		t.Fatal("GetWorkflowStage() found = false, want true")
	}
	if stage != stages[1] {
		t.Fatalf("stage = %+v, want %+v", stage, stages[1])
	}
}

func TestStoreInsertStagePlanRejectsConflict(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project, workflow := insertTestProjectAndWorkflow(t, ctx, store)
	run := insertTestWorkflowRun(t, ctx, store, "run-001", project.ID, workflow.ID)
	stages := []WorkflowStageRecord{testWorkflowStageRecord(run.ID, 0, "ready")}
	if err := store.InsertStagePlan(ctx, run.ID, stages); err != nil {
		t.Fatalf("InsertStagePlan() error = %v", err)
	}

	conflict := []WorkflowStageRecord{testWorkflowStageRecord(run.ID, 0, "blocked")}
	err := store.InsertStagePlan(ctx, run.ID, conflict)
	if err == nil || !strings.Contains(err.Error(), "different values") {
		t.Fatalf("InsertStagePlan(conflict) error = %v, want conflict", err)
	}
}

func TestStoreInsertStagePlanRejectsMissingRun(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	err := store.InsertStagePlan(ctx, "missing-run", []WorkflowStageRecord{testWorkflowStageRecord("missing-run", 0, "ready")})
	if err == nil || !strings.Contains(err.Error(), "insert workflow stage") {
		t.Fatalf("InsertStagePlan() error = %v, want insert failure", err)
	}
}

func TestStoreGetWorkflowStageReturnsMissing(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	stage, found, err := store.GetWorkflowStage(ctx, "missing", 0)
	if err != nil {
		t.Fatalf("GetWorkflowStage() error = %v", err)
	}
	if found {
		t.Fatalf("GetWorkflowStage() found = true with stage %+v, want false", stage)
	}
}

func TestStoreListActiveWorkflowRuns(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	project, workflow := insertTestProjectAndWorkflow(t, ctx, store)

	noStageRun := insertTestWorkflowRun(t, ctx, store, "run-no-stage", project.ID, workflow.ID)
	activeRun := insertTestWorkflowRun(t, ctx, store, "run-active", project.ID, workflow.ID)
	terminalRun := insertTestWorkflowRun(t, ctx, store, "run-terminal", project.ID, workflow.ID)
	failedRun := insertTestWorkflowRun(t, ctx, store, "run-failed", project.ID, workflow.ID)

	if err := store.InsertStagePlan(ctx, activeRun.ID, []WorkflowStageRecord{testWorkflowStageRecord(activeRun.ID, 0, "ready")}); err != nil {
		t.Fatalf("insert active stages: %v", err)
	}
	if err := store.InsertStagePlan(ctx, terminalRun.ID, []WorkflowStageRecord{
		testWorkflowStageRecord(terminalRun.ID, 0, "completed"),
		testWorkflowStageRecord(terminalRun.ID, 1, "skipped"),
	}); err != nil {
		t.Fatalf("insert terminal stages: %v", err)
	}
	if err := store.InsertStagePlan(ctx, failedRun.ID, []WorkflowStageRecord{testWorkflowStageRecord(failedRun.ID, 0, "failed")}); err != nil {
		t.Fatalf("insert failed stages: %v", err)
	}

	runs, err := store.ListActiveWorkflowRuns(ctx)
	if err != nil {
		t.Fatalf("ListActiveWorkflowRuns() error = %v", err)
	}

	got := map[string]bool{}
	for _, run := range runs {
		got[run.ID] = true
	}
	if !got[noStageRun.ID] {
		t.Fatalf("active runs missing no-stage run: %+v", runs)
	}
	if !got[activeRun.ID] {
		t.Fatalf("active runs missing active run: %+v", runs)
	}
	if got[terminalRun.ID] {
		t.Fatalf("active runs included terminal run: %+v", runs)
	}
	if got[failedRun.ID] {
		t.Fatalf("active runs included failed run: %+v", runs)
	}
}

func TestStoreInsertAndGetWorkItem(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	item := testWorkItemRecord("work-001", run.ID, 0, 0)

	if err := store.InsertWorkItems(ctx, []WorkItemRecord{item}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{item}); err != nil {
		t.Fatalf("second InsertWorkItems() error = %v", err)
	}

	got, found, err := store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem() error = %v", err)
	}
	if !found {
		t.Fatal("GetWorkItem() found = false, want true")
	}
	if got != item {
		t.Fatalf("work item = %+v, want %+v", got, item)
	}
}

func TestStoreInsertWorkItemsRejectsConflict(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	item := testWorkItemRecord("work-001", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{item}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}

	conflict := item
	conflict.WorkerPayloadJSON = `{"plugin":"other","parameters":{}}`
	err := store.InsertWorkItems(ctx, []WorkItemRecord{conflict})
	if err == nil || !strings.Contains(err.Error(), "different values") {
		t.Fatalf("InsertWorkItems(conflict) error = %v, want conflict", err)
	}
}

func TestStoreInsertWorkItemsRejectsMissingStage(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	err := store.InsertWorkItems(ctx, []WorkItemRecord{testWorkItemRecord("work-001", "missing-run", 0, 0)})
	if err == nil || !strings.Contains(err.Error(), "insert work item") {
		t.Fatalf("InsertWorkItems() error = %v, want insert failure", err)
	}
}

func TestStoreGetWorkItemReturnsMissing(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	item, found, err := store.GetWorkItem(ctx, "missing")
	if err != nil {
		t.Fatalf("GetWorkItem() error = %v", err)
	}
	if found {
		t.Fatalf("GetWorkItem() found = true with item %+v, want false", item)
	}
}

func TestStoreEnqueueAndListQueuedWorkItems(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	later := testWorkItemRecord("work-later", run.ID, 0, 0)
	earlier := testWorkItemRecord("work-earlier", run.ID, 0, 1)
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{later, earlier}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}

	queued := []QueuedWorkRecord{
		{WorkItemRecord: later, QueuedAt: "2026-07-03T00:00:02Z"},
		{WorkItemRecord: earlier, QueuedAt: "2026-07-03T00:00:01Z"},
	}
	if err := store.EnqueueWorkItems(ctx, queued); err != nil {
		t.Fatalf("EnqueueWorkItems() error = %v", err)
	}
	if err := store.EnqueueWorkItems(ctx, queued); err != nil {
		t.Fatalf("second EnqueueWorkItems() error = %v", err)
	}

	got, err := store.ListQueuedWorkItems(ctx)
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("queued count = %d, want 2: %+v", len(got), got)
	}
	if got[0].ID != earlier.ID || got[1].ID != later.ID {
		t.Fatalf("queued order = %+v, want earlier then later", got)
	}
}

func TestStoreEnqueueWorkItemsRejectsConflictingQueuedAt(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	item := testWorkItemRecord("work-001", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{item}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := store.EnqueueWorkItems(ctx, []QueuedWorkRecord{{WorkItemRecord: item, QueuedAt: "2026-07-03T00:00:00Z"}}); err != nil {
		t.Fatalf("EnqueueWorkItems() error = %v", err)
	}

	err := store.EnqueueWorkItems(ctx, []QueuedWorkRecord{{WorkItemRecord: item, QueuedAt: "2026-07-03T00:00:01Z"}})
	if err == nil || !strings.Contains(err.Error(), "different queued_at") {
		t.Fatalf("EnqueueWorkItems(conflict) error = %v, want queued_at conflict", err)
	}
}

func TestStoreEnqueueWorkItemsRejectsMissingWorkItem(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	err := store.EnqueueWorkItems(ctx, []QueuedWorkRecord{{WorkItemRecord: WorkItemRecord{ID: "missing"}, QueuedAt: "2026-07-03T00:00:00Z"}})
	if err == nil || !strings.Contains(err.Error(), "enqueue work item") {
		t.Fatalf("EnqueueWorkItems() error = %v, want enqueue failure", err)
	}
}

func TestStoreCountWorkItemsForStage(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	queued := testWorkItemRecord("work-queued", run.ID, 0, 0)
	running := testWorkItemRecord("work-running", run.ID, 0, 1)
	completed := testWorkItemRecord("work-completed", run.ID, 0, 2)
	failed := testWorkItemRecord("work-failed", run.ID, 0, 3)
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{queued, running, completed, failed}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := store.EnqueueWorkItems(ctx, []QueuedWorkRecord{{WorkItemRecord: queued, QueuedAt: "2026-07-03T00:00:00Z"}}); err != nil {
		t.Fatalf("EnqueueWorkItems() error = %v", err)
	}
	insertTestRunningWork(t, ctx, store, "attempt-running", running.ID)
	insertTestCompletedWork(t, ctx, store, "attempt-completed", completed.ID)
	insertTestFailedWork(t, ctx, store, "attempt-failed", failed.ID)

	counts, err := store.CountWorkItemsForStage(ctx, run.ID, 0)
	if err != nil {
		t.Fatalf("CountWorkItemsForStage() error = %v", err)
	}
	want := WorkItemStatusCounts{Queued: 1, Running: 1, Completed: 1, Failed: 1}
	if counts != want {
		t.Fatalf("counts = %+v, want %+v", counts, want)
	}
}

func TestClaimWorkRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request ClaimWorkRequest
		want    string
	}{
		{
			name: "worker executor",
			request: ClaimWorkRequest{
				AttemptID:    "attempt-001",
				WorkerID:     "worker-001",
				ExecutorType: ExecutorTypeWorker,
				StartedAt:    "2026-07-03T00:00:00Z",
			},
		},
		{
			name: "controller executor",
			request: ClaimWorkRequest{
				AttemptID:    "attempt-001",
				ExecutorType: ExecutorTypeController,
				StartedAt:    "2026-07-03T00:00:00Z",
			},
		},
		{
			name: "missing attempt",
			request: ClaimWorkRequest{
				ExecutorType: ExecutorTypeWorker,
				StartedAt:    "2026-07-03T00:00:00Z",
			},
			want: "attempt id is required",
		},
		{
			name: "unsupported executor",
			request: ClaimWorkRequest{
				AttemptID:    "attempt-001",
				ExecutorType: "service",
				StartedAt:    "2026-07-03T00:00:00Z",
			},
			want: "unsupported claim executor type",
		},
		{
			name: "missing started at",
			request: ClaimWorkRequest{
				AttemptID:    "attempt-001",
				ExecutorType: ExecutorTypeWorker,
			},
			want: "started at is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validate() error = %v, want %q", err, tt.want)
			}
		})
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

func insertTestProjectAndWorkflow(t *testing.T, ctx context.Context, store *Store) (ProjectRecord, WorkflowRecord) {
	t.Helper()

	project := testProjectRecord("project-001")
	workflow := testWorkflowRecord("workflow-001", project.ID)
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}
	return project, workflow
}

func insertTestWorkflowRun(t *testing.T, ctx context.Context, store *Store, id string, projectID string, workflowID string) WorkflowRunRecord {
	t.Helper()

	run := testWorkflowRunRecord(id, projectID, workflowID)
	if err := store.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	return run
}

func testWorkflowRunRecord(id string, projectID string, workflowID string) WorkflowRunRecord {
	return WorkflowRunRecord{
		ID:                    id,
		ProjectID:             projectID,
		WorkflowID:            workflowID,
		SubmissionContextJSON: `[{"name":{"namespace":"override","key":"code_version"},"type":"string","expression":"test"}]`,
		CreatedAt:             "2026-07-03T00:00:00Z",
	}
}

func testWorkflowStageRecord(runID string, stageIndex int, state string) WorkflowStageRecord {
	return WorkflowStageRecord{
		RunID:                runID,
		StageIndex:           stageIndex,
		StepID:               "step-001",
		StageSourceReference: "workflow.json#/steps/0",
		State:                state,
		CreatedAt:            "2026-07-03T00:00:00Z",
	}
}

func insertTestRunWithStage(t *testing.T, ctx context.Context, store *Store) WorkflowRunRecord {
	t.Helper()

	project, workflow := insertTestProjectAndWorkflow(t, ctx, store)
	run := insertTestWorkflowRun(t, ctx, store, "run-001", project.ID, workflow.ID)
	if err := store.InsertStagePlan(ctx, run.ID, []WorkflowStageRecord{testWorkflowStageRecord(run.ID, 0, "ready")}); err != nil {
		t.Fatalf("InsertStagePlan() error = %v", err)
	}
	return run
}

func testWorkItemRecord(id string, runID string, stageIndex int, workItemIndex int) WorkItemRecord {
	return WorkItemRecord{
		ID:                   id,
		RunID:                runID,
		StageIndex:           stageIndex,
		WorkItemIndex:        workItemIndex,
		WorkerPayloadJSON:    `{"plugin":"plugin-name","parameters":{"param1":"param1value"}}`,
		ResolvedInputsSHA256: strings.Repeat("c", 64),
		CreatedAt:            "2026-07-03T00:00:00Z",
	}
}

func insertTestRunningWork(t *testing.T, ctx context.Context, store *Store, attemptID string, workItemID string) {
	t.Helper()

	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		executor_type,
		started_at
	) VALUES (?, ?, 'worker', '2026-07-03T00:00:00Z')`, attemptID, workItemID); err != nil {
		t.Fatalf("insert attempt: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO running_work (
		attempt_id,
		work_item_id,
		queued_at,
		started_at
	) VALUES (?, ?, '2026-07-03T00:00:00Z', '2026-07-03T00:00:01Z')`, attemptID, workItemID); err != nil {
		t.Fatalf("insert running work: %v", err)
	}
}

func insertTestCompletedWork(t *testing.T, ctx context.Context, store *Store, attemptID string, workItemID string) {
	t.Helper()

	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		executor_type,
		started_at
	) VALUES (?, ?, 'worker', '2026-07-03T00:00:00Z')`, attemptID, workItemID); err != nil {
		t.Fatalf("insert attempt: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO completed_work (
		attempt_id,
		work_item_id,
		output_json,
		output_json_sha256,
		pre_state_sha256,
		post_state_sha256,
		completed_at
	) VALUES (?, ?, '{}', ?, ?, ?, '2026-07-03T00:00:00Z')`,
		attemptID,
		workItemID,
		strings.Repeat("d", 64),
		strings.Repeat("e", 64),
		strings.Repeat("f", 64),
	); err != nil {
		t.Fatalf("insert completed work: %v", err)
	}
}

func insertTestFailedWork(t *testing.T, ctx context.Context, store *Store, attemptID string, workItemID string) {
	t.Helper()

	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		executor_type,
		started_at
	) VALUES (?, ?, 'worker', '2026-07-03T00:00:00Z')`, attemptID, workItemID); err != nil {
		t.Fatalf("insert attempt: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO failed_work (
		attempt_id,
		work_item_id,
		error,
		failed_at
	) VALUES (?, ?, 'failed', '2026-07-03T00:00:00Z')`, attemptID, workItemID); err != nil {
		t.Fatalf("insert failed work: %v", err)
	}
}

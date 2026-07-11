package persistence

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSQLiteSchemaVersionSix(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	version, err := store.CurrentSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion() error = %v", err)
	}
	if version != 6 {
		t.Fatalf("schema version = %d, want 6", version)
	}
}

func TestSQLiteSchemaContainsWorkerSessions(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	assertSQLiteTableExists(t, ctx, store.db, "worker_sessions")
	assertSQLiteIndexColumns(t, ctx, store.db, "idx_worker_sessions_status_heartbeat", []string{"status", "last_heartbeat_at"})
	assertSQLiteIndexColumns(t, ctx, store.db, "idx_worker_sessions_worker_registered", []string{"worker_id", "registered_at"})
}

func TestSQLiteSchemaContainsAbandonedWork(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	assertSQLiteTableExists(t, ctx, store.db, "abandoned_work")
	assertSQLiteIndexColumns(t, ctx, store.db, "idx_abandoned_work_item_time", []string{"work_item_id", "abandoned_at", "attempt_id"})
}

func TestWorkerSessionStatusConstraint(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertWorkerForSessionTest(t, ctx, store, "worker-001")

	_, err := store.db.ExecContext(ctx, `INSERT INTO worker_sessions (
		worker_session_id,
		worker_id,
		status,
		registered_at,
		last_heartbeat_at
	) VALUES ('session-001', 'worker-001', 'paused', '2026-07-03T00:00:00Z', '2026-07-03T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected unsupported worker session status to fail")
	}
}

func TestActiveWorkerSessionRequiresNullEndedAt(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertWorkerForSessionTest(t, ctx, store, "worker-001")

	_, err := store.db.ExecContext(ctx, `INSERT INTO worker_sessions (
		worker_session_id,
		worker_id,
		status,
		registered_at,
		last_heartbeat_at,
		ended_at
	) VALUES ('session-001', 'worker-001', 'active', '2026-07-03T00:00:00Z', '2026-07-03T00:00:00Z', '2026-07-03T00:00:01Z')`)
	if err == nil {
		t.Fatal("expected active session with ended_at to fail")
	}
}

func TestTerminalWorkerSessionRequiresEndedAt(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertWorkerForSessionTest(t, ctx, store, "worker-001")

	_, err := store.db.ExecContext(ctx, `INSERT INTO worker_sessions (
		worker_session_id,
		worker_id,
		status,
		registered_at,
		last_heartbeat_at
	) VALUES ('session-001', 'worker-001', 'dead', '2026-07-03T00:00:00Z', '2026-07-03T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected terminal session without ended_at to fail")
	}
}

func TestRunningWorkAllowsOnlyOneAssignmentPerSession(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	workA := testWorkItemRecord("work-a", run.ID, 0, 0)
	workB := testWorkItemRecord("work-b", run.ID, 0, 1)
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{workA, workB}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	insertWorkerSessionForTest(t, ctx, store, "worker-001", "session-001")
	insertWorkerAttemptForTest(t, ctx, store, "attempt-a", workA.ID, "worker-001", "session-001")
	insertWorkerAttemptForTest(t, ctx, store, "attempt-b", workB.ID, "worker-001", "session-001")

	if _, err := store.db.ExecContext(ctx, `INSERT INTO running_work (
		attempt_id,
		work_item_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at
	) VALUES ('attempt-a', 'work-a', 'worker-001', 'session-001', '2026-07-03T00:00:00Z', '2026-07-03T00:00:01Z')`); err != nil {
		t.Fatalf("insert first running assignment: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO running_work (
		attempt_id,
		work_item_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at
	) VALUES ('attempt-b', 'work-b', 'worker-001', 'session-001', '2026-07-03T00:00:00Z', '2026-07-03T00:00:01Z')`); err == nil {
		t.Fatal("expected second running assignment for same session to fail")
	}
}

func TestWorkerAttemptRequiresSessionForWorkerExecutor(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	work := testWorkItemRecord("work-001", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{work}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	insertWorkerForSessionTest(t, ctx, store, "worker-001")

	_, err := store.db.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		worker_id,
		executor_type,
		started_at
	) VALUES ('attempt-001', 'work-001', 'worker-001', 'worker', '2026-07-03T00:00:00Z')`)
	if err == nil {
		t.Fatal("expected worker attempt without session to fail")
	}
}

func TestControllerAttemptAllowsNullWorkerSession(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	work := testWorkItemRecord("work-001", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{work}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		executor_type,
		started_at
	) VALUES ('attempt-001', 'work-001', 'controller', '2026-07-03T00:00:00Z')`); err != nil {
		t.Fatalf("insert controller attempt: %v", err)
	}
}

func TestOpenVersionFiveStoreFailsWithRebuildInstruction(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.sqlite")
	db := openRawSQLite(t, path)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_version (version INTEGER NOT NULL); INSERT INTO schema_version VALUES (5)`); err != nil {
		t.Fatalf("setup version 5 schema: %v", err)
	}
	db.Close()

	store, err := OpenStore(ctx, Config{Driver: DriverSQLite, ConnectionString: path})
	if err == nil || !strings.Contains(err.Error(), "rebuild the development database") {
		t.Fatalf("OpenStore() error = %v, want rebuild instruction", err)
	}
	if store != nil {
		t.Fatalf("OpenStore() store = %#v, want nil", store)
	}
}

func TestRegisterWorkerSessionCreatesWorkerAndActiveSession(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()

	session, err := store.RegisterWorkerSession(ctx, RegisterWorkerSessionRequest{
		WorkerID:        "worker-001",
		SessionID:       "session-001",
		RegisteredAt:    "2026-07-03T00:00:00Z",
		ExecutionHandle: "slurm-123",
	})
	if err != nil {
		t.Fatalf("RegisterWorkerSession() error = %v", err)
	}
	if session.Status != WorkerSessionStatusActive || session.LastHeartbeatAt != session.RegisteredAt {
		t.Fatalf("session = %+v, want active with heartbeat at registration", session)
	}

	got, found, err := store.GetWorkerSession(ctx, "worker-001", "session-001")
	if err != nil {
		t.Fatalf("GetWorkerSession() error = %v", err)
	}
	if !found || got != session {
		t.Fatalf("GetWorkerSession() = %+v found %v, want %+v", got, found, session)
	}
}

func TestRegisterWorkerSessionRejectsSessionIDReuse(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertWorkerSessionForTest(t, ctx, store, "worker-001", "session-001")

	_, err := store.RegisterWorkerSession(ctx, RegisterWorkerSessionRequest{
		WorkerID:     "worker-002",
		SessionID:    "session-001",
		RegisteredAt: "2026-07-03T00:00:00Z",
	})
	if err == nil || !strings.Contains(err.Error(), "different values") {
		t.Fatalf("RegisterWorkerSession() error = %v, want session reuse conflict", err)
	}
}

func TestHeartbeatWorkerSessionUpdatesActiveSession(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertWorkerSessionForTest(t, ctx, store, "worker-001", "session-001")

	updated, err := store.HeartbeatWorkerSession(ctx, HeartbeatWorkerSessionRequest{
		WorkerID:    "worker-001",
		SessionID:   "session-001",
		HeartbeatAt: "2026-07-03T00:01:00Z",
	})
	if err != nil {
		t.Fatalf("HeartbeatWorkerSession() error = %v", err)
	}
	if !updated {
		t.Fatal("HeartbeatWorkerSession() updated = false, want true")
	}
	session, found, err := store.GetWorkerSession(ctx, "worker-001", "session-001")
	if err != nil || !found {
		t.Fatalf("GetWorkerSession() found=%v error=%v, want session", found, err)
	}
	if session.LastHeartbeatAt != "2026-07-03T00:01:00Z" {
		t.Fatalf("last heartbeat = %q, want updated timestamp", session.LastHeartbeatAt)
	}
}

func TestHeartbeatWorkerSessionDoesNotReviveDeadSession(t *testing.T) {
	assertHeartbeatDoesNotReviveTerminalSession(t, WorkerSessionStatusDead)
}

func TestHeartbeatWorkerSessionDoesNotReviveStoppedSession(t *testing.T) {
	assertHeartbeatDoesNotReviveTerminalSession(t, WorkerSessionStatusStopped)
}

func TestListLiveWorkerSessionsUsesInclusiveCutoff(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertWorkerSessionForTest(t, ctx, store, "worker-001", "session-001")
	insertWorkerSessionForTest(t, ctx, store, "worker-002", "session-002")
	if _, err := store.db.ExecContext(ctx, `UPDATE worker_sessions SET last_heartbeat_at = '2026-07-03T00:05:00Z' WHERE worker_session_id = 'session-002'`); err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}
	cutoff, err := time.Parse(time.RFC3339, "2026-07-03T00:00:00Z")
	if err != nil {
		t.Fatalf("parse cutoff: %v", err)
	}

	live, err := store.ListLiveWorkerSessions(ctx, cutoff)
	if err != nil {
		t.Fatalf("ListLiveWorkerSessions() error = %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("live sessions = %+v, want 2 including exact cutoff", live)
	}
}

func TestListExpiredWorkerSessionsUsesStrictCutoff(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertWorkerSessionForTest(t, ctx, store, "worker-001", "session-001")
	insertWorkerSessionForTest(t, ctx, store, "worker-002", "session-002")
	if _, err := store.db.ExecContext(ctx, `UPDATE worker_sessions SET last_heartbeat_at = '2026-07-02T23:59:59Z' WHERE worker_session_id = 'session-002'`); err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}
	cutoff, err := time.Parse(time.RFC3339, "2026-07-03T00:00:00Z")
	if err != nil {
		t.Fatalf("parse cutoff: %v", err)
	}

	expired, err := store.ListExpiredWorkerSessions(ctx, cutoff)
	if err != nil {
		t.Fatalf("ListExpiredWorkerSessions() error = %v", err)
	}
	if len(expired) != 1 || expired[0].ID != "session-002" {
		t.Fatalf("expired sessions = %+v, want only session-002", expired)
	}
}

func TestEndWorkerSessionIsIdempotentForSameTerminalState(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertWorkerSessionForTest(t, ctx, store, "worker-001", "session-001")
	request := EndWorkerSessionRequest{
		WorkerID:  "worker-001",
		SessionID: "session-001",
		Status:    WorkerSessionStatusStopped,
		EndedAt:   "2026-07-03T00:02:00Z",
		Reason:    "no_work",
	}

	changed, err := store.EndWorkerSession(ctx, request)
	if err != nil {
		t.Fatalf("EndWorkerSession() error = %v", err)
	}
	if !changed {
		t.Fatal("EndWorkerSession() changed = false, want true")
	}
	changed, err = store.EndWorkerSession(ctx, request)
	if err != nil {
		t.Fatalf("second EndWorkerSession() error = %v", err)
	}
	if changed {
		t.Fatal("second EndWorkerSession() changed = true, want false")
	}
}

func TestEndWorkerSessionDoesNotRewriteDeadAsStopped(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertTerminalWorkerSessionForTest(t, ctx, store, "worker-001", "session-001", WorkerSessionStatusDead)

	changed, err := store.EndWorkerSession(ctx, EndWorkerSessionRequest{
		WorkerID:  "worker-001",
		SessionID: "session-001",
		Status:    WorkerSessionStatusStopped,
		EndedAt:   "2026-07-03T00:03:00Z",
		Reason:    "no_work",
	})
	if err != nil {
		t.Fatalf("EndWorkerSession() error = %v", err)
	}
	if changed {
		t.Fatal("EndWorkerSession() changed = true, want false")
	}
	session, found, err := store.GetWorkerSession(ctx, "worker-001", "session-001")
	if err != nil || !found {
		t.Fatalf("GetWorkerSession() found=%v error=%v, want session", found, err)
	}
	if session.Status != WorkerSessionStatusDead {
		t.Fatalf("session status = %q, want dead", session.Status)
	}
}

func TestListAbandonedWorkReturnsAttemptHistory(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	run := insertTestRunWithStage(t, ctx, store)
	work := testWorkItemRecord("work-001", run.ID, 0, 0)
	if err := store.InsertWorkItems(ctx, []WorkItemRecord{work}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	insertWorkerSessionForTest(t, ctx, store, "worker-001", "session-001")
	insertWorkerAttemptForTest(t, ctx, store, "attempt-001", work.ID, "worker-001", "session-001")
	if _, err := store.db.ExecContext(ctx, `INSERT INTO abandoned_work (
		attempt_id,
		work_item_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at,
		abandoned_at,
		reason
	) VALUES ('attempt-001', 'work-001', 'worker-001', 'session-001', '2026-07-03T00:00:00Z', '2026-07-03T00:00:01Z', '2026-07-03T00:05:00Z', 'heartbeat_expired')`); err != nil {
		t.Fatalf("insert abandoned work: %v", err)
	}

	history, err := store.ListAbandonedWorkForItem(ctx, work.ID)
	if err != nil {
		t.Fatalf("ListAbandonedWorkForItem() error = %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history = %+v, want one record", history)
	}
	want := AbandonedWorkRecord{
		AttemptID:       "attempt-001",
		WorkItemID:      "work-001",
		WorkerID:        "worker-001",
		WorkerSessionID: "session-001",
		QueuedAt:        "2026-07-03T00:00:00Z",
		StartedAt:       "2026-07-03T00:00:01Z",
		AbandonedAt:     "2026-07-03T00:05:00Z",
		Reason:          "heartbeat_expired",
	}
	if history[0] != want {
		t.Fatalf("history[0] = %+v, want %+v", history[0], want)
	}
}

func assertHeartbeatDoesNotReviveTerminalSession(t *testing.T, status string) {
	t.Helper()
	ctx := context.Background()
	store := openTestStore(t, ctx, filepath.Join(t.TempDir(), "store.sqlite"))
	defer store.Close()
	insertTerminalWorkerSessionForTest(t, ctx, store, "worker-001", "session-001", status)

	updated, err := store.HeartbeatWorkerSession(ctx, HeartbeatWorkerSessionRequest{
		WorkerID:    "worker-001",
		SessionID:   "session-001",
		HeartbeatAt: "2026-07-03T00:10:00Z",
	})
	if err != nil {
		t.Fatalf("HeartbeatWorkerSession() error = %v", err)
	}
	if updated {
		t.Fatal("HeartbeatWorkerSession() updated = true, want false")
	}
	session, found, err := store.GetWorkerSession(ctx, "worker-001", "session-001")
	if err != nil || !found {
		t.Fatalf("GetWorkerSession() found=%v error=%v, want session", found, err)
	}
	if session.Status != status || session.LastHeartbeatAt == "2026-07-03T00:10:00Z" {
		t.Fatalf("session after heartbeat = %+v, want unchanged terminal session", session)
	}
}

func insertWorkerForSessionTest(t *testing.T, ctx context.Context, store *Store, workerID string) {
	t.Helper()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO workers (
		worker_id,
		created_at
	) VALUES (?, '2026-07-03T00:00:00Z')`, workerID); err != nil {
		t.Fatalf("insert worker %s: %v", workerID, err)
	}
}

func insertWorkerSessionForTest(t *testing.T, ctx context.Context, store *Store, workerID string, sessionID string) {
	t.Helper()
	if _, err := store.RegisterWorkerSession(ctx, RegisterWorkerSessionRequest{
		WorkerID:     workerID,
		SessionID:    sessionID,
		RegisteredAt: "2026-07-03T00:00:00Z",
	}); err != nil {
		t.Fatalf("RegisterWorkerSession(%s/%s) error = %v", workerID, sessionID, err)
	}
}

func insertTerminalWorkerSessionForTest(t *testing.T, ctx context.Context, store *Store, workerID string, sessionID string, status string) {
	t.Helper()
	insertWorkerSessionForTest(t, ctx, store, workerID, sessionID)
	if _, err := store.db.ExecContext(ctx, `UPDATE worker_sessions
	SET status = ?,
		ended_at = '2026-07-03T00:02:00Z',
		end_reason = 'test'
	WHERE worker_session_id = ?`, status, sessionID); err != nil {
		t.Fatalf("make worker session terminal: %v", err)
	}
}

func insertWorkerAttemptForTest(t *testing.T, ctx context.Context, store *Store, attemptID string, workItemID string, workerID string, sessionID string) {
	t.Helper()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		worker_id,
		worker_session_id,
		executor_type,
		started_at
	) VALUES (?, ?, ?, ?, 'worker', '2026-07-03T00:00:01Z')`, attemptID, workItemID, workerID, sessionID); err != nil {
		t.Fatalf("insert worker attempt %s: %v", attemptID, err)
	}
}

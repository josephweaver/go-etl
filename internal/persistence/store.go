package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"goetl/internal/model"
)

const (
	DriverSQLite           = "sqlite"
	SupportedSchemaVersion = 7

	ExecutorTypeWorker     = "worker"
	ExecutorTypeController = "controller"

	WorkerSessionStatusActive  = "active"
	WorkerSessionStatusStopped = "stopped"
	WorkerSessionStatusDead    = "dead"

	CheckpointCaptureKindPeriodic = "periodic"
	CheckpointCaptureKindQuantum  = "quantum"
	CheckpointCaptureKindFinal    = "final"

	CheckpointDispositionContinue = "continue"
	CheckpointDispositionSuspend  = "suspend"

	SuspendReasonQuantum  = "quantum"
	SuspendReasonShutdown = "shutdown"
)

var ErrWorkerSessionNotActive = errors.New("worker session is not active")
var ErrWorkerSessionBusy = errors.New("worker session already owns running work")
var ErrAssignmentNoLongerOwned = errors.New("assignment no longer owned")
var ErrResumeAttemptLimitExceeded = errors.New("resume attempt limit exceeded")

type Config struct {
	Driver           string
	ConnectionString string
}

type Store struct {
	db *sql.DB
}

type ProjectRecord struct {
	ID                 string
	Name               string
	RepositoryIdentity string
	SourceRevisionID   *string
	ConfigPath         string
	SourceObjectID     string
	ConfigSHA256       string
	CreatedAt          string
}

type WorkflowRecord struct {
	ID                 string
	ProjectID          string
	Name               string
	RepositoryIdentity string
	SourceRevisionID   *string
	WorkflowPath       string
	SourceObjectID     string
	WorkflowSHA256     string
	CreatedAt          string
}

type WorkflowRunRecord struct {
	ID                    string
	ProjectID             string
	WorkflowID            string
	SubmissionContextJSON string
	CreatedAt             string
}

type WorkflowStageRecord struct {
	RunID                string
	StageIndex           int
	StepID               string
	StageSourceReference string
	State                string
	CreatedAt            string
	ReadyAt              string
	StartedAt            string
	CompletedAt          string
	FailedAt             string
	OutputJSON           string
	OutputJSONSHA256     string
}

type WorkflowDependencyStepRecord struct {
	RunID        string
	StageIndex   int
	StepIndex    int
	StepID       string
	ParallelWith string
	CreatedAt    string
}

type WorkItemRecord struct {
	ID                   string
	RunID                string
	StageIndex           int
	WorkItemIndex        int
	WorkerPayloadJSON    string
	ResolvedInputsSHA256 string
	CreatedAt            string
}

type WorkItemResourceConstraintRecord struct {
	WorkItemID      string
	ConstraintIndex int
	ResourceKey     string
	RequestedUnits  int
	Operator        string
	TargetUnits     int
	CreatedAt       string
}

type QueuedResourceConstraintCheckRecord struct {
	WorkItemID      string
	QueuedAt        string
	ConstraintIndex int
	ResourceKey     string
	TotalUnits      int64
	RequestedUnits  int64
	Operator        string
	TargetUnits     int64
}

type WorkflowDependencyWorkItemRecord struct {
	RunID         string
	StageIndex    int
	StepIndex     int
	WorkItemID    string
	WorkItemIndex int
	CreatedAt     string
}

type WorkflowStepOutputFactRecord struct {
	RunID            string
	StepIndex        int
	OutputJSON       string
	OutputJSONSHA256 string
	OutputJSONBytes  int
	OutputJSONPruned bool
	OutputKind       string
	CreatedAt        string
	UpdatedAt        string
}

type QueuedWorkRecord struct {
	WorkItemRecord
	QueuedAt         string
	ResumeArtifactID string
}

type ResumeArtifactRecord struct {
	ID                   string
	WorkItemID           string
	ProducingAttemptID   string
	ExecutionLineageID   string
	ResumeGeneration     int
	CaptureKind          string
	PauseStrategy        model.PauseStrategy
	ManifestJSON         string
	ManifestSHA256       string
	StorageScope         string
	ManifestRelativePath string
	CreatedAt            string
	AcceptedAt           string
	Manifest             model.ResumeArtifactManifest
	Reference            model.ResumeArtifactReference
}

type SuspendedWorkRecord struct {
	AttemptID        string
	WorkItemID       string
	ResumeArtifactID string
	WorkerID         string
	WorkerSessionID  string
	QueuedAt         string
	StartedAt        string
	SuspendedAt      string
	SuspendReason    string
}

type ConfirmCheckpointRequest struct {
	AttemptID         string
	WorkerID          string
	WorkerSessionID   string
	LiveSessionCutoff string
	ManifestJSON      string
	Reference         model.ResumeArtifactReference
	CaptureKind       string
	Disposition       string
	AcceptedAt        string
	SuspendedAt       string
}

type ConfirmCheckpointResult struct {
	Artifact     ResumeArtifactRecord
	Suspended    *SuspendedWorkRecord
	Transitioned bool
}

type SuspendFromLatestCheckpointRequest struct {
	AttemptID         string
	WorkerID          string
	WorkerSessionID   string
	LiveSessionCutoff string
	SuspendedAt       string
	SuspendReason     string
}

type SuspendFromLatestCheckpointResult struct {
	Artifact     ResumeArtifactRecord
	Suspended    SuspendedWorkRecord
	Found        bool
	Transitioned bool
}

type ResumeAttemptLimitExceededError struct {
	WorkItemID              string
	ResumeArtifactID        string
	ExecutionLineageID      string
	NextResumeAttemptNumber int
	ConfiguredLimit         int
	QueuedAt                string
}

func (err *ResumeAttemptLimitExceededError) Error() string {
	return fmt.Sprintf(
		"%s for work item %s artifact %s: next attempt %d exceeds limit %d",
		ErrResumeAttemptLimitExceeded,
		err.WorkItemID,
		err.ResumeArtifactID,
		err.NextResumeAttemptNumber,
		err.ConfiguredLimit,
	)
}

func (err *ResumeAttemptLimitExceededError) Unwrap() error {
	return ErrResumeAttemptLimitExceeded
}

type FailPendingResumeAttemptLimitRequest struct {
	AttemptID               string
	WorkItemID              string
	ResumeArtifactID        string
	ExecutionLineageID      string
	NextResumeAttemptNumber int
	ConfiguredLimit         int
	QueuedAt                string
	FailedAt                string
}

type FailPendingResumeAttemptLimitResult struct {
	WorkItem     WorkItemRecord
	Failed       FailedWorkRecord
	Terminalized bool
}

type QueueWorkItemsRequest struct {
	WorkItems           []WorkItemRecord
	ResourceConstraints []WorkItemResourceConstraintRecord
	QueuedWork          []QueuedWorkRecord
}

type WorkItemStatusCounts struct {
	Queued    int
	Running   int
	Completed int
	Failed    int
}

type RunWorkStatusCounts struct {
	Queued    int
	Running   int
	Completed int
	Failed    int
}

type RunningWorkRecord struct {
	AttemptID       string
	WorkItem        WorkItemRecord
	WorkerID        string
	WorkerSessionID string
	ExecutorType    string
	QueuedAt        string
	StartedAt       string
}

type WorkerRecord struct {
	ID              string
	ExecutionHandle string
	CreatedAt       string
}

type WorkerSessionRecord struct {
	ID              string
	WorkerID        string
	Status          string
	RegisteredAt    string
	LastHeartbeatAt string
	EndedAt         string
	EndReason       string
	ExecutionHandle string
}

type RegisterWorkerSessionRequest struct {
	WorkerID        string
	SessionID       string
	RegisteredAt    string
	ExecutionHandle string
}

type HeartbeatWorkerSessionRequest struct {
	WorkerID    string
	SessionID   string
	HeartbeatAt string
}

type EndWorkerSessionRequest struct {
	WorkerID  string
	SessionID string
	Status    string
	EndedAt   string
	Reason    string
}

type StopWorkerSessionAndRecoverWorkRequest struct {
	WorkerID  string
	SessionID string
	StoppedAt string
	Reason    string
}

type StopWorkerSessionAndRecoverWorkResult struct {
	Changed           bool
	AbandonedAttempts int
	RequeuedWorkItems int
}

type RecoverExpiredWorkerSessionsRequest struct {
	Cutoff      string
	RecoveredAt string
	Reason      string
}

type RecoverExpiredWorkerSessionsResult struct {
	ExpiredSessions   int
	AbandonedAttempts int
	RequeuedWorkItems int
}

type AbandonedWorkRecord struct {
	AttemptID       string
	WorkItemID      string
	WorkerID        string
	WorkerSessionID string
	QueuedAt        string
	StartedAt       string
	AbandonedAt     string
	Reason          string
}

type TerminalAttemptRecord struct {
	AttemptID        string
	WorkItem         WorkItemRecord
	TerminalState    string
	WorkerID         string
	ExecutorType     string
	QueuedAt         string
	StartedAt        string
	FinishedAt       string
	Error            string
	SkippedParentID  string
	OutputJSON       string
	OutputJSONSHA256 string
	PreStateSHA256   string
	PostStateSHA256  string
}

type CompleteStageRequest struct {
	RunID                    string
	StageIndex               int
	OutputJSON               string
	OutputJSONSHA256         string
	CompletedAt              string
	ReadyWorkItems           []WorkItemRecord
	ReadyResourceConstraints []WorkItemResourceConstraintRecord
	ReadyQueuedWork          []QueuedWorkRecord
}

type CompleteStageResult struct {
	Stage     WorkflowStageRecord
	Found     bool
	Completed bool
}

type ClaimWorkRequest struct {
	AttemptID          string
	WorkerID           string
	WorkerSessionID    string
	ExecutorType       string
	StartedAt          string
	LiveSessionCutoff  string
	ResumeAttemptLimit int
}

type ClaimedWorkRecord struct {
	AttemptID            string
	WorkItem             WorkItemRecord
	WorkerID             string
	WorkerSessionID      string
	ExecutorType         string
	QueuedAt             string
	StartedAt            string
	ResumedFromAttemptID string
	ExecutionLineageID   string
	ResumeAttemptNumber  int
	ResumeArtifact       *ResumeArtifactRecord
}

type CompleteAttemptRequest struct {
	AttemptID         string
	WorkerID          string
	WorkerSessionID   string
	LiveSessionCutoff string
	SkippedParentID   string
	OutputJSON        string
	OutputJSONSHA256  string
	PreStateSHA256    string
	PostStateSHA256   string
	CompletedAt       string
}

type CompletedWorkRecord struct {
	AttemptID        string
	WorkItemID       string
	SkippedParentID  string
	OutputJSON       string
	OutputJSONSHA256 string
	PreStateSHA256   string
	PostStateSHA256  string
	QueuedAt         string
	StartedAt        string
	CompletedAt      string
}

type FailAttemptRequest struct {
	AttemptID         string
	WorkerID          string
	WorkerSessionID   string
	LiveSessionCutoff string
	Error             string
	FailedAt          string
}

type FailedWorkRecord struct {
	AttemptID  string
	WorkItemID string
	Error      string
	QueuedAt   string
	StartedAt  string
	FailedAt   string
}

func OpenStore(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Driver == "" {
		return nil, fmt.Errorf("database driver is required")
	}
	if cfg.ConnectionString == "" {
		return nil, fmt.Errorf("database connection string is required")
	}

	switch cfg.Driver {
	case DriverSQLite:
		return openSQLiteStore(ctx, cfg.ConnectionString)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) CurrentSchemaVersion(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not open")
	}

	var version int
	if err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func (s *Store) RegisterWorkerSession(ctx context.Context, request RegisterWorkerSessionRequest) (WorkerSessionRecord, error) {
	if err := s.requireOpen(); err != nil {
		return WorkerSessionRecord{}, err
	}
	if err := request.validate(); err != nil {
		return WorkerSessionRecord{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkerSessionRecord{}, fmt.Errorf("begin worker session registration: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getWorkerSessionByID(ctx, tx, request.SessionID)
	if err != nil {
		return WorkerSessionRecord{}, err
	}
	if found {
		if !workerSessionMatchesRegistration(existing, request) {
			return WorkerSessionRecord{}, fmt.Errorf("worker session %s already exists with different values", request.SessionID)
		}
		return existing, tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO workers (
		worker_id,
		execution_handle,
		created_at
	) VALUES (?, ?, ?)
	ON CONFLICT(worker_id) DO NOTHING`,
		request.WorkerID,
		nullString(request.ExecutionHandle),
		request.RegisteredAt,
	); err != nil {
		return WorkerSessionRecord{}, fmt.Errorf("insert worker %s: %w", request.WorkerID, err)
	}

	record := WorkerSessionRecord{
		ID:              request.SessionID,
		WorkerID:        request.WorkerID,
		Status:          WorkerSessionStatusActive,
		RegisteredAt:    request.RegisteredAt,
		LastHeartbeatAt: request.RegisteredAt,
		ExecutionHandle: request.ExecutionHandle,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO worker_sessions (
		worker_session_id,
		worker_id,
		status,
		registered_at,
		last_heartbeat_at,
		execution_handle
	) VALUES (?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.WorkerID,
		record.Status,
		record.RegisteredAt,
		record.LastHeartbeatAt,
		nullString(record.ExecutionHandle),
	); err != nil {
		return WorkerSessionRecord{}, fmt.Errorf("insert worker session %s: %w", request.SessionID, err)
	}

	if err := tx.Commit(); err != nil {
		return WorkerSessionRecord{}, fmt.Errorf("commit worker session registration: %w", err)
	}
	return record, nil
}

func (s *Store) HeartbeatWorkerSession(ctx context.Context, request HeartbeatWorkerSessionRequest) (bool, error) {
	if err := s.requireOpen(); err != nil {
		return false, err
	}
	if err := request.validate(); err != nil {
		return false, err
	}

	result, err := s.db.ExecContext(ctx, `UPDATE worker_sessions
	SET last_heartbeat_at = ?
	WHERE worker_id = ?
		AND worker_session_id = ?
		AND status = ?`,
		request.HeartbeatAt,
		request.WorkerID,
		request.SessionID,
		WorkerSessionStatusActive,
	)
	if err != nil {
		return false, fmt.Errorf("heartbeat worker session %s/%s: %w", request.WorkerID, request.SessionID, err)
	}
	updated, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("heartbeat worker session %s/%s: %w", request.WorkerID, request.SessionID, err)
	}
	return updated, nil
}

func (s *Store) GetWorkerSession(ctx context.Context, workerID string, sessionID string) (WorkerSessionRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkerSessionRecord{}, false, err
	}
	if workerID == "" {
		return WorkerSessionRecord{}, false, fmt.Errorf("worker id is required")
	}
	if sessionID == "" {
		return WorkerSessionRecord{}, false, fmt.Errorf("worker session id is required")
	}
	return getWorkerSession(ctx, s.db, workerID, sessionID)
}

func (s *Store) ListLiveWorkerSessions(ctx context.Context, cutoff time.Time) ([]WorkerSessionRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	return listWorkerSessionsByHeartbeat(ctx, s.db, `>=`, cutoff)
}

func (s *Store) ListExpiredWorkerSessions(ctx context.Context, cutoff time.Time) ([]WorkerSessionRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	return listWorkerSessionsByHeartbeat(ctx, s.db, `<`, cutoff)
}

func (s *Store) RecoverExpiredWorkerSessions(ctx context.Context, request RecoverExpiredWorkerSessionsRequest) (RecoverExpiredWorkerSessionsResult, error) {
	if err := s.requireOpen(); err != nil {
		return RecoverExpiredWorkerSessionsResult{}, err
	}
	if err := request.validate(); err != nil {
		return RecoverExpiredWorkerSessionsResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RecoverExpiredWorkerSessionsResult{}, fmt.Errorf("begin recover expired worker sessions: %w", err)
	}
	defer tx.Rollback()

	sessions, err := listExpiredActiveWorkerSessions(ctx, tx, request.Cutoff)
	if err != nil {
		return RecoverExpiredWorkerSessionsResult{}, err
	}

	var result RecoverExpiredWorkerSessionsResult
	for _, session := range sessions {
		changed, err := markWorkerSessionDeadIfExpired(ctx, tx, session, request)
		if err != nil {
			return RecoverExpiredWorkerSessionsResult{}, err
		}
		if !changed {
			continue
		}
		result.ExpiredSessions++

		assignments, err := listRunningWorkForSession(ctx, tx, session.ID)
		if err != nil {
			return RecoverExpiredWorkerSessionsResult{}, err
		}
		for _, assignment := range assignments {
			if err := abandonRunningWork(ctx, tx, assignment, request.RecoveredAt, request.Reason); err != nil {
				return RecoverExpiredWorkerSessionsResult{}, err
			}
			result.AbandonedAttempts++
			requeued, err := requeueAbandonedWork(ctx, tx, assignment.attemptID, assignment.workItemID, request.RecoveredAt)
			if err != nil {
				return RecoverExpiredWorkerSessionsResult{}, err
			}
			if requeued {
				result.RequeuedWorkItems++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return RecoverExpiredWorkerSessionsResult{}, fmt.Errorf("commit recover expired worker sessions: %w", err)
	}
	return result, nil
}

func (s *Store) StopWorkerSessionAndRecoverWork(ctx context.Context, request StopWorkerSessionAndRecoverWorkRequest) (StopWorkerSessionAndRecoverWorkResult, error) {
	if err := s.requireOpen(); err != nil {
		return StopWorkerSessionAndRecoverWorkResult{}, err
	}
	if err := request.validate(); err != nil {
		return StopWorkerSessionAndRecoverWorkResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StopWorkerSessionAndRecoverWorkResult{}, fmt.Errorf("begin stop worker session and recover work: %w", err)
	}
	defer tx.Rollback()

	session, found, err := getWorkerSession(ctx, tx, request.WorkerID, request.SessionID)
	if err != nil {
		return StopWorkerSessionAndRecoverWorkResult{}, err
	}
	if !found {
		return StopWorkerSessionAndRecoverWorkResult{}, tx.Commit()
	}
	if session.Status != WorkerSessionStatusActive {
		return StopWorkerSessionAndRecoverWorkResult{}, tx.Commit()
	}

	changed, err := markWorkerSessionStopped(ctx, tx, request)
	if err != nil {
		return StopWorkerSessionAndRecoverWorkResult{}, err
	}
	if !changed {
		return StopWorkerSessionAndRecoverWorkResult{}, tx.Commit()
	}

	result := StopWorkerSessionAndRecoverWorkResult{Changed: true}
	assignments, err := listRunningWorkForSession(ctx, tx, request.SessionID)
	if err != nil {
		return StopWorkerSessionAndRecoverWorkResult{}, err
	}
	for _, assignment := range assignments {
		if err := abandonRunningWork(ctx, tx, assignment, request.StoppedAt, "worker_stopped"); err != nil {
			return StopWorkerSessionAndRecoverWorkResult{}, err
		}
		result.AbandonedAttempts++
		requeued, err := requeueAbandonedWork(ctx, tx, assignment.attemptID, assignment.workItemID, request.StoppedAt)
		if err != nil {
			return StopWorkerSessionAndRecoverWorkResult{}, err
		}
		if requeued {
			result.RequeuedWorkItems++
		}
	}

	if err := tx.Commit(); err != nil {
		return StopWorkerSessionAndRecoverWorkResult{}, fmt.Errorf("commit stop worker session and recover work: %w", err)
	}
	return result, nil
}

func (s *Store) EndWorkerSession(ctx context.Context, request EndWorkerSessionRequest) (bool, error) {
	if err := s.requireOpen(); err != nil {
		return false, err
	}
	if err := request.validate(); err != nil {
		return false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin end worker session: %w", err)
	}
	defer tx.Rollback()

	session, found, err := getWorkerSession(ctx, tx, request.WorkerID, request.SessionID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, tx.Commit()
	}
	if session.Status != WorkerSessionStatusActive {
		if session.Status == request.Status && session.EndedAt == request.EndedAt && session.EndReason == request.Reason {
			return false, tx.Commit()
		}
		return false, nil
	}

	var runningAssignments int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM running_work WHERE worker_session_id = ?`, request.SessionID).Scan(&runningAssignments); err != nil {
		return false, fmt.Errorf("count running assignments for worker session %s: %w", request.SessionID, err)
	}
	if runningAssignments != 0 {
		return false, fmt.Errorf("worker session %s has running assignments", request.SessionID)
	}

	result, err := tx.ExecContext(ctx, `UPDATE worker_sessions
	SET status = ?,
		ended_at = ?,
		end_reason = ?
	WHERE worker_id = ?
		AND worker_session_id = ?
		AND status = ?`,
		request.Status,
		request.EndedAt,
		request.Reason,
		request.WorkerID,
		request.SessionID,
		WorkerSessionStatusActive,
	)
	if err != nil {
		return false, fmt.Errorf("end worker session %s/%s: %w", request.WorkerID, request.SessionID, err)
	}
	changed, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("end worker session %s/%s: %w", request.WorkerID, request.SessionID, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit end worker session: %w", err)
	}
	return changed, nil
}

func (s *Store) ListAbandonedWorkForItem(ctx context.Context, workItemID string) ([]AbandonedWorkRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	if workItemID == "" {
		return nil, fmt.Errorf("work item id is required")
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at,
		abandoned_at,
		reason
	FROM abandoned_work
	WHERE work_item_id = ?
	ORDER BY abandoned_at, attempt_id`, workItemID)
	if err != nil {
		return nil, fmt.Errorf("list abandoned work for item %s: %w", workItemID, err)
	}
	defer rows.Close()

	records := []AbandonedWorkRecord{}
	for rows.Next() {
		record, err := scanAbandonedWork(rows)
		if err != nil {
			return nil, fmt.Errorf("list abandoned work for item %s: %w", workItemID, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list abandoned work for item %s: %w", workItemID, err)
	}
	return records, nil
}

func (s *Store) UpsertProject(ctx context.Context, project ProjectRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := project.validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project upsert: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getProject(ctx, tx, project.ID)
	if err != nil {
		return err
	}
	if found {
		if !sameProjectRecord(existing, project) {
			return fmt.Errorf("project %s already exists with different values", project.ID)
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO projects (
		project_id,
		project_name,
		repository_identity,
		source_revision_id,
		config_path,
		source_object_id,
		config_sha256,
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		project.ID,
		project.Name,
		project.RepositoryIdentity,
		nullStringPtr(project.SourceRevisionID),
		project.ConfigPath,
		project.SourceObjectID,
		project.ConfigSHA256,
		project.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert project %s: %w", project.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project upsert: %w", err)
	}
	return nil
}

func (s *Store) GetProject(ctx context.Context, projectID string) (ProjectRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return ProjectRecord{}, false, err
	}
	return getProject(ctx, s.db, projectID)
}

func (s *Store) DeleteProjectIfUnused(ctx context.Context, projectID string) (bool, error) {
	if err := s.requireOpen(); err != nil {
		return false, err
	}
	if projectID == "" {
		return false, fmt.Errorf("project id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin project delete: %w", err)
	}
	defer tx.Rollback()

	var workflowCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflows WHERE project_id = ?`, projectID).Scan(&workflowCount); err != nil {
		return false, fmt.Errorf("count workflows for project %s: %w", projectID, err)
	}
	if workflowCount != 0 {
		return false, nil
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE project_id = ?`, projectID)
	if err != nil {
		return false, fmt.Errorf("delete project %s: %w", projectID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("delete project %s: %w", projectID, err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit project delete: %w", err)
	}
	return deleted, nil
}

func (s *Store) UpsertWorkflow(ctx context.Context, workflow WorkflowRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := workflow.validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow upsert: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getWorkflow(ctx, tx, workflow.ID)
	if err != nil {
		return err
	}
	if found {
		if !sameWorkflowRecord(existing, workflow) {
			return fmt.Errorf("workflow %s already exists with different values", workflow.ID)
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO workflows (
		workflow_id,
		project_id,
		workflow_name,
		repository_identity,
		source_revision_id,
		workflow_path,
		source_object_id,
		workflow_sha256,
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		workflow.ID,
		workflow.ProjectID,
		workflow.Name,
		workflow.RepositoryIdentity,
		nullStringPtr(workflow.SourceRevisionID),
		workflow.WorkflowPath,
		workflow.SourceObjectID,
		workflow.WorkflowSHA256,
		workflow.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert workflow %s: %w", workflow.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow upsert: %w", err)
	}
	return nil
}

func (s *Store) GetWorkflow(ctx context.Context, workflowID string) (WorkflowRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkflowRecord{}, false, err
	}
	return getWorkflow(ctx, s.db, workflowID)
}

func (s *Store) DeleteWorkflowIfUnused(ctx context.Context, workflowID string) (bool, error) {
	if err := s.requireOpen(); err != nil {
		return false, err
	}
	if workflowID == "" {
		return false, fmt.Errorf("workflow id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin workflow delete: %w", err)
	}
	defer tx.Rollback()

	var instanceCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_instances WHERE workflow_id = ?`, workflowID).Scan(&instanceCount); err != nil {
		return false, fmt.Errorf("count workflow instances for workflow %s: %w", workflowID, err)
	}
	if instanceCount != 0 {
		return false, nil
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM workflows WHERE workflow_id = ?`, workflowID)
	if err != nil {
		return false, fmt.Errorf("delete workflow %s: %w", workflowID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("delete workflow %s: %w", workflowID, err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit workflow delete: %w", err)
	}
	return deleted, nil
}

func (s *Store) CreateWorkflowRun(ctx context.Context, run WorkflowRunRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := run.validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow run create: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getWorkflowRun(ctx, tx, run.ID)
	if err != nil {
		return err
	}
	if found {
		if existing != run {
			return fmt.Errorf("workflow run %s already exists with different values", run.ID)
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_instances (
		run_id,
		project_id,
		workflow_id,
		submission_context_json,
		created_at
	) VALUES (?, ?, ?, ?, ?)`,
		run.ID,
		run.ProjectID,
		run.WorkflowID,
		run.SubmissionContextJSON,
		run.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert workflow run %s: %w", run.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow run create: %w", err)
	}
	return nil
}

func (s *Store) UpdateWorkflowRunSubmissionContext(ctx context.Context, runID string, submissionContextJSON string) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	if !json.Valid([]byte(submissionContextJSON)) {
		return fmt.Errorf("run %s submission context json must be valid", runID)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE workflow_instances SET submission_context_json = ? WHERE run_id = ?`,
		submissionContextJSON,
		runID,
	)
	if err != nil {
		return fmt.Errorf("update workflow run %s submission context: %w", runID, err)
	}
	updated, err := rowsAffected(result)
	if err != nil {
		return fmt.Errorf("update workflow run %s submission context: %w", runID, err)
	}
	if !updated {
		return fmt.Errorf("workflow run %s not found", runID)
	}
	return nil
}

func (s *Store) GetWorkflowRun(ctx context.Context, runID string) (WorkflowRunRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkflowRunRecord{}, false, err
	}
	return getWorkflowRun(ctx, s.db, runID)
}

func (s *Store) InsertStagePlan(ctx context.Context, runID string, stages []WorkflowStageRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	if len(stages) == 0 {
		return fmt.Errorf("stage plan is required")
	}
	for index, stage := range stages {
		if err := stage.validate(); err != nil {
			return fmt.Errorf("stage %d: %w", index, err)
		}
		if stage.RunID != runID {
			return fmt.Errorf("stage %d run id %s does not match %s", index, stage.RunID, runID)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin stage plan insert: %w", err)
	}
	defer tx.Rollback()

	existing, err := listStagesForRun(ctx, tx, runID)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		if !sameStagePlan(existing, stages) {
			return fmt.Errorf("stage plan for run %s already exists with different values", runID)
		}
		return tx.Commit()
	}

	for _, stage := range stages {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_stages (
			run_id,
			stage_index,
			step_id,
			stage_source_reference,
			state,
			created_at,
			ready_at,
			started_at,
			completed_at,
			failed_at,
			output_json,
			output_json_sha256
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			stage.RunID,
			stage.StageIndex,
			stage.StepID,
			stage.StageSourceReference,
			stage.State,
			stage.CreatedAt,
			nullString(stage.ReadyAt),
			nullString(stage.StartedAt),
			nullString(stage.CompletedAt),
			nullString(stage.FailedAt),
			nullString(stage.OutputJSON),
			nullString(stage.OutputJSONSHA256),
		); err != nil {
			return fmt.Errorf("insert workflow stage %s/%d: %w", stage.RunID, stage.StageIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit stage plan insert: %w", err)
	}
	return nil
}

func (s *Store) GetWorkflowStage(ctx context.Context, runID string, stageIndex int) (WorkflowStageRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkflowStageRecord{}, false, err
	}
	return getWorkflowStage(ctx, s.db, runID, stageIndex)
}

func (s *Store) InsertWorkflowDependencySteps(ctx context.Context, steps []WorkflowDependencyStepRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := validateWorkflowDependencySteps(steps); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dependency step insert: %w", err)
	}
	defer tx.Rollback()

	steps = append([]WorkflowDependencyStepRecord(nil), steps...)
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].StageIndex == steps[j].StageIndex {
			return steps[i].StepIndex < steps[j].StepIndex
		}
		return steps[i].StageIndex < steps[j].StageIndex
	})

	existing, err := listWorkflowDependencyStepsForRun(ctx, tx, steps[0].RunID)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		if !sameWorkflowDependencyStepPlan(existing, steps) {
			return fmt.Errorf("dependency steps for run %s already exists with different values", steps[0].RunID)
		}
		return tx.Commit()
	}

	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_dependency_steps (
			run_id,
			stage_index,
			step_index,
			step_id,
			parallel_with,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
			step.RunID,
			step.StageIndex,
			step.StepIndex,
			step.StepID,
			step.ParallelWith,
			step.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert workflow dependency step %s/%d/%d: %w", step.RunID, step.StageIndex, step.StepIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dependency step insert: %w", err)
	}
	return nil
}

func (s *Store) ListWorkflowDependencySteps(ctx context.Context, runID string) ([]WorkflowDependencyStepRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	if runID == "" {
		return nil, fmt.Errorf("run id is required")
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		run_id,
		stage_index,
		step_index,
		step_id,
		parallel_with,
		created_at
	FROM workflow_dependency_steps
	WHERE run_id = ?
	ORDER BY stage_index, step_index`, runID)
	if err != nil {
		return nil, fmt.Errorf("list workflow dependency steps for run %s: %w", runID, err)
	}
	defer rows.Close()

	steps := []WorkflowDependencyStepRecord{}
	for rows.Next() {
		step, err := scanWorkflowDependencyStep(rows)
		if err != nil {
			return nil, fmt.Errorf("list workflow dependency steps for run %s: %w", runID, err)
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow dependency steps for run %s: %w", runID, err)
	}
	return steps, nil
}

func (s *Store) InsertWorkflowDependencyWorkItemMembership(ctx context.Context, items []WorkflowDependencyWorkItemRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := validateWorkflowDependencyWorkItemRecords(items); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dependency work item membership insert: %w", err)
	}
	defer tx.Rollback()

	for _, item := range items {
		existing, found, err := getWorkflowDependencyWorkItem(ctx, tx, item.RunID, item.WorkItemID)
		if err != nil {
			return err
		}
		if found {
			if existing != item {
				return fmt.Errorf("dependency work item membership %s already exists with different values", item.WorkItemID)
			}
			continue
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_dependency_work_items (
			run_id,
			stage_index,
			step_index,
			work_item_id,
			work_item_index,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
			item.RunID,
			item.StageIndex,
			item.StepIndex,
			item.WorkItemID,
			item.WorkItemIndex,
			item.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert dependency work item membership %s for step %s/%d: %w", item.WorkItemID, item.RunID, item.StepIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dependency work item membership insert: %w", err)
	}
	return nil
}

func (s *Store) ListWorkflowDependencyWorkItems(ctx context.Context, runID string, stageIndex int, stepIndex int) ([]WorkflowDependencyWorkItemRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	if runID == "" {
		return nil, fmt.Errorf("run id is required")
	}
	if stageIndex < 0 {
		return nil, fmt.Errorf("stage index must be non-negative")
	}
	if stepIndex < 0 {
		return nil, fmt.Errorf("step index must be non-negative")
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		run_id,
		stage_index,
		step_index,
		work_item_id,
		work_item_index,
		created_at
	FROM workflow_dependency_work_items
	WHERE run_id = ? AND stage_index = ? AND step_index = ?
	ORDER BY work_item_index, work_item_id`, runID, stageIndex, stepIndex)
	if err != nil {
		return nil, fmt.Errorf("list dependency work items for run %s/%d/%d: %w", runID, stageIndex, stepIndex, err)
	}
	defer rows.Close()

	items := []WorkflowDependencyWorkItemRecord{}
	for rows.Next() {
		item, err := scanWorkflowDependencyWorkItem(rows)
		if err != nil {
			return nil, fmt.Errorf("list dependency work items for run %s/%d/%d: %w", runID, stageIndex, stepIndex, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list dependency work items for run %s/%d/%d: %w", runID, stageIndex, stepIndex, err)
	}
	return items, nil
}

func (s *Store) UpsertWorkflowStepOutputFact(ctx context.Context, fact WorkflowStepOutputFactRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := fact.validate(); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `INSERT INTO workflow_step_output_facts (
		run_id,
		step_index,
		output_json,
		output_json_sha256,
		output_json_bytes,
		output_json_pruned,
		output_kind,
		created_at,
		updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (run_id, step_index) DO UPDATE SET
		output_json = excluded.output_json,
		output_json_sha256 = excluded.output_json_sha256,
		output_json_bytes = excluded.output_json_bytes,
		output_json_pruned = excluded.output_json_pruned,
		output_kind = excluded.output_kind,
		updated_at = excluded.updated_at`,
		fact.RunID,
		fact.StepIndex,
		nullString(fact.OutputJSON),
		fact.OutputJSONSHA256,
		fact.OutputJSONBytes,
		fact.OutputJSONPruned,
		fact.OutputKind,
		fact.CreatedAt,
		fact.UpdatedAt,
	); err != nil {
		return fmt.Errorf("upsert workflow step output fact %s/%d: %w", fact.RunID, fact.StepIndex, err)
	}
	return nil
}

func (s *Store) GetWorkflowStepOutputFact(ctx context.Context, runID string, stepIndex int) (WorkflowStepOutputFactRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkflowStepOutputFactRecord{}, false, err
	}
	return getWorkflowStepOutputFact(ctx, s.db, runID, stepIndex)
}

func (s *Store) ListActiveWorkflowRuns(ctx context.Context) ([]WorkflowRunRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		run_id,
		project_id,
		workflow_id,
		submission_context_json,
		created_at
	FROM workflow_instances
	WHERE NOT EXISTS (
		SELECT 1
		FROM workflow_stages
		WHERE workflow_stages.run_id = workflow_instances.run_id
	)
	OR EXISTS (
		SELECT 1
		FROM workflow_stages
		WHERE workflow_stages.run_id = workflow_instances.run_id
		AND workflow_stages.state NOT IN ('completed', 'failed', 'skipped')
	)
	ORDER BY created_at, run_id`)
	if err != nil {
		return nil, fmt.Errorf("list active workflow runs: %w", err)
	}
	defer rows.Close()

	runs := []WorkflowRunRecord{}
	for rows.Next() {
		run, err := scanWorkflowRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list active workflow runs: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active workflow runs: %w", err)
	}
	return runs, nil
}

func (s *Store) InsertWorkItems(ctx context.Context, items []WorkItemRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := validateWorkItems(items); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin work item insert: %w", err)
	}
	defer tx.Rollback()

	if err := insertWorkItems(ctx, tx, items); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit work item insert: %w", err)
	}
	return nil
}

func (s *Store) QueueWorkItems(ctx context.Context, request QueueWorkItemsRequest) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := request.validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queued work item insert: %w", err)
	}
	defer tx.Rollback()

	if err := insertWorkItems(ctx, tx, request.WorkItems); err != nil {
		return err
	}
	if len(request.ResourceConstraints) != 0 {
		if err := insertWorkItemResourceConstraints(ctx, tx, request.ResourceConstraints); err != nil {
			return err
		}
	}
	if err := enqueueWorkItems(ctx, tx, request.QueuedWork); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit queued work item insert: %w", err)
	}
	return nil
}

func (s *Store) EnqueueWorkItems(ctx context.Context, items []QueuedWorkRecord) error {
	if err := s.requireOpen(); err != nil {
		return err
	}
	if err := validateQueuedWorkItems(items); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queued work insert: %w", err)
	}
	defer tx.Rollback()

	if err := enqueueWorkItems(ctx, tx, items); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit queued work insert: %w", err)
	}
	return nil
}

func (s *Store) GetWorkItem(ctx context.Context, workItemID string) (WorkItemRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return WorkItemRecord{}, false, err
	}
	return getWorkItem(ctx, s.db, workItemID)
}

func (s *Store) ListWorkItemsForRun(ctx context.Context, runID string) ([]WorkItemRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	if runID == "" {
		return nil, fmt.Errorf("run id is required")
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		work_item_id,
		run_id,
		stage_index,
		work_item_index,
		worker_payload_json,
		resolved_inputs_sha256,
		created_at
	FROM work_items
	WHERE run_id = ?
	ORDER BY stage_index, work_item_index, work_item_id`, runID)
	if err != nil {
		return nil, fmt.Errorf("list work items for run %s: %w", runID, err)
	}
	defer rows.Close()

	items := []WorkItemRecord{}
	for rows.Next() {
		item, err := scanWorkItem(rows)
		if err != nil {
			return nil, fmt.Errorf("list work items for run %s: %w", runID, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list work items for run %s: %w", runID, err)
	}
	return items, nil
}

func (s *Store) ListWorkItemResourceConstraints(ctx context.Context, workItemID string) ([]WorkItemResourceConstraintRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	if workItemID == "" {
		return nil, fmt.Errorf("work item id is required")
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		work_item_id,
		constraint_index,
		resource_key,
		requested_units,
		operator,
		target_units,
		created_at
	FROM work_item_resource_constraints
	WHERE work_item_id = ?
	ORDER BY constraint_index`, workItemID)
	if err != nil {
		return nil, fmt.Errorf("list work item resource constraints %s: %w", workItemID, err)
	}
	defer rows.Close()

	constraints := []WorkItemResourceConstraintRecord{}
	for rows.Next() {
		var constraint WorkItemResourceConstraintRecord
		if err := rows.Scan(
			&constraint.WorkItemID,
			&constraint.ConstraintIndex,
			&constraint.ResourceKey,
			&constraint.RequestedUnits,
			&constraint.Operator,
			&constraint.TargetUnits,
			&constraint.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("list work item resource constraints %s: %w", workItemID, err)
		}
		constraints = append(constraints, constraint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list work item resource constraints %s: %w", workItemID, err)
	}
	return constraints, nil
}

func (s *Store) ListQueuedResourceConstraintChecks(ctx context.Context) ([]QueuedResourceConstraintCheckRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		work_item_id,
		queued_at,
		constraint_index,
		resource_key,
		total_units,
		requested_units,
		operator,
		target_units
	FROM queued_resource_constraint_checks
	ORDER BY queued_at, work_item_id, constraint_index`)
	if err != nil {
		return nil, fmt.Errorf("list queued resource constraint checks: %w", err)
	}
	defer rows.Close()

	checks := []QueuedResourceConstraintCheckRecord{}
	for rows.Next() {
		check, err := scanQueuedResourceConstraintCheck(rows)
		if err != nil {
			return nil, fmt.Errorf("list queued resource constraint checks: %w", err)
		}
		checks = append(checks, check)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list queued resource constraint checks: %w", err)
	}
	return checks, nil
}

func (s *Store) ListQueuedWorkItems(ctx context.Context) ([]QueuedWorkRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		work_items.work_item_id,
		work_items.run_id,
		work_items.stage_index,
		work_items.work_item_index,
		work_items.worker_payload_json,
		work_items.resolved_inputs_sha256,
		work_items.created_at,
		queued_work.queued_at,
		COALESCE(queued_work.resume_artifact_id, '')
	FROM queued_work
	JOIN work_items ON work_items.work_item_id = queued_work.work_item_id
	ORDER BY queued_work.queued_at, queued_work.work_item_id`)
	if err != nil {
		return nil, fmt.Errorf("list queued work items: %w", err)
	}
	defer rows.Close()

	items := []QueuedWorkRecord{}
	for rows.Next() {
		item, err := scanQueuedWork(rows)
		if err != nil {
			return nil, fmt.Errorf("list queued work items: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list queued work items: %w", err)
	}
	return items, nil
}

func (s *Store) CountWorkItemsForStage(ctx context.Context, runID string, stageIndex int) (WorkItemStatusCounts, error) {
	if err := s.requireOpen(); err != nil {
		return WorkItemStatusCounts{}, err
	}
	if runID == "" {
		return WorkItemStatusCounts{}, fmt.Errorf("run id is required")
	}
	if stageIndex < 0 {
		return WorkItemStatusCounts{}, fmt.Errorf("stage index must be non-negative")
	}

	var counts WorkItemStatusCounts
	queries := []struct {
		name string
		sql  string
		dest *int
	}{
		{name: "queued", sql: `SELECT COUNT(*) FROM queued_work JOIN work_items ON work_items.work_item_id = queued_work.work_item_id WHERE work_items.run_id = ? AND work_items.stage_index = ?`, dest: &counts.Queued},
		{name: "running", sql: `SELECT COUNT(*) FROM running_work JOIN work_items ON work_items.work_item_id = running_work.work_item_id WHERE work_items.run_id = ? AND work_items.stage_index = ?`, dest: &counts.Running},
		{name: "completed", sql: `SELECT COUNT(*) FROM completed_work JOIN work_items ON work_items.work_item_id = completed_work.work_item_id WHERE work_items.run_id = ? AND work_items.stage_index = ?`, dest: &counts.Completed},
		{name: "failed", sql: `SELECT COUNT(*) FROM failed_work JOIN work_items ON work_items.work_item_id = failed_work.work_item_id WHERE work_items.run_id = ? AND work_items.stage_index = ?`, dest: &counts.Failed},
	}
	for _, query := range queries {
		if err := s.db.QueryRowContext(ctx, query.sql, runID, stageIndex).Scan(query.dest); err != nil {
			return WorkItemStatusCounts{}, fmt.Errorf("count %s work items for stage %s/%d: %w", query.name, runID, stageIndex, err)
		}
	}
	return counts, nil
}

func (s *Store) CountWorkItemsForRun(ctx context.Context, runID string) (RunWorkStatusCounts, error) {
	if err := s.requireOpen(); err != nil {
		return RunWorkStatusCounts{}, err
	}
	if runID == "" {
		return RunWorkStatusCounts{}, fmt.Errorf("run id is required")
	}

	var counts RunWorkStatusCounts
	queries := []struct {
		name string
		sql  string
		dest *int
	}{
		{name: "queued", sql: `SELECT COUNT(*) FROM queued_work JOIN work_items ON work_items.work_item_id = queued_work.work_item_id WHERE work_items.run_id = ?`, dest: &counts.Queued},
		{name: "running", sql: `SELECT COUNT(*) FROM running_work JOIN work_items ON work_items.work_item_id = running_work.work_item_id WHERE work_items.run_id = ?`, dest: &counts.Running},
		{name: "completed", sql: `SELECT COUNT(*) FROM completed_work JOIN work_items ON work_items.work_item_id = completed_work.work_item_id WHERE work_items.run_id = ?`, dest: &counts.Completed},
		{name: "failed", sql: `SELECT COUNT(*) FROM failed_work JOIN work_items ON work_items.work_item_id = failed_work.work_item_id WHERE work_items.run_id = ?`, dest: &counts.Failed},
	}
	for _, query := range queries {
		if err := s.db.QueryRowContext(ctx, query.sql, runID).Scan(query.dest); err != nil {
			return RunWorkStatusCounts{}, fmt.Errorf("count %s work items for run %s: %w", query.name, runID, err)
		}
	}
	return counts, nil
}

func (s *Store) ListRunningWork(ctx context.Context) ([]RunningWorkRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, runningWorkSelectSQL()+`
	ORDER BY running_work.started_at, running_work.attempt_id`)
	if err != nil {
		return nil, fmt.Errorf("list running work: %w", err)
	}
	defer rows.Close()

	records := []RunningWorkRecord{}
	for rows.Next() {
		record, err := scanRunningWork(rows)
		if err != nil {
			return nil, fmt.Errorf("list running work: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list running work: %w", err)
	}
	return records, nil
}

func (s *Store) GetRunningWork(ctx context.Context, attemptID string) (RunningWorkRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return RunningWorkRecord{}, false, err
	}
	if attemptID == "" {
		return RunningWorkRecord{}, false, fmt.Errorf("attempt id is required")
	}

	record, err := scanRunningWork(s.db.QueryRowContext(ctx, runningWorkSelectSQL()+`
	WHERE running_work.attempt_id = ?`, attemptID))
	if err == sql.ErrNoRows {
		return RunningWorkRecord{}, false, nil
	}
	if err != nil {
		return RunningWorkRecord{}, false, fmt.Errorf("get running work %s: %w", attemptID, err)
	}
	return record, true, nil
}

func (s *Store) ListTerminalAttemptsForRun(ctx context.Context, runID string) ([]TerminalAttemptRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	if runID == "" {
		return nil, fmt.Errorf("run id is required")
	}

	rows, err := s.db.QueryContext(ctx, terminalAttemptSelectSQL()+`
	WHERE run_id = ?
	ORDER BY finished_at, attempt_id`, runID)
	if err != nil {
		return nil, fmt.Errorf("list terminal attempts for run %s: %w", runID, err)
	}
	defer rows.Close()

	records := []TerminalAttemptRecord{}
	for rows.Next() {
		record, err := scanTerminalAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("list terminal attempts for run %s: %w", runID, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list terminal attempts for run %s: %w", runID, err)
	}
	return records, nil
}

func (s *Store) CompleteStageIfReady(ctx context.Context, request CompleteStageRequest) (CompleteStageResult, error) {
	if err := s.requireOpen(); err != nil {
		return CompleteStageResult{}, err
	}
	if err := request.validate(); err != nil {
		return CompleteStageResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompleteStageResult{}, fmt.Errorf("begin complete stage: %w", err)
	}
	defer tx.Rollback()

	stage, found, err := getWorkflowStage(ctx, tx, request.RunID, request.StageIndex)
	if err != nil {
		return CompleteStageResult{}, err
	}
	if !found {
		return CompleteStageResult{Found: false}, nil
	}
	if stage.State == "completed" {
		if !stageMatchesCompletionRequest(stage, request) {
			return CompleteStageResult{}, fmt.Errorf("stage %s/%d already completed with different values", request.RunID, request.StageIndex)
		}
		return CompleteStageResult{Stage: stage, Found: true, Completed: true}, nil
	}
	if stage.State == "failed" || stage.State == "skipped" || stage.State == "blocked" {
		return CompleteStageResult{}, fmt.Errorf("stage %s/%d is %s and cannot be completed", request.RunID, request.StageIndex, stage.State)
	}

	ready, err := stageWorkComplete(ctx, tx, request.RunID, request.StageIndex)
	if err != nil {
		return CompleteStageResult{}, err
	}
	if !ready {
		return CompleteStageResult{Stage: stage, Found: true, Completed: false}, nil
	}

	if _, err := tx.ExecContext(ctx, `UPDATE workflow_stages
	SET state = 'completed',
		completed_at = ?,
		output_json = ?,
		output_json_sha256 = ?
	WHERE run_id = ? AND stage_index = ?`,
		request.CompletedAt,
		request.OutputJSON,
		request.OutputJSONSHA256,
		request.RunID,
		request.StageIndex,
	); err != nil {
		return CompleteStageResult{}, fmt.Errorf("complete stage %s/%d: %w", request.RunID, request.StageIndex, err)
	}

	if len(request.ReadyWorkItems) != 0 {
		if err := insertWorkItems(ctx, tx, request.ReadyWorkItems); err != nil {
			return CompleteStageResult{}, err
		}
	}
	if len(request.ReadyResourceConstraints) != 0 {
		if err := insertWorkItemResourceConstraints(ctx, tx, request.ReadyResourceConstraints); err != nil {
			return CompleteStageResult{}, err
		}
	}
	if len(request.ReadyQueuedWork) != 0 {
		if err := enqueueWorkItems(ctx, tx, request.ReadyQueuedWork); err != nil {
			return CompleteStageResult{}, err
		}
	}

	stage.State = "completed"
	stage.CompletedAt = request.CompletedAt
	stage.OutputJSON = request.OutputJSON
	stage.OutputJSONSHA256 = request.OutputJSONSHA256
	if err := tx.Commit(); err != nil {
		return CompleteStageResult{}, fmt.Errorf("commit complete stage: %w", err)
	}
	return CompleteStageResult{Stage: stage, Found: true, Completed: true}, nil
}

func (s *Store) ConfirmCheckpoint(ctx context.Context, request ConfirmCheckpointRequest) (ConfirmCheckpointResult, error) {
	if err := s.requireOpen(); err != nil {
		return ConfirmCheckpointResult{}, err
	}
	artifact, suspendReason, err := validateConfirmCheckpointRequest(request)
	if err != nil {
		return ConfirmCheckpointResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ConfirmCheckpointResult{}, fmt.Errorf("begin confirm checkpoint: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getResumeArtifact(ctx, tx, artifact.ID)
	if err != nil {
		return ConfirmCheckpointResult{}, err
	}
	if found {
		if !resumeArtifactMatchesConfirmation(existing, artifact) {
			return ConfirmCheckpointResult{}, fmt.Errorf("checkpoint artifact %s conflicts with existing accepted artifact", artifact.ID)
		}
		return replayCheckpointConfirmation(ctx, tx, request, existing, suspendReason)
	}

	running, found, err := getRunningWork(ctx, tx, request.AttemptID)
	if err != nil {
		return ConfirmCheckpointResult{}, fmt.Errorf("get running work %s: %w", request.AttemptID, err)
	}
	if !found {
		return ConfirmCheckpointResult{}, ErrAssignmentNoLongerOwned
	}
	if err := validateRunningAssignmentOwner(ctx, tx, running, request.WorkerID, request.WorkerSessionID, request.LiveSessionCutoff); err != nil {
		return ConfirmCheckpointResult{}, err
	}
	if artifact.WorkItemID != running.workItemID || artifact.ProducingAttemptID != running.attemptID {
		return ConfirmCheckpointResult{}, fmt.Errorf("checkpoint artifact identity does not match running assignment")
	}
	if err := validateNextCheckpointGeneration(ctx, tx, running, artifact); err != nil {
		return ConfirmCheckpointResult{}, err
	}
	if err := insertResumeArtifact(ctx, tx, artifact); err != nil {
		return ConfirmCheckpointResult{}, err
	}
	if err := assignAttemptExecutionLineage(ctx, tx, running.attemptID, artifact.ExecutionLineageID); err != nil {
		return ConfirmCheckpointResult{}, err
	}

	result := ConfirmCheckpointResult{Artifact: artifact}
	if request.Disposition == CheckpointDispositionSuspend {
		suspended, err := suspendRunningWork(
			ctx,
			tx,
			running,
			artifact.ID,
			request.SuspendedAt,
			suspendReason,
		)
		if err != nil {
			return ConfirmCheckpointResult{}, err
		}
		result.Suspended = &suspended
		result.Transitioned = true
	}

	if err := tx.Commit(); err != nil {
		return ConfirmCheckpointResult{}, fmt.Errorf("commit confirm checkpoint: %w", err)
	}
	return result, nil
}

func (s *Store) SuspendFromLatestCheckpoint(ctx context.Context, request SuspendFromLatestCheckpointRequest) (SuspendFromLatestCheckpointResult, error) {
	if err := s.requireOpen(); err != nil {
		return SuspendFromLatestCheckpointResult{}, err
	}
	if err := request.validate(); err != nil {
		return SuspendFromLatestCheckpointResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SuspendFromLatestCheckpointResult{}, fmt.Errorf("begin suspend from latest checkpoint: %w", err)
	}
	defer tx.Rollback()

	running, found, err := getRunningWork(ctx, tx, request.AttemptID)
	if err != nil {
		return SuspendFromLatestCheckpointResult{}, fmt.Errorf("get running work %s: %w", request.AttemptID, err)
	}
	if !found {
		existing, suspended, err := getSuspendedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return SuspendFromLatestCheckpointResult{}, err
		}
		if !suspended {
			return SuspendFromLatestCheckpointResult{}, ErrAssignmentNoLongerOwned
		}
		if !suspendedWorkMatchesFallbackRequest(existing, request) {
			return SuspendFromLatestCheckpointResult{}, fmt.Errorf("suspend attempt %s conflicts with existing suspended work", request.AttemptID)
		}
		artifact, found, err := getResumeArtifact(ctx, tx, existing.ResumeArtifactID)
		if err != nil {
			return SuspendFromLatestCheckpointResult{}, err
		}
		if !found {
			return SuspendFromLatestCheckpointResult{}, fmt.Errorf("suspended attempt %s references missing artifact", request.AttemptID)
		}
		return SuspendFromLatestCheckpointResult{
			Artifact:     artifact,
			Suspended:    existing,
			Found:        true,
			Transitioned: false,
		}, nil
	}
	if err := validateRunningAssignmentOwner(ctx, tx, running, request.WorkerID, request.WorkerSessionID, request.LiveSessionCutoff); err != nil {
		return SuspendFromLatestCheckpointResult{}, err
	}

	artifact, found, err := latestAcceptedCheckpointForAttempt(ctx, tx, running.attemptID)
	if err != nil {
		return SuspendFromLatestCheckpointResult{}, err
	}
	if !found {
		if err := tx.Commit(); err != nil {
			return SuspendFromLatestCheckpointResult{}, fmt.Errorf("commit empty suspend from latest checkpoint: %w", err)
		}
		return SuspendFromLatestCheckpointResult{}, nil
	}

	suspended, err := suspendRunningWork(
		ctx,
		tx,
		running,
		artifact.ID,
		request.SuspendedAt,
		request.SuspendReason,
	)
	if err != nil {
		return SuspendFromLatestCheckpointResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SuspendFromLatestCheckpointResult{}, fmt.Errorf("commit suspend from latest checkpoint: %w", err)
	}
	return SuspendFromLatestCheckpointResult{
		Artifact:     artifact,
		Suspended:    suspended,
		Found:        true,
		Transitioned: true,
	}, nil
}

func (s *Store) FailPendingResumeAttemptLimit(ctx context.Context, request FailPendingResumeAttemptLimitRequest) (FailPendingResumeAttemptLimitResult, error) {
	if err := s.requireOpen(); err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}
	if err := request.validate(); err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("begin fail pending resume attempt limit: %w", err)
	}
	defer tx.Rollback()

	if existing, found, err := getFailedWork(ctx, tx, request.AttemptID); err != nil {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("get retry-limit failed work %s: %w", request.AttemptID, err)
	} else if found {
		return replayPendingResumeAttemptLimitFailure(ctx, tx, request, existing)
	}

	queued, found, err := getQueuedResumeWork(ctx, tx, request.WorkItemID)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}
	if !found {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("pending resume work %s no longer exists", request.WorkItemID)
	}
	if queued.ResumeArtifactID != request.ResumeArtifactID || queued.QueuedAt != request.QueuedAt {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("pending resume work %s changed before limit terminalization", request.WorkItemID)
	}

	artifact, found, err := getResumeArtifact(ctx, tx, request.ResumeArtifactID)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}
	if !found {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("pending resume artifact %s no longer exists", request.ResumeArtifactID)
	}
	if artifact.WorkItemID != request.WorkItemID || artifact.ExecutionLineageID != request.ExecutionLineageID {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("pending resume artifact %s changed before limit terminalization", request.ResumeArtifactID)
	}
	nextAttemptNumber, err := nextResumeAttemptNumber(ctx, tx, artifact.ID)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}
	if nextAttemptNumber != request.NextResumeAttemptNumber || nextAttemptNumber <= request.ConfiguredLimit {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("pending resume work %s is not over the configured attempt limit", request.WorkItemID)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		executor_type,
		started_at,
		resumed_from_attempt_id,
		resume_artifact_id,
		execution_lineage_id,
		resume_attempt_number
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		request.AttemptID,
		request.WorkItemID,
		ExecutorTypeController,
		request.FailedAt,
		artifact.ProducingAttemptID,
		artifact.ID,
		artifact.ExecutionLineageID,
		nextAttemptNumber,
	); err != nil {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("insert retry-limit attempt %s: %w", request.AttemptID, err)
	}

	failed := FailedWorkRecord{
		AttemptID:  request.AttemptID,
		WorkItemID: request.WorkItemID,
		Error:      "resume_attempt_limit_exhausted",
		QueuedAt:   request.QueuedAt,
		StartedAt:  request.FailedAt,
		FailedAt:   request.FailedAt,
	}
	if err := insertFailedWork(ctx, tx, failed); err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM queued_work
		WHERE work_item_id = ?
			AND resume_artifact_id = ?
			AND queued_at = ?`, request.WorkItemID, request.ResumeArtifactID, request.QueuedAt)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("delete retry-exhausted pending work %s: %w", request.WorkItemID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("delete retry-exhausted pending work %s: %w", request.WorkItemID, err)
	}
	if !deleted {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("pending resume work %s changed before limit terminalization", request.WorkItemID)
	}

	if err := tx.Commit(); err != nil {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("commit fail pending resume attempt limit: %w", err)
	}
	return FailPendingResumeAttemptLimitResult{
		WorkItem:     queued.WorkItemRecord,
		Failed:       failed,
		Terminalized: true,
	}, nil
}

func (s *Store) GetResumeArtifact(ctx context.Context, artifactID string) (ResumeArtifactRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return ResumeArtifactRecord{}, false, err
	}
	if artifactID == "" {
		return ResumeArtifactRecord{}, false, fmt.Errorf("resume artifact id is required")
	}
	return getResumeArtifact(ctx, s.db, artifactID)
}

func (s *Store) GetLatestAcceptedCheckpoint(ctx context.Context, executionLineageID string) (ResumeArtifactRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return ResumeArtifactRecord{}, false, err
	}
	if executionLineageID == "" {
		return ResumeArtifactRecord{}, false, fmt.Errorf("execution lineage id is required")
	}
	return getLatestResumeArtifactForLineage(ctx, s.db, executionLineageID)
}

func (s *Store) ListSuspendedWorkForItem(ctx context.Context, workItemID string) ([]SuspendedWorkRecord, error) {
	if err := s.requireOpen(); err != nil {
		return nil, err
	}
	if workItemID == "" {
		return nil, fmt.Errorf("work item id is required")
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		resume_artifact_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at,
		suspended_at,
		suspend_reason
	FROM suspended_work
	WHERE work_item_id = ?
	ORDER BY suspended_at, attempt_id`, workItemID)
	if err != nil {
		return nil, fmt.Errorf("list suspended work for item %s: %w", workItemID, err)
	}
	defer rows.Close()

	records := []SuspendedWorkRecord{}
	for rows.Next() {
		record, err := scanSuspendedWork(rows)
		if err != nil {
			return nil, fmt.Errorf("list suspended work for item %s: %w", workItemID, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list suspended work for item %s: %w", workItemID, err)
	}
	return records, nil
}

func (s *Store) ClaimNextWork(ctx context.Context, request ClaimWorkRequest) (ClaimedWorkRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return ClaimedWorkRecord{}, false, err
	}
	if err := request.validate(); err != nil {
		return ClaimedWorkRecord{}, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("begin claim next work: %w", err)
	}
	defer tx.Rollback()

	if request.ExecutorType == ExecutorTypeWorker {
		if err := validateWorkerSessionCanClaim(ctx, tx, request); err != nil {
			return ClaimedWorkRecord{}, false, err
		}
	}

	queuedItems, err := listQueuedWorkForClaim(ctx, tx)
	if err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("claim next work: %w", err)
	}

	for _, queued := range queuedItems {
		allowed, err := queuedResourceConstraintsAllow(ctx, tx, queued.ID)
		if err != nil {
			return ClaimedWorkRecord{}, false, fmt.Errorf("claim queued work %s: %w", queued.ID, err)
		}
		if !allowed {
			continue
		}
		return claimQueuedWork(ctx, tx, request, queued)
	}

	if err := tx.Commit(); err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("commit empty claim next work: %w", err)
	}
	return ClaimedWorkRecord{}, false, nil
}

func claimQueuedWork(ctx context.Context, tx *sql.Tx, request ClaimWorkRequest, queued QueuedWorkRecord) (ClaimedWorkRecord, bool, error) {
	var resumeArtifact *ResumeArtifactRecord
	var resumedFromAttemptID string
	var executionLineageID string
	var resumeAttemptNumber int
	if queued.ResumeArtifactID != "" {
		if request.ResumeAttemptLimit <= 0 {
			return ClaimedWorkRecord{}, false, fmt.Errorf("resume attempt limit must be positive for pending resume work")
		}
		artifact, found, err := getResumeArtifact(ctx, tx, queued.ResumeArtifactID)
		if err != nil {
			return ClaimedWorkRecord{}, false, err
		}
		if !found {
			return ClaimedWorkRecord{}, false, fmt.Errorf("queued work %s references missing resume artifact", queued.ID)
		}
		if artifact.WorkItemID != queued.ID {
			return ClaimedWorkRecord{}, false, fmt.Errorf("queued work %s resume artifact belongs to another work item", queued.ID)
		}
		resumeAttemptNumber, err = nextResumeAttemptNumber(ctx, tx, artifact.ID)
		if err != nil {
			return ClaimedWorkRecord{}, false, err
		}
		if resumeAttemptNumber > request.ResumeAttemptLimit {
			return ClaimedWorkRecord{}, false, &ResumeAttemptLimitExceededError{
				WorkItemID:              queued.ID,
				ResumeArtifactID:        artifact.ID,
				ExecutionLineageID:      artifact.ExecutionLineageID,
				NextResumeAttemptNumber: resumeAttemptNumber,
				ConfiguredLimit:         request.ResumeAttemptLimit,
				QueuedAt:                queued.QueuedAt,
			}
		}
		resumedFromAttemptID = artifact.ProducingAttemptID
		executionLineageID = artifact.ExecutionLineageID
		resumeArtifact = &artifact
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO work_item_attempts (
		attempt_id,
		work_item_id,
		worker_id,
		worker_session_id,
		executor_type,
		started_at,
		resumed_from_attempt_id,
		resume_artifact_id,
		execution_lineage_id,
		resume_attempt_number
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		request.AttemptID,
		queued.ID,
		nullString(request.WorkerID),
		nullString(request.WorkerSessionID),
		request.ExecutorType,
		request.StartedAt,
		nullString(resumedFromAttemptID),
		nullString(queued.ResumeArtifactID),
		nullString(executionLineageID),
		nullPositiveInt(resumeAttemptNumber),
	); err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("insert work item attempt %s: %w", request.AttemptID, err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO running_work (
		attempt_id,
		work_item_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at
	) VALUES (?, ?, ?, ?, ?, ?)`,
		request.AttemptID,
		queued.ID,
		nullString(request.WorkerID),
		nullString(request.WorkerSessionID),
		queued.QueuedAt,
		request.StartedAt,
	); err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("insert running work %s: %w", request.AttemptID, err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM queued_work WHERE work_item_id = ?`, queued.ID)
	if err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("delete queued work %s: %w", queued.ID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("delete queued work %s: %w", queued.ID, err)
	}
	if !deleted {
		return ClaimedWorkRecord{}, false, fmt.Errorf("delete queued work %s: no row deleted", queued.ID)
	}

	claimed := ClaimedWorkRecord{
		AttemptID:            request.AttemptID,
		WorkItem:             queued.WorkItemRecord,
		WorkerID:             request.WorkerID,
		WorkerSessionID:      request.WorkerSessionID,
		ExecutorType:         request.ExecutorType,
		QueuedAt:             queued.QueuedAt,
		StartedAt:            request.StartedAt,
		ResumedFromAttemptID: resumedFromAttemptID,
		ExecutionLineageID:   executionLineageID,
		ResumeAttemptNumber:  resumeAttemptNumber,
		ResumeArtifact:       resumeArtifact,
	}
	if err := tx.Commit(); err != nil {
		return ClaimedWorkRecord{}, false, fmt.Errorf("commit claim next work: %w", err)
	}
	return claimed, true, nil
}

func validateWorkerSessionCanClaim(ctx context.Context, tx *sql.Tx, request ClaimWorkRequest) error {
	session, found, err := getWorkerSession(ctx, tx, request.WorkerID, request.WorkerSessionID)
	if err != nil {
		return err
	}
	if !found || session.Status != WorkerSessionStatusActive {
		return ErrWorkerSessionNotActive
	}
	if request.LiveSessionCutoff != "" && session.LastHeartbeatAt < request.LiveSessionCutoff {
		return ErrWorkerSessionNotActive
	}

	var runningAssignments int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM running_work WHERE worker_session_id = ?`, request.WorkerSessionID).Scan(&runningAssignments); err != nil {
		return fmt.Errorf("count running assignments for worker session %s: %w", request.WorkerSessionID, err)
	}
	if runningAssignments != 0 {
		return ErrWorkerSessionBusy
	}
	return nil
}

func (s *Store) CompleteAttempt(ctx context.Context, request CompleteAttemptRequest) (CompletedWorkRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return CompletedWorkRecord{}, false, err
	}
	if err := request.validate(); err != nil {
		return CompletedWorkRecord{}, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompletedWorkRecord{}, false, fmt.Errorf("begin complete attempt: %w", err)
	}
	defer tx.Rollback()

	running, found, err := getRunningWork(ctx, tx, request.AttemptID)
	if err != nil {
		return CompletedWorkRecord{}, false, fmt.Errorf("get running work %s: %w", request.AttemptID, err)
	}
	if !found {
		existing, completed, err := getCompletedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return CompletedWorkRecord{}, false, fmt.Errorf("get completed work %s: %w", request.AttemptID, err)
		}
		if completed {
			if !completedWorkMatchesRequest(existing, request) {
				return CompletedWorkRecord{}, false, fmt.Errorf("complete attempt %s conflicts with existing completed work", request.AttemptID)
			}
			return existing, true, nil
		}
		_, failed, err := getFailedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return CompletedWorkRecord{}, false, fmt.Errorf("get failed work %s: %w", request.AttemptID, err)
		}
		if failed {
			return CompletedWorkRecord{}, false, fmt.Errorf("complete attempt %s conflicts with existing failed work", request.AttemptID)
		}
		if request.WorkerID != "" {
			abandoned, err := hasAbandonedWork(ctx, tx, request.AttemptID)
			if err != nil {
				return CompletedWorkRecord{}, false, err
			}
			if abandoned {
				return CompletedWorkRecord{}, false, ErrAssignmentNoLongerOwned
			}
			suspended, err := hasSuspendedWork(ctx, tx, request.AttemptID)
			if err != nil {
				return CompletedWorkRecord{}, false, err
			}
			if suspended {
				return CompletedWorkRecord{}, false, ErrAssignmentNoLongerOwned
			}
		}
		return CompletedWorkRecord{}, false, nil
	}
	if err := validateRunningAssignmentOwner(ctx, tx, running, request.WorkerID, request.WorkerSessionID, request.LiveSessionCutoff); err != nil {
		return CompletedWorkRecord{}, false, err
	}

	completed := CompletedWorkRecord{
		AttemptID:        request.AttemptID,
		WorkItemID:       running.workItemID,
		SkippedParentID:  request.SkippedParentID,
		OutputJSON:       request.OutputJSON,
		OutputJSONSHA256: request.OutputJSONSHA256,
		PreStateSHA256:   request.PreStateSHA256,
		PostStateSHA256:  request.PostStateSHA256,
		QueuedAt:         running.queuedAt,
		StartedAt:        running.startedAt,
		CompletedAt:      request.CompletedAt,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO completed_work (
		attempt_id,
		work_item_id,
		skipped_parent_id,
		output_json,
		output_json_sha256,
		pre_state_sha256,
		post_state_sha256,
		queued_at,
		started_at,
		completed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		completed.AttemptID,
		completed.WorkItemID,
		nullString(completed.SkippedParentID),
		completed.OutputJSON,
		completed.OutputJSONSHA256,
		completed.PreStateSHA256,
		completed.PostStateSHA256,
		completed.QueuedAt,
		completed.StartedAt,
		completed.CompletedAt,
	); err != nil {
		return CompletedWorkRecord{}, false, fmt.Errorf("insert completed work %s: %w", request.AttemptID, err)
	}
	if err := deleteRunningWork(ctx, tx, request.AttemptID); err != nil {
		return CompletedWorkRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return CompletedWorkRecord{}, false, fmt.Errorf("commit complete attempt: %w", err)
	}
	return completed, true, nil
}

func (s *Store) FailAttempt(ctx context.Context, request FailAttemptRequest) (FailedWorkRecord, bool, error) {
	if err := s.requireOpen(); err != nil {
		return FailedWorkRecord{}, false, err
	}
	if err := request.validate(); err != nil {
		return FailedWorkRecord{}, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FailedWorkRecord{}, false, fmt.Errorf("begin fail attempt: %w", err)
	}
	defer tx.Rollback()

	running, found, err := getRunningWork(ctx, tx, request.AttemptID)
	if err != nil {
		return FailedWorkRecord{}, false, fmt.Errorf("get running work %s: %w", request.AttemptID, err)
	}
	if !found {
		existing, failed, err := getFailedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return FailedWorkRecord{}, false, fmt.Errorf("get failed work %s: %w", request.AttemptID, err)
		}
		if failed {
			if !failedWorkMatchesRequest(existing, request) {
				return FailedWorkRecord{}, false, fmt.Errorf("fail attempt %s conflicts with existing failed work", request.AttemptID)
			}
			return existing, true, nil
		}
		_, completed, err := getCompletedWork(ctx, tx, request.AttemptID)
		if err != nil {
			return FailedWorkRecord{}, false, fmt.Errorf("get completed work %s: %w", request.AttemptID, err)
		}
		if completed {
			return FailedWorkRecord{}, false, fmt.Errorf("fail attempt %s conflicts with existing completed work", request.AttemptID)
		}
		if request.WorkerID != "" {
			abandoned, err := hasAbandonedWork(ctx, tx, request.AttemptID)
			if err != nil {
				return FailedWorkRecord{}, false, err
			}
			if abandoned {
				return FailedWorkRecord{}, false, ErrAssignmentNoLongerOwned
			}
			suspended, err := hasSuspendedWork(ctx, tx, request.AttemptID)
			if err != nil {
				return FailedWorkRecord{}, false, err
			}
			if suspended {
				return FailedWorkRecord{}, false, ErrAssignmentNoLongerOwned
			}
		}
		return FailedWorkRecord{}, false, nil
	}
	if err := validateRunningAssignmentOwner(ctx, tx, running, request.WorkerID, request.WorkerSessionID, request.LiveSessionCutoff); err != nil {
		return FailedWorkRecord{}, false, err
	}

	failed := FailedWorkRecord{
		AttemptID:  request.AttemptID,
		WorkItemID: running.workItemID,
		Error:      request.Error,
		QueuedAt:   running.queuedAt,
		StartedAt:  running.startedAt,
		FailedAt:   request.FailedAt,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO failed_work (
		attempt_id,
		work_item_id,
		error,
		queued_at,
		started_at,
		failed_at
	) VALUES (?, ?, ?, ?, ?, ?)`,
		failed.AttemptID,
		failed.WorkItemID,
		failed.Error,
		failed.QueuedAt,
		failed.StartedAt,
		failed.FailedAt,
	); err != nil {
		return FailedWorkRecord{}, false, fmt.Errorf("insert failed work %s: %w", request.AttemptID, err)
	}
	if err := deleteRunningWork(ctx, tx, request.AttemptID); err != nil {
		return FailedWorkRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return FailedWorkRecord{}, false, fmt.Errorf("commit fail attempt: %w", err)
	}
	return failed, true, nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type rowsQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type scanner interface {
	Scan(...any) error
}

type runningWorkRecord struct {
	attemptID       string
	workItemID      string
	workerID        string
	workerSessionID string
	queuedAt        string
	startedAt       string
}

type attemptResumeState struct {
	executionLineageID string
	resumeArtifactID   string
}

func validateConfirmCheckpointRequest(request ConfirmCheckpointRequest) (ResumeArtifactRecord, string, error) {
	if request.AttemptID == "" {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint attempt id is required")
	}
	if request.WorkerID == "" || request.WorkerSessionID == "" {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint worker and session ids are required")
	}
	if request.LiveSessionCutoff != "" {
		if err := validateTimestamp("checkpoint live session cutoff", request.LiveSessionCutoff); err != nil {
			return ResumeArtifactRecord{}, "", err
		}
	}
	if request.AcceptedAt == "" {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint accepted at is required")
	}
	if err := validateTimestamp("checkpoint accepted at", request.AcceptedAt); err != nil {
		return ResumeArtifactRecord{}, "", err
	}

	var suspendReason string
	switch {
	case request.CaptureKind == CheckpointCaptureKindPeriodic &&
		request.Disposition == CheckpointDispositionContinue:
		if request.SuspendedAt != "" {
			return ResumeArtifactRecord{}, "", fmt.Errorf("periodic checkpoint must not include suspended at")
		}
	case request.CaptureKind == CheckpointCaptureKindQuantum &&
		request.Disposition == CheckpointDispositionSuspend:
		suspendReason = SuspendReasonQuantum
	case request.CaptureKind == CheckpointCaptureKindFinal &&
		request.Disposition == CheckpointDispositionSuspend:
		suspendReason = SuspendReasonShutdown
	default:
		return ResumeArtifactRecord{}, "", fmt.Errorf(
			"unsupported checkpoint capture/disposition combination %s/%s",
			request.CaptureKind,
			request.Disposition,
		)
	}
	if request.Disposition == CheckpointDispositionSuspend {
		if request.SuspendedAt == "" {
			return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint suspended at is required")
		}
		if err := validateTimestamp("checkpoint suspended at", request.SuspendedAt); err != nil {
			return ResumeArtifactRecord{}, "", err
		}
	}
	if request.ManifestJSON == "" {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint manifest json is required")
	}

	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(request.ManifestJSON), &manifest); err != nil {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint manifest json is invalid")
	}
	if err := manifest.Validate(); err != nil {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint manifest validation: %w", err)
	}
	if err := request.Reference.Validate(); err != nil {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint reference validation: %w", err)
	}
	if manifest.ResumeArtifactID != request.Reference.ResumeArtifactID {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint manifest and reference artifact ids do not match")
	}
	if manifest.StorageScope != request.Reference.StorageScope {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint manifest and reference storage scopes do not match")
	}
	if manifest.ProducingAttemptID != request.AttemptID {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint manifest producing attempt does not match request")
	}
	if !relativePathInside(request.Reference.ManifestRelativePath, manifest.StorageRelativePath) {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint manifest path is outside artifact storage directory")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(request.ManifestJSON)))
	if digest != request.Reference.ManifestSHA256 {
		return ResumeArtifactRecord{}, "", fmt.Errorf("checkpoint manifest sha256 does not match exact json")
	}

	return ResumeArtifactRecord{
		ID:                   manifest.ResumeArtifactID,
		WorkItemID:           manifest.WorkItemID,
		ProducingAttemptID:   manifest.ProducingAttemptID,
		ExecutionLineageID:   manifest.ExecutionLineageID,
		ResumeGeneration:     manifest.ResumeGeneration,
		CaptureKind:          request.CaptureKind,
		PauseStrategy:        manifest.PauseStrategy,
		ManifestJSON:         request.ManifestJSON,
		ManifestSHA256:       request.Reference.ManifestSHA256,
		StorageScope:         manifest.StorageScope,
		ManifestRelativePath: request.Reference.ManifestRelativePath,
		CreatedAt:            manifest.CreatedAt,
		AcceptedAt:           request.AcceptedAt,
		Manifest:             manifest,
		Reference:            request.Reference,
	}, suspendReason, nil
}

func relativePathInside(candidate string, directory string) bool {
	return path.Dir(candidate) == directory || strings.HasPrefix(candidate, directory+"/")
}

func resumeArtifactMatchesConfirmation(existing ResumeArtifactRecord, requested ResumeArtifactRecord) bool {
	return existing.ID == requested.ID &&
		existing.WorkItemID == requested.WorkItemID &&
		existing.ProducingAttemptID == requested.ProducingAttemptID &&
		existing.ExecutionLineageID == requested.ExecutionLineageID &&
		existing.ResumeGeneration == requested.ResumeGeneration &&
		existing.CaptureKind == requested.CaptureKind &&
		existing.PauseStrategy == requested.PauseStrategy &&
		existing.ManifestJSON == requested.ManifestJSON &&
		existing.ManifestSHA256 == requested.ManifestSHA256 &&
		existing.StorageScope == requested.StorageScope &&
		existing.ManifestRelativePath == requested.ManifestRelativePath &&
		existing.CreatedAt == requested.CreatedAt
}

func replayCheckpointConfirmation(
	ctx context.Context,
	tx *sql.Tx,
	request ConfirmCheckpointRequest,
	artifact ResumeArtifactRecord,
	suspendReason string,
) (ConfirmCheckpointResult, error) {
	if request.Disposition == CheckpointDispositionContinue {
		running, found, err := getRunningWork(ctx, tx, request.AttemptID)
		if err != nil {
			return ConfirmCheckpointResult{}, err
		}
		if !found {
			return ConfirmCheckpointResult{}, ErrAssignmentNoLongerOwned
		}
		if err := validateRunningAssignmentOwner(ctx, tx, running, request.WorkerID, request.WorkerSessionID, request.LiveSessionCutoff); err != nil {
			return ConfirmCheckpointResult{}, err
		}
		return ConfirmCheckpointResult{Artifact: artifact}, nil
	}

	suspended, found, err := getSuspendedWork(ctx, tx, request.AttemptID)
	if err != nil {
		return ConfirmCheckpointResult{}, err
	}
	if !found ||
		suspended.ResumeArtifactID != artifact.ID ||
		suspended.WorkerID != request.WorkerID ||
		suspended.WorkerSessionID != request.WorkerSessionID ||
		suspended.SuspendedAt != request.SuspendedAt ||
		suspended.SuspendReason != suspendReason {
		return ConfirmCheckpointResult{}, fmt.Errorf("checkpoint suspension %s conflicts with existing state", request.AttemptID)
	}
	return ConfirmCheckpointResult{Artifact: artifact, Suspended: &suspended}, nil
}

func (request FailPendingResumeAttemptLimitRequest) validate() error {
	if request.AttemptID == "" {
		return fmt.Errorf("retry-limit attempt id is required")
	}
	if request.WorkItemID == "" {
		return fmt.Errorf("retry-limit work item id is required")
	}
	if request.ResumeArtifactID == "" {
		return fmt.Errorf("retry-limit resume artifact id is required")
	}
	if request.ExecutionLineageID == "" {
		return fmt.Errorf("retry-limit execution lineage id is required")
	}
	if request.ConfiguredLimit <= 0 {
		return fmt.Errorf("retry-limit configured limit must be positive")
	}
	if request.NextResumeAttemptNumber <= request.ConfiguredLimit {
		return fmt.Errorf("retry-limit next resume attempt number must exceed configured limit")
	}
	if err := validateTimestamp("retry-limit queued at", request.QueuedAt); err != nil {
		return err
	}
	if err := validateTimestamp("retry-limit failed at", request.FailedAt); err != nil {
		return err
	}
	return nil
}

type resumeTerminalDecisionState struct {
	workItemID           string
	workerID             string
	workerSessionID      string
	executorType         string
	startedAt            string
	resumedFromAttemptID string
	resumeArtifactID     string
	executionLineageID   string
	resumeAttemptNumber  int
}

func replayPendingResumeAttemptLimitFailure(
	ctx context.Context,
	tx *sql.Tx,
	request FailPendingResumeAttemptLimitRequest,
	failed FailedWorkRecord,
) (FailPendingResumeAttemptLimitResult, error) {
	state, found, err := getResumeTerminalDecisionState(ctx, tx, request.AttemptID)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}
	if !found ||
		state.workItemID != request.WorkItemID ||
		state.workerID != "" ||
		state.workerSessionID != "" ||
		state.executorType != ExecutorTypeController ||
		state.startedAt != request.FailedAt ||
		state.resumeArtifactID != request.ResumeArtifactID ||
		state.executionLineageID != request.ExecutionLineageID ||
		state.resumeAttemptNumber != request.NextResumeAttemptNumber ||
		failed.WorkItemID != request.WorkItemID ||
		failed.Error != "resume_attempt_limit_exhausted" ||
		failed.QueuedAt != request.QueuedAt ||
		failed.StartedAt != request.FailedAt ||
		failed.FailedAt != request.FailedAt {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("retry-limit attempt %s conflicts with existing terminal state", request.AttemptID)
	}

	artifact, found, err := getResumeArtifact(ctx, tx, request.ResumeArtifactID)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}
	if !found || artifact.ProducingAttemptID != state.resumedFromAttemptID {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("retry-limit attempt %s references conflicting resume state", request.AttemptID)
	}
	workItem, found, err := getWorkItem(ctx, tx, request.WorkItemID)
	if err != nil {
		return FailPendingResumeAttemptLimitResult{}, err
	}
	if !found {
		return FailPendingResumeAttemptLimitResult{}, fmt.Errorf("retry-limit work item %s no longer exists", request.WorkItemID)
	}
	return FailPendingResumeAttemptLimitResult{
		WorkItem:     workItem,
		Failed:       failed,
		Terminalized: false,
	}, nil
}

func getResumeTerminalDecisionState(ctx context.Context, q queryer, attemptID string) (resumeTerminalDecisionState, bool, error) {
	var state resumeTerminalDecisionState
	err := q.QueryRowContext(ctx, `SELECT
		work_item_id,
		COALESCE(worker_id, ''),
		COALESCE(worker_session_id, ''),
		executor_type,
		started_at,
		COALESCE(resumed_from_attempt_id, ''),
		COALESCE(resume_artifact_id, ''),
		COALESCE(execution_lineage_id, ''),
		COALESCE(resume_attempt_number, 0)
	FROM work_item_attempts
	WHERE attempt_id = ?`, attemptID).Scan(
		&state.workItemID,
		&state.workerID,
		&state.workerSessionID,
		&state.executorType,
		&state.startedAt,
		&state.resumedFromAttemptID,
		&state.resumeArtifactID,
		&state.executionLineageID,
		&state.resumeAttemptNumber,
	)
	if err == sql.ErrNoRows {
		return resumeTerminalDecisionState{}, false, nil
	}
	if err != nil {
		return resumeTerminalDecisionState{}, false, fmt.Errorf("get retry-limit attempt %s: %w", attemptID, err)
	}
	return state, true, nil
}

func getQueuedResumeWork(ctx context.Context, q queryer, workItemID string) (QueuedWorkRecord, bool, error) {
	record, err := scanQueuedWork(q.QueryRowContext(ctx, `SELECT
		work_items.work_item_id,
		work_items.run_id,
		work_items.stage_index,
		work_items.work_item_index,
		work_items.worker_payload_json,
		work_items.resolved_inputs_sha256,
		work_items.created_at,
		queued_work.queued_at,
		COALESCE(queued_work.resume_artifact_id, '')
	FROM queued_work
	JOIN work_items ON work_items.work_item_id = queued_work.work_item_id
	WHERE queued_work.work_item_id = ?`, workItemID))
	if err == sql.ErrNoRows {
		return QueuedWorkRecord{}, false, nil
	}
	if err != nil {
		return QueuedWorkRecord{}, false, fmt.Errorf("get pending resume work %s: %w", workItemID, err)
	}
	return record, true, nil
}

func insertFailedWork(ctx context.Context, tx *sql.Tx, failed FailedWorkRecord) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO failed_work (
		attempt_id,
		work_item_id,
		error,
		queued_at,
		started_at,
		failed_at
	) VALUES (?, ?, ?, ?, ?, ?)`,
		failed.AttemptID,
		failed.WorkItemID,
		failed.Error,
		failed.QueuedAt,
		failed.StartedAt,
		failed.FailedAt,
	); err != nil {
		return fmt.Errorf("insert failed work %s: %w", failed.AttemptID, err)
	}
	return nil
}

func validateNextCheckpointGeneration(
	ctx context.Context,
	tx *sql.Tx,
	running runningWorkRecord,
	artifact ResumeArtifactRecord,
) error {
	state, found, err := getAttemptResumeState(ctx, tx, running.attemptID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("checkpoint producing attempt %s does not exist", running.attemptID)
	}
	if state.executionLineageID != "" && state.executionLineageID != artifact.ExecutionLineageID {
		return fmt.Errorf("checkpoint execution lineage does not match producing attempt")
	}

	latest, hasLatest, err := getLatestResumeArtifactForLineage(ctx, tx, artifact.ExecutionLineageID)
	if err != nil {
		return err
	}
	if !hasLatest {
		if state.executionLineageID != "" || state.resumeArtifactID != "" {
			return fmt.Errorf("checkpoint execution lineage has no accepted starting artifact")
		}
		if artifact.ResumeGeneration != 1 {
			return fmt.Errorf("first checkpoint generation must be 1")
		}
		return nil
	}
	if latest.WorkItemID != running.workItemID {
		return fmt.Errorf("checkpoint execution lineage belongs to another work item")
	}
	if state.executionLineageID == "" {
		return fmt.Errorf("fresh attempt cannot join an existing execution lineage")
	}
	if artifact.ResumeGeneration != latest.ResumeGeneration+1 {
		return fmt.Errorf(
			"checkpoint generation %d is not next after accepted generation %d",
			artifact.ResumeGeneration,
			latest.ResumeGeneration,
		)
	}
	return nil
}

func insertResumeArtifact(ctx context.Context, tx *sql.Tx, artifact ResumeArtifactRecord) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO resume_artifacts (
		resume_artifact_id,
		work_item_id,
		producing_attempt_id,
		execution_lineage_id,
		resume_generation,
		capture_kind,
		pause_strategy,
		manifest_json,
		manifest_sha256,
		storage_scope,
		manifest_relative_path,
		created_at,
		accepted_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		artifact.ID,
		artifact.WorkItemID,
		artifact.ProducingAttemptID,
		artifact.ExecutionLineageID,
		artifact.ResumeGeneration,
		artifact.CaptureKind,
		string(artifact.PauseStrategy),
		artifact.ManifestJSON,
		artifact.ManifestSHA256,
		artifact.StorageScope,
		artifact.ManifestRelativePath,
		artifact.CreatedAt,
		artifact.AcceptedAt,
	); err != nil {
		return fmt.Errorf("insert resume artifact %s: %w", artifact.ID, err)
	}
	return nil
}

func assignAttemptExecutionLineage(ctx context.Context, tx *sql.Tx, attemptID string, lineageID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE work_item_attempts
	SET execution_lineage_id = ?
	WHERE attempt_id = ?
		AND (execution_lineage_id IS NULL OR execution_lineage_id = ?)`,
		lineageID,
		attemptID,
		lineageID,
	)
	if err != nil {
		return fmt.Errorf("assign execution lineage to attempt %s: %w", attemptID, err)
	}
	updated, err := rowsAffected(result)
	if err != nil {
		return fmt.Errorf("assign execution lineage to attempt %s: %w", attemptID, err)
	}
	if !updated {
		return fmt.Errorf("assign execution lineage to attempt %s: lineage conflict", attemptID)
	}
	return nil
}

func suspendRunningWork(
	ctx context.Context,
	tx *sql.Tx,
	running runningWorkRecord,
	artifactID string,
	suspendedAt string,
	suspendReason string,
) (SuspendedWorkRecord, error) {
	suspended := SuspendedWorkRecord{
		AttemptID:        running.attemptID,
		WorkItemID:       running.workItemID,
		ResumeArtifactID: artifactID,
		WorkerID:         running.workerID,
		WorkerSessionID:  running.workerSessionID,
		QueuedAt:         running.queuedAt,
		StartedAt:        running.startedAt,
		SuspendedAt:      suspendedAt,
		SuspendReason:    suspendReason,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO suspended_work (
		attempt_id,
		work_item_id,
		resume_artifact_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at,
		suspended_at,
		suspend_reason
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		suspended.AttemptID,
		suspended.WorkItemID,
		suspended.ResumeArtifactID,
		suspended.WorkerID,
		suspended.WorkerSessionID,
		suspended.QueuedAt,
		suspended.StartedAt,
		suspended.SuspendedAt,
		suspended.SuspendReason,
	); err != nil {
		return SuspendedWorkRecord{}, fmt.Errorf("insert suspended work %s: %w", running.attemptID, err)
	}
	if err := deleteRunningWork(ctx, tx, running.attemptID); err != nil {
		return SuspendedWorkRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO queued_work (
		work_item_id,
		queued_at,
		resume_artifact_id
	) VALUES (?, ?, ?)`,
		running.workItemID,
		suspendedAt,
		artifactID,
	); err != nil {
		return SuspendedWorkRecord{}, fmt.Errorf("queue suspended work %s: %w", running.attemptID, err)
	}
	return suspended, nil
}

func (request SuspendFromLatestCheckpointRequest) validate() error {
	if request.AttemptID == "" {
		return fmt.Errorf("suspend attempt id is required")
	}
	if request.WorkerID == "" || request.WorkerSessionID == "" {
		return fmt.Errorf("suspend worker and session ids are required")
	}
	if request.LiveSessionCutoff != "" {
		if err := validateTimestamp("suspend live session cutoff", request.LiveSessionCutoff); err != nil {
			return err
		}
	}
	if request.SuspendedAt == "" {
		return fmt.Errorf("suspend suspended at is required")
	}
	if err := validateTimestamp("suspend suspended at", request.SuspendedAt); err != nil {
		return err
	}
	if request.SuspendReason != SuspendReasonQuantum && request.SuspendReason != SuspendReasonShutdown {
		return fmt.Errorf("unsupported suspend reason %q", request.SuspendReason)
	}
	return nil
}

func suspendedWorkMatchesFallbackRequest(existing SuspendedWorkRecord, request SuspendFromLatestCheckpointRequest) bool {
	return existing.AttemptID == request.AttemptID &&
		existing.WorkerID == request.WorkerID &&
		existing.WorkerSessionID == request.WorkerSessionID &&
		existing.SuspendedAt == request.SuspendedAt &&
		existing.SuspendReason == request.SuspendReason
}

func getResumeArtifact(ctx context.Context, q queryer, artifactID string) (ResumeArtifactRecord, bool, error) {
	record, err := scanResumeArtifact(q.QueryRowContext(ctx, `SELECT
		resume_artifact_id,
		work_item_id,
		producing_attempt_id,
		execution_lineage_id,
		resume_generation,
		capture_kind,
		pause_strategy,
		manifest_json,
		manifest_sha256,
		storage_scope,
		manifest_relative_path,
		created_at,
		accepted_at
	FROM resume_artifacts
	WHERE resume_artifact_id = ?`, artifactID))
	if err == sql.ErrNoRows {
		return ResumeArtifactRecord{}, false, nil
	}
	if err != nil {
		return ResumeArtifactRecord{}, false, fmt.Errorf("get resume artifact %s: %w", artifactID, err)
	}
	record, err = validatePersistedResumeArtifact(record)
	if err != nil {
		return ResumeArtifactRecord{}, false, fmt.Errorf("get resume artifact %s: %w", artifactID, err)
	}
	return record, true, nil
}

func getLatestResumeArtifactForLineage(ctx context.Context, q queryer, lineageID string) (ResumeArtifactRecord, bool, error) {
	record, err := scanResumeArtifact(q.QueryRowContext(ctx, `SELECT
		resume_artifact_id,
		work_item_id,
		producing_attempt_id,
		execution_lineage_id,
		resume_generation,
		capture_kind,
		pause_strategy,
		manifest_json,
		manifest_sha256,
		storage_scope,
		manifest_relative_path,
		created_at,
		accepted_at
	FROM resume_artifacts
	WHERE execution_lineage_id = ?
	ORDER BY resume_generation DESC
	LIMIT 1`, lineageID))
	if err == sql.ErrNoRows {
		return ResumeArtifactRecord{}, false, nil
	}
	if err != nil {
		return ResumeArtifactRecord{}, false, fmt.Errorf("get latest resume artifact for lineage %s: %w", lineageID, err)
	}
	record, err = validatePersistedResumeArtifact(record)
	if err != nil {
		return ResumeArtifactRecord{}, false, fmt.Errorf("get latest resume artifact for lineage %s: %w", lineageID, err)
	}
	return record, true, nil
}

func scanResumeArtifact(row scanner) (ResumeArtifactRecord, error) {
	var record ResumeArtifactRecord
	var pauseStrategy string
	err := row.Scan(
		&record.ID,
		&record.WorkItemID,
		&record.ProducingAttemptID,
		&record.ExecutionLineageID,
		&record.ResumeGeneration,
		&record.CaptureKind,
		&pauseStrategy,
		&record.ManifestJSON,
		&record.ManifestSHA256,
		&record.StorageScope,
		&record.ManifestRelativePath,
		&record.CreatedAt,
		&record.AcceptedAt,
	)
	record.PauseStrategy = model.PauseStrategy(pauseStrategy)
	return record, err
}

func validatePersistedResumeArtifact(record ResumeArtifactRecord) (ResumeArtifactRecord, error) {
	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(record.ManifestJSON), &manifest); err != nil {
		return ResumeArtifactRecord{}, fmt.Errorf("persisted manifest json is invalid")
	}
	if err := manifest.Validate(); err != nil {
		return ResumeArtifactRecord{}, fmt.Errorf("persisted manifest validation: %w", err)
	}
	reference := model.ResumeArtifactReference{
		Schema:               manifest.Schema,
		ResumeArtifactID:     record.ID,
		StorageScope:         record.StorageScope,
		ManifestRelativePath: record.ManifestRelativePath,
		ManifestSHA256:       record.ManifestSHA256,
	}
	if err := reference.Validate(); err != nil {
		return ResumeArtifactRecord{}, fmt.Errorf("persisted reference validation: %w", err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(record.ManifestJSON)))
	if digest != record.ManifestSHA256 {
		return ResumeArtifactRecord{}, fmt.Errorf("persisted manifest sha256 mismatch")
	}
	if manifest.ResumeArtifactID != record.ID ||
		manifest.WorkItemID != record.WorkItemID ||
		manifest.ProducingAttemptID != record.ProducingAttemptID ||
		manifest.ExecutionLineageID != record.ExecutionLineageID ||
		manifest.ResumeGeneration != record.ResumeGeneration ||
		manifest.PauseStrategy != record.PauseStrategy ||
		manifest.StorageScope != record.StorageScope ||
		manifest.CreatedAt != record.CreatedAt {
		return ResumeArtifactRecord{}, fmt.Errorf("persisted manifest identity mismatch")
	}
	if !relativePathInside(record.ManifestRelativePath, manifest.StorageRelativePath) {
		return ResumeArtifactRecord{}, fmt.Errorf("persisted manifest path is outside artifact storage directory")
	}
	record.Manifest = manifest
	record.Reference = reference
	return record, nil
}

func getAttemptResumeState(ctx context.Context, q queryer, attemptID string) (attemptResumeState, bool, error) {
	var state attemptResumeState
	var executionLineageID sql.NullString
	var resumeArtifactID sql.NullString
	err := q.QueryRowContext(ctx, `SELECT
		execution_lineage_id,
		resume_artifact_id
	FROM work_item_attempts
	WHERE attempt_id = ?`, attemptID).Scan(&executionLineageID, &resumeArtifactID)
	if err == sql.ErrNoRows {
		return attemptResumeState{}, false, nil
	}
	if err != nil {
		return attemptResumeState{}, false, fmt.Errorf("get attempt resume state %s: %w", attemptID, err)
	}
	state.executionLineageID = executionLineageID.String
	state.resumeArtifactID = resumeArtifactID.String
	return state, true, nil
}

func latestAcceptedCheckpointForAttempt(ctx context.Context, q queryer, attemptID string) (ResumeArtifactRecord, bool, error) {
	state, found, err := getAttemptResumeState(ctx, q, attemptID)
	if err != nil || !found {
		return ResumeArtifactRecord{}, false, err
	}
	if state.executionLineageID != "" {
		artifact, found, err := getLatestResumeArtifactForLineage(ctx, q, state.executionLineageID)
		if err != nil || found {
			return artifact, found, err
		}
	}
	if state.resumeArtifactID != "" {
		return getResumeArtifact(ctx, q, state.resumeArtifactID)
	}
	return ResumeArtifactRecord{}, false, nil
}

func getSuspendedWork(ctx context.Context, q queryer, attemptID string) (SuspendedWorkRecord, bool, error) {
	record, err := scanSuspendedWork(q.QueryRowContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		resume_artifact_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at,
		suspended_at,
		suspend_reason
	FROM suspended_work
	WHERE attempt_id = ?`, attemptID))
	if err == sql.ErrNoRows {
		return SuspendedWorkRecord{}, false, nil
	}
	if err != nil {
		return SuspendedWorkRecord{}, false, fmt.Errorf("get suspended work %s: %w", attemptID, err)
	}
	return record, true, nil
}

func scanSuspendedWork(row scanner) (SuspendedWorkRecord, error) {
	var record SuspendedWorkRecord
	err := row.Scan(
		&record.AttemptID,
		&record.WorkItemID,
		&record.ResumeArtifactID,
		&record.WorkerID,
		&record.WorkerSessionID,
		&record.QueuedAt,
		&record.StartedAt,
		&record.SuspendedAt,
		&record.SuspendReason,
	)
	return record, err
}

func hasSuspendedWork(ctx context.Context, q queryer, attemptID string) (bool, error) {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM suspended_work WHERE attempt_id = ?`, attemptID).Scan(&count); err != nil {
		return false, fmt.Errorf("count suspended work %s: %w", attemptID, err)
	}
	return count != 0, nil
}

func nextResumeAttemptNumber(ctx context.Context, q queryer, artifactID string) (int, error) {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*)
	FROM work_item_attempts
	WHERE resume_artifact_id = ?`, artifactID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count resume attempts for artifact %s: %w", artifactID, err)
	}
	return count + 1, nil
}

func validateRunningAssignmentOwner(ctx context.Context, tx *sql.Tx, running runningWorkRecord, workerID string, sessionID string, cutoff string) error {
	if workerID == "" && sessionID == "" {
		return nil
	}
	if workerID == "" || sessionID == "" {
		return ErrAssignmentNoLongerOwned
	}
	if running.workerID != workerID || running.workerSessionID != sessionID {
		return ErrAssignmentNoLongerOwned
	}
	session, found, err := getWorkerSession(ctx, tx, workerID, sessionID)
	if err != nil {
		return err
	}
	if !found || session.Status != WorkerSessionStatusActive {
		return ErrAssignmentNoLongerOwned
	}
	if cutoff != "" && session.LastHeartbeatAt < cutoff {
		return ErrAssignmentNoLongerOwned
	}
	return nil
}

func listExpiredActiveWorkerSessions(ctx context.Context, tx *sql.Tx, cutoff string) ([]WorkerSessionRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT
		worker_session_id,
		worker_id,
		status,
		registered_at,
		last_heartbeat_at,
		ended_at,
		end_reason,
		execution_handle
	FROM worker_sessions
	WHERE status = ?
		AND last_heartbeat_at < ?
	ORDER BY last_heartbeat_at, worker_session_id`,
		WorkerSessionStatusActive,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("list expired worker sessions: %w", err)
	}
	defer rows.Close()

	sessions := []WorkerSessionRecord{}
	for rows.Next() {
		session, err := scanWorkerSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list expired worker sessions: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expired worker sessions: %w", err)
	}
	return sessions, nil
}

func markWorkerSessionDeadIfExpired(ctx context.Context, tx *sql.Tx, session WorkerSessionRecord, request RecoverExpiredWorkerSessionsRequest) (bool, error) {
	result, err := tx.ExecContext(ctx, `UPDATE worker_sessions
	SET status = ?,
		ended_at = ?,
		end_reason = ?
	WHERE worker_session_id = ?
		AND worker_id = ?
		AND status = ?
		AND last_heartbeat_at < ?`,
		WorkerSessionStatusDead,
		request.RecoveredAt,
		request.Reason,
		session.ID,
		session.WorkerID,
		WorkerSessionStatusActive,
		request.Cutoff,
	)
	if err != nil {
		return false, fmt.Errorf("mark worker session %s dead: %w", session.ID, err)
	}
	changed, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("mark worker session %s dead: %w", session.ID, err)
	}
	return changed, nil
}

func markWorkerSessionStopped(ctx context.Context, tx *sql.Tx, request StopWorkerSessionAndRecoverWorkRequest) (bool, error) {
	result, err := tx.ExecContext(ctx, `UPDATE worker_sessions
	SET status = ?,
		ended_at = ?,
		end_reason = ?
	WHERE worker_session_id = ?
		AND worker_id = ?
		AND status = ?`,
		WorkerSessionStatusStopped,
		request.StoppedAt,
		request.Reason,
		request.SessionID,
		request.WorkerID,
		WorkerSessionStatusActive,
	)
	if err != nil {
		return false, fmt.Errorf("mark worker session %s stopped: %w", request.SessionID, err)
	}
	changed, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("mark worker session %s stopped: %w", request.SessionID, err)
	}
	return changed, nil
}

func listRunningWorkForSession(ctx context.Context, tx *sql.Tx, sessionID string) ([]runningWorkRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		COALESCE(worker_id, ''),
		COALESCE(worker_session_id, ''),
		queued_at,
		started_at
	FROM running_work
	WHERE worker_session_id = ?
	ORDER BY started_at, attempt_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list running work for worker session %s: %w", sessionID, err)
	}
	defer rows.Close()

	assignments := []runningWorkRecord{}
	for rows.Next() {
		var running runningWorkRecord
		if err := rows.Scan(
			&running.attemptID,
			&running.workItemID,
			&running.workerID,
			&running.workerSessionID,
			&running.queuedAt,
			&running.startedAt,
		); err != nil {
			return nil, fmt.Errorf("list running work for worker session %s: %w", sessionID, err)
		}
		assignments = append(assignments, running)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list running work for worker session %s: %w", sessionID, err)
	}
	return assignments, nil
}

func abandonRunningWork(ctx context.Context, tx *sql.Tx, assignment runningWorkRecord, abandonedAt string, reason string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO abandoned_work (
		attempt_id,
		work_item_id,
		worker_id,
		worker_session_id,
		queued_at,
		started_at,
		abandoned_at,
		reason
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		assignment.attemptID,
		assignment.workItemID,
		assignment.workerID,
		assignment.workerSessionID,
		assignment.queuedAt,
		assignment.startedAt,
		abandonedAt,
		reason,
	); err != nil {
		return fmt.Errorf("insert abandoned work %s: %w", assignment.attemptID, err)
	}
	if err := deleteRunningWork(ctx, tx, assignment.attemptID); err != nil {
		return err
	}
	return nil
}

func requeueAbandonedWork(ctx context.Context, tx *sql.Tx, attemptID string, workItemID string, queuedAt string) (bool, error) {
	artifact, found, err := latestAcceptedCheckpointForAttempt(ctx, tx, attemptID)
	if err != nil {
		return false, err
	}
	var resumeArtifactID string
	if found {
		resumeArtifactID = artifact.ID
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO queued_work (
		work_item_id,
		queued_at,
		resume_artifact_id
	) VALUES (?, ?, ?)
	ON CONFLICT(work_item_id) DO NOTHING`, workItemID, queuedAt, nullString(resumeArtifactID))
	if err != nil {
		return false, fmt.Errorf("requeue abandoned work %s: %w", workItemID, err)
	}
	inserted, err := rowsAffected(result)
	if err != nil {
		return false, fmt.Errorf("requeue abandoned work %s: %w", workItemID, err)
	}
	return inserted, nil
}

func hasAbandonedWork(ctx context.Context, tx *sql.Tx, attemptID string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM abandoned_work WHERE attempt_id = ?`, attemptID).Scan(&count); err != nil {
		return false, fmt.Errorf("count abandoned work %s: %w", attemptID, err)
	}
	return count != 0, nil
}

func getRunningWork(ctx context.Context, q queryer, attemptID string) (runningWorkRecord, bool, error) {
	var running runningWorkRecord
	err := q.QueryRowContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		COALESCE(worker_id, ''),
		COALESCE(worker_session_id, ''),
		queued_at,
		started_at
	FROM running_work
	WHERE attempt_id = ?`, attemptID).Scan(
		&running.attemptID,
		&running.workItemID,
		&running.workerID,
		&running.workerSessionID,
		&running.queuedAt,
		&running.startedAt,
	)
	if err == sql.ErrNoRows {
		return runningWorkRecord{}, false, nil
	}
	if err != nil {
		return runningWorkRecord{}, false, err
	}
	return running, true, nil
}

func getCompletedWork(ctx context.Context, q queryer, attemptID string) (CompletedWorkRecord, bool, error) {
	completed, err := scanCompletedWork(q.QueryRowContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		skipped_parent_id,
		output_json,
		output_json_sha256,
		pre_state_sha256,
		post_state_sha256,
		queued_at,
		started_at,
		completed_at
	FROM completed_work
	WHERE attempt_id = ?`, attemptID))
	if err == sql.ErrNoRows {
		return CompletedWorkRecord{}, false, nil
	}
	if err != nil {
		return CompletedWorkRecord{}, false, err
	}
	return completed, true, nil
}

func getFailedWork(ctx context.Context, q queryer, attemptID string) (FailedWorkRecord, bool, error) {
	failed, err := scanFailedWork(q.QueryRowContext(ctx, `SELECT
		attempt_id,
		work_item_id,
		error,
		queued_at,
		started_at,
		failed_at
	FROM failed_work
	WHERE attempt_id = ?`, attemptID))
	if err == sql.ErrNoRows {
		return FailedWorkRecord{}, false, nil
	}
	if err != nil {
		return FailedWorkRecord{}, false, err
	}
	return failed, true, nil
}

func deleteRunningWork(ctx context.Context, tx *sql.Tx, attemptID string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM running_work WHERE attempt_id = ?`, attemptID)
	if err != nil {
		return fmt.Errorf("delete running work %s: %w", attemptID, err)
	}
	deleted, err := rowsAffected(result)
	if err != nil {
		return fmt.Errorf("delete running work %s: %w", attemptID, err)
	}
	if !deleted {
		return fmt.Errorf("delete running work %s: no row deleted", attemptID)
	}
	return nil
}

func stageWorkComplete(ctx context.Context, q queryer, runID string, stageIndex int) (bool, error) {
	var total int
	var completed int
	var queued int
	var running int
	var failed int
	err := q.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM work_items WHERE run_id = ? AND stage_index = ?),
		(SELECT COUNT(DISTINCT completed_work.work_item_id)
			FROM completed_work
			JOIN work_items ON work_items.work_item_id = completed_work.work_item_id
			WHERE work_items.run_id = ? AND work_items.stage_index = ?),
		(SELECT COUNT(*)
			FROM queued_work
			JOIN work_items ON work_items.work_item_id = queued_work.work_item_id
			WHERE work_items.run_id = ? AND work_items.stage_index = ?),
		(SELECT COUNT(*)
			FROM running_work
			JOIN work_items ON work_items.work_item_id = running_work.work_item_id
			WHERE work_items.run_id = ? AND work_items.stage_index = ?),
		(SELECT COUNT(*)
			FROM failed_work
			JOIN work_items ON work_items.work_item_id = failed_work.work_item_id
			WHERE work_items.run_id = ? AND work_items.stage_index = ?)`,
		runID, stageIndex,
		runID, stageIndex,
		runID, stageIndex,
		runID, stageIndex,
		runID, stageIndex,
	).Scan(&total, &completed, &queued, &running, &failed)
	if err != nil {
		return false, fmt.Errorf("check stage work completion %s/%d: %w", runID, stageIndex, err)
	}
	return total > 0 && completed == total && queued == 0 && running == 0 && failed == 0, nil
}

func insertWorkItems(ctx context.Context, tx *sql.Tx, items []WorkItemRecord) error {
	for _, item := range items {
		existing, found, err := getWorkItem(ctx, tx, item.ID)
		if err != nil {
			return err
		}
		if found {
			if existing != item {
				return fmt.Errorf("work item %s already exists with different values", item.ID)
			}
			continue
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO work_items (
			work_item_id,
			run_id,
			stage_index,
			work_item_index,
			worker_payload_json,
			resolved_inputs_sha256,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.ID,
			item.RunID,
			item.StageIndex,
			item.WorkItemIndex,
			item.WorkerPayloadJSON,
			item.ResolvedInputsSHA256,
			item.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert work item %s: %w", item.ID, err)
		}
	}
	return nil
}

func enqueueWorkItems(ctx context.Context, tx *sql.Tx, items []QueuedWorkRecord) error {
	for _, item := range items {
		existingQueuedAt, found, err := getQueuedWork(ctx, tx, item.ID)
		if err != nil {
			return err
		}
		if found {
			if existingQueuedAt != item.QueuedAt {
				return fmt.Errorf("queued work item %s already exists with different queued_at", item.ID)
			}
			continue
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO queued_work (
			work_item_id,
			queued_at
		) VALUES (?, ?)`,
			item.ID,
			item.QueuedAt,
		); err != nil {
			return fmt.Errorf("enqueue work item %s: %w", item.ID, err)
		}
	}
	return nil
}

func insertWorkItemResourceConstraints(ctx context.Context, tx *sql.Tx, constraints []WorkItemResourceConstraintRecord) error {
	for _, constraint := range constraints {
		existing, found, err := getWorkItemResourceConstraint(ctx, tx, constraint.WorkItemID, constraint.ConstraintIndex)
		if err != nil {
			return err
		}
		if found {
			if existing != constraint {
				return fmt.Errorf("work item resource constraint %s/%d already exists with different values", constraint.WorkItemID, constraint.ConstraintIndex)
			}
			continue
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO work_item_resource_constraints (
			work_item_id,
			constraint_index,
			resource_key,
			requested_units,
			operator,
			target_units,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			constraint.WorkItemID,
			constraint.ConstraintIndex,
			constraint.ResourceKey,
			constraint.RequestedUnits,
			constraint.Operator,
			constraint.TargetUnits,
			constraint.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert work item resource constraint %s/%d: %w", constraint.WorkItemID, constraint.ConstraintIndex, err)
		}
	}
	return nil
}

func getProject(ctx context.Context, q queryer, projectID string) (ProjectRecord, bool, error) {
	if projectID == "" {
		return ProjectRecord{}, false, fmt.Errorf("project id is required")
	}

	var project ProjectRecord
	var sourceRevisionID sql.NullString
	err := q.QueryRowContext(ctx, `SELECT
		project_id,
		project_name,
		repository_identity,
		source_revision_id,
		config_path,
		source_object_id,
		config_sha256,
		created_at
	FROM projects
	WHERE project_id = ?`, projectID).Scan(
		&project.ID,
		&project.Name,
		&project.RepositoryIdentity,
		&sourceRevisionID,
		&project.ConfigPath,
		&project.SourceObjectID,
		&project.ConfigSHA256,
		&project.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return ProjectRecord{}, false, nil
	}
	if err != nil {
		return ProjectRecord{}, false, fmt.Errorf("get project %s: %w", projectID, err)
	}
	project.SourceRevisionID = stringPtrFromNull(sourceRevisionID)
	return project, true, nil
}

func getWorkflow(ctx context.Context, q queryer, workflowID string) (WorkflowRecord, bool, error) {
	if workflowID == "" {
		return WorkflowRecord{}, false, fmt.Errorf("workflow id is required")
	}

	var workflow WorkflowRecord
	var sourceRevisionID sql.NullString
	err := q.QueryRowContext(ctx, `SELECT
		workflow_id,
		project_id,
		workflow_name,
		repository_identity,
		source_revision_id,
		workflow_path,
		source_object_id,
		workflow_sha256,
		created_at
	FROM workflows
	WHERE workflow_id = ?`, workflowID).Scan(
		&workflow.ID,
		&workflow.ProjectID,
		&workflow.Name,
		&workflow.RepositoryIdentity,
		&sourceRevisionID,
		&workflow.WorkflowPath,
		&workflow.SourceObjectID,
		&workflow.WorkflowSHA256,
		&workflow.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return WorkflowRecord{}, false, nil
	}
	if err != nil {
		return WorkflowRecord{}, false, fmt.Errorf("get workflow %s: %w", workflowID, err)
	}
	workflow.SourceRevisionID = stringPtrFromNull(sourceRevisionID)
	return workflow, true, nil
}

func getWorkflowRun(ctx context.Context, q queryer, runID string) (WorkflowRunRecord, bool, error) {
	if runID == "" {
		return WorkflowRunRecord{}, false, fmt.Errorf("run id is required")
	}

	run, err := scanWorkflowRun(q.QueryRowContext(ctx, `SELECT
		run_id,
		project_id,
		workflow_id,
		submission_context_json,
		created_at
	FROM workflow_instances
	WHERE run_id = ?`, runID))
	if err == sql.ErrNoRows {
		return WorkflowRunRecord{}, false, nil
	}
	if err != nil {
		return WorkflowRunRecord{}, false, fmt.Errorf("get workflow run %s: %w", runID, err)
	}
	return run, true, nil
}

func getWorkflowStage(ctx context.Context, q queryer, runID string, stageIndex int) (WorkflowStageRecord, bool, error) {
	if runID == "" {
		return WorkflowStageRecord{}, false, fmt.Errorf("run id is required")
	}
	if stageIndex < 0 {
		return WorkflowStageRecord{}, false, fmt.Errorf("stage index must be non-negative")
	}

	stage, err := scanWorkflowStage(q.QueryRowContext(ctx, workflowStageSelectSQL()+` WHERE run_id = ? AND stage_index = ?`, runID, stageIndex))
	if err == sql.ErrNoRows {
		return WorkflowStageRecord{}, false, nil
	}
	if err != nil {
		return WorkflowStageRecord{}, false, fmt.Errorf("get workflow stage %s/%d: %w", runID, stageIndex, err)
	}
	return stage, true, nil
}

func getWorkItem(ctx context.Context, q queryer, workItemID string) (WorkItemRecord, bool, error) {
	if workItemID == "" {
		return WorkItemRecord{}, false, fmt.Errorf("work item id is required")
	}

	item, err := scanWorkItem(q.QueryRowContext(ctx, `SELECT
		work_item_id,
		run_id,
		stage_index,
		work_item_index,
		worker_payload_json,
		resolved_inputs_sha256,
		created_at
	FROM work_items
	WHERE work_item_id = ?`, workItemID))
	if err == sql.ErrNoRows {
		return WorkItemRecord{}, false, nil
	}
	if err != nil {
		return WorkItemRecord{}, false, fmt.Errorf("get work item %s: %w", workItemID, err)
	}
	return item, true, nil
}

func getWorkItemResourceConstraint(ctx context.Context, q queryer, workItemID string, constraintIndex int) (WorkItemResourceConstraintRecord, bool, error) {
	if workItemID == "" {
		return WorkItemResourceConstraintRecord{}, false, fmt.Errorf("work item id is required")
	}
	if constraintIndex < 0 {
		return WorkItemResourceConstraintRecord{}, false, fmt.Errorf("constraint index must be non-negative")
	}

	var constraint WorkItemResourceConstraintRecord
	err := q.QueryRowContext(ctx, `SELECT
		work_item_id,
		constraint_index,
		resource_key,
		requested_units,
		operator,
		target_units,
		created_at
	FROM work_item_resource_constraints
	WHERE work_item_id = ? AND constraint_index = ?`,
		workItemID,
		constraintIndex,
	).Scan(
		&constraint.WorkItemID,
		&constraint.ConstraintIndex,
		&constraint.ResourceKey,
		&constraint.RequestedUnits,
		&constraint.Operator,
		&constraint.TargetUnits,
		&constraint.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return WorkItemResourceConstraintRecord{}, false, nil
	}
	if err != nil {
		return WorkItemResourceConstraintRecord{}, false, fmt.Errorf("get work item resource constraint %s/%d: %w", workItemID, constraintIndex, err)
	}
	return constraint, true, nil
}

func getQueuedWork(ctx context.Context, q queryer, workItemID string) (string, bool, error) {
	if workItemID == "" {
		return "", false, fmt.Errorf("work item id is required")
	}

	var queuedAt string
	err := q.QueryRowContext(ctx, `SELECT queued_at FROM queued_work WHERE work_item_id = ?`, workItemID).Scan(&queuedAt)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get queued work item %s: %w", workItemID, err)
	}
	return queuedAt, true, nil
}

func listQueuedWorkForClaim(ctx context.Context, tx *sql.Tx) ([]QueuedWorkRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT
		work_items.work_item_id,
		work_items.run_id,
		work_items.stage_index,
		work_items.work_item_index,
		work_items.worker_payload_json,
		work_items.resolved_inputs_sha256,
		work_items.created_at,
		queued_work.queued_at,
		COALESCE(queued_work.resume_artifact_id, '')
	FROM queued_work
	JOIN work_items ON work_items.work_item_id = queued_work.work_item_id
	ORDER BY queued_work.queued_at, queued_work.work_item_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	queued := []QueuedWorkRecord{}
	for rows.Next() {
		item, err := scanQueuedWork(rows)
		if err != nil {
			return nil, err
		}
		queued = append(queued, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return queued, nil
}

func queuedResourceConstraintsAllow(ctx context.Context, tx *sql.Tx, workItemID string) (bool, error) {
	checks, err := listQueuedResourceConstraintChecksForWorkItem(ctx, tx, workItemID)
	if err != nil {
		return false, err
	}

	modelChecks := make([]model.ResourceConstraintCheck, 0, len(checks))
	for _, check := range checks {
		modelChecks = append(modelChecks, model.ResourceConstraintCheck{
			TotalUnits:     check.TotalUnits,
			RequestedUnits: check.RequestedUnits,
			Operator:       model.ResourceOperator(check.Operator),
			TargetUnits:    check.TargetUnits,
		})
	}
	return model.ResourceConstraintChecksAllow(modelChecks)
}

func listQueuedResourceConstraintChecksForWorkItem(ctx context.Context, tx *sql.Tx, workItemID string) ([]QueuedResourceConstraintCheckRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT
		work_item_id,
		queued_at,
		constraint_index,
		resource_key,
		total_units,
		requested_units,
		operator,
		target_units
	FROM queued_resource_constraint_checks
	WHERE work_item_id = ?
	ORDER BY constraint_index`, workItemID)
	if err != nil {
		return nil, fmt.Errorf("list queued resource constraint checks for work item %s: %w", workItemID, err)
	}
	defer rows.Close()

	checks := []QueuedResourceConstraintCheckRecord{}
	for rows.Next() {
		check, err := scanQueuedResourceConstraintCheck(rows)
		if err != nil {
			return nil, fmt.Errorf("list queued resource constraint checks for work item %s: %w", workItemID, err)
		}
		checks = append(checks, check)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list queued resource constraint checks for work item %s: %w", workItemID, err)
	}
	return checks, nil
}

func getWorkflowDependencyWorkItem(ctx context.Context, q queryer, runID, workItemID string) (WorkflowDependencyWorkItemRecord, bool, error) {
	if runID == "" {
		return WorkflowDependencyWorkItemRecord{}, false, fmt.Errorf("run id is required")
	}
	if workItemID == "" {
		return WorkflowDependencyWorkItemRecord{}, false, fmt.Errorf("work item id is required")
	}

	item, err := scanWorkflowDependencyWorkItem(q.QueryRowContext(ctx, `SELECT
		run_id,
		stage_index,
		step_index,
		work_item_id,
		work_item_index,
		created_at
	FROM workflow_dependency_work_items
	WHERE run_id = ? AND work_item_id = ?`, runID, workItemID))
	if err == sql.ErrNoRows {
		return WorkflowDependencyWorkItemRecord{}, false, nil
	}
	if err != nil {
		return WorkflowDependencyWorkItemRecord{}, false, fmt.Errorf("get dependency work item %s for run %s: %w", workItemID, runID, err)
	}
	return item, true, nil
}

func getWorkflowStepOutputFact(ctx context.Context, q queryer, runID string, stepIndex int) (WorkflowStepOutputFactRecord, bool, error) {
	if runID == "" {
		return WorkflowStepOutputFactRecord{}, false, fmt.Errorf("run id is required")
	}
	if stepIndex < 0 {
		return WorkflowStepOutputFactRecord{}, false, fmt.Errorf("step index must be non-negative")
	}

	fact, err := scanWorkflowStepOutputFact(q.QueryRowContext(ctx, `SELECT
		run_id,
		step_index,
		output_json,
		output_json_sha256,
		output_json_bytes,
		output_json_pruned,
		output_kind,
		created_at,
		updated_at
	FROM workflow_step_output_facts
	WHERE run_id = ? AND step_index = ?`, runID, stepIndex))
	if err == sql.ErrNoRows {
		return WorkflowStepOutputFactRecord{}, false, nil
	}
	if err != nil {
		return WorkflowStepOutputFactRecord{}, false, fmt.Errorf("get workflow step output fact %s/%d: %w", runID, stepIndex, err)
	}
	return fact, true, nil
}

func listStagesForRun(ctx context.Context, tx *sql.Tx, runID string) ([]WorkflowStageRecord, error) {
	rows, err := tx.QueryContext(ctx, workflowStageSelectSQL()+` WHERE run_id = ? ORDER BY stage_index`, runID)
	if err != nil {
		return nil, fmt.Errorf("list workflow stages for run %s: %w", runID, err)
	}
	defer rows.Close()

	stages := []WorkflowStageRecord{}
	for rows.Next() {
		stage, err := scanWorkflowStage(rows)
		if err != nil {
			return nil, fmt.Errorf("list workflow stages for run %s: %w", runID, err)
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow stages for run %s: %w", runID, err)
	}
	return stages, nil
}

func listWorkflowDependencyStepsForRun(ctx context.Context, tx *sql.Tx, runID string) ([]WorkflowDependencyStepRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT
		run_id,
		stage_index,
		step_index,
		step_id,
		parallel_with,
		created_at
	FROM workflow_dependency_steps
	WHERE run_id = ?
	ORDER BY stage_index, step_index`, runID)
	if err != nil {
		return nil, fmt.Errorf("list workflow dependency steps for run %s: %w", runID, err)
	}
	defer rows.Close()

	steps := []WorkflowDependencyStepRecord{}
	for rows.Next() {
		step, err := scanWorkflowDependencyStep(rows)
		if err != nil {
			return nil, fmt.Errorf("list workflow dependency steps for run %s: %w", runID, err)
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workflow dependency steps for run %s: %w", runID, err)
	}
	return steps, nil
}

func workflowStageSelectSQL() string {
	return `SELECT
		run_id,
		stage_index,
		step_id,
		stage_source_reference,
		state,
		created_at,
		ready_at,
		started_at,
		completed_at,
		failed_at,
		output_json,
		output_json_sha256
	FROM workflow_stages`
}

func scanWorkflowRun(row scanner) (WorkflowRunRecord, error) {
	var run WorkflowRunRecord
	err := row.Scan(
		&run.ID,
		&run.ProjectID,
		&run.WorkflowID,
		&run.SubmissionContextJSON,
		&run.CreatedAt,
	)
	return run, err
}

func scanWorkflowStage(row scanner) (WorkflowStageRecord, error) {
	var stage WorkflowStageRecord
	var readyAt sql.NullString
	var startedAt sql.NullString
	var completedAt sql.NullString
	var failedAt sql.NullString
	var outputJSON sql.NullString
	var outputJSONSHA256 sql.NullString

	err := row.Scan(
		&stage.RunID,
		&stage.StageIndex,
		&stage.StepID,
		&stage.StageSourceReference,
		&stage.State,
		&stage.CreatedAt,
		&readyAt,
		&startedAt,
		&completedAt,
		&failedAt,
		&outputJSON,
		&outputJSONSHA256,
	)
	if err != nil {
		return WorkflowStageRecord{}, err
	}

	stage.ReadyAt = readyAt.String
	stage.StartedAt = startedAt.String
	stage.CompletedAt = completedAt.String
	stage.FailedAt = failedAt.String
	stage.OutputJSON = outputJSON.String
	stage.OutputJSONSHA256 = outputJSONSHA256.String
	return stage, nil
}

func scanWorkItem(row scanner) (WorkItemRecord, error) {
	var item WorkItemRecord
	err := row.Scan(
		&item.ID,
		&item.RunID,
		&item.StageIndex,
		&item.WorkItemIndex,
		&item.WorkerPayloadJSON,
		&item.ResolvedInputsSHA256,
		&item.CreatedAt,
	)
	return item, err
}

func scanQueuedWork(row scanner) (QueuedWorkRecord, error) {
	var item QueuedWorkRecord
	err := row.Scan(
		&item.ID,
		&item.RunID,
		&item.StageIndex,
		&item.WorkItemIndex,
		&item.WorkerPayloadJSON,
		&item.ResolvedInputsSHA256,
		&item.CreatedAt,
		&item.QueuedAt,
		&item.ResumeArtifactID,
	)
	return item, err
}

func scanQueuedResourceConstraintCheck(row scanner) (QueuedResourceConstraintCheckRecord, error) {
	var check QueuedResourceConstraintCheckRecord
	err := row.Scan(
		&check.WorkItemID,
		&check.QueuedAt,
		&check.ConstraintIndex,
		&check.ResourceKey,
		&check.TotalUnits,
		&check.RequestedUnits,
		&check.Operator,
		&check.TargetUnits,
	)
	return check, err
}

func workerSessionSelectSQL() string {
	return `SELECT
		worker_session_id,
		worker_id,
		status,
		registered_at,
		last_heartbeat_at,
		ended_at,
		end_reason,
		execution_handle
	FROM worker_sessions`
}

func getWorkerSession(ctx context.Context, q queryer, workerID string, sessionID string) (WorkerSessionRecord, bool, error) {
	session, err := scanWorkerSession(q.QueryRowContext(ctx, workerSessionSelectSQL()+`
	WHERE worker_id = ? AND worker_session_id = ?`, workerID, sessionID))
	if err == sql.ErrNoRows {
		return WorkerSessionRecord{}, false, nil
	}
	if err != nil {
		return WorkerSessionRecord{}, false, fmt.Errorf("get worker session %s/%s: %w", workerID, sessionID, err)
	}
	return session, true, nil
}

func getWorkerSessionByID(ctx context.Context, q queryer, sessionID string) (WorkerSessionRecord, bool, error) {
	session, err := scanWorkerSession(q.QueryRowContext(ctx, workerSessionSelectSQL()+`
	WHERE worker_session_id = ?`, sessionID))
	if err == sql.ErrNoRows {
		return WorkerSessionRecord{}, false, nil
	}
	if err != nil {
		return WorkerSessionRecord{}, false, fmt.Errorf("get worker session %s: %w", sessionID, err)
	}
	return session, true, nil
}

func listWorkerSessionsByHeartbeat(ctx context.Context, q rowsQueryer, operator string, cutoff time.Time) ([]WorkerSessionRecord, error) {
	if operator != `<` && operator != `>=` {
		return nil, fmt.Errorf("unsupported worker session heartbeat operator %q", operator)
	}
	rows, err := q.QueryContext(ctx, workerSessionSelectSQL()+`
	WHERE status = ?
		AND last_heartbeat_at `+operator+` ?
	ORDER BY last_heartbeat_at, worker_session_id`,
		WorkerSessionStatusActive,
		cutoff.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("list worker sessions by heartbeat: %w", err)
	}
	defer rows.Close()

	records := []WorkerSessionRecord{}
	for rows.Next() {
		record, err := scanWorkerSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list worker sessions by heartbeat: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list worker sessions by heartbeat: %w", err)
	}
	return records, nil
}

func scanWorkerSession(row scanner) (WorkerSessionRecord, error) {
	var session WorkerSessionRecord
	var endedAt sql.NullString
	var endReason sql.NullString
	var executionHandle sql.NullString
	err := row.Scan(
		&session.ID,
		&session.WorkerID,
		&session.Status,
		&session.RegisteredAt,
		&session.LastHeartbeatAt,
		&endedAt,
		&endReason,
		&executionHandle,
	)
	if err != nil {
		return WorkerSessionRecord{}, err
	}
	session.EndedAt = endedAt.String
	session.EndReason = endReason.String
	session.ExecutionHandle = executionHandle.String
	return session, nil
}

func scanAbandonedWork(row scanner) (AbandonedWorkRecord, error) {
	var record AbandonedWorkRecord
	err := row.Scan(
		&record.AttemptID,
		&record.WorkItemID,
		&record.WorkerID,
		&record.WorkerSessionID,
		&record.QueuedAt,
		&record.StartedAt,
		&record.AbandonedAt,
		&record.Reason,
	)
	return record, err
}

func scanWorkflowDependencyStep(row scanner) (WorkflowDependencyStepRecord, error) {
	var step WorkflowDependencyStepRecord
	err := row.Scan(
		&step.RunID,
		&step.StageIndex,
		&step.StepIndex,
		&step.StepID,
		&step.ParallelWith,
		&step.CreatedAt,
	)
	return step, err
}

func scanWorkflowDependencyWorkItem(row scanner) (WorkflowDependencyWorkItemRecord, error) {
	var item WorkflowDependencyWorkItemRecord
	err := row.Scan(
		&item.RunID,
		&item.StageIndex,
		&item.StepIndex,
		&item.WorkItemID,
		&item.WorkItemIndex,
		&item.CreatedAt,
	)
	return item, err
}

func scanWorkflowStepOutputFact(row scanner) (WorkflowStepOutputFactRecord, error) {
	var fact WorkflowStepOutputFactRecord
	var outputJSON sql.NullString
	var outputJSONPruned int
	err := row.Scan(
		&fact.RunID,
		&fact.StepIndex,
		&outputJSON,
		&fact.OutputJSONSHA256,
		&fact.OutputJSONBytes,
		&outputJSONPruned,
		&fact.OutputKind,
		&fact.CreatedAt,
		&fact.UpdatedAt,
	)
	if err != nil {
		return WorkflowStepOutputFactRecord{}, err
	}
	fact.OutputJSON = outputJSON.String
	fact.OutputJSONPruned = outputJSONPruned != 0
	return fact, nil
}

func runningWorkSelectSQL() string {
	return `SELECT
		running_work.attempt_id,
		work_items.work_item_id,
		work_items.run_id,
		work_items.stage_index,
		work_items.work_item_index,
		work_items.worker_payload_json,
		work_items.resolved_inputs_sha256,
		work_items.created_at,
		running_work.worker_id,
		running_work.worker_session_id,
		work_item_attempts.executor_type,
		running_work.queued_at,
		running_work.started_at
	FROM running_work
	JOIN work_items ON work_items.work_item_id = running_work.work_item_id
	JOIN work_item_attempts ON work_item_attempts.attempt_id = running_work.attempt_id`
}

func scanRunningWork(row scanner) (RunningWorkRecord, error) {
	var record RunningWorkRecord
	var workerID sql.NullString
	var workerSessionID sql.NullString
	err := row.Scan(
		&record.AttemptID,
		&record.WorkItem.ID,
		&record.WorkItem.RunID,
		&record.WorkItem.StageIndex,
		&record.WorkItem.WorkItemIndex,
		&record.WorkItem.WorkerPayloadJSON,
		&record.WorkItem.ResolvedInputsSHA256,
		&record.WorkItem.CreatedAt,
		&workerID,
		&workerSessionID,
		&record.ExecutorType,
		&record.QueuedAt,
		&record.StartedAt,
	)
	if err != nil {
		return RunningWorkRecord{}, err
	}
	record.WorkerID = workerID.String
	record.WorkerSessionID = workerSessionID.String
	return record, nil
}

func terminalAttemptSelectSQL() string {
	return `SELECT
		terminal_attempts.attempt_id,
		terminal_attempts.work_item_id,
		terminal_attempts.run_id,
		terminal_attempts.stage_index,
		terminal_attempts.work_item_index,
		terminal_attempts.worker_payload_json,
		terminal_attempts.resolved_inputs_sha256,
		terminal_attempts.created_at,
		terminal_attempts.terminal_state,
		terminal_attempts.worker_id,
		terminal_attempts.executor_type,
		terminal_attempts.queued_at,
		terminal_attempts.started_at,
		terminal_attempts.finished_at,
		terminal_attempts.error,
		terminal_attempts.skipped_parent_id,
		terminal_attempts.output_json,
		terminal_attempts.output_json_sha256,
		terminal_attempts.pre_state_sha256,
		terminal_attempts.post_state_sha256
	FROM (
		SELECT
			completed_work.attempt_id,
			work_items.work_item_id,
			work_items.run_id,
			work_items.stage_index,
			work_items.work_item_index,
			work_items.worker_payload_json,
			work_items.resolved_inputs_sha256,
			work_items.created_at,
			'completed' AS terminal_state,
			work_item_attempts.worker_id,
			work_item_attempts.executor_type,
			completed_work.queued_at,
			completed_work.started_at,
			completed_work.completed_at AS finished_at,
			NULL AS error,
			completed_work.skipped_parent_id,
			completed_work.output_json,
			completed_work.output_json_sha256,
			completed_work.pre_state_sha256,
			completed_work.post_state_sha256
		FROM completed_work
		JOIN work_items ON work_items.work_item_id = completed_work.work_item_id
		JOIN work_item_attempts ON work_item_attempts.attempt_id = completed_work.attempt_id
		UNION ALL
		SELECT
			failed_work.attempt_id,
			work_items.work_item_id,
			work_items.run_id,
			work_items.stage_index,
			work_items.work_item_index,
			work_items.worker_payload_json,
			work_items.resolved_inputs_sha256,
			work_items.created_at,
			'failed' AS terminal_state,
			work_item_attempts.worker_id,
			work_item_attempts.executor_type,
			failed_work.queued_at,
			failed_work.started_at,
			failed_work.failed_at AS finished_at,
			failed_work.error,
			NULL AS skipped_parent_id,
			NULL AS output_json,
			NULL AS output_json_sha256,
			NULL AS pre_state_sha256,
			NULL AS post_state_sha256
		FROM failed_work
		JOIN work_items ON work_items.work_item_id = failed_work.work_item_id
		JOIN work_item_attempts ON work_item_attempts.attempt_id = failed_work.attempt_id
	) AS terminal_attempts`
}

func scanTerminalAttempt(row scanner) (TerminalAttemptRecord, error) {
	var record TerminalAttemptRecord
	var workerID sql.NullString
	var errorText sql.NullString
	var skippedParentID sql.NullString
	var outputJSON sql.NullString
	var outputJSONSHA256 sql.NullString
	var preStateSHA256 sql.NullString
	var postStateSHA256 sql.NullString
	err := row.Scan(
		&record.AttemptID,
		&record.WorkItem.ID,
		&record.WorkItem.RunID,
		&record.WorkItem.StageIndex,
		&record.WorkItem.WorkItemIndex,
		&record.WorkItem.WorkerPayloadJSON,
		&record.WorkItem.ResolvedInputsSHA256,
		&record.WorkItem.CreatedAt,
		&record.TerminalState,
		&workerID,
		&record.ExecutorType,
		&record.QueuedAt,
		&record.StartedAt,
		&record.FinishedAt,
		&errorText,
		&skippedParentID,
		&outputJSON,
		&outputJSONSHA256,
		&preStateSHA256,
		&postStateSHA256,
	)
	if err != nil {
		return TerminalAttemptRecord{}, err
	}
	record.WorkerID = workerID.String
	record.Error = errorText.String
	record.SkippedParentID = skippedParentID.String
	record.OutputJSON = outputJSON.String
	record.OutputJSONSHA256 = outputJSONSHA256.String
	record.PreStateSHA256 = preStateSHA256.String
	record.PostStateSHA256 = postStateSHA256.String
	return record, nil
}

func scanCompletedWork(row scanner) (CompletedWorkRecord, error) {
	var completed CompletedWorkRecord
	var skippedParentID sql.NullString
	err := row.Scan(
		&completed.AttemptID,
		&completed.WorkItemID,
		&skippedParentID,
		&completed.OutputJSON,
		&completed.OutputJSONSHA256,
		&completed.PreStateSHA256,
		&completed.PostStateSHA256,
		&completed.QueuedAt,
		&completed.StartedAt,
		&completed.CompletedAt,
	)
	if err != nil {
		return CompletedWorkRecord{}, err
	}
	completed.SkippedParentID = skippedParentID.String
	return completed, nil
}

func scanFailedWork(row scanner) (FailedWorkRecord, error) {
	var failed FailedWorkRecord
	err := row.Scan(
		&failed.AttemptID,
		&failed.WorkItemID,
		&failed.Error,
		&failed.QueuedAt,
		&failed.StartedAt,
		&failed.FailedAt,
	)
	return failed, err
}

func (s *Store) requireOpen() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not open")
	}
	return nil
}

func (r QueueWorkItemsRequest) validate() error {
	if err := validateWorkItems(r.WorkItems); err != nil {
		return err
	}
	if err := validateQueuedWorkItems(r.QueuedWork); err != nil {
		return err
	}
	if len(r.ResourceConstraints) != 0 {
		if err := validateWorkItemResourceConstraints(r.ResourceConstraints); err != nil {
			return err
		}
	}
	return nil
}

func (r WorkflowRunRecord) validate() error {
	if r.ID == "" {
		return fmt.Errorf("run id is required")
	}
	if r.ProjectID == "" {
		return fmt.Errorf("run project id is required")
	}
	if r.WorkflowID == "" {
		return fmt.Errorf("run workflow id is required")
	}
	if r.SubmissionContextJSON == "" {
		return fmt.Errorf("run submission context json is required")
	}
	if !json.Valid([]byte(r.SubmissionContextJSON)) {
		return fmt.Errorf("run submission context json must be valid JSON")
	}
	if r.CreatedAt == "" {
		return fmt.Errorf("run created at is required")
	}
	return nil
}

func (s WorkflowStageRecord) validate() error {
	if s.RunID == "" {
		return fmt.Errorf("stage run id is required")
	}
	if s.StageIndex < 0 {
		return fmt.Errorf("stage index must be non-negative")
	}
	if s.StepID == "" {
		return fmt.Errorf("stage step id is required")
	}
	if s.StageSourceReference == "" {
		return fmt.Errorf("stage source reference is required")
	}
	if !validStageState(s.State) {
		return fmt.Errorf("unsupported stage state: %s", s.State)
	}
	if s.CreatedAt == "" {
		return fmt.Errorf("stage created at is required")
	}
	if s.OutputJSON != "" && !json.Valid([]byte(s.OutputJSON)) {
		return fmt.Errorf("stage output json must be valid JSON")
	}
	return nil
}

func (r CompleteStageRequest) validate() error {
	if r.RunID == "" {
		return fmt.Errorf("stage run id is required")
	}
	if r.StageIndex < 0 {
		return fmt.Errorf("stage index must be non-negative")
	}
	if r.OutputJSON == "" {
		return fmt.Errorf("stage output json is required")
	}
	if !json.Valid([]byte(r.OutputJSON)) {
		return fmt.Errorf("stage output json must be valid JSON")
	}
	if r.OutputJSONSHA256 == "" {
		return fmt.Errorf("stage output json sha256 is required")
	}
	if r.CompletedAt == "" {
		return fmt.Errorf("stage completed at is required")
	}
	if len(r.ReadyWorkItems) != 0 {
		if err := validateWorkItems(r.ReadyWorkItems); err != nil {
			return fmt.Errorf("ready work: %w", err)
		}
	}
	if len(r.ReadyResourceConstraints) != 0 {
		if err := validateWorkItemResourceConstraints(r.ReadyResourceConstraints); err != nil {
			return fmt.Errorf("ready resource constraints: %w", err)
		}
	}
	if len(r.ReadyQueuedWork) != 0 {
		if err := validateQueuedWorkItems(r.ReadyQueuedWork); err != nil {
			return fmt.Errorf("ready queue: %w", err)
		}
	}
	return nil
}

func validStageState(state string) bool {
	switch state {
	case "ready", "running", "completed", "failed", "skipped", "blocked":
		return true
	default:
		return false
	}
}

func (w WorkItemRecord) validate() error {
	if w.ID == "" {
		return fmt.Errorf("work item id is required")
	}
	if w.RunID == "" {
		return fmt.Errorf("work item run id is required")
	}
	if w.StageIndex < 0 {
		return fmt.Errorf("work item stage index must be non-negative")
	}
	if w.WorkItemIndex < 0 {
		return fmt.Errorf("work item index must be non-negative")
	}
	if w.WorkerPayloadJSON == "" {
		return fmt.Errorf("work item worker payload json is required")
	}
	if !json.Valid([]byte(w.WorkerPayloadJSON)) {
		return fmt.Errorf("work item worker payload json must be valid JSON")
	}
	if w.ResolvedInputsSHA256 == "" {
		return fmt.Errorf("work item resolved inputs sha256 is required")
	}
	if w.CreatedAt == "" {
		return fmt.Errorf("work item created at is required")
	}
	return nil
}

func (r WorkItemResourceConstraintRecord) validate() error {
	if r.WorkItemID == "" {
		return fmt.Errorf("resource constraint work item id is required")
	}
	if r.ConstraintIndex < 0 {
		return fmt.Errorf("resource constraint index must be non-negative")
	}
	if r.ResourceKey == "" {
		return fmt.Errorf("resource constraint key is required")
	}
	if r.RequestedUnits <= 0 {
		return fmt.Errorf("resource constraint requested units must be positive")
	}
	if !validResourceConstraintOperator(r.Operator) {
		return fmt.Errorf("unsupported resource constraint operator: %s", r.Operator)
	}
	if r.TargetUnits < 0 {
		return fmt.Errorf("resource constraint target units must be non-negative")
	}
	if r.CreatedAt == "" {
		return fmt.Errorf("resource constraint created at is required")
	}
	return nil
}

func (s WorkflowDependencyStepRecord) validate() error {
	if s.RunID == "" {
		return fmt.Errorf("dependency step run id is required")
	}
	if s.StageIndex < 0 {
		return fmt.Errorf("dependency step stage index must be non-negative")
	}
	if s.StepIndex < 0 {
		return fmt.Errorf("dependency step index must be non-negative")
	}
	if s.StepID == "" {
		return fmt.Errorf("dependency step id is required")
	}
	if s.CreatedAt == "" {
		return fmt.Errorf("dependency step created at is required")
	}
	return nil
}

func (r WorkflowDependencyWorkItemRecord) validate() error {
	if r.RunID == "" {
		return fmt.Errorf("dependency work item run id is required")
	}
	if r.WorkItemID == "" {
		return fmt.Errorf("dependency work item id is required")
	}
	if r.WorkItemIndex < 0 {
		return fmt.Errorf("dependency work item index must be non-negative")
	}
	if r.StageIndex < 0 {
		return fmt.Errorf("dependency stage index must be non-negative")
	}
	if r.StepIndex < 0 {
		return fmt.Errorf("dependency step index must be non-negative")
	}
	if r.CreatedAt == "" {
		return fmt.Errorf("dependency work item created at is required")
	}
	return nil
}

func (r WorkflowStepOutputFactRecord) validate() error {
	if r.RunID == "" {
		return fmt.Errorf("workflow step output fact run id is required")
	}
	if r.StepIndex < 0 {
		return fmt.Errorf("workflow step output fact step index must be non-negative")
	}
	if r.OutputJSON != "" && !json.Valid([]byte(r.OutputJSON)) {
		return fmt.Errorf("workflow step output fact output json must be valid JSON")
	}
	if r.OutputJSONSHA256 == "" {
		return fmt.Errorf("workflow step output fact output json sha256 is required")
	}
	if r.OutputJSONBytes < 0 {
		return fmt.Errorf("workflow step output fact output json bytes must be non-negative")
	}
	if !validWorkflowStepOutputKind(r.OutputKind) {
		return fmt.Errorf("unsupported workflow step output kind: %s", r.OutputKind)
	}
	if r.CreatedAt == "" {
		return fmt.Errorf("workflow step output fact created at is required")
	}
	if r.UpdatedAt == "" {
		return fmt.Errorf("workflow step output fact updated at is required")
	}
	return nil
}

func validateWorkItems(items []WorkItemRecord) error {
	if len(items) == 0 {
		return fmt.Errorf("work items are required")
	}
	for index, item := range items {
		if err := item.validate(); err != nil {
			return fmt.Errorf("work item %d: %w", index, err)
		}
	}
	return nil
}

func validateWorkItemResourceConstraints(constraints []WorkItemResourceConstraintRecord) error {
	if len(constraints) == 0 {
		return fmt.Errorf("resource constraints are required")
	}
	for index, constraint := range constraints {
		if err := constraint.validate(); err != nil {
			return fmt.Errorf("resource constraint %d: %w", index, err)
		}
	}
	return nil
}

func validateWorkflowDependencySteps(steps []WorkflowDependencyStepRecord) error {
	if len(steps) == 0 {
		return fmt.Errorf("dependency steps are required")
	}
	for index, step := range steps {
		if err := step.validate(); err != nil {
			return fmt.Errorf("dependency step %d: %w", index, err)
		}
	}
	return nil
}

func validateWorkflowDependencyWorkItemRecords(items []WorkflowDependencyWorkItemRecord) error {
	if len(items) == 0 {
		return fmt.Errorf("dependency work items are required")
	}
	for index, item := range items {
		if err := item.validate(); err != nil {
			return fmt.Errorf("dependency work item %d: %w", index, err)
		}
	}
	return nil
}

func validateQueuedWorkItems(items []QueuedWorkRecord) error {
	if len(items) == 0 {
		return fmt.Errorf("queued work items are required")
	}
	for index, item := range items {
		if item.ID == "" {
			return fmt.Errorf("queued work item %d id is required", index)
		}
		if item.QueuedAt == "" {
			return fmt.Errorf("queued work item %d queued at is required", index)
		}
	}
	return nil
}

func (r ClaimWorkRequest) validate() error {
	if r.AttemptID == "" {
		return fmt.Errorf("claim attempt id is required")
	}
	if !validExecutorType(r.ExecutorType) {
		return fmt.Errorf("unsupported claim executor type: %s", r.ExecutorType)
	}
	switch r.ExecutorType {
	case ExecutorTypeWorker:
		if r.WorkerID == "" {
			return fmt.Errorf("claim worker id is required for worker executor")
		}
		if r.WorkerSessionID == "" {
			return fmt.Errorf("claim worker session id is required for worker executor")
		}
	case ExecutorTypeController:
		if r.WorkerID != "" {
			return fmt.Errorf("claim worker id must be empty for controller executor")
		}
		if r.WorkerSessionID != "" {
			return fmt.Errorf("claim worker session id must be empty for controller executor")
		}
	}
	if r.StartedAt == "" {
		return fmt.Errorf("claim started at is required")
	}
	if err := validateTimestamp("claim started at", r.StartedAt); err != nil {
		return err
	}
	if r.LiveSessionCutoff != "" {
		if err := validateTimestamp("claim live session cutoff", r.LiveSessionCutoff); err != nil {
			return err
		}
	}
	if r.ResumeAttemptLimit < 0 {
		return fmt.Errorf("claim resume attempt limit must not be negative")
	}
	return nil
}

func (r RegisterWorkerSessionRequest) validate() error {
	if r.WorkerID == "" {
		return fmt.Errorf("worker id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("worker session id is required")
	}
	if r.RegisteredAt == "" {
		return fmt.Errorf("worker session registered at is required")
	}
	return validateTimestamp("worker session registered at", r.RegisteredAt)
}

func (r HeartbeatWorkerSessionRequest) validate() error {
	if r.WorkerID == "" {
		return fmt.Errorf("worker id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("worker session id is required")
	}
	if r.HeartbeatAt == "" {
		return fmt.Errorf("worker session heartbeat at is required")
	}
	return validateTimestamp("worker session heartbeat at", r.HeartbeatAt)
}

func (r EndWorkerSessionRequest) validate() error {
	if r.WorkerID == "" {
		return fmt.Errorf("worker id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("worker session id is required")
	}
	if r.Status != WorkerSessionStatusStopped && r.Status != WorkerSessionStatusDead {
		return fmt.Errorf("worker session terminal status must be stopped or dead")
	}
	if r.EndedAt == "" {
		return fmt.Errorf("worker session ended at is required")
	}
	if err := validateTimestamp("worker session ended at", r.EndedAt); err != nil {
		return err
	}
	if r.Reason == "" {
		return fmt.Errorf("worker session end reason is required")
	}
	return nil
}

func (r StopWorkerSessionAndRecoverWorkRequest) validate() error {
	if r.WorkerID == "" {
		return fmt.Errorf("worker id is required")
	}
	if r.SessionID == "" {
		return fmt.Errorf("worker session id is required")
	}
	if r.StoppedAt == "" {
		return fmt.Errorf("worker session stopped at is required")
	}
	if err := validateTimestamp("worker session stopped at", r.StoppedAt); err != nil {
		return err
	}
	if r.Reason == "" {
		return fmt.Errorf("worker session stop reason is required")
	}
	return nil
}

func (r RecoverExpiredWorkerSessionsRequest) validate() error {
	if r.Cutoff == "" {
		return fmt.Errorf("recover expired worker sessions cutoff is required")
	}
	if err := validateTimestamp("recover expired worker sessions cutoff", r.Cutoff); err != nil {
		return err
	}
	if r.RecoveredAt == "" {
		return fmt.Errorf("recover expired worker sessions recovered at is required")
	}
	if err := validateTimestamp("recover expired worker sessions recovered at", r.RecoveredAt); err != nil {
		return err
	}
	if r.Reason == "" {
		return fmt.Errorf("recover expired worker sessions reason is required")
	}
	return nil
}

func (r CompleteAttemptRequest) validate() error {
	if r.AttemptID == "" {
		return fmt.Errorf("complete attempt id is required")
	}
	if (r.WorkerID == "") != (r.WorkerSessionID == "") {
		return fmt.Errorf("complete worker id and worker session id must be provided together")
	}
	if r.LiveSessionCutoff != "" {
		if err := validateTimestamp("complete live session cutoff", r.LiveSessionCutoff); err != nil {
			return err
		}
	}
	if r.OutputJSON == "" {
		return fmt.Errorf("complete output json is required")
	}
	if !json.Valid([]byte(r.OutputJSON)) {
		return fmt.Errorf("complete output json must be valid JSON")
	}
	if r.OutputJSONSHA256 == "" {
		return fmt.Errorf("complete output json sha256 is required")
	}
	if r.PreStateSHA256 == "" {
		return fmt.Errorf("complete pre state sha256 is required")
	}
	if r.PostStateSHA256 == "" {
		return fmt.Errorf("complete post state sha256 is required")
	}
	if r.CompletedAt == "" {
		return fmt.Errorf("complete completed at is required")
	}
	return nil
}

func (r FailAttemptRequest) validate() error {
	if r.AttemptID == "" {
		return fmt.Errorf("fail attempt id is required")
	}
	if (r.WorkerID == "") != (r.WorkerSessionID == "") {
		return fmt.Errorf("fail worker id and worker session id must be provided together")
	}
	if r.LiveSessionCutoff != "" {
		if err := validateTimestamp("fail live session cutoff", r.LiveSessionCutoff); err != nil {
			return err
		}
	}
	if r.Error == "" {
		return fmt.Errorf("fail error is required")
	}
	if r.FailedAt == "" {
		return fmt.Errorf("fail failed at is required")
	}
	return nil
}

func completedWorkMatchesRequest(completed CompletedWorkRecord, request CompleteAttemptRequest) bool {
	return completed.AttemptID == request.AttemptID &&
		completed.SkippedParentID == request.SkippedParentID &&
		completed.OutputJSON == request.OutputJSON &&
		completed.OutputJSONSHA256 == request.OutputJSONSHA256 &&
		completed.PreStateSHA256 == request.PreStateSHA256 &&
		completed.PostStateSHA256 == request.PostStateSHA256
}

func failedWorkMatchesRequest(failed FailedWorkRecord, request FailAttemptRequest) bool {
	return failed.AttemptID == request.AttemptID &&
		failed.Error == request.Error &&
		failed.FailedAt == request.FailedAt
}

func stageMatchesCompletionRequest(stage WorkflowStageRecord, request CompleteStageRequest) bool {
	return stage.RunID == request.RunID &&
		stage.StageIndex == request.StageIndex &&
		stage.CompletedAt == request.CompletedAt &&
		stage.OutputJSON == request.OutputJSON &&
		stage.OutputJSONSHA256 == request.OutputJSONSHA256
}

func validExecutorType(executorType string) bool {
	switch executorType {
	case ExecutorTypeWorker, ExecutorTypeController:
		return true
	default:
		return false
	}
}

func validateTimestamp(name string, value string) error {
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return fmt.Errorf("%s must be RFC3339/RFC3339Nano: %w", name, err)
	}
	return nil
}

func workerSessionMatchesRegistration(session WorkerSessionRecord, request RegisterWorkerSessionRequest) bool {
	return session.ID == request.SessionID &&
		session.WorkerID == request.WorkerID &&
		session.Status == WorkerSessionStatusActive &&
		session.RegisteredAt == request.RegisteredAt &&
		session.LastHeartbeatAt == request.RegisteredAt &&
		session.EndedAt == "" &&
		session.EndReason == "" &&
		session.ExecutionHandle == request.ExecutionHandle
}

func validResourceConstraintOperator(operator string) bool {
	switch operator {
	case "=", "!=", "<", ">", "<=", ">=":
		return true
	default:
		return false
	}
}

func validWorkflowStepOutputKind(outputKind string) bool {
	switch outputKind {
	case "aggregate", "empty_fanout", "skipped":
		return true
	default:
		return false
	}
}

func (p ProjectRecord) validate() error {
	if p.ID == "" {
		return fmt.Errorf("project id is required")
	}
	if p.RepositoryIdentity == "" {
		return fmt.Errorf("project repository identity is required")
	}
	if p.ConfigPath == "" {
		return fmt.Errorf("project config path is required")
	}
	if p.ConfigSHA256 == "" {
		return fmt.Errorf("project config sha256 is required")
	}
	if p.CreatedAt == "" {
		return fmt.Errorf("project created at is required")
	}
	return nil
}

func (w WorkflowRecord) validate() error {
	if w.ID == "" {
		return fmt.Errorf("workflow id is required")
	}
	if w.ProjectID == "" {
		return fmt.Errorf("workflow project id is required")
	}
	if w.RepositoryIdentity == "" {
		return fmt.Errorf("workflow repository identity is required")
	}
	if w.WorkflowPath == "" {
		return fmt.Errorf("workflow path is required")
	}
	if w.WorkflowSHA256 == "" {
		return fmt.Errorf("workflow sha256 is required")
	}
	if w.CreatedAt == "" {
		return fmt.Errorf("workflow created at is required")
	}
	return nil
}

func rowsAffected(result sql.Result) (bool, error) {
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count != 0, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullPositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullStringPtr(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func stringPtrFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func sameStringPtr(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func sameProjectRecord(left ProjectRecord, right ProjectRecord) bool {
	return left.ID == right.ID &&
		left.Name == right.Name &&
		left.RepositoryIdentity == right.RepositoryIdentity &&
		sameStringPtr(left.SourceRevisionID, right.SourceRevisionID) &&
		left.ConfigPath == right.ConfigPath &&
		left.SourceObjectID == right.SourceObjectID &&
		left.ConfigSHA256 == right.ConfigSHA256 &&
		left.CreatedAt == right.CreatedAt
}

func sameWorkflowRecord(left WorkflowRecord, right WorkflowRecord) bool {
	return left.ID == right.ID &&
		left.ProjectID == right.ProjectID &&
		left.Name == right.Name &&
		left.RepositoryIdentity == right.RepositoryIdentity &&
		sameStringPtr(left.SourceRevisionID, right.SourceRevisionID) &&
		left.WorkflowPath == right.WorkflowPath &&
		left.SourceObjectID == right.SourceObjectID &&
		left.WorkflowSHA256 == right.WorkflowSHA256 &&
		left.CreatedAt == right.CreatedAt
}

func sameStagePlan(left []WorkflowStageRecord, right []WorkflowStageRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameWorkflowDependencyStepPlan(left []WorkflowDependencyStepRecord, right []WorkflowDependencyStepRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

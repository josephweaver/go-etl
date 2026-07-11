package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"goetl/internal/controllerhttp"
	"goetl/internal/model"
)

type WorkEvidence struct {
	Skipped         bool
	SkippedParentID string
	SkipReason      string
	InputSHA256     string
	OutputSHA256    string
	PreStateSHA256  string
	PostStateSHA256 string
	OutputJSON      string
	PreStateJSON    string
	PostStateJSON   string
}

const maxWorkerControllerTokenBytes = 32 * 1024
const workerIDHeader = "X-Goetl-Worker-Id"
const workerSessionIDHeader = "X-Goetl-Worker-Session-Id"

type WorkerControllerClient struct {
	client        controllerhttp.Client
	authenticated bool
	initialized   bool
}

func NewWorkerControllerClient(cfg Config) (WorkerControllerClient, error) {
	requiresToken, err := controllerURLRequiresTokenFile(cfg.ControllerURL)
	if err != nil {
		return WorkerControllerClient{}, err
	}
	if requiresToken && cfg.ControllerTokenFile == "" {
		return WorkerControllerClient{}, fmt.Errorf("controller token file is required for controller url %s", cfg.ControllerURL)
	}
	tokenProvider, authenticated, err := loadWorkerControllerTokenProvider(cfg.ControllerTokenFile)
	if err != nil {
		return WorkerControllerClient{}, err
	}
	client, err := controllerhttp.New(controllerhttp.Config{
		BaseURL:                   cfg.ControllerURL,
		Token:                     tokenProvider,
		Caller:                    "goetl-worker/1",
		AllowInsecureExternalHTTP: cfg.ControllerInsecureExternalHTTPAllowed,
	})
	if err != nil {
		return WorkerControllerClient{}, err
	}
	return WorkerControllerClient{client: client, authenticated: authenticated, initialized: true}, nil
}

func (c WorkerControllerClient) Initialized() bool {
	return c.initialized
}

func loadWorkerControllerTokenProvider(path string) (controllerhttp.TokenProvider, bool, error) {
	if path == "" {
		return nil, false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, fmt.Errorf("controller token file %q stat failed: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("controller token file %q must be a regular file", path)
	}
	if info.Size() == 0 {
		return nil, false, fmt.Errorf("controller token file %q is empty", path)
	}
	if info.Size() > maxWorkerControllerTokenBytes {
		return nil, false, fmt.Errorf("controller token file %q exceeds %d bytes", path, maxWorkerControllerTokenBytes)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, false, fmt.Errorf("controller token file %q permissions must not grant group or other access", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("controller token file %q read failed: %w", path, err)
	}
	token, err := controllerhttp.NewSensitiveToken(trimOneTrailingLineEnding(string(data)))
	if err != nil {
		return nil, false, fmt.Errorf("controller token file %q: %w", path, err)
	}
	return controllerhttp.NewStaticTokenProvider(token), true, nil
}

func trimOneTrailingLineEnding(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return strings.TrimSuffix(value, "\r\n")
	}
	if strings.HasSuffix(value, "\n") {
		return strings.TrimSuffix(value, "\n")
	}
	if strings.HasSuffix(value, "\r") {
		return strings.TrimSuffix(value, "\r")
	}
	return value
}

func reportWorkComplete(controllerURL string, item model.WorkItem, startedAt time.Time, evidence WorkEvidence) error {
	client, err := newUnauthenticatedWorkerControllerClient(controllerURL)
	if err != nil {
		return err
	}
	return client.ReportWorkComplete(item, startedAt, evidence)
}

func (c WorkerControllerClient) ReportWorkComplete(item model.WorkItem, startedAt time.Time, evidence WorkEvidence, sessions ...WorkerSession) error {
	request, err := c.newJSONRequest(context.Background(), http.MethodPost, "/work/complete", workCompletion(item, startedAt, evidence))
	if err != nil {
		return fmt.Errorf("create work completion request: %w", err)
	}
	if err := addWorkerSessionHeaders(request, sessions...); err != nil {
		return err
	}
	response, err := c.client.Do(request, http.StatusNoContent)
	if err != nil {
		return fmt.Errorf("post work completion: %w", err)
	}
	defer response.Body.Close()
	return nil
}

func workCompletion(item model.WorkItem, startedAt time.Time, evidence WorkEvidence) model.WorkCompletion {
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	completedAt := time.Now().UTC().Format(time.RFC3339)
	attemptID := item.AttemptID
	if attemptID == "" {
		attemptID = item.ID + "-attempt-" + randomHex(8)
	}

	completion := model.WorkCompletion{
		ID:                   item.ID,
		AttemptID:            attemptID,
		Skipped:              evidence.Skipped,
		SkippedParentID:      evidence.SkippedParentID,
		SkipReason:           evidence.SkipReason,
		InputSHA256:          evidence.InputSHA256,
		OutputSHA256:         evidence.OutputSHA256,
		PreStateSHA256:       evidence.PreStateSHA256,
		PostStateSHA256:      evidence.PostStateSHA256,
		OutputJSON:           evidence.OutputJSON,
		PreStateJSON:         evidence.PreStateJSON,
		PostStateJSON:        evidence.PostStateJSON,
		WorkflowDefinitionID: item.WorkflowDefinitionID,
		WorkflowFingerprint:  item.WorkflowFingerprint,
		WorkflowInstanceID:   item.WorkflowInstanceID,
		StepDefinitionID:     item.StepDefinitionID,
		StepFingerprint:      item.StepFingerprint,
		StepInstanceID:       item.StepInstanceID,
		WorkItemFingerprint:  item.WorkItemFingerprint,
		InputFingerprint:     item.InputFingerprint,
		OutputFingerprint:    item.OutputFingerprint,
		CodeVersion:          item.CodeVersion,
		StartedAt:            startedAt.UTC().Format(time.RFC3339),
		CompletedAt:          completedAt,
		Parameters:           item.Parameters,
	}

	if completion.WorkflowInstanceID == "" {
		completion.WorkflowInstanceID = "demo-workflow-instance"
	}
	if completion.WorkflowDefinitionID == "" {
		completion.WorkflowDefinitionID = "demo-workflow"
	}
	if completion.WorkflowFingerprint == "" {
		completion.WorkflowFingerprint = "demo-workflow-fingerprint"
	}
	if completion.StepInstanceID == "" {
		completion.StepInstanceID = "demo-step-instance"
	}
	if completion.StepDefinitionID == "" {
		completion.StepDefinitionID = "demo-step"
	}
	if completion.StepFingerprint == "" {
		completion.StepFingerprint = "demo-step-fingerprint"
	}
	if completion.WorkItemFingerprint == "" {
		completion.WorkItemFingerprint = "demo-work-item:" + item.ID
	}
	if completion.InputFingerprint == "" {
		completion.InputFingerprint = "demo-input:" + item.ID
	}
	if completion.OutputFingerprint == "" {
		completion.OutputFingerprint = "demo-output:" + item.ID
	}
	if completion.CodeVersion == "" {
		completion.CodeVersion = "demo"
	}

	return completion
}

func randomHex(byteCount int) string {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(data)
}

func reportWorkFailed(controllerURL string, item model.WorkItem, workErr error) error {
	client, err := newUnauthenticatedWorkerControllerClient(controllerURL)
	if err != nil {
		return err
	}
	return client.ReportWorkFailed(item, workErr)
}

func (c WorkerControllerClient) ReportWorkFailed(item model.WorkItem, workErr error, sessions ...WorkerSession) error {
	request, err := c.newJSONRequest(context.Background(), http.MethodPost, "/work/fail", model.WorkFailure{
		ID:        item.ID,
		AttemptID: item.AttemptID,
		FailedAt:  time.Now().UTC().Format(time.RFC3339),
		Error:     workErr.Error(),
	})
	if err != nil {
		return fmt.Errorf("create work failure request: %w", err)
	}
	if err := addWorkerSessionHeaders(request, sessions...); err != nil {
		return err
	}
	response, err := c.client.Do(request, http.StatusNoContent)
	if err != nil {
		return fmt.Errorf("post work failure: %w", err)
	}
	defer response.Body.Close()
	return nil
}

func fetchWorkItem(controllerURL string) (model.WorkItem, bool, error) {
	client, err := newUnauthenticatedWorkerControllerClient(controllerURL)
	if err != nil {
		return model.WorkItem{}, false, err
	}
	return client.FetchWorkItem()
}

func (c WorkerControllerClient) FetchWorkItem(sessions ...WorkerSession) (model.WorkItem, bool, error) {
	request, err := c.newRequest(context.Background(), http.MethodGet, "/work/next", nil)
	if err != nil {
		return model.WorkItem{}, false, fmt.Errorf("create work claim request: %w", err)
	}
	if err := addWorkerSessionHeaders(request, sessions...); err != nil {
		return model.WorkItem{}, false, err
	}
	response, err := c.client.Do(request, http.StatusOK, http.StatusNoContent)
	if err != nil {
		return model.WorkItem{}, false, fmt.Errorf("get work item: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return model.WorkItem{}, false, nil
	}

	var item model.WorkItem
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		return model.WorkItem{}, false, fmt.Errorf("decode work item: %w", err)
	}

	if err := item.Validate(); err != nil {
		return model.WorkItem{}, false, fmt.Errorf("validate work item: %w", err)
	}

	return item, true, nil
}

func addWorkerSessionHeaders(request *http.Request, sessions ...WorkerSession) error {
	if len(sessions) == 0 {
		return nil
	}
	if len(sessions) > 1 {
		return fmt.Errorf("at most one worker session can be attached to a controller request")
	}
	session := sessions[0]
	if err := session.ValidateIdentity(); err != nil {
		return err
	}
	request.Header.Set(workerIDHeader, session.WorkerID)
	request.Header.Set(workerSessionIDHeader, session.WorkerSessionID)
	return nil
}

func (c WorkerControllerClient) SourceBundle(runID string) ([]byte, error) {
	requestPath, err := controllerhttp.PathJoin("/workflow-runs", runID, "source-bundle.zip")
	if err != nil {
		return nil, fmt.Errorf("source bundle path: %w", err)
	}
	request, err := c.newRequest(context.Background(), http.MethodGet, requestPath, nil)
	if err != nil {
		return nil, fmt.Errorf("create source bundle request: %w", err)
	}
	response, err := c.client.Do(request, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("get source bundle: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read source bundle: %w", err)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("read source bundle: empty body")
	}
	return body, nil
}

func (c WorkerControllerClient) newRequest(ctx context.Context, method string, path string, body io.Reader) (*http.Request, error) {
	if c.authenticated {
		return c.client.NewRequest(ctx, method, path, body)
	}
	return c.client.NewPublicRequest(ctx, method, path, body)
}

func (c WorkerControllerClient) newJSONRequest(ctx context.Context, method string, path string, value any) (*http.Request, error) {
	if c.authenticated {
		return c.client.NewJSONRequest(ctx, method, path, value)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode controller request body: %w", err)
	}
	request, err := c.client.NewPublicRequest(ctx, method, path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func newUnauthenticatedWorkerControllerClient(controllerURL string) (WorkerControllerClient, error) {
	client, err := controllerhttp.New(controllerhttp.Config{
		BaseURL: controllerURL,
		Caller:  "goetl-worker/1",
	})
	if err != nil {
		return WorkerControllerClient{}, err
	}
	return WorkerControllerClient{client: client, initialized: true}, nil
}

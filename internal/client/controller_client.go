package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"goetl/internal/controllerhttp"
	"goetl/internal/model"
	"goetl/internal/reposource"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

type WorkflowSubmission struct {
	Workflow       workflow.Workflow                    `json:"workflow"`
	SourceManifest reposource.SourceManifestDeclaration `json:"source_manifest,omitempty"`
	Variables      []variable.Variable                  `json:"variables"`
}

type WorkflowRunSubmission struct {
	Project   SourceDocumentReference `json:"project"`
	Workflow  SourceDocumentReference `json:"workflow"`
	Variables []variable.Variable     `json:"variables,omitempty"`
}

type SourceDocumentReference struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Path       string `json:"path"`
}

type ControllerClient struct {
	httpClient    *http.Client
	resolver      variable.Resolver
	starter       ControllerStarter
	tokenProvider controllerhttp.TokenProvider
}

type ControllerStarter interface {
	StartController() error
}

type SubmissionLogsFilters struct {
	Tail      int
	TailSet   bool
	Level     string
	Stream    string
	AttemptID string
}

type SubmissionLogsResponse struct {
	SubmissionID string                 `json:"submission_id"`
	Entries      []model.LogObservation `json:"entries"`
	Tail         int                    `json:"tail"`
	Truncated    bool                   `json:"truncated"`
}

type ControllerClientOptions struct {
	Starter       ControllerStarter
	TokenProvider controllerhttp.TokenProvider
}

func NewControllerClient(httpClient *http.Client, resolver variable.Resolver) ControllerClient {
	return NewControllerClientWithStarter(httpClient, resolver, nil)
}

func NewControllerClientWithStarter(httpClient *http.Client, resolver variable.Resolver, starter ControllerStarter) ControllerClient {
	return NewControllerClientWithOptions(httpClient, resolver, ControllerClientOptions{Starter: starter})
}

func NewControllerClientWithOptions(httpClient *http.Client, resolver variable.Resolver, options ControllerClientOptions) ControllerClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return ControllerClient{
		httpClient:    httpClient,
		resolver:      resolver,
		starter:       options.Starter,
		tokenProvider: options.TokenProvider,
	}
}

func (c ControllerClient) SubmitWorkflowRun(submission WorkflowRunSubmission) error {
	_, err := c.SubmitWorkflowRunAcknowledgement(submission)
	return err
}

// SubmitWorkflow submits a legacy inline workflow payload. Prefer SubmitWorkflowRun.
func (c ControllerClient) SubmitWorkflow(submission WorkflowSubmission) error {
	_, err := c.SubmitWorkflowAcknowledgement(submission)
	return err
}

func (c ControllerClient) SubmitWorkflowRunAcknowledgement(submission WorkflowRunSubmission) (model.SubmissionAcknowledgement, error) {
	return c.submitWorkflowPayload(submission)
}

// SubmitWorkflowAcknowledgement submits a legacy inline workflow payload and returns the controller acknowledgement.
func (c ControllerClient) SubmitWorkflowAcknowledgement(submission WorkflowSubmission) (model.SubmissionAcknowledgement, error) {
	if err := submission.SourceManifest.Validate(); err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	return c.submitWorkflowPayload(submission)
}

func (c ControllerClient) submitWorkflowPayload(submission any) (model.SubmissionAcknowledgement, error) {
	controllerURL, err := c.controllerURL()
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}

	if err := c.EnsureController(controllerURL); err != nil {
		return model.SubmissionAcknowledgement{}, err
	}

	return c.SubmitWorkflowPayloadAcknowledgement(controllerURL, submission)
}

func (c ControllerClient) SubmitWorkflowPayloadAcknowledgement(controllerURL string, payload any) (model.SubmissionAcknowledgement, error) {
	httpClient, err := c.protectedControllerHTTPClient(controllerURL)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	request, err := httpClient.NewJSONRequest(context.Background(), http.MethodPost, "/workflow", payload)
	if err != nil {
		return model.SubmissionAcknowledgement{}, wrapControllerRequestError("submit workflow", err)
	}
	response, err := httpClient.Do(request, http.StatusAccepted)
	if err != nil {
		return model.SubmissionAcknowledgement{}, wrapControllerRequestError("submit workflow", err)
	}
	defer response.Body.Close()

	var acknowledgement model.SubmissionAcknowledgement
	if err := json.NewDecoder(response.Body).Decode(&acknowledgement); err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("decode submission acknowledgement: %w", err)
	}

	return acknowledgement, nil
}

func (c ControllerClient) SubmitWorkflowRunFile(path string) error {
	submission, err := LoadWorkflowRunSubmissionFile(path)
	if err != nil {
		return err
	}

	return c.SubmitWorkflowRun(submission)
}

// SubmitWorkflowFile submits a legacy inline workflow file. Prefer SubmitWorkflowRunFile.
func (c ControllerClient) SubmitWorkflowFile(path string) error {
	submission, err := LoadWorkflowSubmissionFile(path)
	if err != nil {
		return err
	}

	return c.SubmitWorkflow(submission)
}

// LoadWorkflowRunSubmissionFile loads the source-reference workflow-run submission format.
func LoadWorkflowRunSubmissionFile(path string) (WorkflowRunSubmission, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowRunSubmission{}, fmt.Errorf("read workflow run submission file: %w", err)
	}

	var submission WorkflowRunSubmission
	if err := json.Unmarshal(data, &submission); err != nil {
		return WorkflowRunSubmission{}, fmt.Errorf("decode workflow run submission file: %w", err)
	}

	return submission, nil
}

// LoadWorkflowSubmissionFile loads the legacy inline workflow submission format.
func LoadWorkflowSubmissionFile(path string) (WorkflowSubmission, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowSubmission{}, fmt.Errorf("read workflow submission file: %w", err)
	}

	var submission WorkflowSubmission
	if err := json.Unmarshal(data, &submission); err != nil {
		return WorkflowSubmission{}, fmt.Errorf("decode workflow submission file: %w", err)
	}
	if err := submission.SourceManifest.Validate(); err != nil {
		return WorkflowSubmission{}, fmt.Errorf("validate workflow source manifest: %w", err)
	}

	return submission, nil
}

func (c ControllerClient) EnsureController(controllerURL string) error {
	if err := c.CheckController(controllerURL); err == nil {
		return nil
	}

	if c.starter == nil || !localAutoStartControllerURL(controllerURL) {
		return c.CheckController(controllerURL)
	}

	if err := c.starter.StartController(); err != nil {
		return fmt.Errorf("start controller: %w", err)
	}

	return c.WaitForController(controllerURL, 10)
}

func (c ControllerClient) CheckController(controllerURL string) error {
	httpClient, err := c.controllerHTTPClient(controllerURL)
	if err != nil {
		return err
	}
	request, err := httpClient.NewPublicRequest(context.Background(), http.MethodGet, "/healthz", nil)
	if err != nil {
		return wrapControllerRequestError("check controller", err)
	}
	response, err := httpClient.Do(request, http.StatusNoContent)
	if err != nil {
		return wrapControllerRequestError("check controller", err)
	}
	return response.Body.Close()
}

func (c ControllerClient) WaitForController(controllerURL string, maxChecks int) error {
	if maxChecks <= 0 {
		return fmt.Errorf("max checks must be positive")
	}

	interval, err := c.statusPollInterval()
	if err != nil {
		return err
	}

	var lastErr error
	for check := range maxChecks {
		if err := c.CheckController(controllerURL); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if check < maxChecks-1 {
			time.Sleep(interval)
		}
	}

	return fmt.Errorf("controller did not become reachable: %w", lastErr)
}

func (c ControllerClient) ShutdownWhenIdle(maxChecks int) (model.ControllerStatus, error) {
	if maxChecks <= 0 {
		return model.ControllerStatus{}, fmt.Errorf("max checks must be positive")
	}

	controllerURL, err := c.controllerURL()
	if err != nil {
		return model.ControllerStatus{}, err
	}

	interval, err := c.statusPollInterval()
	if err != nil {
		return model.ControllerStatus{}, err
	}

	for check := range maxChecks {
		status, err := c.Status(controllerURL)
		if err != nil {
			return model.ControllerStatus{}, err
		}

		if status.Pending == 0 && status.Assigned == 0 {
			if err := c.Shutdown(controllerURL); err != nil {
				return model.ControllerStatus{}, err
			}
			return status, nil
		}

		if check < maxChecks-1 {
			time.Sleep(interval)
		}
	}

	return model.ControllerStatus{}, fmt.Errorf("controller still has pending or assigned work")
}

func (c ControllerClient) Status(controllerURL string) (model.ControllerStatus, error) {
	httpClient, err := c.protectedControllerHTTPClient(controllerURL)
	if err != nil {
		return model.ControllerStatus{}, err
	}
	request, err := httpClient.NewRequest(context.Background(), http.MethodGet, "/status", nil)
	if err != nil {
		return model.ControllerStatus{}, wrapControllerRequestError("get controller status", err)
	}
	response, err := httpClient.Do(request, http.StatusOK)
	if err != nil {
		return model.ControllerStatus{}, wrapControllerRequestError("get controller status", err)
	}
	defer response.Body.Close()

	var status model.ControllerStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return model.ControllerStatus{}, fmt.Errorf("decode controller status: %w", err)
	}

	return status, nil
}

func (c ControllerClient) SubmissionStatus(submissionID string) (model.SubmissionStatus, error) {
	controllerURL, err := c.controllerURL()
	if err != nil {
		return model.SubmissionStatus{}, err
	}

	return c.submissionStatus(controllerURL, submissionID)
}

func (c ControllerClient) SubmissionLogs(submissionID string, filters SubmissionLogsFilters) (SubmissionLogsResponse, error) {
	controllerURL, err := c.controllerURL()
	if err != nil {
		return SubmissionLogsResponse{}, err
	}

	if filters.TailSet && filters.Tail <= 0 {
		return SubmissionLogsResponse{}, fmt.Errorf("tail must be positive")
	}

	return c.submissionLogs(controllerURL, submissionID, filters)
}

func (c ControllerClient) submissionLogs(controllerURL, submissionID string, filters SubmissionLogsFilters) (SubmissionLogsResponse, error) {
	if strings.TrimSpace(submissionID) == "" {
		return SubmissionLogsResponse{}, fmt.Errorf("submission_id is required")
	}

	query := url.Values{}
	if filters.TailSet {
		query.Set("tail", strconv.Itoa(filters.Tail))
	}
	if filters.Level != "" {
		query.Set("level", filters.Level)
	}
	if filters.Stream != "" {
		query.Set("stream", filters.Stream)
	}
	if filters.AttemptID != "" {
		query.Set("attempt-id", filters.AttemptID)
	}

	httpClient, err := c.protectedControllerHTTPClient(controllerURL)
	if err != nil {
		return SubmissionLogsResponse{}, err
	}
	path, err := controllerhttp.PathJoin("/submissions", submissionID, "logs")
	if err != nil {
		return SubmissionLogsResponse{}, fmt.Errorf("create submission logs request: %w", err)
	}
	request, err := httpClient.NewRequestWithQuery(context.Background(), http.MethodGet, path, query, nil)
	if err != nil {
		return SubmissionLogsResponse{}, wrapControllerRequestError("get submission logs", err)
	}
	response, err := httpClient.Do(request, http.StatusOK)
	if err != nil {
		if statusBody, ok := controllerStatusError(err, http.StatusNotFound); ok {
			if statusBody != "" {
				return SubmissionLogsResponse{}, fmt.Errorf("submission %q not found: %s", submissionID, statusBody)
			}
			return SubmissionLogsResponse{}, fmt.Errorf("submission %q not found", submissionID)
		}
		return SubmissionLogsResponse{}, wrapControllerRequestError("get submission logs", err)
	}
	defer response.Body.Close()

	var responseBody SubmissionLogsResponse
	if err := json.NewDecoder(response.Body).Decode(&responseBody); err != nil {
		return SubmissionLogsResponse{}, fmt.Errorf("decode submission logs: %w", err)
	}

	return responseBody, nil
}

func (c ControllerClient) WaitForSubmission(submissionID string) (model.SubmissionStatus, error) {
	controllerURL, err := c.controllerURL()
	if err != nil {
		return model.SubmissionStatus{}, err
	}

	interval, err := c.statusPollInterval()
	if err != nil {
		return model.SubmissionStatus{}, err
	}
	if interval <= 0 {
		interval = time.Second
	}

	return c.waitForSubmission(controllerURL, submissionID, interval)
}

func (c ControllerClient) waitForSubmission(controllerURL string, submissionID string, interval time.Duration) (model.SubmissionStatus, error) {
	if strings.TrimSpace(submissionID) == "" {
		return model.SubmissionStatus{}, fmt.Errorf("submission_id is required")
	}

	for {
		status, err := c.submissionStatus(controllerURL, submissionID)
		if err != nil {
			return model.SubmissionStatus{}, err
		}

		switch status.Status {
		case "queued", "running":
			time.Sleep(interval)
		case "completed":
			return status, nil
		case "failed":
			return status, fmt.Errorf("submission %q failed", submissionID)
		default:
			return status, fmt.Errorf("submission %q has unrecognized status %q", submissionID, status.Status)
		}
	}
}

func (c ControllerClient) submissionStatus(controllerURL string, submissionID string) (model.SubmissionStatus, error) {
	if strings.TrimSpace(submissionID) == "" {
		return model.SubmissionStatus{}, fmt.Errorf("submission_id is required")
	}

	httpClient, err := c.protectedControllerHTTPClient(controllerURL)
	if err != nil {
		return model.SubmissionStatus{}, err
	}
	path, err := controllerhttp.PathJoin("/submissions", submissionID, "status")
	if err != nil {
		return model.SubmissionStatus{}, fmt.Errorf("create submission status request: %w", err)
	}
	request, err := httpClient.NewRequest(context.Background(), http.MethodGet, path, nil)
	if err != nil {
		return model.SubmissionStatus{}, wrapControllerRequestError("get submission status", err)
	}
	response, err := httpClient.Do(request, http.StatusOK)
	if err != nil {
		if statusBody, ok := controllerStatusError(err, http.StatusNotFound); ok {
			if statusBody != "" {
				return model.SubmissionStatus{}, fmt.Errorf("submission %q not found: %s", submissionID, statusBody)
			}
			return model.SubmissionStatus{}, fmt.Errorf("submission %q not found", submissionID)
		}
		return model.SubmissionStatus{}, wrapControllerRequestError("get submission status", err)
	}
	defer response.Body.Close()

	var status model.SubmissionStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return model.SubmissionStatus{}, fmt.Errorf("decode submission status: %w", err)
	}

	return status, nil
}

func (c ControllerClient) Shutdown(controllerURL string) error {
	httpClient, err := c.protectedControllerHTTPClient(controllerURL)
	if err != nil {
		return err
	}
	request, err := httpClient.NewRequest(context.Background(), http.MethodPost, "/shutdown", nil)
	if err != nil {
		return wrapControllerRequestError("shutdown controller", err)
	}
	response, err := httpClient.Do(request, http.StatusNoContent)
	if err != nil {
		return wrapControllerRequestError("shutdown controller", err)
	}
	return response.Body.Close()
}

func (c ControllerClient) controllerURL() (string, error) {
	reference, err := variable.ParseReference("controller_url")
	if err != nil {
		return "", err
	}

	value, err := c.resolver.Resolve(reference)
	if err != nil {
		return "", err
	}

	if value.Type != variable.TypeString {
		return "", fmt.Errorf("controller_url has type %s, want string", value.Type)
	}

	controllerURL, ok := value.Value.(string)
	if !ok || strings.TrimSpace(controllerURL) == "" {
		return "", fmt.Errorf("controller_url is required")
	}

	return controllerURL, nil
}

func (c ControllerClient) statusPollInterval() (time.Duration, error) {
	reference, err := variable.ParseReference("client_status_poll_interval")
	if err != nil {
		return 0, err
	}

	value, err := c.resolver.Resolve(reference)
	if err != nil {
		return 0, nil
	}

	if value.Type != variable.TypeString {
		return 0, fmt.Errorf("client_status_poll_interval has type %s, want string", value.Type)
	}

	interval, ok := value.Value.(string)
	if !ok || strings.TrimSpace(interval) == "" {
		return 0, fmt.Errorf("client_status_poll_interval is required")
	}

	duration, err := time.ParseDuration(interval)
	if err != nil {
		return 0, fmt.Errorf("parse client_status_poll_interval: %w", err)
	}

	return duration, nil
}

func (c ControllerClient) controllerHTTPClient(controllerURL string) (controllerhttp.Client, error) {
	client, err := controllerhttp.New(controllerhttp.Config{
		BaseURL: controllerURL,
		HTTP:    c.httpClient,
		Token:   c.tokenProvider,
		Caller:  "goetl-cli/1",
	})
	if err != nil {
		return controllerhttp.Client{}, err
	}
	return client, nil
}

func (c ControllerClient) protectedControllerHTTPClient(controllerURL string) (controllerhttp.Client, error) {
	if c.tokenProvider == nil {
		return controllerhttp.Client{}, missingControllerTokenError()
	}
	return c.controllerHTTPClient(controllerURL)
}

func missingControllerTokenError() error {
	return fmt.Errorf("missing controller token: configure --controller-token-file, %s, or %s", ControllerTokenFileEnv, ControllerTokenEnv)
}

func wrapControllerRequestError(operation string, err error) error {
	var statusErr controllerhttp.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%s: missing or invalid controller token", operation)
		case http.StatusForbidden:
			return fmt.Errorf("%s: controller token has insufficient role", operation)
		}
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "certificate") || strings.Contains(text, "tls"):
		return fmt.Errorf("%s: TLS trust failure: %w", operation, err)
	case strings.Contains(text, "connection refused") || strings.Contains(text, "no such host") || strings.Contains(text, "connect:"):
		return fmt.Errorf("%s: unreachable controller endpoint: %w", operation, err)
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}

func controllerStatusError(err error, statusCode int) (string, bool) {
	var statusErr controllerhttp.StatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != statusCode {
		return "", false
	}
	return statusErr.Body, true
}

func localAutoStartControllerURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

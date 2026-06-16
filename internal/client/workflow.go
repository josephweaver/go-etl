package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"goetl/internal/model"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

type WorkflowSubmission struct {
	Workflow  workflow.Workflow   `json:"workflow"`
	Variables []variable.Variable `json:"variables"`
}

type WorkflowClient struct {
	httpClient *http.Client
	resolver   variable.Resolver
	starter    ControllerStarter
}

type ControllerStarter interface {
	StartController() error
}

func NewWorkflowClient(httpClient *http.Client, resolver variable.Resolver) WorkflowClient {
	return NewWorkflowClientWithStarter(httpClient, resolver, nil)
}

func NewWorkflowClientWithStarter(httpClient *http.Client, resolver variable.Resolver, starter ControllerStarter) WorkflowClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return WorkflowClient{
		httpClient: httpClient,
		resolver:   resolver,
		starter:    starter,
	}
}

func (c WorkflowClient) SubmitWorkflow(submission WorkflowSubmission) error {
	controllerURL, err := c.controllerURL()
	if err != nil {
		return err
	}

	if err := c.EnsureController(controllerURL); err != nil {
		return err
	}

	body, err := json.Marshal(submission)
	if err != nil {
		return fmt.Errorf("encode workflow submission: %w", err)
	}

	url := strings.TrimRight(controllerURL, "/") + "/workflow"
	response, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("submit workflow: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("submit workflow: unexpected status %d", response.StatusCode)
	}

	return nil
}

func (c WorkflowClient) SubmitWorkflowFile(path string) error {
	submission, err := LoadWorkflowSubmissionFile(path)
	if err != nil {
		return err
	}

	return c.SubmitWorkflow(submission)
}

func LoadWorkflowSubmissionFile(path string) (WorkflowSubmission, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowSubmission{}, fmt.Errorf("read workflow submission file: %w", err)
	}

	var submission WorkflowSubmission
	if err := json.Unmarshal(data, &submission); err != nil {
		return WorkflowSubmission{}, fmt.Errorf("decode workflow submission file: %w", err)
	}

	return submission, nil
}

func (c WorkflowClient) EnsureController(controllerURL string) error {
	if err := c.CheckController(controllerURL); err == nil {
		return nil
	}

	if c.starter == nil {
		return c.CheckController(controllerURL)
	}

	if err := c.starter.StartController(); err != nil {
		return fmt.Errorf("start controller: %w", err)
	}

	return c.WaitForController(controllerURL, 10)
}

func (c WorkflowClient) CheckController(controllerURL string) error {
	url := strings.TrimRight(controllerURL, "/") + "/status"
	response, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("check controller: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("check controller: unexpected status %d", response.StatusCode)
	}

	return nil
}

func (c WorkflowClient) WaitForController(controllerURL string, maxChecks int) error {
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

func (c WorkflowClient) ShutdownWhenIdle(maxChecks int) (model.ControllerStatus, error) {
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

func (c WorkflowClient) Status(controllerURL string) (model.ControllerStatus, error) {
	url := strings.TrimRight(controllerURL, "/") + "/status"
	response, err := c.httpClient.Get(url)
	if err != nil {
		return model.ControllerStatus{}, fmt.Errorf("get controller status: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return model.ControllerStatus{}, fmt.Errorf("get controller status: unexpected status %d", response.StatusCode)
	}

	var status model.ControllerStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return model.ControllerStatus{}, fmt.Errorf("decode controller status: %w", err)
	}

	return status, nil
}

func (c WorkflowClient) Shutdown(controllerURL string) error {
	url := strings.TrimRight(controllerURL, "/") + "/shutdown"
	request, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create shutdown request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("shutdown controller: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("shutdown controller: unexpected status %d", response.StatusCode)
	}

	return nil
}

func (c WorkflowClient) controllerURL() (string, error) {
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

func (c WorkflowClient) statusPollInterval() (time.Duration, error) {
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

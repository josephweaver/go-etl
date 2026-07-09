package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"goetl/internal/client"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func main() {
	if err := run(os.Args, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, httpClient *http.Client) error {
	command, err := parseCommand(args)
	if err != nil {
		return err
	}

	return executeCommand(command, httpClient)
}

func executeCommand(command cliCommand, httpClient *http.Client) error {
	if command.Kind == commandSubmit {
		return submitCommand(command, httpClient)
	}
	if command.Kind == commandStatus {
		return statusCommand(command, httpClient)
	}
	if command.Kind == commandLogs {
		return logsCommand(command, httpClient)
	}
	if command.Kind != commandDemo {
		return nil
	}

	resolver, err := demoResolver()
	if err != nil {
		return fmt.Errorf("invalid demo variables: %w", err)
	}

	starter := client.NewLocalControllerStarter(resolver)
	controllerClient := client.NewControllerClientWithStarter(httpClient, resolver, starter)

	if err := controllerClient.SubmitWorkflowRunFile(command.WorkflowRunPath); err != nil {
		return fmt.Errorf("submit workflow: %w", err)
	}

	status, err := controllerClient.ShutdownWhenIdle(60)
	if err != nil {
		return fmt.Errorf("wait for shutdown: %w", err)
	}

	fmt.Println(formatFinalStatus(status))
	return nil
}

func submitCommand(command cliCommand, httpClient *http.Client) error {
	controllerURL, resolver, starter, err := submitControllerRuntime(command)
	if err != nil {
		return fmt.Errorf("goet submit: %w", err)
	}

	controllerClient := client.NewControllerClientWithStarter(httpClient, resolver, starter)
	payload, err := submitPayload(command)
	if err != nil {
		return fmt.Errorf("goet submit: %w", err)
	}
	acknowledgement, err := submitPayloadAcknowledgement(httpClient, controllerClient, controllerURL, payload)
	if err != nil {
		return fmt.Errorf("goet submit: %w", err)
	}

	fmt.Println(formatSubmissionAcknowledgement(acknowledgement))
	if !command.Wait {
		return nil
	}

	status, waitErr := controllerClient.WaitForSubmission(acknowledgement.SubmissionID)
	if status.SubmissionID != "" {
		fmt.Println(formatSubmissionStatus(status))
	}
	if waitErr != nil {
		return fmt.Errorf("goet submit: %w", waitErr)
	}
	return nil
}

func statusCommand(command cliCommand, httpClient *http.Client) error {
	resolver, err := statusResolver(command.ControllerURL)
	if err != nil {
		return fmt.Errorf("goet status: %w", err)
	}

	controllerClient := client.NewControllerClient(httpClient, resolver)
	status, err := controllerClient.SubmissionStatus(command.SubmissionID)
	if err != nil {
		return fmt.Errorf("goet status: %w", err)
	}

	if command.JSON {
		payload, err := jsonMarshalSubmissionStatus(status)
		if err != nil {
			return fmt.Errorf("goet status: encode status: %w", err)
		}
		fmt.Println(payload)
		return nil
	}

	fmt.Println(formatSubmissionStatus(status))
	return nil
}

func logsCommand(command cliCommand, httpClient *http.Client) error {
	resolver, err := statusResolver(command.ControllerURL)
	if err != nil {
		return fmt.Errorf("goet logs: %w", err)
	}

	controllerClient := client.NewControllerClient(httpClient, resolver)
	logs, err := controllerClient.SubmissionLogs(command.SubmissionID, client.SubmissionLogsFilters{
		Tail:      command.Tail,
		TailSet:   command.TailSet,
		Level:     command.Level,
		Stream:    command.Stream,
		AttemptID: command.AttemptID,
	})
	if err != nil {
		return fmt.Errorf("goet logs: %w", err)
	}

	if command.JSON {
		payload, err := jsonMarshalLogs(logs)
		if err != nil {
			return fmt.Errorf("goet logs: encode logs: %w", err)
		}
		fmt.Println(payload)
		return nil
	}

	for _, observation := range logs.Entries {
		fmt.Println(formatSubmissionLog(observation))
	}
	return nil
}

type commandKind string

const (
	commandDemo   commandKind = "demo"
	commandSubmit commandKind = "submit"
	commandStatus commandKind = "status"
	commandLogs   commandKind = "logs"
)

const defaultControllerURL = "http://localhost:8080"

type cliCommand struct {
	Kind            commandKind
	WorkflowRunPath string
	ControllerPath  string
	ControllerURL   string
	Repository      string
	Ref             string
	ProjectPath     string
	WorkflowPath    string
	SubmissionID    string
	Tail            int
	TailSet         bool
	Level           string
	Stream          string
	AttemptID       string
	Wait            bool
	JSON            bool
}

func parseCommand(args []string) (cliCommand, error) {
	if len(args) <= 1 {
		return cliCommand{
			Kind:            commandDemo,
			WorkflowRunPath: demoWorkflowRunPath(args),
		}, nil
	}

	switch args[1] {
	case "submit":
		return parseSubmitCommand(args[2:])
	case "status":
		return parseStatusCommand(args[2:])
	case "logs":
		return parseLogsCommand(args[2:])
	default:
		return cliCommand{}, fmt.Errorf("unknown goet command %q; expected submit, status, or logs", args[1])
	}
}

func parseSubmitCommand(args []string) (cliCommand, error) {
	flags := flag.NewFlagSet("goet submit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	command := cliCommand{Kind: commandSubmit}
	flags.StringVar(&command.ControllerPath, "controller", "", "controller configuration path")
	flags.StringVar(&command.ControllerURL, "controller-url", "", "controller URL")
	flags.StringVar(&command.Repository, "repo", "", "source repository identity")
	flags.StringVar(&command.Repository, "repository", "", "source repository identity")
	flags.StringVar(&command.Ref, "ref", "", "source repository ref")
	flags.StringVar(&command.ProjectPath, "project", "", "project configuration path")
	flags.StringVar(&command.WorkflowPath, "workflow", "", "workflow configuration path")
	flags.BoolVar(&command.Wait, "wait", false, "wait for completion")
	flags.BoolVar(&command.JSON, "json", false, "write JSON output")

	if err := flags.Parse(args); err != nil {
		return cliCommand{}, fmt.Errorf("goet submit: %w", err)
	}
	if flags.NArg() > 0 {
		return cliCommand{}, fmt.Errorf("goet submit: unexpected positional argument %q", flags.Arg(0))
	}
	if err := validateSubmitCommand(command); err != nil {
		return cliCommand{}, err
	}
	if command.Repository != "" && command.Ref == "" {
		command.Ref = "main"
	}

	return command, nil
}

func validateSubmitCommand(command cliCommand) error {
	if command.ControllerPath == "" && command.ControllerURL == "" {
		return errors.New("goet submit: exactly one of --controller or --controller-url is required")
	}
	if command.ControllerPath != "" && command.ControllerURL != "" {
		return errors.New("goet submit: --controller and --controller-url cannot both be supplied")
	}
	if command.ProjectPath == "" {
		return errors.New("goet submit: --project is required")
	}
	if command.WorkflowPath == "" {
		return errors.New("goet submit: --workflow is required")
	}
	if command.Ref != "" && command.Repository == "" {
		return errors.New("goet submit: --ref requires --repo")
	}

	return nil
}

type inlineSubmitPayload struct {
	Project  json.RawMessage `json:"project"`
	Workflow json.RawMessage `json:"workflow"`
}

func submitPayload(command cliCommand) (any, error) {
	if command.Repository != "" {
		return client.WorkflowRunSubmission{
			Project: client.SourceDocumentReference{
				Repository: command.Repository,
				Ref:        command.Ref,
				Path:       filepath.ToSlash(command.ProjectPath),
			},
			Workflow: client.SourceDocumentReference{
				Repository: command.Repository,
				Ref:        command.Ref,
				Path:       filepath.ToSlash(command.WorkflowPath),
			},
		}, nil
	}

	project, err := readJSONFile(command.ProjectPath, "project")
	if err != nil {
		return nil, err
	}
	workflow, err := readJSONFile(command.WorkflowPath, "workflow")
	if err != nil {
		return nil, err
	}
	return inlineSubmitPayload{
		Project:  project,
		Workflow: workflow,
	}, nil
}

func readJSONFile(path string, name string) (json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s file %q: %w", name, path, err)
	}
	if err := validateSingleJSONDocument(data); err != nil {
		return nil, fmt.Errorf("decode %s file %q: %w", name, path, err)
	}
	return json.RawMessage(data), nil
}

func validateSingleJSONDocument(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var document any
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("multiple JSON values")
	}
	return nil
}

func submitControllerRuntime(command cliCommand) (string, variable.Resolver, client.ControllerStarter, error) {
	var controllerVariables []variable.Variable
	var starter client.ControllerStarter
	controllerURL := command.ControllerURL

	if command.ControllerPath != "" {
		if _, err := readJSONFile(command.ControllerPath, "controller"); err != nil {
			return "", variable.Resolver{}, nil, err
		}
		controllerURL = defaultControllerURL
		controllerVariables = localControllerVariables(command.ControllerPath, controllerURL)
	} else {
		controllerVariables = remoteControllerVariables(controllerURL)
	}

	controllerScope, err := variable.NewScope(controllerVariables...)
	if err != nil {
		return "", variable.Resolver{}, nil, fmt.Errorf("build CLI controller variables: %w", err)
	}
	resolver := variable.NewResolver(variable.NewSet(controllerScope), variable.ResolverConfig{})
	if command.ControllerPath != "" {
		starter = client.NewLocalControllerStarter(resolver)
	}
	return controllerURL, resolver, starter, nil
}

func submitPayloadAcknowledgement(httpClient *http.Client, controllerClient client.ControllerClient, controllerURL string, payload any) (model.SubmissionAcknowledgement, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if err := controllerClient.EnsureController(controllerURL); err != nil {
		return model.SubmissionAcknowledgement{}, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("encode workflow submission: %w", err)
	}
	url := strings.TrimRight(controllerURL, "/") + "/workflow"
	response, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("submit workflow: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("submit workflow: unexpected status %d", response.StatusCode)
	}

	var acknowledgement model.SubmissionAcknowledgement
	if err := json.NewDecoder(response.Body).Decode(&acknowledgement); err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("decode submission acknowledgement: %w", err)
	}
	return acknowledgement, nil
}

func remoteControllerVariables(controllerURL string) []variable.Variable {
	return []variable.Variable{
		{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_url"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: controllerURL}},
		{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "client_status_poll_interval"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "1s"}},
	}
}

func localControllerVariables(controllerPath, controllerURL string) []variable.Variable {
	variables := remoteControllerVariables(controllerURL)
	variables = append(variables,
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_executable"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "go"}},
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_args"}, TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: []variable.TypedExpression{
			{Type: variable.TypeString, Expression: "run"},
			{Type: variable.TypeString, Expression: "./cmd/controller"},
			{Type: variable.TypeString, Expression: "--config"},
			{Type: variable.TypeString, Expression: controllerPath},
		}}},
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_lock_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "controller-start.lock"}},
	)
	return variables
}

func parseStatusCommand(args []string) (cliCommand, error) {
	flags := flag.NewFlagSet("goet status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	command := cliCommand{
		Kind:          commandStatus,
		ControllerURL: defaultControllerURL,
	}
	flags.StringVar(&command.ControllerURL, "controller-url", defaultControllerURL, "controller URL")
	flags.BoolVar(&command.JSON, "json", false, "write JSON output")

	flagArgs, positionals := splitFlagArgs(args)
	if err := flags.Parse(flagArgs); err != nil {
		return cliCommand{}, fmt.Errorf("goet status: %w", err)
	}
	if len(positionals) == 0 {
		return cliCommand{}, errors.New("goet status: submission_id is required")
	}
	if len(positionals) > 1 {
		return cliCommand{}, fmt.Errorf("goet status: unexpected positional argument %q", positionals[1])
	}

	command.SubmissionID = positionals[0]
	return command, nil
}

func parseLogsCommand(args []string) (cliCommand, error) {
	flags := flag.NewFlagSet("goet logs", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	command := cliCommand{
		Kind:          commandLogs,
		ControllerURL: defaultControllerURL,
	}
	var tailValue string
	flags.StringVar(&command.ControllerURL, "controller-url", defaultControllerURL, "controller URL")
	flags.StringVar(&tailValue, "tail", "", "limit results to this many lines")
	flags.StringVar(&command.Level, "level", "", "minimum level filter")
	flags.StringVar(&command.Stream, "stream", "", "stream filter")
	flags.StringVar(&command.AttemptID, "attempt-id", "", "attempt ID filter")
	flags.BoolVar(&command.JSON, "json", false, "write JSON output")

	flagArgs, positionals := splitFlagArgs(args)
	if err := flags.Parse(flagArgs); err != nil {
		return cliCommand{}, fmt.Errorf("goet logs: %w", err)
	}
	if len(positionals) == 0 {
		return cliCommand{}, errors.New("goet logs: submission_id is required")
	}
	if len(positionals) > 1 {
		return cliCommand{}, fmt.Errorf("goet logs: unexpected positional argument %q", positionals[1])
	}

	command.SubmissionID = positionals[0]
	if tailValue != "" {
		tail, err := strconv.Atoi(tailValue)
		if err != nil || tail <= 0 {
			return cliCommand{}, errors.New("goet logs: tail must be a positive integer")
		}
		command.Tail = tail
		command.TailSet = true
	}

	return command, nil
}

func splitFlagArgs(args []string) ([]string, []string) {
	var flagArgs []string
	var positionals []string

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--controller-url" || strings.HasPrefix(arg, "--controller-url=") {
			flagArgs = append(flagArgs, arg)
			if arg == "--controller-url" && index+1 < len(args) {
				index++
				flagArgs = append(flagArgs, args[index])
			}
			continue
		}
		if arg == "--tail" || strings.HasPrefix(arg, "--tail=") {
			flagArgs = append(flagArgs, arg)
			if arg == "--tail" && index+1 < len(args) {
				index++
				flagArgs = append(flagArgs, args[index])
			}
			continue
		}
		if arg == "--level" || strings.HasPrefix(arg, "--level=") ||
			arg == "--stream" || strings.HasPrefix(arg, "--stream=") ||
			arg == "--attempt-id" || strings.HasPrefix(arg, "--attempt-id=") {
			flagArgs = append(flagArgs, arg)
			if (arg == "--level" || arg == "--stream" || arg == "--attempt-id") && index+1 < len(args) {
				index++
				flagArgs = append(flagArgs, args[index])
			}
			continue
		}
		if arg == "--json" || strings.HasPrefix(arg, "--json=") ||
			arg == "--watch" || strings.HasPrefix(arg, "--watch=") ||
			arg == "--follow" || strings.HasPrefix(arg, "--follow=") {
			flagArgs = append(flagArgs, arg)
			continue
		}

		positionals = append(positionals, arg)
	}

	return flagArgs, positionals
}

func formatFinalStatus(status model.ControllerStatus) string {
	lines := []string{fmt.Sprintf("final status: pending=%d assigned=%d failed=%d pending_reuse_candidates=%d attempts=%d attempt_variables=%d",
		status.Pending,
		status.Assigned,
		status.Failed,
		status.PendingReuseCandidates,
		status.Attempts,
		status.AttemptVariables,
	)}

	if status.QueuedResourceEligibleCount == 0 && status.QueuedResourceBlockedCount == 0 && status.RunningResourceClaimCount == 0 && len(status.ResourceConstraintSummaries) == 0 {
		return lines[0]
	}

	lines = append(lines, fmt.Sprintf("resources: %d eligible, %d blocked", status.QueuedResourceEligibleCount, status.QueuedResourceBlockedCount))
	for _, summary := range status.ResourceConstraintSummaries {
		lines = append(lines, fmt.Sprintf("  %s running=%d blocked=%d", summary.ResourceKey, summary.TotalUnits, summary.BlockedCandidateCount))
	}

	return strings.Join(lines, "\n")
}

func formatSubmissionAcknowledgement(acknowledgement model.SubmissionAcknowledgement) string {
	return fmt.Sprintf("Submission: %s\nWorkflow: %s\nInitial work items: %d",
		acknowledgement.SubmissionID,
		acknowledgement.WorkflowID,
		acknowledgement.InitialWorkItemCount,
	)
}

func formatSubmissionStatus(status model.SubmissionStatus) string {
	lines := []string{fmt.Sprintf(
		"Submission: %s\nWorkflow: %s\nStatus: %s\nKnown work items: %d\nQueued: %d\nRunning: %d\nCompleted: %d\nFailed: %d\nSkipped: %d",
		status.SubmissionID,
		status.WorkflowID,
		status.Status,
		status.KnownWorkItems,
		status.Queued,
		status.Running,
		status.Completed,
		status.Failed,
		status.Skipped,
	)}
	if status.Dependency != nil {
		lines = append(lines, formatDependencyStatus(*status.Dependency))
	}
	return strings.Join(lines, "\n")
}

func formatDependencyStatus(status model.SubmissionDependencyStatus) string {
	lines := []string{
		fmt.Sprintf("Dependency workflow: %s", status.WorkflowState),
		fmt.Sprintf("Dependency stages: %d", status.StageCount),
	}
	if status.CurrentStageIndex != nil {
		lines = append(lines, fmt.Sprintf("Current stage: %d", *status.CurrentStageIndex))
	}
	if status.Failed != nil {
		step := "unknown"
		if status.Failed.StepIndex != nil {
			step = strconv.Itoa(*status.Failed.StepIndex)
		}
		lines = append(lines, fmt.Sprintf("Dependency failure: stage=%d step=%s work_item=%s reason=%s",
			status.Failed.StageIndex,
			step,
			status.Failed.WorkItemID,
			status.Failed.FailureReason,
		))
	}
	for _, stage := range status.Stages {
		lines = append(lines, fmt.Sprintf(
			"Stage %d: %s steps=%d assignable_pending=%d blocked_future=%d active=%d completed=%d failed=%d skipped=%d",
			stage.StageIndex,
			stage.State,
			stage.StepCount,
			stage.Counts.AssignablePending,
			stage.Counts.BlockedFuture,
			stage.Counts.Active,
			stage.Counts.Completed,
			stage.Counts.Failed,
			stage.Counts.Skipped,
		))
	}
	return strings.Join(lines, "\n")
}

func formatSubmissionLog(observation model.LogObservation) string {
	parts := []string{
		observation.Timestamp,
		string(observation.Level),
		observation.Component,
	}

	if observation.Stream != "" {
		parts = append(parts, observation.Stream)
	}
	if observation.AttemptID != "" {
		parts = append(parts, "attempt="+observation.AttemptID)
	}

	parts = append(parts, observation.Message)
	return strings.Join(parts, " ")
}

func jsonMarshalLogs(logs client.SubmissionLogsResponse) (string, error) {
	payload, err := json.Marshal(logs)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func jsonMarshalSubmissionStatus(status model.SubmissionStatus) (string, error) {
	payload, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func demoWorkflowRunPath(args []string) string {
	if len(args) > 1 {
		return args[1]
	}

	return filepath.Join("..", "go-etl-demo-project", "submissions", "demo-workflow-run.json")
}

func demoResolver() (variable.Resolver, error) {
	scope, err := variable.NewScope(
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_url"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "http://localhost:8080"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_executable"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "go"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_args"}, TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: []variable.TypedExpression{
			{Type: variable.TypeString, Expression: "run"},
			{Type: variable.TypeString, Expression: "./cmd/controller"},
			{Type: variable.TypeString, Expression: "--config"},
			{Type: variable.TypeString, Expression: "./cmd/controller/demo-config.json"},
		}}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_lock_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "controller-start.lock"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "client_status_poll_interval"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "1s"}},
	)
	if err != nil {
		return variable.Resolver{}, err
	}

	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{}), nil
}

func statusResolver(controllerURL string) (variable.Resolver, error) {
	scope, err := variable.NewScope(
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_url"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: controllerURL}},
	)
	if err != nil {
		return variable.Resolver{}, err
	}

	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{}), nil
}

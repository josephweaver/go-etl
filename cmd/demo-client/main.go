package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"goetl/internal/client"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func main() {
	command, err := parseCommand(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	if err := executeCommand(command, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}

func executeCommand(command cliCommand, httpClient *http.Client) error {
	if command.Kind == commandSubmit {
		return submitCommand(command, httpClient)
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
	inputs, err := client.LoadCLIInputs(client.CLIInputPaths{
		ControllerPath: command.ControllerPath,
		ControllerURL:  command.ControllerURL,
		ProjectPath:    command.ProjectPath,
		WorkflowPath:   command.WorkflowPath,
	})
	if err != nil {
		return fmt.Errorf("goet submit: %w", err)
	}

	controllerClient := client.NewControllerClientWithStarter(httpClient, inputs.Resolver, inputs.Starter)
	if err := controllerClient.SubmitWorkflow(inputs.Submission); err != nil {
		return fmt.Errorf("goet submit: %w", err)
	}

	fmt.Println("workflow submitted")
	return nil
}

type commandKind string

const (
	commandDemo   commandKind = "demo"
	commandSubmit commandKind = "submit"
	commandStatus commandKind = "status"
)

const defaultControllerURL = "http://localhost:8080"

type cliCommand struct {
	Kind            commandKind
	WorkflowRunPath string
	ControllerPath  string
	ControllerURL   string
	ProjectPath     string
	WorkflowPath    string
	SubmissionID    string
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
	default:
		return cliCommand{}, fmt.Errorf("unknown goet command %q; expected submit or status", args[1])
	}
}

func parseSubmitCommand(args []string) (cliCommand, error) {
	flags := flag.NewFlagSet("goet submit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	command := cliCommand{Kind: commandSubmit}
	flags.StringVar(&command.ControllerPath, "controller", "", "controller configuration path")
	flags.StringVar(&command.ControllerURL, "controller-url", "", "controller URL")
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

	return nil
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
		if arg == "--json" || strings.HasPrefix(arg, "--json=") || arg == "--watch" || strings.HasPrefix(arg, "--watch=") {
			flagArgs = append(flagArgs, arg)
			continue
		}

		positionals = append(positionals, arg)
	}

	return flagArgs, positionals
}

func formatFinalStatus(status model.ControllerStatus) string {
	return fmt.Sprintf("final status: pending=%d assigned=%d failed=%d pending_reuse_candidates=%d attempts=%d attempt_variables=%d",
		status.Pending,
		status.Assigned,
		status.Failed,
		status.PendingReuseCandidates,
		status.Attempts,
		status.AttemptVariables,
	)
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

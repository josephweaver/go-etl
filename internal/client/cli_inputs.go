package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"goetl/internal/variable"
)

const defaultCLIControllerURL = "http://localhost:8080"

type CLIInputPaths struct {
	ControllerPath string
	ControllerURL  string
	ProjectPath    string
	WorkflowPath   string
}

type CLIInputs struct {
	ControllerPath string
	ControllerURL  string
	Resolver       variable.Resolver
	Starter        ControllerStarter
	Submission     WorkflowSubmission
}

func LoadCLIInputs(paths CLIInputPaths) (CLIInputs, error) {
	var controllerVariables []variable.Variable
	var starter ControllerStarter
	controllerURL := paths.ControllerURL

	if paths.ControllerPath != "" {
		if err := validateControllerJSON(paths.ControllerPath); err != nil {
			return CLIInputs{}, err
		}
		controllerURL = defaultCLIControllerURL
		controllerVariables = localControllerVariables(paths.ControllerPath, controllerURL)
	} else {
		controllerVariables = remoteControllerVariables(controllerURL)
	}

	projectVariables, err := loadProjectVariables(paths.ProjectPath)
	if err != nil {
		return CLIInputs{}, err
	}

	submission, err := loadCLIWorkflowSubmission(paths.WorkflowPath)
	if err != nil {
		return CLIInputs{}, err
	}
	submission.Variables = append(projectVariables, submission.Variables...)

	projectScope, err := variable.NewScope(projectVariables...)
	if err != nil {
		return CLIInputs{}, fmt.Errorf("build CLI input variables: %w", err)
	}
	controllerScope, err := variable.NewScope(controllerVariables...)
	if err != nil {
		return CLIInputs{}, fmt.Errorf("build CLI controller variables: %w", err)
	}
	resolver := variable.NewResolver(variable.NewSet(projectScope, controllerScope), variable.ResolverConfig{})

	if paths.ControllerPath != "" {
		starter = NewLocalControllerStarter(resolver)
	}

	return CLIInputs{
		ControllerPath: paths.ControllerPath,
		ControllerURL:  controllerURL,
		Resolver:       resolver,
		Starter:        starter,
		Submission:     submission,
	}, nil
}

func validateControllerJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read controller file %q: %w", path, err)
	}

	var document any
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("decode controller file %q: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("decode controller file %q: multiple JSON values", path)
	}

	return nil
}

func loadProjectVariables(path string) ([]variable.Variable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project file %q: %w", path, err)
	}

	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return nil, fmt.Errorf("decode project file %q: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode project file %q: multiple JSON values", path)
	}
	if fields == nil {
		return nil, fmt.Errorf("decode project file %q: project input must be a JSON object", path)
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	variables := make([]variable.Variable, 0, len(fields))
	for _, key := range keys {
		value := fields[key]
		expression, err := typedExpressionFromJSON(value)
		if err != nil {
			return nil, fmt.Errorf("decode project file %q field %q: %w", path, key, err)
		}
		variables = append(variables, variable.Variable{
			Name:            variable.Name{Namespace: variable.NamespaceProjectConfig, Key: key},
			TypedExpression: expression,
		})
	}

	return variables, nil
}

func loadCLIWorkflowSubmission(path string) (WorkflowSubmission, error) {
	submission, err := LoadWorkflowSubmissionFile(path)
	if err != nil {
		return WorkflowSubmission{}, fmt.Errorf("load workflow file %q: %w", path, err)
	}
	return submission, nil
}

func typedExpressionFromJSON(value any) (variable.TypedExpression, error) {
	switch typed := value.(type) {
	case string:
		return variable.TypedExpression{Type: variable.TypeString, Expression: typed}, nil
	case json.Number:
		integer, err := strconv.Atoi(typed.String())
		if err != nil {
			return variable.TypedExpression{}, fmt.Errorf("number %q is not a supported integer", typed.String())
		}
		return variable.TypedExpression{Type: variable.TypeInt, Expression: integer}, nil
	case bool:
		return variable.TypedExpression{Type: variable.TypeBool, Expression: typed}, nil
	case map[string]any:
		fields := make(map[string]variable.TypedExpression, len(typed))
		for key, child := range typed {
			expression, err := typedExpressionFromJSON(child)
			if err != nil {
				return variable.TypedExpression{}, fmt.Errorf("object field %q: %w", key, err)
			}
			fields[key] = expression
		}
		return variable.TypedExpression{Type: variable.TypeObject, Expression: fields}, nil
	case []any:
		items := make([]variable.TypedExpression, 0, len(typed))
		for index, child := range typed {
			expression, err := typedExpressionFromJSON(child)
			if err != nil {
				return variable.TypedExpression{}, fmt.Errorf("list item %d: %w", index, err)
			}
			items = append(items, expression)
		}
		return variable.TypedExpression{Type: variable.TypeList, Expression: items}, nil
	case nil:
		return variable.TypedExpression{}, fmt.Errorf("null is not supported")
	default:
		return variable.TypedExpression{}, fmt.Errorf("unsupported JSON value %T", value)
	}
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

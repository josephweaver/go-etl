package main

import (
	"fmt"

	"goetl/internal/client"
	"goetl/internal/variable"
)

func main() {
	resolver, err := demoResolver()
	if err != nil {
		fmt.Println("invalid demo variables:", err)
		return
	}

	starter := client.NewLocalControllerStarter(resolver)
	workflowClient := client.NewWorkflowClientWithStarter(nil, resolver, starter)

	if err := workflowClient.SubmitWorkflowFile("demo-workflow.json"); err != nil {
		fmt.Println("submit workflow:", err)
		return
	}

	if err := workflowClient.ShutdownWhenIdle(60); err != nil {
		fmt.Println("wait for shutdown:", err)
		return
	}
}

func demoResolver() (variable.Resolver, error) {
	scope, err := variable.NewScope(
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_url"},
			Type:       variable.TypeString,
			Expression: "http://localhost:8080",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_executable"},
			Type:       variable.TypeString,
			Expression: "go",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_args"},
			Type:       variable.TypeList(variable.TypeString),
			Expression: `["run", "./cmd/controller"]`,
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "controller_start_lock_path"},
			Type:       variable.TypeString,
			Expression: "controller-start.lock",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "client_status_poll_interval"},
			Type:       variable.TypeString,
			Expression: "1s",
		},
	)
	if err != nil {
		return variable.Resolver{}, err
	}

	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{}), nil
}

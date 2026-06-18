package main

import (
	"testing"

	"goetl/internal/variable"
)

func TestLocalWorkerStarterResolvesCommand(t *testing.T) {
	starter := LocalWorkerStarter{}
	executable, args, err := starter.command(testControllerResolver(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"},
			Type:       variable.TypeString,
			Expression: "go",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_args"},
			Type:       variable.TypeList(variable.TypeString),
			Expression: `["run", "./cmd/worker"]`,
		},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "go" {
		t.Fatalf("unexpected executable: %s", executable)
	}

	if len(args) != 2 {
		t.Fatalf("unexpected arg count: %d", len(args))
	}

	if args[1] != "./cmd/worker" {
		t.Fatalf("unexpected second arg: %s", args[1])
	}
}

func TestLocalWorkerStarterSupportsCommandBackedTargets(t *testing.T) {
	for _, target := range []string{"local", "hpcc"} {
		if !isCommandBackedWorkerTarget(target) {
			t.Fatalf("expected target %q to be supported", target)
		}
	}
}

func TestLocalWorkerStarterRejectsUnsupportedTarget(t *testing.T) {
	starter := LocalWorkerStarter{}

	err := starter.StartWorker("unknown", variable.NewResolver(variable.NewSet(), variable.ResolverConfig{}))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestLocalWorkerStarterRejectsMissingExecutable(t *testing.T) {
	starter := LocalWorkerStarter{}

	if _, _, err := starter.command(variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})); err == nil {
		t.Fatal("expected an error")
	}
}

func testControllerResolver(t *testing.T, variables ...variable.Variable) variable.Resolver {
	t.Helper()

	scope, err := variable.NewScope(variables...)
	if err != nil {
		t.Fatal(err)
	}

	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
}

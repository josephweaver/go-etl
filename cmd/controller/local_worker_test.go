package main

import (
	"encoding/json"
	"os"
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
			Type:       variable.TypeList,
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

func TestFakeHPCCWorkflowFixtureResolvesWorkerCommand(t *testing.T) {
	data, err := os.ReadFile("../../demo-fake-hpcc-workflow.json")
	if err != nil {
		t.Fatal(err)
	}

	var submission WorkflowSubmission
	if err := json.Unmarshal(data, &submission); err != nil {
		t.Fatal(err)
	}

	scope, err := variable.NewScope(submission.Variables...)
	if err != nil {
		t.Fatal(err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	target, err := workerTargetEnvironment(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if target != "hpcc" {
		t.Fatalf("unexpected target: %s", target)
	}

	starter := LocalWorkerStarter{}
	executable, args, err := starter.command(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if executable != "bash" {
		t.Fatalf("unexpected executable: %s", executable)
	}
	if len(args) != 2 || args[0] != "-lc" || args[1] != "FAKE_SLURM_FOREGROUND=1 scripts/fake-hpcc/sbatch .run/fake-hpcc/worker.slurm" {
		t.Fatalf("unexpected args: %#v", args)
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

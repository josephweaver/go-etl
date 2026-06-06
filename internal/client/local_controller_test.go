package client

import (
	"os"
	"path/filepath"
	"testing"

	"goetl/internal/variable"
)

func TestLocalControllerStarterResolvesCommand(t *testing.T) {
	starter := NewLocalControllerStarter(testResolverWithVariables(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceBackend, Key: "controller_start_executable"},
			Type:       variable.TypeString,
			Expression: "go",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceBackend, Key: "controller_start_args"},
			Type:       variable.TypeList(variable.TypeString),
			Expression: `["run", "./cmd/controller"]`,
		},
	))

	executable, args, err := starter.command()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "go" {
		t.Fatalf("unexpected executable: %s", executable)
	}

	if len(args) != 2 {
		t.Fatalf("unexpected arg count: %d", len(args))
	}

	if args[1] != "./cmd/controller" {
		t.Fatalf("unexpected second arg: %s", args[1])
	}
}

func TestLocalControllerStarterRejectsMissingExecutable(t *testing.T) {
	starter := NewLocalControllerStarter(variable.NewResolver(variable.NewSet(), variable.ResolverConfig{}))

	if _, _, err := starter.command(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLocalControllerStarterAcquireStartLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "controller.lock")
	starter := NewLocalControllerStarter(testResolverWithVariables(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceBackend, Key: "controller_start_lock_path"},
			Type:       variable.TypeString,
			Expression: lockPath,
		},
	))

	unlock, err := starter.acquireStartLock()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected lock file: %v", err)
	}

	unlock()

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file to be removed, got: %v", err)
	}
}

func TestLocalControllerStarterTreatsExistingStartLockAsRace(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "controller.lock")
	if err := os.WriteFile(lockPath, []byte("locked"), 0600); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	starter := NewLocalControllerStarter(testResolverWithVariables(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceBackend, Key: "controller_start_lock_path"},
			Type:       variable.TypeString,
			Expression: lockPath,
		},
	))

	unlock, err := starter.acquireStartLock()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	unlock()

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected existing lock file to remain: %v", err)
	}
}

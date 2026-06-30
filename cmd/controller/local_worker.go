package main

import (
	"fmt"
	"os/exec"

	"goetl/internal/variable"
)

type LocalWorkerStarter struct{}

func (s LocalWorkerStarter) StartWorker(targetEnvironment string, resolver variable.Resolver) error {
	if !isCommandBackedWorkerTarget(targetEnvironment) {
		return fmt.Errorf("unsupported worker target environment: %s", targetEnvironment)
	}

	executable, args, err := s.command(resolver)
	if err != nil {
		return err
	}

	command := exec.Command(executable, args...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start worker command: %w", err)
	}

	return nil
}

func isCommandBackedWorkerTarget(targetEnvironment string) bool {
	return targetEnvironment == "local" || targetEnvironment == "hpcc"
}

func (s LocalWorkerStarter) command(resolver variable.Resolver) (string, []string, error) {
	executable, err := controllerStringVariable(resolver, "worker_start_executable")
	if err != nil {
		return "", nil, err
	}

	args, err := controllerStringListVariable(resolver, "worker_start_args")
	if err != nil {
		return "", nil, err
	}

	return executable, args, nil
}

func controllerStringVariable(resolver variable.Resolver, name string) (string, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return "", err
	}

	if value.Type != variable.TypeString {
		return "", fmt.Errorf("%s has type %s, want string", name, value.Type)
	}

	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return text, nil
}

func controllerStringListVariable(resolver variable.Resolver, name string) ([]string, error) {
	return resolver.StringList(name)
}

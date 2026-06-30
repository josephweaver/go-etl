package client

import (
	"fmt"
	"os"
	"os/exec"

	"goetl/internal/variable"
)

type LocalControllerStarter struct {
	resolver variable.Resolver
}

func NewLocalControllerStarter(resolver variable.Resolver) LocalControllerStarter {
	return LocalControllerStarter{resolver: resolver}
}

func (s LocalControllerStarter) StartController() error {
	unlock, err := s.acquireStartLock()
	if err != nil {
		return err
	}
	defer unlock()

	executable, args, err := s.command()
	if err != nil {
		return err
	}

	command := exec.Command(executable, args...)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start controller command: %w", err)
	}

	return nil
}

func (s LocalControllerStarter) acquireStartLock() (func(), error) {
	path, err := s.optionalStringVariable("controller_start_lock_path")
	if err != nil {
		return nil, err
	}

	if path == "" {
		return func() {}, nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return func() {}, nil
		}
		return nil, fmt.Errorf("create controller start lock: %w", err)
	}

	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close controller start lock: %w", err)
	}

	return func() {
		_ = os.Remove(path)
	}, nil
}

func (s LocalControllerStarter) command() (string, []string, error) {
	executable, err := s.stringVariable("controller_start_executable")
	if err != nil {
		return "", nil, err
	}

	args, err := s.stringListVariable("controller_start_args")
	if err != nil {
		return "", nil, err
	}

	return executable, args, nil
}

func (s LocalControllerStarter) stringVariable(name string) (string, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", err
	}

	value, err := s.resolver.Resolve(reference)
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

func (s LocalControllerStarter) optionalStringVariable(name string) (string, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", err
	}

	value, err := s.resolver.Resolve(reference)
	if err != nil {
		return "", nil
	}

	if value.Type != variable.TypeString {
		return "", fmt.Errorf("%s has type %s, want string", name, value.Type)
	}

	text, ok := value.Value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}

	return text, nil
}

func (s LocalControllerStarter) stringListVariable(name string) ([]string, error) {
	return s.resolver.StringList(name)
}

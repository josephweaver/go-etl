package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type DockerTransport struct {
	Executable string
}

type DockerContainerTransport struct {
	Docker    DockerTransport
	Container string
}

func (t DockerContainerTransport) Prepare(ctx context.Context) error {
	_, err := t.Exec(ctx, "true")
	return err
}

func (t DockerContainerTransport) Copy(ctx context.Context, localPath string, remotePath string) error {
	_, err := t.Docker.CopyToContainer(ctx, localPath, t.Container, remotePath)
	return err
}

func (t DockerContainerTransport) Exec(ctx context.Context, args ...string) ([]byte, error) {
	return t.Docker.Exec(ctx, t.Container, args...)
}

func (d DockerTransport) Exec(ctx context.Context, container string, args ...string) ([]byte, error) {
	executable, commandArgs, err := d.execCommand(container, args...)
	if err != nil {
		return nil, err
	}

	output, err := exec.CommandContext(ctx, executable, commandArgs...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("docker exec: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func (d DockerTransport) CopyToContainer(ctx context.Context, localPath string, container string, remotePath string) ([]byte, error) {
	executable, commandArgs, err := d.copyToContainerCommand(localPath, container, remotePath)
	if err != nil {
		return nil, err
	}

	output, err := exec.CommandContext(ctx, executable, commandArgs...).CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("docker cp: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func (d DockerTransport) execCommand(container string, args ...string) (string, []string, error) {
	executable, err := d.executable()
	if err != nil {
		return "", nil, err
	}
	if err := validateDockerValue("container", container); err != nil {
		return "", nil, err
	}
	for index, arg := range args {
		if err := validateDockerValue(fmt.Sprintf("arg[%d]", index), arg); err != nil {
			return "", nil, err
		}
	}

	commandArgs := append([]string{"exec", container}, args...)
	return executable, commandArgs, nil
}

func (d DockerTransport) copyToContainerCommand(localPath string, container string, remotePath string) (string, []string, error) {
	executable, err := d.executable()
	if err != nil {
		return "", nil, err
	}
	if err := validateDockerValue("local path", localPath); err != nil {
		return "", nil, err
	}
	if err := validateDockerValue("container", container); err != nil {
		return "", nil, err
	}
	if err := validateDockerValue("remote path", remotePath); err != nil {
		return "", nil, err
	}

	return executable, []string{"cp", localPath, container + ":" + remotePath}, nil
}

func (d DockerTransport) executable() (string, error) {
	executable := d.Executable
	if executable == "" {
		executable = "docker"
	}
	if err := validateDockerValue("docker executable", executable); err != nil {
		return "", err
	}
	return executable, nil
}

func validateDockerValue(name string, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if containsNewline(value) {
		return fmt.Errorf("%s must not contain newlines", name)
	}
	return nil
}

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type DockerSlurmSubmitConfig struct {
	DockerExecutable string
	SlurmContainer   string
	ScriptPath       string
}

type DockerSlurmScriptConfig struct {
	DockerExecutable string
	SlurmContainer   string
	ScriptPath       string
	Script           string
}

func WriteAndSubmitDockerSlurmScript(ctx context.Context, cfg DockerSlurmScriptConfig) (string, error) {
	if err := WriteDockerSlurmScript(ctx, cfg); err != nil {
		return "", err
	}
	return SubmitDockerSlurmScript(ctx, DockerSlurmSubmitConfig{
		DockerExecutable: cfg.DockerExecutable,
		SlurmContainer:   cfg.SlurmContainer,
		ScriptPath:       cfg.ScriptPath,
	})
}

func WriteDockerSlurmScript(ctx context.Context, cfg DockerSlurmScriptConfig) error {
	executable, args, err := dockerSlurmWriteScriptCommand(cfg)
	if err != nil {
		return err
	}

	command := exec.CommandContext(ctx, executable, args...)
	command.Stdin = strings.NewReader(cfg.Script)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write docker slurm script: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func SubmitDockerSlurmScript(ctx context.Context, cfg DockerSlurmSubmitConfig) (string, error) {
	executable, args, err := dockerSlurmSbatchCommand(cfg)
	if err != nil {
		return "", err
	}

	output, err := exec.CommandContext(ctx, executable, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("submit docker slurm script: %w: %s", err, strings.TrimSpace(string(output)))
	}

	jobID, err := parseSubmittedSlurmJobID(string(output))
	if err != nil {
		return "", err
	}
	return jobID, nil
}

func dockerSlurmSbatchCommand(cfg DockerSlurmSubmitConfig) (string, []string, error) {
	if cfg.DockerExecutable == "" {
		cfg.DockerExecutable = "docker"
	}
	if cfg.SlurmContainer == "" {
		cfg.SlurmContainer = "slurmctld"
	}
	if cfg.ScriptPath == "" {
		return "", nil, fmt.Errorf("slurm script path is required")
	}
	if containsNewline(cfg.DockerExecutable) || containsNewline(cfg.SlurmContainer) || containsNewline(cfg.ScriptPath) {
		return "", nil, fmt.Errorf("docker slurm submit values must not contain newlines")
	}

	return cfg.DockerExecutable, []string{"exec", cfg.SlurmContainer, "sbatch", cfg.ScriptPath}, nil
}

func dockerSlurmWriteScriptCommand(cfg DockerSlurmScriptConfig) (string, []string, error) {
	if cfg.DockerExecutable == "" {
		cfg.DockerExecutable = "docker"
	}
	if cfg.SlurmContainer == "" {
		cfg.SlurmContainer = "slurmctld"
	}
	if cfg.ScriptPath == "" {
		return "", nil, fmt.Errorf("slurm script path is required")
	}
	if cfg.Script == "" {
		return "", nil, fmt.Errorf("slurm script content is required")
	}
	if containsNewline(cfg.DockerExecutable) || containsNewline(cfg.SlurmContainer) || containsNewline(cfg.ScriptPath) {
		return "", nil, fmt.Errorf("docker slurm script values must not contain newlines")
	}

	return cfg.DockerExecutable, []string{"exec", "-i", cfg.SlurmContainer, "tee", cfg.ScriptPath}, nil
}

func parseSubmittedSlurmJobID(output string) (string, error) {
	fields := strings.Fields(output)
	for index := 0; index+3 < len(fields); index++ {
		if fields[index] == "Submitted" && fields[index+1] == "batch" && fields[index+2] == "job" {
			return fields[index+3], nil
		}
	}
	return "", fmt.Errorf("parse sbatch job id from output: %q", strings.TrimSpace(output))
}

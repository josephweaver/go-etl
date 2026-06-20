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

func parseSubmittedSlurmJobID(output string) (string, error) {
	fields := strings.Fields(output)
	for index := 0; index+3 < len(fields); index++ {
		if fields[index] == "Submitted" && fields[index+1] == "batch" && fields[index+2] == "job" {
			return fields[index+3], nil
		}
	}
	return "", fmt.Errorf("parse sbatch job id from output: %q", strings.TrimSpace(output))
}

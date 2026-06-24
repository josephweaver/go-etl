package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type SlurmScheduler struct {
	Transport Transport
	TempDir   string
}

type SlurmExecutionConfig struct {
	RemoteScriptPath string
	WorkerScript     SlurmWorkerScriptConfig
}

func (s SlurmScheduler) Submit(ctx context.Context, job JobSpec) (JobHandle, error) {
	id, err := s.Execute(ctx, SlurmExecutionConfig{
		RemoteScriptPath: job.RemoteScriptPath,
		WorkerScript:     job.WorkerScript,
	})
	if err != nil {
		return JobHandle{}, err
	}
	return JobHandle{ID: id}, nil
}

func (s SlurmScheduler) Execute(ctx context.Context, cfg SlurmExecutionConfig) (string, error) {
	if s.Transport == nil {
		return "", fmt.Errorf("slurm transport is required")
	}
	if cfg.RemoteScriptPath == "" {
		return "", fmt.Errorf("remote slurm script path is required")
	}

	script, err := GenerateSlurmWorkerScript(cfg.WorkerScript)
	if err != nil {
		return "", err
	}

	localScriptPath, err := s.writeTempScript(script)
	if err != nil {
		return "", err
	}
	defer os.Remove(localScriptPath)

	if err := s.Transport.Copy(ctx, localScriptPath, cfg.RemoteScriptPath); err != nil {
		return "", err
	}

	output, err := s.Transport.Exec(ctx, "sbatch", cfg.RemoteScriptPath)
	if err != nil {
		return "", err
	}

	return parseSubmittedSlurmJobID(string(output))
}

func (s SlurmScheduler) writeTempScript(script string) (string, error) {
	file, err := os.CreateTemp(s.TempDir, "goetl-worker-*.slurm")
	if err != nil {
		return "", fmt.Errorf("create temp slurm script: %w", err)
	}
	path := file.Name()

	if _, err := file.WriteString(script); err != nil {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("write temp slurm script: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("close temp slurm script: %w", err)
	}

	return filepath.Clean(path), nil
}

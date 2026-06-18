package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SlurmWorkerScriptConfig struct {
	JobName          string
	WorkerExecutable string
	WorkerArgs       []string
	WorkerConfigPath string
	LogDir           string
}

func GenerateSlurmWorkerScript(cfg SlurmWorkerScriptConfig) (string, error) {
	if err := cfg.validate(); err != nil {
		return "", err
	}

	var script strings.Builder
	script.WriteString("#!/usr/bin/env bash\n")
	script.WriteString("#SBATCH --job-name=" + cfg.JobName + "\n")
	script.WriteString("#SBATCH --output=" + cfg.LogDir + "/%x-%j.out\n")
	script.WriteString("#SBATCH --error=" + cfg.LogDir + "/%x-%j.err\n")
	script.WriteString("set -euo pipefail\n")
	script.WriteString("mkdir -p " + shellQuote(cfg.LogDir) + "\n")
	script.WriteString(shellCommand(append([]string{cfg.WorkerExecutable}, append(cfg.WorkerArgs, cfg.WorkerConfigPath)...)) + "\n")
	return script.String(), nil
}

func WriteSlurmWorkerScript(path string, cfg SlurmWorkerScriptConfig) error {
	if path == "" {
		return fmt.Errorf("slurm script path is required")
	}

	script, err := GenerateSlurmWorkerScript(cfg)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create slurm script dir: %w", err)
		}
	}

	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		return fmt.Errorf("write slurm script: %w", err)
	}
	return nil
}

func WriteFakeHPCCWorkerScript() error {
	return WriteSlurmWorkerScript(".run/fake-hpcc/worker.slurm", SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: "go",
		WorkerArgs:       []string{"run", "./cmd/worker"},
		WorkerConfigPath: "./cmd/worker/demo-config.json",
		LogDir:           ".run/fake-hpcc/logs",
	})
}

func (cfg SlurmWorkerScriptConfig) validate() error {
	if cfg.JobName == "" {
		return fmt.Errorf("slurm job name is required")
	}
	if strings.ContainsAny(cfg.JobName, " \t\r\n/") {
		return fmt.Errorf("slurm job name must not contain whitespace or path separators")
	}
	if cfg.WorkerExecutable == "" {
		return fmt.Errorf("worker executable is required")
	}
	if cfg.WorkerConfigPath == "" {
		return fmt.Errorf("worker config path is required")
	}
	if cfg.LogDir == "" {
		return fmt.Errorf("log dir is required")
	}
	if containsNewline(cfg.WorkerExecutable) || containsNewline(cfg.WorkerConfigPath) || containsNewline(cfg.LogDir) || containsNewlineInList(cfg.WorkerArgs) {
		return fmt.Errorf("slurm script values must not contain newlines")
	}
	return nil
}

func containsNewline(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func containsNewlineInList(values []string) bool {
	for _, value := range values {
		if containsNewline(value) {
			return true
		}
	}
	return false
}

func shellCommand(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

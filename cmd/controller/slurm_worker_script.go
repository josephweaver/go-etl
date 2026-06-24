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
	Platform         ShellDialect
}

func GenerateSlurmWorkerScript(cfg SlurmWorkerScriptConfig) (string, error) {
	if err := cfg.validate(); err != nil {
		return "", err
	}
	platform := slurmScriptPlatform(cfg)
	workerExecutable, err := platform.LocalizePath(cfg.WorkerExecutable)
	if err != nil {
		return "", fmt.Errorf("worker executable: %w", err)
	}
	workerConfigPath, err := platform.LocalizePath(cfg.WorkerConfigPath)
	if err != nil {
		return "", fmt.Errorf("worker config path: %w", err)
	}
	logDir, err := platform.LocalizePath(cfg.LogDir)
	if err != nil {
		return "", fmt.Errorf("log dir: %w", err)
	}

	var script strings.Builder
	newline := platform.Newline()
	script.WriteString("#!/usr/bin/env bash" + newline)
	script.WriteString("#SBATCH --job-name=" + cfg.JobName + newline)
	script.WriteString("#SBATCH --output=" + logDir + "/%x-%j.out" + newline)
	script.WriteString("#SBATCH --error=" + logDir + "/%x-%j.err" + newline)
	script.WriteString("set -euo pipefail" + newline)
	script.WriteString("mkdir -p " + platform.QuoteArg(logDir) + newline)
	script.WriteString(shellCommandWithPlatform(platform, append([]string{workerExecutable}, append(cfg.WorkerArgs, workerConfigPath)...)) + newline)
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

func slurmScriptPlatform(cfg SlurmWorkerScriptConfig) ShellDialect {
	if cfg.Platform != nil {
		return cfg.Platform
	}
	return BashShellPlatform{}
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
	return shellCommandWithPlatform(BashShellPlatform{}, args)
}

func shellCommandWithPlatform(platform ShellDialect, args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, platform.QuoteArg(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

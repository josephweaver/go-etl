package main

import (
	"fmt"
	"strings"
)

type SlurmWorkerScriptConfig struct {
	JobName          string
	WorkerExecutable string
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
	script.WriteString(shellQuote(cfg.WorkerExecutable) + " " + shellQuote(cfg.WorkerConfigPath) + "\n")
	return script.String(), nil
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
	if containsNewline(cfg.WorkerExecutable) || containsNewline(cfg.WorkerConfigPath) || containsNewline(cfg.LogDir) {
		return fmt.Errorf("slurm script values must not contain newlines")
	}
	return nil
}

func containsNewline(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

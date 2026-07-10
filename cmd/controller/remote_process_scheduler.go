package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type RemoteProcessScheduler struct {
	Transport Transport
	TempDir   string
}

func (s RemoteProcessScheduler) Submit(ctx context.Context, job JobSpec) (JobHandle, error) {
	pid, err := s.Execute(ctx, job)
	if err != nil {
		return JobHandle{}, err
	}
	return JobHandle{ID: "remote-pid:" + pid}, nil
}

func (s RemoteProcessScheduler) Execute(ctx context.Context, job JobSpec) (string, error) {
	if s.Transport == nil {
		return "", fmt.Errorf("remote process transport is required")
	}
	if job.RemoteScriptPath == "" {
		return "", fmt.Errorf("remote process script path is required")
	}

	script, err := GenerateSlurmWorkerScript(job.WorkerScript)
	if err != nil {
		return "", err
	}
	localScriptPath, err := (SlurmScheduler{TempDir: s.TempDir}).writeTempScript(script)
	if err != nil {
		return "", err
	}
	defer os.Remove(localScriptPath)

	if err := s.Transport.Copy(ctx, localScriptPath, job.RemoteScriptPath); err != nil {
		return "", err
	}

	platform := slurmScriptPlatform(job.WorkerScript)
	remoteScriptPath, err := platform.LocalizePath(job.RemoteScriptPath)
	if err != nil {
		return "", fmt.Errorf("remote process script path: %w", err)
	}
	logDir, err := platform.LocalizePath(job.WorkerScript.LogDir)
	if err != nil {
		return "", fmt.Errorf("remote process log dir: %w", err)
	}

	command := "mkdir -p " + platform.QuoteArg(logDir) +
		" && nohup bash " + platform.QuoteArg(remoteScriptPath) +
		" > " + platform.QuoteArg(logDir+"/remote-process.out") +
		" 2> " + platform.QuoteArg(logDir+"/remote-process.err") +
		" < /dev/null & echo $!"
	output, err := s.Transport.Exec(ctx, "sh", "-c", command)
	if err != nil {
		return "", fmt.Errorf("start remote process worker: %w", err)
	}
	pid := strings.TrimSpace(string(output))
	if pid == "" {
		return "", fmt.Errorf("start remote process worker: missing pid")
	}
	for _, r := range pid {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("start remote process worker: invalid pid %q", pid)
		}
	}
	return pid, nil
}

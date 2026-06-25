package main

import (
	"context"
	"fmt"
	"os/exec"
)

type DirectProcessScheduler struct{}

func (s DirectProcessScheduler) Submit(ctx context.Context, job JobSpec) (JobHandle, error) {
	worker := job.WorkerScript
	if err := worker.validate(); err != nil {
		return JobHandle{}, err
	}

	args := append([]string{}, worker.WorkerArgs...)
	args = append(args, worker.WorkerConfigPath)

	command := exec.CommandContext(ctx, worker.WorkerExecutable, args...)
	if err := command.Start(); err != nil {
		return JobHandle{}, fmt.Errorf("start direct worker process: %w", err)
	}
	go func() {
		_ = command.Wait()
	}()

	return JobHandle{ID: fmt.Sprintf("pid:%d", command.Process.Pid)}, nil
}

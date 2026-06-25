package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type LocalTransport struct{}

func (t LocalTransport) Copy(ctx context.Context, localPath string, remotePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local source %s: %w", localPath, err)
	}
	defer source.Close()

	if dir := filepath.Dir(remotePath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create local destination dir: %w", err)
		}
	}

	destination, err := os.OpenFile(remotePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open local destination %s: %w", remotePath, err)
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf("copy %s to %s: %w", localPath, remotePath, err)
	}
	return nil
}

func (t LocalTransport) Exec(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("local exec command is required")
	}

	command := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run local command %s: %w", args[0], err)
	}
	return output, nil
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
)

type Runtime interface {
	Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error
}

type SharedFilesystemWorkerRuntime struct {
	Root                string
	ControllerURL       string
	LocalWorkerArtifact string
}

func (r SharedFilesystemWorkerRuntime) Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error {
	if transport == nil {
		return fmt.Errorf("runtime transport is required")
	}
	if dialect == nil {
		return fmt.Errorf("runtime shell dialect is required")
	}

	paths, err := r.paths()
	if err != nil {
		return err
	}

	dirs := []string{
		path.Dir(paths.WorkerExecutable),
		path.Dir(paths.WorkerConfigPath),
		path.Dir(paths.WorkerScriptPath),
		paths.LogDir,
		paths.TmpDir,
		paths.DataDir,
	}
	localizedDirs := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		localized, err := dialect.LocalizePath(dir)
		if err != nil {
			return fmt.Errorf("runtime dir %q: %w", dir, err)
		}
		localizedDirs = append(localizedDirs, localized)
	}
	if _, err := transport.Exec(ctx, append([]string{"mkdir", "-p"}, localizedDirs...)...); err != nil {
		return fmt.Errorf("create runtime dirs: %w", err)
	}

	if r.ControllerURL != "" {
		if err := r.writeWorkerConfig(ctx, transport, paths); err != nil {
			return err
		}
	}
	if r.LocalWorkerArtifact != "" {
		if err := r.uploadWorkerArtifact(ctx, transport, dialect, paths); err != nil {
			return err
		}
	}

	return nil
}

type SharedFilesystemWorkerRuntimePaths struct {
	Root             string
	WorkerExecutable string
	WorkerConfigPath string
	WorkerScriptPath string
	LogDir           string
	TmpDir           string
	DataDir          string
}

type SharedFilesystemWorkerConfig struct {
	LogDir        string `json:"log_dir"`
	TmpDir        string `json:"tmp_dir"`
	DataDir       string `json:"data_dir"`
	ControllerURL string `json:"controller_url"`
}

func (r SharedFilesystemWorkerRuntime) paths() (SharedFilesystemWorkerRuntimePaths, error) {
	root := strings.TrimRight(r.Root, "/")
	if root == "" {
		root = "/data/goetl"
	}
	if containsNewline(root) {
		return SharedFilesystemWorkerRuntimePaths{}, fmt.Errorf("runtime root must not contain newlines")
	}

	return SharedFilesystemWorkerRuntimePaths{
		Root:             root,
		WorkerExecutable: path.Join(root, "artifacts", "goetl-worker"),
		WorkerConfigPath: path.Join(root, "config", "worker.json"),
		WorkerScriptPath: path.Join(root, "scripts", "worker.slurm"),
		LogDir:           path.Join(root, "logs"),
		TmpDir:           path.Join(root, "tmp"),
		DataDir:          path.Join(root, "data"),
	}, nil
}

func (r SharedFilesystemWorkerRuntime) writeWorkerConfig(ctx context.Context, transport Transport, paths SharedFilesystemWorkerRuntimePaths) error {
	data, err := json.MarshalIndent(SharedFilesystemWorkerConfig{
		LogDir:        paths.LogDir,
		TmpDir:        paths.TmpDir,
		DataDir:       paths.DataDir,
		ControllerURL: r.ControllerURL,
	}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	file, err := os.CreateTemp("", "goetl-worker-*.json")
	if err != nil {
		return fmt.Errorf("create temp worker config: %w", err)
	}
	localPath := file.Name()
	defer os.Remove(localPath)

	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temp worker config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp worker config: %w", err)
	}

	if err := transport.Copy(ctx, localPath, paths.WorkerConfigPath); err != nil {
		return fmt.Errorf("copy worker config: %w", err)
	}
	return nil
}

func (r SharedFilesystemWorkerRuntime) uploadWorkerArtifact(ctx context.Context, transport Transport, dialect ShellDialect, paths SharedFilesystemWorkerRuntimePaths) error {
	if err := transport.Copy(ctx, r.LocalWorkerArtifact, paths.WorkerExecutable); err != nil {
		return fmt.Errorf("copy worker artifact: %w", err)
	}
	workerExecutable, err := dialect.LocalizePath(paths.WorkerExecutable)
	if err != nil {
		return fmt.Errorf("worker executable path: %w", err)
	}
	if _, err := transport.Exec(ctx, "chmod", "0755", workerExecutable); err != nil {
		return fmt.Errorf("chmod worker artifact: %w", err)
	}
	return nil
}

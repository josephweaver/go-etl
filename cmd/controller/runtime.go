package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Runtime interface {
	Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error
}

type WorkerScriptRuntime interface {
	Runtime
	WorkerScript(cfg SlurmWorkerScriptConfig) (SlurmWorkerScriptConfig, error)
}

type WorkerRuntime struct {
	Root                string
	ControllerURL       string
	LocalWorkerArtifact string
	DataDir             string
	AssetCacheDir       string
	DataLocationRoots   map[string]string
}

func (r WorkerRuntime) Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error {
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

	dirs := r.runtimeDirs(paths)
	if err := r.createRuntimeDirs(ctx, transport, dialect, dirs); err != nil {
		return err
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

func (r WorkerRuntime) runtimeDirs(paths WorkerRuntimePaths) []string {
	dirs := []string{
		path.Dir(paths.WorkerExecutable),
		path.Dir(paths.WorkerConfigPath),
		path.Dir(paths.WorkerScriptPath),
		paths.LogDir,
		paths.TmpDir,
		paths.DataDir,
	}
	if r.AssetCacheDir != "" {
		dirs = append(dirs, r.AssetCacheDir)
	}
	for _, root := range r.DataLocationRoots {
		dirs = append(dirs, root)
	}
	return dirs
}

func (r WorkerRuntime) createRuntimeDirs(ctx context.Context, transport Transport, dialect ShellDialect, dirs []string) error {
	if _, ok := transport.(LocalTransport); ok {
		for _, dir := range dirs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
				return fmt.Errorf("create local runtime dir %s: %w", dir, err)
			}
		}
		return nil
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
	return nil
}

type WorkerRuntimePaths struct {
	Root             string
	WorkerExecutable string
	WorkerConfigPath string
	WorkerScriptPath string
	LogDir           string
	TmpDir           string
	DataDir          string
}

type WorkerConfig struct {
	LogDir            string            `json:"log_dir"`
	TmpDir            string            `json:"tmp_dir"`
	DataDir           string            `json:"data_dir"`
	ControllerURL     string            `json:"controller_url"`
	AssetCacheDir     string            `json:"asset_cache_dir,omitempty"`
	DataLocationRoots map[string]string `json:"data_location_roots,omitempty"`
}

func (r WorkerRuntime) paths() (WorkerRuntimePaths, error) {
	root := strings.TrimRight(r.Root, "/")
	if root == "" {
		root = "/data/goetl"
	}
	if containsNewline(root) {
		return WorkerRuntimePaths{}, fmt.Errorf("runtime root must not contain newlines")
	}
	dataDir := r.DataDir
	if dataDir == "" {
		dataDir = path.Join(root, "data")
	}
	if containsNewline(dataDir) {
		return WorkerRuntimePaths{}, fmt.Errorf("runtime data dir must not contain newlines")
	}

	return WorkerRuntimePaths{
		Root:             root,
		WorkerExecutable: path.Join(root, "artifacts", "goetl-worker"),
		WorkerConfigPath: path.Join(root, "config", "worker.json"),
		WorkerScriptPath: path.Join(root, "scripts", "worker.slurm"),
		LogDir:           path.Join(root, "logs"),
		TmpDir:           path.Join(root, "tmp"),
		DataDir:          dataDir,
	}, nil
}

func (r WorkerRuntime) writeWorkerConfig(ctx context.Context, transport Transport, paths WorkerRuntimePaths) error {
	if containsNewline(r.AssetCacheDir) {
		return fmt.Errorf("worker asset cache dir must not contain newlines")
	}
	for name, root := range r.DataLocationRoots {
		if containsNewline(name) || containsNewline(root) {
			return fmt.Errorf("worker data location roots must not contain newlines")
		}
	}
	data, err := json.MarshalIndent(WorkerConfig{
		LogDir:            paths.LogDir,
		TmpDir:            paths.TmpDir,
		DataDir:           paths.DataDir,
		ControllerURL:     r.ControllerURL,
		AssetCacheDir:     r.AssetCacheDir,
		DataLocationRoots: r.DataLocationRoots,
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

func (r WorkerRuntime) uploadWorkerArtifact(ctx context.Context, transport Transport, dialect ShellDialect, paths WorkerRuntimePaths) error {
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

type SingularityWorkerRuntime struct {
	WorkerRuntime
	SingularityExecutable     string
	ImagePath                 string
	ContainerWorkerExecutable string
	Bind                      string
}

func (r SingularityWorkerRuntime) WorkerScript(cfg SlurmWorkerScriptConfig) (SlurmWorkerScriptConfig, error) {
	executable := r.SingularityExecutable
	if executable == "" {
		executable = "singularity"
	}
	if containsNewline(executable) {
		return SlurmWorkerScriptConfig{}, fmt.Errorf("singularity executable must not contain newlines")
	}
	if r.ImagePath == "" {
		return SlurmWorkerScriptConfig{}, fmt.Errorf("singularity image path is required")
	}
	if r.ContainerWorkerExecutable == "" {
		return SlurmWorkerScriptConfig{}, fmt.Errorf("container worker executable is required")
	}
	if containsNewline(r.ImagePath) || containsNewline(r.ContainerWorkerExecutable) || containsNewline(r.Bind) {
		return SlurmWorkerScriptConfig{}, fmt.Errorf("singularity runtime values must not contain newlines")
	}

	args := []string{"exec"}
	if r.Bind != "" {
		args = append(args, "--bind", r.Bind)
	}
	args = append(args, r.ImagePath, r.ContainerWorkerExecutable)
	args = append(args, cfg.WorkerArgs...)

	cfg.WorkerExecutable = executable
	cfg.WorkerArgs = args
	return cfg, nil
}

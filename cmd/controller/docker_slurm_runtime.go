package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"strings"
)

type DockerSlurmRuntimePaths struct {
	Root             string
	WorkerExecutable string
	WorkerConfigPath string
	WorkerScriptPath string
	LogDir           string
	TmpDir           string
	DataDir          string
}

type DockerSlurmWorkerConfig struct {
	LogDir        string `json:"log_dir"`
	TmpDir        string `json:"tmp_dir"`
	DataDir       string `json:"data_dir"`
	ControllerURL string `json:"controller_url"`
}

type DockerSlurmRuntimeConfig struct {
	DockerExecutable string
	SlurmContainer   string
	ControllerURL    string
	Paths            DockerSlurmRuntimePaths
}

func DefaultDockerSlurmRuntimePaths() DockerSlurmRuntimePaths {
	return DockerSlurmRuntimePathsForRoot("/data/goetl")
}

func DockerSlurmRuntimePathsForRoot(root string) DockerSlurmRuntimePaths {
	root = strings.TrimRight(root, "/")
	return DockerSlurmRuntimePaths{
		Root:             root,
		WorkerExecutable: path.Join(root, "artifacts", "goetl-worker"),
		WorkerConfigPath: path.Join(root, "config", "worker.json"),
		WorkerScriptPath: path.Join(root, "scripts", "worker.slurm"),
		LogDir:           path.Join(root, "logs"),
		TmpDir:           path.Join(root, "tmp"),
		DataDir:          path.Join(root, "data"),
	}
}

func GenerateDockerSlurmWorkerConfig(controllerURL string, paths DockerSlurmRuntimePaths) (string, error) {
	if controllerURL == "" {
		return "", fmt.Errorf("controller url is required")
	}
	if paths.LogDir == "" || paths.TmpDir == "" || paths.DataDir == "" {
		return "", fmt.Errorf("worker runtime directories are required")
	}

	data, err := json.MarshalIndent(DockerSlurmWorkerConfig{
		LogDir:        paths.LogDir,
		TmpDir:        paths.TmpDir,
		DataDir:       paths.DataDir,
		ControllerURL: controllerURL,
	}, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data) + "\n", nil
}

func PrepareDockerSlurmRuntime(ctx context.Context, cfg DockerSlurmRuntimeConfig) error {
	paths := cfg.Paths
	if paths.Root == "" {
		paths = DefaultDockerSlurmRuntimePaths()
	}

	workerConfig, err := GenerateDockerSlurmWorkerConfig(cfg.ControllerURL, paths)
	if err != nil {
		return err
	}

	executable, args, err := dockerSlurmMkdirCommand(cfg, paths)
	if err != nil {
		return err
	}
	if output, err := exec.CommandContext(ctx, executable, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("prepare docker slurm runtime dirs: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return WriteDockerSlurmScript(ctx, DockerSlurmScriptConfig{
		DockerExecutable: cfg.DockerExecutable,
		SlurmContainer:   cfg.SlurmContainer,
		ScriptPath:       paths.WorkerConfigPath,
		Script:           workerConfig,
	})
}

func dockerSlurmMkdirCommand(cfg DockerSlurmRuntimeConfig, paths DockerSlurmRuntimePaths) (string, []string, error) {
	executable := cfg.DockerExecutable
	if executable == "" {
		executable = "docker"
	}
	container := cfg.SlurmContainer
	if container == "" {
		container = "slurmctld"
	}
	if paths.Root == "" || paths.LogDir == "" || paths.TmpDir == "" || paths.DataDir == "" {
		return "", nil, fmt.Errorf("docker slurm runtime paths are required")
	}
	if containsNewline(executable) || containsNewline(container) || containsNewline(paths.Root) || containsNewline(paths.LogDir) || containsNewline(paths.TmpDir) || containsNewline(paths.DataDir) {
		return "", nil, fmt.Errorf("docker slurm runtime values must not contain newlines")
	}

	dirs := []string{
		path.Dir(paths.WorkerExecutable),
		path.Dir(paths.WorkerConfigPath),
		path.Dir(paths.WorkerScriptPath),
		paths.LogDir,
		paths.TmpDir,
		paths.DataDir,
	}
	args := append([]string{"exec", container, "mkdir", "-p"}, dirs...)
	return executable, args, nil
}

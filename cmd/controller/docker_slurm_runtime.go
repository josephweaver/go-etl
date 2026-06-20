package main

import (
	"encoding/json"
	"fmt"
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

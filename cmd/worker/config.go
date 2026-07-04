package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	LogDir           string `json:"log_dir"`
	TmpDir           string `json:"tmp_dir"`
	DataDir          string `json:"data_dir"`
	ControllerURL    string `json:"controller_url"`
	PythonExecutable string `json:"python_executable,omitempty"`
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config file %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config file %s: %w", path, err)
	}

	cfg.resolveRelativePaths(filepath.Dir(path))
	return cfg, nil
}

func (c *Config) resolveRelativePaths(root string) {
	c.LogDir = resolveRelativePath(root, c.LogDir)
	c.TmpDir = resolveRelativePath(root, c.TmpDir)
	c.DataDir = resolveRelativePath(root, c.DataDir)
}

func resolveRelativePath(root string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(root, path)
}

func (c Config) Validate() error {
	if c.LogDir == "" {
		return fmt.Errorf("log dir is required")
	}

	if c.TmpDir == "" {
		return fmt.Errorf("tmp dir is required")
	}

	if c.DataDir == "" {
		return fmt.Errorf("data dir is required")
	}

	if c.ControllerURL == "" {
		return fmt.Errorf("controller url is required")
	}

	return nil
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	LogDir        string `json:"log_dir"`
	TmpDir        string `json:"tmp_dir"`
	DataDir       string `json:"data_dir"`
	ControllerURL string `json:"controller_url"`
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

	return cfg, nil
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

func defaultConfig() Config {
	return Config{
		LogDir:        ".run/logs",
		TmpDir:        ".run/tmp",
		DataDir:       ".run/data",
		ControllerURL: "https://controller.local",
	}
}

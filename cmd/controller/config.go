package main

import (
	"encoding/json"
	"fmt"
	"os"

	"goetl/internal/variable"
)

type ControllerConfig struct {
	Variables            []variable.Variable        `json:"variables"`
	ExecutionEnvironment ExecutionEnvironmentConfig `json:"execution_environment"`
}

func loadControllerConfig(path string) (ControllerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ControllerConfig{}, fmt.Errorf("read controller config file %s: %w", path, err)
	}

	var cfg ControllerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ControllerConfig{}, fmt.Errorf("decode controller config file %s: %w", path, err)
	}

	cfg.normalizeVariables()
	if err := cfg.Validate(); err != nil {
		return ControllerConfig{}, fmt.Errorf("validate controller config file %s: %w", path, err)
	}

	return cfg, nil
}

func (c *ControllerConfig) normalizeVariables() {
	for index := range c.Variables {
		c.Variables[index].Name.Namespace = variable.NamespaceControllerConfig
	}
}

func (c ControllerConfig) Validate() error {
	if len(c.Variables) == 0 {
		return fmt.Errorf("variables are required")
	}

	if _, err := variable.NewScope(c.Variables...); err != nil {
		return err
	}

	if !c.ExecutionEnvironment.IsZero() {
		if err := c.ExecutionEnvironment.Validate(); err != nil {
			return err
		}
	}

	return nil
}

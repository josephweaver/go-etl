package main

import (
	"encoding/json"
	"fmt"
	"os"

	"goetl/internal/variable"
)

const (
	controllerAPIVersion = "goet/v1alpha1"
	controllerKind       = "Controller"
)

type ControllerConfig struct {
	APIVersion           string                     `json:"api_version"`
	Kind                 string                     `json:"kind"`
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

	if err := cfg.validateEnvelope(); err != nil {
		return ControllerConfig{}, fmt.Errorf("validate controller config file %s: %w", path, err)
	}

	cfg.normalizeVariables()
	if err := cfg.Validate(); err != nil {
		return ControllerConfig{}, fmt.Errorf("validate controller config file %s: %w", path, err)
	}

	return cfg, nil
}

func (c ControllerConfig) validateEnvelope() error {
	if c.APIVersion != controllerAPIVersion {
		return fmt.Errorf("api_version must be %q, got %q", controllerAPIVersion, c.APIVersion)
	}
	if c.Kind != controllerKind {
		return fmt.Errorf("kind must be %q, got %q", controllerKind, c.Kind)
	}

	return nil
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

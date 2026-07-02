package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/variable"
)

const (
	controllerAPIVersion = "goet/v1alpha1"
	controllerKind       = "Controller"
	defaultsKind         = "Defaults"
	defaultsFilename     = "defaults.json"
)

type ControllerConfig struct {
	APIVersion           string                     `json:"api_version"`
	Kind                 string                     `json:"kind"`
	Variables            []variable.Variable        `json:"variables"`
	ExecutionEnvironment ExecutionEnvironmentConfig `json:"execution_environment"`
}

type DefaultsDocument struct {
	APIVersion string              `json:"api_version"`
	Kind       string              `json:"kind"`
	Variables  []variable.Variable `json:"variables"`
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

func defaultsPathForControllerConfig(controllerPath string) string {
	return filepath.Join(filepath.Dir(controllerPath), defaultsFilename)
}

func loadDefaultsDocument(path string) (DefaultsDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultsDocument{}, fmt.Errorf("read defaults file %s: %w", path, err)
	}

	var document DefaultsDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return DefaultsDocument{}, fmt.Errorf("decode defaults file %s: %w", path, err)
	}
	if err := document.validateEnvelope(); err != nil {
		return DefaultsDocument{}, fmt.Errorf("validate defaults file %s: %w", path, err)
	}
	if err := document.Validate(); err != nil {
		return DefaultsDocument{}, fmt.Errorf("validate defaults file %s: %w", path, err)
	}

	return document, nil
}

func (d DefaultsDocument) validateEnvelope() error {
	if d.APIVersion != controllerAPIVersion {
		return fmt.Errorf("api_version must be %q, got %q", controllerAPIVersion, d.APIVersion)
	}
	if d.Kind != defaultsKind {
		return fmt.Errorf("kind must be %q, got %q", defaultsKind, d.Kind)
	}

	return nil
}

func (d DefaultsDocument) Validate() error {
	if len(d.Variables) == 0 {
		return fmt.Errorf("variables are required")
	}

	byNamespace := make(map[variable.Namespace][]variable.Variable)
	for _, item := range d.Variables {
		if !defaultsNamespaceAllowed(item.Name.Namespace) {
			return fmt.Errorf("defaults variable %s uses disallowed namespace %q", item.Name, item.Name.Namespace)
		}
		byNamespace[item.Name.Namespace] = append(byNamespace[item.Name.Namespace], item)
	}
	for namespace, variables := range byNamespace {
		if _, err := variable.NewScope(variables...); err != nil {
			return fmt.Errorf("namespace %s: %w", namespace, err)
		}
	}

	return nil
}

func defaultsNamespaceAllowed(namespace variable.Namespace) bool {
	switch namespace {
	case variable.NamespaceClientConfig,
		variable.NamespaceControllerConfig,
		variable.NamespaceWorkerConfig,
		variable.NamespaceProjectConfig:
		return true
	default:
		return false
	}
}

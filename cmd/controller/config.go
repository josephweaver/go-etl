package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

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

type controllerStartupSources struct {
	ControllerPath string
	DefaultsPath   string
	Controller     ControllerConfig
	Defaults       DefaultsDocument
}

type WorkerHeartbeatPolicy struct {
	HeartbeatInterval time.Duration
	DeadAfter         time.Duration
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

func (c ControllerConfig) Validate() error {
	if len(c.Variables) == 0 {
		return fmt.Errorf("variables are required")
	}
	for _, item := range c.Variables {
		if item.Name.Namespace != variable.NamespaceControllerConfig {
			return fmt.Errorf("controller variable %s must use namespace %q, got %q", item.Name, variable.NamespaceControllerConfig, item.Name.Namespace)
		}
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

func defaultWorkerHeartbeatPolicy() WorkerHeartbeatPolicy {
	return WorkerHeartbeatPolicy{
		HeartbeatInterval: time.Minute,
		DeadAfter:         5 * time.Minute,
	}
}

func workerHeartbeatPolicyConfig(resolver variable.Resolver, defaults WorkerHeartbeatPolicy) (WorkerHeartbeatPolicy, error) {
	policy := defaults

	var err error
	if policy.HeartbeatInterval, err = optionalDurationVariable(resolver, "worker_heartbeat_interval", policy.HeartbeatInterval); err != nil {
		return WorkerHeartbeatPolicy{}, err
	}
	if policy.DeadAfter, err = optionalDurationVariable(resolver, "worker_dead_after", policy.DeadAfter); err != nil {
		return WorkerHeartbeatPolicy{}, err
	}
	if err := validateWorkerHeartbeatPolicy(policy); err != nil {
		return WorkerHeartbeatPolicy{}, err
	}
	return policy, nil
}

func validateWorkerHeartbeatPolicy(policy WorkerHeartbeatPolicy) error {
	if policy.HeartbeatInterval <= 0 {
		return fmt.Errorf("worker_heartbeat_interval must be greater than zero")
	}
	if policy.DeadAfter <= policy.HeartbeatInterval {
		return fmt.Errorf("worker_dead_after must be greater than worker_heartbeat_interval")
	}
	if policy.DeadAfter < 2*policy.HeartbeatInterval {
		return fmt.Errorf("worker_dead_after must be at least twice worker_heartbeat_interval")
	}
	return nil
}

func loadControllerStartupSources(controllerPath string) (controllerStartupSources, error) {
	controller, err := loadControllerConfig(controllerPath)
	if err != nil {
		return controllerStartupSources{}, err
	}

	defaultsPath := defaultsPathForControllerConfig(controllerPath)
	defaults, err := loadDefaultsDocument(defaultsPath)
	if err != nil {
		return controllerStartupSources{}, err
	}

	return controllerStartupSources{
		ControllerPath: controllerPath,
		DefaultsPath:   defaultsPath,
		Controller:     controller,
		Defaults:       defaults,
	}, nil
}

func (s controllerStartupSources) controllerScopes() (variable.Scope, variable.Scope, error) {
	defaultVariables := make([]variable.Variable, 0, len(s.Defaults.Variables))
	for _, item := range s.Defaults.Variables {
		if item.Name.Namespace == variable.NamespaceControllerConfig {
			defaultVariables = append(defaultVariables, item)
		}
	}

	defaultScope, err := variable.NewScope(defaultVariables...)
	if err != nil {
		return nil, nil, fmt.Errorf("build defaults controller scope: %w", err)
	}
	controllerScope, err := variable.NewScope(s.Controller.Variables...)
	if err != nil {
		return nil, nil, fmt.Errorf("build explicit controller scope: %w", err)
	}

	return defaultScope, controllerScope, nil
}

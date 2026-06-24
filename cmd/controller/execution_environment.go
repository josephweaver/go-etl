package main

import (
	"context"
	"fmt"
)

type ExecutionEnvironment struct {
	Config     ExecutionEnvironmentConfig
	Transports []Transport
	Dialect    ShellDialect
	Scheduler  Scheduler
	Runtime    Runtime
}

type ExecutionEnvironmentConfig struct {
	Name       string                     `json:"name"`
	Transports []ExecutionComponentConfig `json:"transports"`
	Dialect    ExecutionComponentConfig   `json:"dialect"`
	Scheduler  ExecutionComponentConfig   `json:"scheduler"`
	Runtime    ExecutionComponentConfig   `json:"runtime"`
}

type ExecutionComponentConfig struct {
	Name     string            `json:"name,omitempty"`
	Type     string            `json:"type"`
	Settings map[string]string `json:"settings,omitempty"`
}

func (cfg ExecutionEnvironmentConfig) IsZero() bool {
	return cfg.Name == "" &&
		len(cfg.Transports) == 0 &&
		cfg.Dialect.Type == "" &&
		cfg.Scheduler.Type == "" &&
		cfg.Runtime.Type == ""
}

func (cfg ExecutionEnvironmentConfig) Validate() error {
	if cfg.Name == "" {
		return fmt.Errorf("execution environment name is required")
	}
	if len(cfg.Transports) == 0 {
		return fmt.Errorf("at least one transport is required")
	}
	for index, transport := range cfg.Transports {
		if err := transport.validate("transport"); err != nil {
			return fmt.Errorf("transports[%d]: %w", index, err)
		}
	}
	if err := cfg.Dialect.validate("dialect"); err != nil {
		return err
	}
	if err := cfg.Scheduler.validate("scheduler"); err != nil {
		return err
	}
	if err := cfg.Runtime.validate("runtime"); err != nil {
		return err
	}
	return nil
}

func NewExecutionEnvironment(cfg ExecutionEnvironmentConfig) (ExecutionEnvironment, error) {
	if err := cfg.Validate(); err != nil {
		return ExecutionEnvironment{}, err
	}

	transports := make([]Transport, 0, len(cfg.Transports))
	for index, transportConfig := range cfg.Transports {
		transport, err := newTransportFromConfig(transportConfig)
		if err != nil {
			return ExecutionEnvironment{}, fmt.Errorf("transports[%d]: %w", index, err)
		}
		transports = append(transports, transport)
	}

	dialect, err := newShellDialectFromConfig(cfg.Dialect)
	if err != nil {
		return ExecutionEnvironment{}, err
	}

	scheduler, err := newSchedulerFromConfig(cfg.Scheduler, transports[0])
	if err != nil {
		return ExecutionEnvironment{}, err
	}

	runtime, err := newRuntimeFromConfig(cfg.Runtime)
	if err != nil {
		return ExecutionEnvironment{}, err
	}

	return ExecutionEnvironment{
		Config:     cfg,
		Transports: transports,
		Dialect:    dialect,
		Scheduler:  scheduler,
		Runtime:    runtime,
	}, nil
}

func (e ExecutionEnvironment) Prepare(ctx context.Context) error {
	for index, transport := range e.Transports {
		if err := prepareIfSupported(ctx, transport); err != nil {
			return fmt.Errorf("prepare transport[%d]: %w", index, err)
		}
	}
	if err := prepareIfSupported(ctx, e.Scheduler); err != nil {
		return fmt.Errorf("prepare scheduler: %w", err)
	}
	if e.Runtime != nil {
		var transport Transport
		if len(e.Transports) > 0 {
			transport = e.Transports[0]
		}
		if err := e.Runtime.Prepare(ctx, transport, e.Dialect); err != nil {
			return fmt.Errorf("prepare runtime: %w", err)
		}
	}
	return nil
}

func (cfg ExecutionComponentConfig) validate(role string) error {
	if cfg.Type == "" {
		return fmt.Errorf("%s type is required", role)
	}
	return nil
}

func newTransportFromConfig(cfg ExecutionComponentConfig) (Transport, error) {
	switch cfg.Type {
	case "docker":
		container := cfg.Settings["container"]
		if container == "" {
			return nil, fmt.Errorf("docker transport setting container is required")
		}
		return DockerContainerTransport{
			Docker: DockerTransport{
				Executable: cfg.Settings["executable"],
			},
			Container: container,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported transport type %q", cfg.Type)
	}
}

func newShellDialectFromConfig(cfg ExecutionComponentConfig) (ShellDialect, error) {
	switch cfg.Type {
	case "bash":
		return BashShellPlatform{}, nil
	default:
		return nil, fmt.Errorf("unsupported dialect type %q", cfg.Type)
	}
}

func newSchedulerFromConfig(cfg ExecutionComponentConfig, transport Transport) (Scheduler, error) {
	switch cfg.Type {
	case "slurm":
		return SlurmScheduler{Transport: transport}, nil
	default:
		return nil, fmt.Errorf("unsupported scheduler type %q", cfg.Type)
	}
}

func newRuntimeFromConfig(cfg ExecutionComponentConfig) (Runtime, error) {
	switch cfg.Type {
	case "worker":
		return WorkerRuntime{
			Root:                cfg.Settings["root"],
			ControllerURL:       cfg.Settings["controller_url"],
			LocalWorkerArtifact: cfg.Settings["local_worker_artifact"],
		}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime type %q", cfg.Type)
	}
}

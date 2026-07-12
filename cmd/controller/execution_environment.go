package main

import (
	"context"
	"fmt"
	"strconv"
)

type ExecutionEnvironment struct {
	Config         ExecutionEnvironmentConfig
	Transports     []Transport
	Dialect        ShellDialect
	Scheduler      Scheduler
	Runtime        Runtime
	CallbackTunnel *SSHReverseCallbackTunnel
}

type ExecutionEnvironmentConfig struct {
	Name           string                     `json:"name"`
	Transports     []ExecutionComponentConfig `json:"transports"`
	Dialect        ExecutionComponentConfig   `json:"dialect"`
	Scheduler      ExecutionComponentConfig   `json:"scheduler"`
	Runtime        ExecutionComponentConfig   `json:"runtime"`
	CallbackTunnel CallbackTunnelConfig       `json:"callback_tunnel,omitempty"`
}

type ExecutionComponentConfig struct {
	Name     string                     `json:"name,omitempty"`
	Type     string                     `json:"type"`
	Settings ExecutionComponentSettings `json:"settings,omitempty"`
}

type ExecutionComponentSettings map[string]any

func (cfg ExecutionEnvironmentConfig) IsZero() bool {
	return cfg.Name == "" &&
		len(cfg.Transports) == 0 &&
		cfg.Dialect.Type == "" &&
		cfg.Scheduler.Type == "" &&
		cfg.Runtime.Type == "" &&
		cfg.CallbackTunnel.IsZero()
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
	if err := cfg.CallbackTunnel.Validate(); err != nil {
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

	callbackTunnel, err := newCallbackTunnelFromConfig(cfg.CallbackTunnel, cfg.Transports, transports, scheduler, runtime)
	if err != nil {
		return ExecutionEnvironment{}, err
	}

	return ExecutionEnvironment{
		Config:         cfg,
		Transports:     transports,
		Dialect:        dialect,
		Scheduler:      scheduler,
		Runtime:        runtime,
		CallbackTunnel: callbackTunnel,
	}, nil
}

func (e *ExecutionEnvironment) Prepare(ctx context.Context) error {
	for index, transport := range e.Transports {
		if err := prepareIfSupported(ctx, transport); err != nil {
			return fmt.Errorf("prepare transport[%d]: %w", index, err)
		}
	}
	if e.CallbackTunnel != nil {
		if err := e.CallbackTunnel.Prepare(ctx); err != nil {
			return fmt.Errorf("prepare callback tunnel: %w", err)
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

func (e *ExecutionEnvironment) Close() error {
	var closeErr error
	if e.CallbackTunnel != nil {
		if err := e.CallbackTunnel.Close(); err != nil {
			closeErr = err
		}
	}
	for index := len(e.Transports) - 1; index >= 0; index-- {
		closer, ok := e.Transports[index].(interface{ Close() error })
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (e ExecutionEnvironment) Preflight(ctx context.Context) []PreflightIssue {
	var issues []PreflightIssue
	for index, transport := range e.Transports {
		issues = append(issues, componentPreflightIssues(ctx, fmt.Sprintf("transport[%d]", index), transport)...)
	}
	if e.CallbackTunnel != nil {
		issues = append(issues, componentPreflightIssues(ctx, "callback_tunnel", e.CallbackTunnel)...)
	}
	issues = append(issues, componentPreflightIssues(ctx, "scheduler", e.Scheduler)...)
	issues = append(issues, runtimePreflightIssues(ctx, "runtime", e.Runtime, e.primaryTransport(), e.Dialect)...)
	return issues
}

func (e ExecutionEnvironment) primaryTransport() Transport {
	if len(e.Transports) == 0 {
		return nil
	}
	return e.Transports[0]
}

func componentPreflightIssues(ctx context.Context, componentName string, component any) []PreflightIssue {
	issues := preflightIfSupported(ctx, component)
	for index := range issues {
		if issues[index].Component == "" {
			issues[index].Component = componentName
		}
	}
	return issues
}

type RuntimePreflightComponent interface {
	RuntimePreflight(ctx context.Context, transport Transport, dialect ShellDialect) []PreflightIssue
}

func runtimePreflightIssues(ctx context.Context, componentName string, runtime Runtime, transport Transport, dialect ShellDialect) []PreflightIssue {
	if runtime == nil {
		return nil
	}
	component, ok := runtime.(RuntimePreflightComponent)
	if !ok {
		return componentPreflightIssues(ctx, componentName, runtime)
	}
	issues := component.RuntimePreflight(ctx, transport, dialect)
	for index := range issues {
		if issues[index].Component == "" {
			issues[index].Component = componentName
		}
	}
	return issues
}

func (cfg ExecutionComponentConfig) validate(role string) error {
	if cfg.Type == "" {
		return fmt.Errorf("%s type is required", role)
	}
	return nil
}

func newTransportFromConfig(cfg ExecutionComponentConfig) (Transport, error) {
	switch cfg.Type {
	case "local":
		return LocalTransport{}, nil
	case "docker":
		container, err := cfg.Settings.String("container")
		if err != nil {
			return nil, err
		}
		if container == "" {
			return nil, fmt.Errorf("docker transport setting container is required")
		}
		executable, err := cfg.Settings.String("executable")
		if err != nil {
			return nil, err
		}
		return DockerContainerTransport{
			Docker: DockerTransport{
				Executable: executable,
			},
			Container: container,
		}, nil
	case "ssh":
		sshConfig, err := sshTransportConfigFromSettings(cfg.Settings)
		if err != nil {
			return nil, err
		}
		return &SSHTransport{Config: sshConfig}, nil
	default:
		return nil, fmt.Errorf("unsupported transport type %q", cfg.Type)
	}
}

func sshTransportConfigFromSettings(settings ExecutionComponentSettings) (SSHTransportConfig, error) {
	host, err := settings.String("host")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	user, err := settings.String("user")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	identityFile, err := settings.String("identity_file")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	identityEnv, err := settings.String("identity_env")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	knownHostsFile, err := settings.String("known_hosts_file")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	hostKeyPolicy, err := settings.String("host_key_policy")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	pinnedHostKey, err := settings.String("pinned_host_key")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	connectTimeout, err := settings.String("connect_timeout")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	commandTimeout, err := settings.String("command_timeout")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	jumpHosts, err := sshJumpHostConfigsFromSettings(settings)
	if err != nil {
		return SSHTransportConfig{}, err
	}
	cfg := SSHTransportConfig{
		Host:           host,
		User:           user,
		IdentityFile:   identityFile,
		IdentityEnv:    identityEnv,
		KnownHostsFile: knownHostsFile,
		HostKeyPolicy:  hostKeyPolicy,
		PinnedHostKey:  pinnedHostKey,
		ConnectTimeout: connectTimeout,
		CommandTimeout: commandTimeout,
		JumpHosts:      jumpHosts,
	}
	port, err := settings.String("port")
	if err != nil {
		return SSHTransportConfig{}, err
	}
	if port != "" {
		parsed, err := strconv.Atoi(port)
		if err != nil {
			return SSHTransportConfig{}, fmt.Errorf("ssh transport setting port must be an integer: %w", err)
		}
		cfg.Port = parsed
	}
	if err := cfg.Validate(); err != nil {
		return SSHTransportConfig{}, err
	}
	return cfg, nil
}

func sshJumpHostConfigsFromSettings(settings ExecutionComponentSettings) ([]SSHJumpHostConfig, error) {
	jumpHostSettings, err := settings.ObjectList("jump_hosts")
	if err != nil {
		return nil, err
	}
	jumpHosts := make([]SSHJumpHostConfig, 0, len(jumpHostSettings))
	for index, item := range jumpHostSettings {
		host, err := item.String("host")
		if err != nil {
			return nil, fmt.Errorf("jump_hosts[%d]: %w", index, err)
		}
		user, err := item.String("user")
		if err != nil {
			return nil, fmt.Errorf("jump_hosts[%d]: %w", index, err)
		}
		identityFile, err := item.String("identity_file")
		if err != nil {
			return nil, fmt.Errorf("jump_hosts[%d]: %w", index, err)
		}
		identityEnv, err := item.String("identity_env")
		if err != nil {
			return nil, fmt.Errorf("jump_hosts[%d]: %w", index, err)
		}
		knownHostsFile, err := item.String("known_hosts_file")
		if err != nil {
			return nil, fmt.Errorf("jump_hosts[%d]: %w", index, err)
		}
		hostKeyPolicy, err := item.String("host_key_policy")
		if err != nil {
			return nil, fmt.Errorf("jump_hosts[%d]: %w", index, err)
		}
		pinnedHostKey, err := item.String("pinned_host_key")
		if err != nil {
			return nil, fmt.Errorf("jump_hosts[%d]: %w", index, err)
		}
		jumpHost := SSHJumpHostConfig{
			Host:           host,
			User:           user,
			IdentityFile:   identityFile,
			IdentityEnv:    identityEnv,
			KnownHostsFile: knownHostsFile,
			HostKeyPolicy:  hostKeyPolicy,
			PinnedHostKey:  pinnedHostKey,
		}
		port, err := item.String("port")
		if err != nil {
			return nil, fmt.Errorf("jump_hosts[%d]: %w", index, err)
		}
		if port != "" {
			parsed, err := strconv.Atoi(port)
			if err != nil {
				return nil, fmt.Errorf("jump_hosts[%d]: ssh transport setting port must be an integer: %w", index, err)
			}
			jumpHost.Port = parsed
		}
		jumpHosts = append(jumpHosts, jumpHost)
	}
	return jumpHosts, nil
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
	case "direct_process":
		return DirectProcessScheduler{}, nil
	case "remote_process":
		return RemoteProcessScheduler{Transport: transport}, nil
	case "slurm":
		memoryMB, err := cfg.Settings.Int64("memory_mb")
		if err != nil {
			return nil, err
		}
		timeLimit, err := cfg.Settings.String("time_limit")
		if err != nil {
			return nil, err
		}
		return SlurmScheduler{Transport: transport, MemoryMB: memoryMB, TimeLimit: timeLimit}, nil
	default:
		return nil, fmt.Errorf("unsupported scheduler type %q", cfg.Type)
	}
}

func newRuntimeFromConfig(cfg ExecutionComponentConfig) (Runtime, error) {
	workerRuntime, err := workerRuntimeFromSettings(cfg.Settings)
	if err != nil {
		return nil, err
	}
	switch cfg.Type {
	case "worker":
		return workerRuntime, nil
	case "singularity_worker":
		singularityExecutable, err := cfg.Settings.String("singularity_executable")
		if err != nil {
			return nil, err
		}
		imagePath, err := cfg.Settings.String("image_path")
		if err != nil {
			return nil, err
		}
		containerWorkerExecutable, err := cfg.Settings.String("container_worker_executable")
		if err != nil {
			return nil, err
		}
		bind, err := cfg.Settings.String("bind")
		if err != nil {
			return nil, err
		}
		return SingularityWorkerRuntime{
			WorkerRuntime:             workerRuntime,
			SingularityExecutable:     singularityExecutable,
			ImagePath:                 imagePath,
			ContainerWorkerExecutable: containerWorkerExecutable,
			Bind:                      bind,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime type %q", cfg.Type)
	}
}

func workerRuntimeFromSettings(settings ExecutionComponentSettings) (WorkerRuntime, error) {
	root, err := settings.String("root")
	if err != nil {
		return WorkerRuntime{}, err
	}
	controllerURL, err := settings.String("controller_url")
	if err != nil {
		return WorkerRuntime{}, err
	}
	controllerTokenFile, err := settings.String("controller_token_file")
	if err != nil {
		return WorkerRuntime{}, err
	}
	controllerInsecureExternalHTTPAllowed, err := settings.Bool("controller_insecure_external_http_allowed")
	if err != nil {
		return WorkerRuntime{}, err
	}
	localWorkerArtifact, err := settings.String("local_worker_artifact")
	if err != nil {
		return WorkerRuntime{}, err
	}
	dataDir, err := settings.String("data_dir")
	if err != nil {
		return WorkerRuntime{}, err
	}
	assetCacheDir, err := settings.String("asset_cache_dir")
	if err != nil {
		return WorkerRuntime{}, err
	}
	pythonExecutable, err := settings.String("python_executable")
	if err != nil {
		return WorkerRuntime{}, err
	}
	sevenZipExecutable, err := settings.String("seven_zip_executable")
	if err != nil {
		return WorkerRuntime{}, err
	}
	rcloneExecutable, err := settings.String("rclone_executable")
	if err != nil {
		return WorkerRuntime{}, err
	}
	rcloneConfigPath, err := settings.String("rclone_config_path")
	if err != nil {
		return WorkerRuntime{}, err
	}
	enableGDriveRcloneProvider, err := settings.Bool("enable_gdrive_rclone_provider")
	if err != nil {
		return WorkerRuntime{}, err
	}
	maxAssetBytes, err := settings.Int64("max_asset_bytes")
	if err != nil {
		return WorkerRuntime{}, err
	}
	idlePollIntervalSeconds, err := nonNegativeIntSetting(settings, "idle_poll_interval_seconds")
	if err != nil {
		return WorkerRuntime{}, err
	}
	idleTimeoutSeconds, err := nonNegativeIntSetting(settings, "idle_timeout_seconds")
	if err != nil {
		return WorkerRuntime{}, err
	}
	roots, err := settings.StringMap("data_location_roots")
	if err != nil {
		return WorkerRuntime{}, err
	}
	return WorkerRuntime{
		Root:                                  root,
		ControllerURL:                         controllerURL,
		ControllerTokenFile:                   controllerTokenFile,
		ControllerInsecureExternalHTTPAllowed: controllerInsecureExternalHTTPAllowed,
		LocalWorkerArtifact:                   localWorkerArtifact,
		DataDir:                               dataDir,
		AssetCacheDir:                         assetCacheDir,
		PythonExecutable:                      pythonExecutable,
		SevenZipExecutable:                    sevenZipExecutable,
		RcloneExecutable:                      rcloneExecutable,
		RcloneConfigPath:                      rcloneConfigPath,
		EnableGDriveRcloneProvider:            enableGDriveRcloneProvider,
		MaxAssetBytes:                         maxAssetBytes,
		DataLocationRoots:                     roots,
		IdlePollIntervalSeconds:               idlePollIntervalSeconds,
		IdleTimeoutSeconds:                    idleTimeoutSeconds,
	}, nil
}

func nonNegativeIntSetting(settings ExecutionComponentSettings, name string) (int, error) {
	value, err := settings.Int64(name)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("setting %s must be non-negative", name)
	}
	maxInt := int64(int(^uint(0) >> 1))
	if value > maxInt {
		return 0, fmt.Errorf("setting %s is too large", name)
	}
	return int(value), nil
}

func (settings ExecutionComponentSettings) String(name string) (string, error) {
	if len(settings) == 0 {
		return "", nil
	}
	value, ok := settings[name]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("setting %s must be a string", name)
	}
	return text, nil
}

func (settings ExecutionComponentSettings) StringMap(name string) (map[string]string, error) {
	if len(settings) == 0 {
		return nil, nil
	}
	value, ok := settings[name]
	if !ok || value == nil {
		return nil, nil
	}
	typed, ok := value.(map[string]any)
	if !ok {
		if stringMap, ok := value.(map[string]string); ok {
			copied := make(map[string]string, len(stringMap))
			for key, child := range stringMap {
				copied[key] = child
			}
			return copied, nil
		}
		return nil, fmt.Errorf("setting %s must be an object with string values", name)
	}
	result := make(map[string]string, len(typed))
	for key, child := range typed {
		text, ok := child.(string)
		if !ok {
			return nil, fmt.Errorf("setting %s.%s must be a string", name, key)
		}
		result[key] = text
	}
	return result, nil
}

func (settings ExecutionComponentSettings) ObjectList(name string) ([]ExecutionComponentSettings, error) {
	if len(settings) == 0 {
		return nil, nil
	}
	value, ok := settings[name]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []ExecutionComponentSettings:
		return typed, nil
	case []map[string]any:
		result := make([]ExecutionComponentSettings, 0, len(typed))
		for _, item := range typed {
			result = append(result, ExecutionComponentSettings(item))
		}
		return result, nil
	case []any:
		result := make([]ExecutionComponentSettings, 0, len(typed))
		for index, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("setting %s[%d] must be an object", name, index)
			}
			result = append(result, ExecutionComponentSettings(object))
		}
		return result, nil
	default:
		return nil, fmt.Errorf("setting %s must be a list of objects", name)
	}
}

func (settings ExecutionComponentSettings) Int64(name string) (int64, error) {
	if len(settings) == 0 {
		return 0, nil
	}
	value, ok := settings[name]
	if !ok || value == nil {
		return 0, nil
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	case float64:
		asInt := int64(typed)
		if typed != float64(asInt) {
			return 0, fmt.Errorf("setting %s must be an integer", name)
		}
		return asInt, nil
	default:
		return 0, fmt.Errorf("setting %s must be an integer", name)
	}
}

func (settings ExecutionComponentSettings) Bool(name string) (bool, error) {
	if len(settings) == 0 {
		return false, nil
	}
	value, ok := settings[name]
	if !ok || value == nil {
		return false, nil
	}
	typed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("setting %s must be a boolean", name)
	}
	return typed, nil
}

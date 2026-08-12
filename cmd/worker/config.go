package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultWorkerIdlePollInterval = 30 * time.Second

type CheckpointMode string

type PauseAdapterProfile string

const (
	CheckpointModeDisabled CheckpointMode = ""
	CheckpointModeShutdown CheckpointMode = "shutdown"
	CheckpointModePeriodic CheckpointMode = "periodic"
	CheckpointModeYield    CheckpointMode = "yield"

	PauseAdapterProfileDirectPythonDMTCP42 PauseAdapterProfile = "direct_python_dmtcp_4_2"
)

type DMTCPProfileConfig struct {
	LaunchExecutable               string `json:"launch_executable"`
	CommandExecutable              string `json:"command_executable"`
	RestartExecutable              string `json:"restart_executable"`
	PythonExecutable               string `json:"python_executable"`
	SharedTmpRoot                  string `json:"shared_tmp_root"`
	CheckpointSignal               string `json:"checkpoint_signal,omitempty"`
	ExpectedClients                int    `json:"expected_clients"`
	BuildIdentity                  string `json:"build_identity"`
	AdapterID                      string `json:"adapter_id"`
	AdapterVersion                 string `json:"adapter_version"`
	WorkerExecutionContractVersion string `json:"worker_execution_contract_version"`
	WorkerVersion                  string `json:"worker_version"`
	ContainerImageIdentity         string `json:"container_image_identity"`
	OperatingSystem                string `json:"operating_system"`
	Architecture                   string `json:"architecture"`
	ContainerRuntime               string `json:"container_runtime"`
}

func (profile DMTCPProfileConfig) validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "launch_executable", value: profile.LaunchExecutable},
		{name: "command_executable", value: profile.CommandExecutable},
		{name: "restart_executable", value: profile.RestartExecutable},
		{name: "python_executable", value: profile.PythonExecutable},
		{name: "shared_tmp_root", value: profile.SharedTmpRoot},
		{name: "build_identity", value: profile.BuildIdentity},
		{name: "adapter_id", value: profile.AdapterID},
		{name: "adapter_version", value: profile.AdapterVersion},
		{name: "worker_execution_contract_version", value: profile.WorkerExecutionContractVersion},
		{name: "worker_version", value: profile.WorkerVersion},
		{name: "container_image_identity", value: profile.ContainerImageIdentity},
		{name: "operating_system", value: profile.OperatingSystem},
		{name: "architecture", value: profile.Architecture},
		{name: "container_runtime", value: profile.ContainerRuntime},
	} {
		if err := validateAdapterContractValue("dmtcp_profile."+field.name, field.value); err != nil {
			return err
		}
	}
	if profile.CheckpointSignal != "" && profile.CheckpointSignal != "12" {
		return fmt.Errorf("dmtcp_profile.checkpoint_signal must be 12 when set")
	}
	if profile.ExpectedClients < 1 {
		return fmt.Errorf("dmtcp_profile.expected_clients must be at least 1")
	}
	return nil
}

func (profile DMTCPProfileConfig) launchProfile() DMTCPLaunchProfile {
	return DMTCPLaunchProfile{
		LaunchExecutable:               profile.LaunchExecutable,
		CommandExecutable:              profile.CommandExecutable,
		RestartExecutable:              profile.RestartExecutable,
		PythonExecutable:               profile.PythonExecutable,
		SharedTmpRoot:                  profile.SharedTmpRoot,
		CheckpointSignal:               profile.CheckpointSignal,
		ExpectedClients:                profile.ExpectedClients,
		BuildIdentity:                  profile.BuildIdentity,
		AdapterID:                      profile.AdapterID,
		AdapterVersion:                 profile.AdapterVersion,
		WorkerExecutionContractVersion: profile.WorkerExecutionContractVersion,
		WorkerVersion:                  profile.WorkerVersion,
		ContainerImageIdentity:         profile.ContainerImageIdentity,
		OperatingSystem:                profile.OperatingSystem,
		Architecture:                   profile.Architecture,
		ContainerRuntime:               profile.ContainerRuntime,
	}
}

type Config struct {
	LogDir                                string              `json:"log_dir"`
	TmpDir                                string              `json:"tmp_dir"`
	DataDir                               string              `json:"data_dir"`
	ControllerURL                         string              `json:"controller_url"`
	ControllerTokenFile                   string              `json:"controller_token_file,omitempty"`
	ControllerInsecureExternalHTTPAllowed bool                `json:"controller_insecure_external_http_allowed,omitempty"`
	PythonExecutable                      string              `json:"python_executable,omitempty"`
	SevenZipExecutable                    string              `json:"seven_zip_executable,omitempty"`
	RcloneExecutable                      string              `json:"rclone_executable,omitempty"`
	RcloneConfigPath                      string              `json:"rclone_config_path,omitempty"`
	EnableGDriveRcloneProvider            bool                `json:"enable_gdrive_rclone_provider,omitempty"`
	AssetCacheDir                         string              `json:"asset_cache_dir,omitempty"`
	MaxAssetBytes                         int64               `json:"max_asset_bytes,omitempty"`
	DataLocationRoots                     map[string]string   `json:"data_location_roots,omitempty"`
	IdlePollIntervalSeconds               int                 `json:"idle_poll_interval_seconds,omitempty"`
	IdleTimeoutSeconds                    int                 `json:"idle_timeout_seconds,omitempty"`
	CheckpointMode                        CheckpointMode      `json:"checkpoint_mode,omitempty"`
	CheckpointIntervalSeconds             int                 `json:"checkpoint_interval_seconds,omitempty"`
	WorkItemExecutionQuantumSeconds       int                 `json:"work_item_execution_quantum_seconds,omitempty"`
	DrainPauseDelaySeconds                int                 `json:"drain_pause_delay_seconds,omitempty"`
	CheckpointCaptureTimeoutSeconds       int                 `json:"checkpoint_capture_timeout_seconds,omitempty"`
	CheckpointReportTimeoutSeconds        int                 `json:"checkpoint_report_timeout_seconds,omitempty"`
	ExecutionTerminationGraceSeconds      int                 `json:"execution_termination_grace_seconds,omitempty"`
	PauseAdapterProfile                   PauseAdapterProfile `json:"pause_adapter_profile,omitempty"`
	DMTCPProfile                          *DMTCPProfileConfig `json:"dmtcp_profile,omitempty"`
}

func loadConfig(path string) (Config, error) {
	return loadConfigWithValidation(path, Config.Validate)
}

func loadDirectConfig(path string) (Config, error) {
	return loadConfigWithValidation(path, Config.ValidateRuntime)
}

func loadConfigWithValidation(path string, validate func(Config) error) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config file %s: %w", path, err)
	}

	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config file %s: %w", path, err)
	}

	cfg.resolveRelativePaths(filepath.Dir(path))
	return cfg, nil
}

func (c *Config) resolveRelativePaths(root string) {
	c.LogDir = resolveRelativePath(root, c.LogDir)
	c.TmpDir = resolveRelativePath(root, c.TmpDir)
	c.DataDir = resolveRelativePath(root, c.DataDir)
	if c.ControllerTokenFile != "" {
		c.ControllerTokenFile = resolveRelativePath(root, c.ControllerTokenFile)
	}
	if c.AssetCacheDir != "" {
		c.AssetCacheDir = resolveRelativePath(root, c.AssetCacheDir)
	}
	if c.SevenZipExecutable != "" && pathLooksRelative(c.SevenZipExecutable) {
		c.SevenZipExecutable = resolveRelativePath(root, c.SevenZipExecutable)
	}
	if c.RcloneExecutable != "" && pathLooksRelative(c.RcloneExecutable) {
		c.RcloneExecutable = resolveRelativePath(root, c.RcloneExecutable)
	}
	if c.RcloneConfigPath != "" {
		c.RcloneConfigPath = resolveRelativePath(root, c.RcloneConfigPath)
	}
	if c.DMTCPProfile != nil {
		if pathLooksRelative(c.DMTCPProfile.LaunchExecutable) {
			c.DMTCPProfile.LaunchExecutable = resolveRelativePath(root, c.DMTCPProfile.LaunchExecutable)
		}
		if pathLooksRelative(c.DMTCPProfile.CommandExecutable) {
			c.DMTCPProfile.CommandExecutable = resolveRelativePath(root, c.DMTCPProfile.CommandExecutable)
		}
		if pathLooksRelative(c.DMTCPProfile.RestartExecutable) {
			c.DMTCPProfile.RestartExecutable = resolveRelativePath(root, c.DMTCPProfile.RestartExecutable)
		}
		if pathLooksRelative(c.DMTCPProfile.PythonExecutable) {
			c.DMTCPProfile.PythonExecutable = resolveRelativePath(root, c.DMTCPProfile.PythonExecutable)
		}
		c.DMTCPProfile.SharedTmpRoot = resolveRelativePath(root, c.DMTCPProfile.SharedTmpRoot)
	}
	for name, dataRoot := range c.DataLocationRoots {
		c.DataLocationRoots[name] = resolveRelativePath(root, dataRoot)
	}
}

func resolveRelativePath(root string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(root, path)
}

func pathLooksRelative(path string) bool {
	return !filepath.IsAbs(path) && (filepath.Dir(path) != "." || filepath.Base(path) != path)
}

func (c Config) Validate() error {
	if err := c.ValidateRuntime(); err != nil {
		return err
	}
	return c.ValidateControllerMode()
}

func (c Config) ValidateRuntime() error {
	if c.LogDir == "" {
		return fmt.Errorf("log dir is required")
	}

	if c.TmpDir == "" {
		return fmt.Errorf("tmp dir is required")
	}

	if c.DataDir == "" {
		return fmt.Errorf("data dir is required")
	}

	if c.MaxAssetBytes < 0 {
		return fmt.Errorf("max asset bytes must be non-negative")
	}

	if c.IdlePollIntervalSeconds < 0 {
		return fmt.Errorf("idle poll interval seconds must be non-negative")
	}

	if c.IdleTimeoutSeconds < 0 {
		return fmt.Errorf("idle timeout seconds must be non-negative")
	}

	if err := c.validateCheckpointPolicy(); err != nil {
		return err
	}
	if err := c.validatePauseAdapterProfile(); err != nil {
		return err
	}

	return nil
}

func (c Config) validatePauseAdapterProfile() error {
	switch c.PauseAdapterProfile {
	case "":
		if c.DMTCPProfile != nil {
			return fmt.Errorf("dmtcp_profile requires pause_adapter_profile")
		}
		return nil
	case PauseAdapterProfileDirectPythonDMTCP42:
		if c.CheckpointMode == CheckpointModeDisabled {
			return fmt.Errorf("pause_adapter_profile %q requires checkpoint_mode", c.PauseAdapterProfile)
		}
		if c.DMTCPProfile == nil {
			return fmt.Errorf("pause_adapter_profile %q requires dmtcp_profile", c.PauseAdapterProfile)
		}
		if err := c.DMTCPProfile.validate(); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported pause_adapter_profile %q", c.PauseAdapterProfile)
	}
}

func (c Config) validateCheckpointPolicy() error {
	values := []struct {
		name  string
		value int
	}{
		{name: "checkpoint_interval_seconds", value: c.CheckpointIntervalSeconds},
		{name: "work_item_execution_quantum_seconds", value: c.WorkItemExecutionQuantumSeconds},
		{name: "drain_pause_delay_seconds", value: c.DrainPauseDelaySeconds},
		{name: "checkpoint_capture_timeout_seconds", value: c.CheckpointCaptureTimeoutSeconds},
		{name: "checkpoint_report_timeout_seconds", value: c.CheckpointReportTimeoutSeconds},
		{name: "execution_termination_grace_seconds", value: c.ExecutionTerminationGraceSeconds},
	}
	for _, field := range values {
		if field.value < 0 {
			return fmt.Errorf("%s must be non-negative", field.name)
		}
	}

	switch c.CheckpointMode {
	case CheckpointModeDisabled:
		for _, field := range values {
			if field.value != 0 {
				return fmt.Errorf("%s requires checkpoint_mode", field.name)
			}
		}
		return nil
	case CheckpointModeShutdown, CheckpointModePeriodic, CheckpointModeYield:
	default:
		return fmt.Errorf("unsupported checkpoint_mode %q", c.CheckpointMode)
	}

	required := []struct {
		name  string
		value int
	}{
		{name: "drain_pause_delay_seconds", value: c.DrainPauseDelaySeconds},
		{name: "checkpoint_capture_timeout_seconds", value: c.CheckpointCaptureTimeoutSeconds},
		{name: "checkpoint_report_timeout_seconds", value: c.CheckpointReportTimeoutSeconds},
		{name: "execution_termination_grace_seconds", value: c.ExecutionTerminationGraceSeconds},
	}
	for _, field := range required {
		if field.value == 0 {
			return fmt.Errorf("%s must be greater than zero when checkpoint_mode is %q", field.name, c.CheckpointMode)
		}
	}

	switch c.CheckpointMode {
	case CheckpointModeShutdown:
		if c.CheckpointIntervalSeconds != 0 {
			return fmt.Errorf("checkpoint_interval_seconds must be zero when checkpoint_mode is %q", c.CheckpointMode)
		}
		if c.WorkItemExecutionQuantumSeconds != 0 {
			return fmt.Errorf("work_item_execution_quantum_seconds must be zero when checkpoint_mode is %q", c.CheckpointMode)
		}
	case CheckpointModePeriodic:
		if c.CheckpointIntervalSeconds == 0 {
			return fmt.Errorf("checkpoint_interval_seconds must be greater than zero when checkpoint_mode is %q", c.CheckpointMode)
		}
		if c.WorkItemExecutionQuantumSeconds != 0 {
			return fmt.Errorf("work_item_execution_quantum_seconds must be zero when checkpoint_mode is %q", c.CheckpointMode)
		}
	case CheckpointModeYield:
		if c.CheckpointIntervalSeconds != 0 {
			return fmt.Errorf("checkpoint_interval_seconds must be zero when checkpoint_mode is %q", c.CheckpointMode)
		}
		if c.WorkItemExecutionQuantumSeconds == 0 {
			return fmt.Errorf("work_item_execution_quantum_seconds must be greater than zero when checkpoint_mode is %q", c.CheckpointMode)
		}
	}

	remaining := c.DrainPauseDelaySeconds
	if c.CheckpointCaptureTimeoutSeconds >= remaining {
		return fmt.Errorf("checkpoint capture, report, and termination budget must be less than drain_pause_delay_seconds")
	}
	remaining -= c.CheckpointCaptureTimeoutSeconds
	if c.CheckpointReportTimeoutSeconds >= remaining {
		return fmt.Errorf("checkpoint capture, report, and termination budget must be less than drain_pause_delay_seconds")
	}
	remaining -= c.CheckpointReportTimeoutSeconds
	if c.ExecutionTerminationGraceSeconds >= remaining {
		return fmt.Errorf("checkpoint capture, report, and termination budget must be less than drain_pause_delay_seconds")
	}

	return nil
}

func (c Config) ValidateControllerMode() error {
	if c.ControllerURL == "" {
		return fmt.Errorf("controller url is required")
	}
	requiresToken, err := controllerURLRequiresTokenFile(c.ControllerURL)
	if err != nil {
		return err
	}
	if requiresToken && c.ControllerTokenFile == "" {
		return fmt.Errorf("controller token file is required for controller url %s", c.ControllerURL)
	}
	return nil
}

func controllerURLRequiresTokenFile(raw string) (bool, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false, fmt.Errorf("controller url is invalid: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return false, fmt.Errorf("controller url requires a scheme and host")
	}
	switch parsed.Scheme {
	case "https":
		return true, nil
	case "http":
		return !isLoopbackHost(parsed.Hostname()), nil
	default:
		return false, fmt.Errorf("controller url scheme %q is unsupported", parsed.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Config) effectiveAssetCacheDir() string {
	if c.AssetCacheDir != "" {
		return c.AssetCacheDir
	}
	return filepath.Join(c.DataDir, "cache", "assets")
}

func (c Config) effectiveMaxAssetBytes() int64 {
	if c.MaxAssetBytes > 0 {
		return c.MaxAssetBytes
	}
	return 5 * 1024 * 1024
}

func (c Config) effectiveIdlePollInterval() time.Duration {
	if c.IdlePollIntervalSeconds > 0 {
		return time.Duration(c.IdlePollIntervalSeconds) * time.Second
	}
	return defaultWorkerIdlePollInterval
}

func (c Config) effectiveIdleTimeout() time.Duration {
	if c.IdleTimeoutSeconds > 0 {
		return time.Duration(c.IdleTimeoutSeconds) * time.Second
	}
	return 0
}

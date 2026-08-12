package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")

	content := []byte(`{
		"log_dir": "logs",
		"tmp_dir": "tmp",
		"data_dir": "data",
		"controller_url": "https://controller.local",
		"controller_token_file": "secrets/controller-worker-token",
		"controller_insecure_external_http_allowed": true,
		"python_executable": "python3",
		"seven_zip_executable": "tools/7z",
		"rclone_executable": "tools/rclone",
		"rclone_config_path": "secrets/rclone.conf",
		"enable_gdrive_rclone_provider": true,
		"asset_cache_dir": "asset-cache",
		"max_asset_bytes": 1024,
		"idle_poll_interval_seconds": 15,
		"idle_timeout_seconds": 300,
		"checkpoint_mode": "periodic",
		"checkpoint_interval_seconds": 300,
		"drain_pause_delay_seconds": 300,
		"checkpoint_capture_timeout_seconds": 120,
		"checkpoint_report_timeout_seconds": 60,
		"execution_termination_grace_seconds": 30,
		"pause_adapter_profile": "direct_python_dmtcp_4_2",
		"dmtcp_profile": {
			"launch_executable": "tools/dmtcp_launch",
			"command_executable": "tools/dmtcp_command",
			"restart_executable": "tools/dmtcp_restart",
			"python_executable": "tools/python3",
			"shared_tmp_root": "shared-checkpoints",
			"checkpoint_signal": "12",
			"expected_clients": 2,
			"build_identity": "dmtcp-4.2.0-f8009ce7-python-3.11",
			"adapter_id": "direct-interpreter-dmtcp",
			"adapter_version": "1",
			"worker_execution_contract_version": "goet/worker-execution/v1",
			"worker_version": "worker-build-001",
			"container_image_identity": "sha256:image-001",
			"operating_system": "linux",
			"architecture": "amd64",
			"container_runtime": "singularity-ce-4.1.2"
		},
		"data_location_roots": {
			"fixture": "fixtures"
		}
	}`)

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.LogDir != filepath.Join(root, "logs") {
		t.Fatalf("unexpected log dir: %q", config.LogDir)
	}

	if config.TmpDir != filepath.Join(root, "tmp") {
		t.Fatalf("unexpected tmp dir: %q", config.TmpDir)
	}

	if config.DataDir != filepath.Join(root, "data") {
		t.Fatalf("unexpected data dir: %q", config.DataDir)
	}

	if config.ControllerURL != "https://controller.local" {
		t.Fatalf("unexpected controller url: %q", config.ControllerURL)
	}

	if config.ControllerTokenFile != filepath.Join(root, "secrets", "controller-worker-token") {
		t.Fatalf("unexpected controller token file: %q", config.ControllerTokenFile)
	}

	if !config.ControllerInsecureExternalHTTPAllowed {
		t.Fatal("expected insecure external HTTP to be allowed")
	}

	if config.PythonExecutable != "python3" {
		t.Fatalf("unexpected python executable: %q", config.PythonExecutable)
	}

	if config.SevenZipExecutable != filepath.Join(root, "tools", "7z") {
		t.Fatalf("unexpected seven zip executable: %q", config.SevenZipExecutable)
	}

	if config.RcloneExecutable != filepath.Join(root, "tools", "rclone") {
		t.Fatalf("unexpected rclone executable: %q", config.RcloneExecutable)
	}

	if config.RcloneConfigPath != filepath.Join(root, "secrets", "rclone.conf") {
		t.Fatalf("unexpected rclone config path: %q", config.RcloneConfigPath)
	}

	if !config.EnableGDriveRcloneProvider {
		t.Fatal("expected gdrive rclone provider to be enabled")
	}

	if config.AssetCacheDir != filepath.Join(root, "asset-cache") {
		t.Fatalf("unexpected asset cache dir: %q", config.AssetCacheDir)
	}

	if config.MaxAssetBytes != 1024 {
		t.Fatalf("unexpected max asset bytes: %d", config.MaxAssetBytes)
	}

	if config.IdlePollIntervalSeconds != 15 {
		t.Fatalf("unexpected idle poll interval seconds: %d", config.IdlePollIntervalSeconds)
	}

	if config.IdleTimeoutSeconds != 300 {
		t.Fatalf("unexpected idle timeout seconds: %d", config.IdleTimeoutSeconds)
	}

	if config.CheckpointMode != CheckpointModePeriodic {
		t.Fatalf("unexpected checkpoint mode: %q", config.CheckpointMode)
	}

	if config.CheckpointIntervalSeconds != 300 {
		t.Fatalf("unexpected checkpoint interval seconds: %d", config.CheckpointIntervalSeconds)
	}

	if config.WorkItemExecutionQuantumSeconds != 0 {
		t.Fatalf("unexpected work-item execution quantum seconds: %d", config.WorkItemExecutionQuantumSeconds)
	}

	if config.DrainPauseDelaySeconds != 300 {
		t.Fatalf("unexpected drain pause delay seconds: %d", config.DrainPauseDelaySeconds)
	}

	if config.CheckpointCaptureTimeoutSeconds != 120 {
		t.Fatalf("unexpected checkpoint capture timeout seconds: %d", config.CheckpointCaptureTimeoutSeconds)
	}

	if config.CheckpointReportTimeoutSeconds != 60 {
		t.Fatalf("unexpected checkpoint report timeout seconds: %d", config.CheckpointReportTimeoutSeconds)
	}

	if config.ExecutionTerminationGraceSeconds != 30 {
		t.Fatalf("unexpected execution termination grace seconds: %d", config.ExecutionTerminationGraceSeconds)
	}

	if config.PauseAdapterProfile != PauseAdapterProfileDirectPythonDMTCP42 || config.DMTCPProfile == nil {
		t.Fatalf("unexpected pause adapter profile: %q %+v", config.PauseAdapterProfile, config.DMTCPProfile)
	}
	if config.DMTCPProfile.LaunchExecutable != filepath.Join(root, "tools", "dmtcp_launch") ||
		config.DMTCPProfile.CommandExecutable != filepath.Join(root, "tools", "dmtcp_command") ||
		config.DMTCPProfile.RestartExecutable != filepath.Join(root, "tools", "dmtcp_restart") ||
		config.DMTCPProfile.PythonExecutable != filepath.Join(root, "tools", "python3") ||
		config.DMTCPProfile.SharedTmpRoot != filepath.Join(root, "shared-checkpoints") {
		t.Fatalf("unexpected resolved DMTCP profile: %+v", config.DMTCPProfile)
	}
	if config.DMTCPProfile.ExpectedClients != 2 || config.DMTCPProfile.BuildIdentity != "dmtcp-4.2.0-f8009ce7-python-3.11" {
		t.Fatalf("unexpected DMTCP profile identity: %+v", config.DMTCPProfile)
	}
	if err := config.DMTCPProfile.launchProfile().validate(); err != nil {
		t.Fatalf("resolved DMTCP launch profile is invalid: %v", err)
	}

	if config.DataLocationRoots["fixture"] != filepath.Join(root, "fixtures") {
		t.Fatalf("unexpected data location root: %q", config.DataLocationRoots["fixture"])
	}
}

func TestLoadConfigRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoadConfigRejectsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")

	if err := os.WriteFile(path, []byte(`{"log_dir":`), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoadConfigRejectsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")

	content := []byte(`{
		"log_dir": "logs",
		"tmp_dir": "tmp",
		"data_dir": "data"
	}`)

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadConfig(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoadDirectConfigAllowsMissingControllerURL(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	content := []byte(`{
		"log_dir": "logs",
		"tmp_dir": "tmp",
		"data_dir": "data"
	}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	config, err := loadDirectConfig(path)
	if err != nil {
		t.Fatalf("loadDirectConfig() error = %v", err)
	}
	if config.ControllerURL != "" {
		t.Fatalf("controller URL = %q, want empty", config.ControllerURL)
	}
	if config.LogDir != filepath.Join(root, "logs") {
		t.Fatalf("log dir = %q, want %q", config.LogDir, filepath.Join(root, "logs"))
	}
}

func TestLoadConfigStillRequiresControllerURL(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	content := []byte(`{
		"log_dir": "logs",
		"tmp_dir": "tmp",
		"data_dir": "data"
	}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadConfig(path); err == nil {
		t.Fatal("loadConfig() expected missing controller URL error")
	}
}

func TestConfigValidate(t *testing.T) {
	valid := Config{
		LogDir:              "logs",
		TmpDir:              "tmp",
		DataDir:             "data",
		ControllerURL:       "https://controller.local",
		ControllerTokenFile: "secrets/controller-worker-token",
	}

	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{name: "valid config", config: valid},
		{name: "missing log dir", config: Config{
			TmpDir: "tmp", DataDir: "data", ControllerURL: "url",
		}, wantErr: true},
		{name: "missing tmp dir", config: Config{
			LogDir: "logs", DataDir: "data", ControllerURL: "url",
		}, wantErr: true},
		{name: "missing data dir", config: Config{
			LogDir: "logs", TmpDir: "tmp", ControllerURL: "url",
		}, wantErr: true},
		{name: "missing controller url", config: Config{
			LogDir: "logs", TmpDir: "tmp", DataDir: "data",
		}, wantErr: true},
		{name: "external controller url without token file", config: Config{
			LogDir: "logs", TmpDir: "tmp", DataDir: "data", ControllerURL: "https://controller.local",
		}, wantErr: true},
		{name: "loopback controller url without token file", config: Config{
			LogDir: "logs", TmpDir: "tmp", DataDir: "data", ControllerURL: "http://localhost:8080",
		}},
		{name: "negative max asset bytes", config: Config{
			LogDir: "logs", TmpDir: "tmp", DataDir: "data", ControllerURL: "http://localhost:8080", MaxAssetBytes: -1,
		}, wantErr: true},
		{name: "negative idle poll interval", config: Config{
			LogDir: "logs", TmpDir: "tmp", DataDir: "data", ControllerURL: "http://localhost:8080", IdlePollIntervalSeconds: -1,
		}, wantErr: true},
		{name: "negative idle timeout", config: Config{
			LogDir: "logs", TmpDir: "tmp", DataDir: "data", ControllerURL: "http://localhost:8080", IdleTimeoutSeconds: -1,
		}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()

			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfigValidateCheckpointPolicy(t *testing.T) {
	base := Config{
		LogDir:              "logs",
		TmpDir:              "tmp",
		DataDir:             "data",
		ControllerURL:       "https://controller.local",
		ControllerTokenFile: "secrets/controller-worker-token",
	}
	enabled := func(mode CheckpointMode) Config {
		config := base
		config.CheckpointMode = mode
		config.DrainPauseDelaySeconds = 300
		config.CheckpointCaptureTimeoutSeconds = 120
		config.CheckpointReportTimeoutSeconds = 60
		config.ExecutionTerminationGraceSeconds = 30
		return config
	}
	periodic := enabled(CheckpointModePeriodic)
	periodic.CheckpointIntervalSeconds = 300
	yield := enabled(CheckpointModeYield)
	yield.WorkItemExecutionQuantumSeconds = 1800

	tests := []struct {
		name    string
		config  Config
		wantErr string
	}{
		{name: "disabled", config: base},
		{name: "shutdown", config: enabled(CheckpointModeShutdown)},
		{name: "periodic", config: periodic},
		{name: "yield", config: yield},
		{name: "unsupported mode", config: func() Config {
			config := enabled("future")
			return config
		}(), wantErr: "unsupported checkpoint_mode"},
		{name: "disabled with policy field", config: func() Config {
			config := base
			config.DrainPauseDelaySeconds = 300
			return config
		}(), wantErr: "requires checkpoint_mode"},
		{name: "negative policy field", config: func() Config {
			config := base
			config.CheckpointIntervalSeconds = -1
			return config
		}(), wantErr: "checkpoint_interval_seconds must be non-negative"},
		{name: "missing drain delay", config: func() Config {
			config := enabled(CheckpointModeShutdown)
			config.DrainPauseDelaySeconds = 0
			return config
		}(), wantErr: "drain_pause_delay_seconds must be greater than zero"},
		{name: "missing capture timeout", config: func() Config {
			config := enabled(CheckpointModeShutdown)
			config.CheckpointCaptureTimeoutSeconds = 0
			return config
		}(), wantErr: "checkpoint_capture_timeout_seconds must be greater than zero"},
		{name: "missing report timeout", config: func() Config {
			config := enabled(CheckpointModeShutdown)
			config.CheckpointReportTimeoutSeconds = 0
			return config
		}(), wantErr: "checkpoint_report_timeout_seconds must be greater than zero"},
		{name: "missing termination grace", config: func() Config {
			config := enabled(CheckpointModeShutdown)
			config.ExecutionTerminationGraceSeconds = 0
			return config
		}(), wantErr: "execution_termination_grace_seconds must be greater than zero"},
		{name: "shutdown with interval", config: func() Config {
			config := enabled(CheckpointModeShutdown)
			config.CheckpointIntervalSeconds = 1
			return config
		}(), wantErr: "checkpoint_interval_seconds must be zero"},
		{name: "shutdown with quantum", config: func() Config {
			config := enabled(CheckpointModeShutdown)
			config.WorkItemExecutionQuantumSeconds = 1
			return config
		}(), wantErr: "work_item_execution_quantum_seconds must be zero"},
		{name: "periodic without interval", config: enabled(CheckpointModePeriodic), wantErr: "checkpoint_interval_seconds must be greater than zero"},
		{name: "periodic with quantum", config: func() Config {
			config := periodic
			config.WorkItemExecutionQuantumSeconds = 1
			return config
		}(), wantErr: "work_item_execution_quantum_seconds must be zero"},
		{name: "yield without quantum", config: enabled(CheckpointModeYield), wantErr: "work_item_execution_quantum_seconds must be greater than zero"},
		{name: "yield with interval", config: func() Config {
			config := yield
			config.CheckpointIntervalSeconds = 1
			return config
		}(), wantErr: "checkpoint_interval_seconds must be zero"},
		{name: "budget equals delay", config: func() Config {
			config := enabled(CheckpointModeShutdown)
			config.DrainPauseDelaySeconds = 210
			return config
		}(), wantErr: "budget must be less than drain_pause_delay_seconds"},
		{name: "budget exceeds delay", config: func() Config {
			config := enabled(CheckpointModeShutdown)
			config.DrainPauseDelaySeconds = 200
			return config
		}(), wantErr: "budget must be less than drain_pause_delay_seconds"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want text %q", err, test.wantErr)
			}
		})
	}
}

func TestConfigValidatePauseAdapterProfile(t *testing.T) {
	profile := DMTCPProfileConfig{
		LaunchExecutable:               "dmtcp_launch",
		CommandExecutable:              "dmtcp_command",
		RestartExecutable:              "dmtcp_restart",
		PythonExecutable:               "python3",
		SharedTmpRoot:                  "shared-checkpoints",
		CheckpointSignal:               "12",
		ExpectedClients:                2,
		BuildIdentity:                  "dmtcp-4.2.0-f8009ce7-python-3.11",
		AdapterID:                      "direct-interpreter-dmtcp",
		AdapterVersion:                 "1",
		WorkerExecutionContractVersion: "goet/worker-execution/v1",
		WorkerVersion:                  "worker-build-001",
		ContainerImageIdentity:         "sha256:image-001",
		OperatingSystem:                "linux",
		Architecture:                   "amd64",
		ContainerRuntime:               "singularity-ce-4.1.2",
	}
	periodic := Config{
		LogDir: "logs", TmpDir: "tmp", DataDir: "data",
		CheckpointMode: CheckpointModePeriodic, CheckpointIntervalSeconds: 300,
		DrainPauseDelaySeconds: 300, CheckpointCaptureTimeoutSeconds: 120,
		CheckpointReportTimeoutSeconds: 60, ExecutionTerminationGraceSeconds: 30,
	}
	selected := periodic
	selected.PauseAdapterProfile = PauseAdapterProfileDirectPythonDMTCP42
	selected.DMTCPProfile = &profile

	tests := []struct {
		name    string
		config  Config
		mutate  func(*Config)
		wantErr string
	}{
		{name: "omitted", config: periodic},
		{name: "selected", config: selected},
		{name: "profile without selector", config: periodic, mutate: func(config *Config) { config.DMTCPProfile = &profile }, wantErr: "requires pause_adapter_profile"},
		{name: "selector without profile", config: periodic, mutate: func(config *Config) { config.PauseAdapterProfile = PauseAdapterProfileDirectPythonDMTCP42 }, wantErr: "requires dmtcp_profile"},
		{name: "selector without checkpoint mode", config: Config{LogDir: "logs", TmpDir: "tmp", DataDir: "data"}, mutate: func(config *Config) {
			config.PauseAdapterProfile = PauseAdapterProfileDirectPythonDMTCP42
			config.DMTCPProfile = &profile
		}, wantErr: "requires checkpoint_mode"},
		{name: "unsupported selector", config: periodic, mutate: func(config *Config) { config.PauseAdapterProfile = "future" }, wantErr: "unsupported pause_adapter_profile"},
		{name: "missing executable", config: selected, mutate: func(config *Config) {
			copy := *config.DMTCPProfile
			copy.RestartExecutable = ""
			config.DMTCPProfile = &copy
		}, wantErr: "dmtcp_profile.restart_executable is required"},
		{name: "invalid client count", config: selected, mutate: func(config *Config) {
			copy := *config.DMTCPProfile
			copy.ExpectedClients = 0
			config.DMTCPProfile = &copy
		}, wantErr: "expected_clients must be at least 1"},
		{name: "unsupported signal", config: selected, mutate: func(config *Config) {
			copy := *config.DMTCPProfile
			copy.CheckpointSignal = "10"
			config.DMTCPProfile = &copy
		}, wantErr: "checkpoint_signal must be 12"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := test.config
			if test.mutate != nil {
				test.mutate(&config)
			}
			err := config.ValidateRuntime()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateRuntime() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateRuntime() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

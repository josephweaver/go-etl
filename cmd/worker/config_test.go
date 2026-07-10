package main

import (
	"os"
	"path/filepath"
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
		"python_executable": "python3",
		"seven_zip_executable": "tools/7z",
		"rclone_executable": "tools/rclone",
		"rclone_config_path": "secrets/rclone.conf",
		"enable_gdrive_rclone_provider": true,
		"asset_cache_dir": "asset-cache",
		"max_asset_bytes": 1024,
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

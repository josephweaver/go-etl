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

type Config struct {
	LogDir                                string            `json:"log_dir"`
	TmpDir                                string            `json:"tmp_dir"`
	DataDir                               string            `json:"data_dir"`
	ControllerURL                         string            `json:"controller_url"`
	ControllerTokenFile                   string            `json:"controller_token_file,omitempty"`
	ControllerInsecureExternalHTTPAllowed bool              `json:"controller_insecure_external_http_allowed,omitempty"`
	PythonExecutable                      string            `json:"python_executable,omitempty"`
	SevenZipExecutable                    string            `json:"seven_zip_executable,omitempty"`
	RcloneExecutable                      string            `json:"rclone_executable,omitempty"`
	RcloneConfigPath                      string            `json:"rclone_config_path,omitempty"`
	EnableGDriveRcloneProvider            bool              `json:"enable_gdrive_rclone_provider,omitempty"`
	AssetCacheDir                         string            `json:"asset_cache_dir,omitempty"`
	MaxAssetBytes                         int64             `json:"max_asset_bytes,omitempty"`
	DataLocationRoots                     map[string]string `json:"data_location_roots,omitempty"`
	IdlePollIntervalSeconds               int               `json:"idle_poll_interval_seconds,omitempty"`
	IdleTimeoutSeconds                    int               `json:"idle_timeout_seconds,omitempty"`
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

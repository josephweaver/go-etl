package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
)

type Runtime interface {
	Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error
}

type WorkerScriptRuntime interface {
	Runtime
	WorkerScript(cfg SlurmWorkerScriptConfig) (SlurmWorkerScriptConfig, error)
}

type WorkerRuntime struct {
	Root                string
	ControllerURL       string
	ControllerTokenFile string
	LocalWorkerArtifact string
	DataDir             string
	AssetCacheDir       string
	PythonExecutable    string
	MaxAssetBytes       int64
	DataLocationRoots   map[string]string
}

func (r WorkerRuntime) Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error {
	if transport == nil {
		return fmt.Errorf("runtime transport is required")
	}
	if dialect == nil {
		return fmt.Errorf("runtime shell dialect is required")
	}

	paths, err := r.paths()
	if err != nil {
		return err
	}
	if err := r.validateControllerTokenFileConfig(); err != nil {
		return err
	}

	dirs := r.runtimeDirs(paths)
	if err := r.createRuntimeDirs(ctx, transport, dialect, dirs); err != nil {
		return err
	}

	if r.ControllerURL != "" {
		if err := r.writeWorkerConfig(ctx, transport, paths); err != nil {
			return err
		}
	}
	if r.LocalWorkerArtifact != "" {
		if err := r.uploadWorkerArtifact(ctx, transport, dialect, paths); err != nil {
			return err
		}
	}

	return nil
}

func (r WorkerRuntime) runtimeDirs(paths WorkerRuntimePaths) []string {
	dirs := []string{
		path.Dir(paths.WorkerExecutable),
		path.Dir(paths.WorkerConfigPath),
		path.Dir(paths.WorkerScriptPath),
		paths.LogDir,
		paths.TmpDir,
		paths.DataDir,
	}
	if r.AssetCacheDir != "" {
		dirs = append(dirs, r.AssetCacheDir)
	}
	for _, root := range r.DataLocationRoots {
		dirs = append(dirs, root)
	}
	return dirs
}

func (r WorkerRuntime) createRuntimeDirs(ctx context.Context, transport Transport, dialect ShellDialect, dirs []string) error {
	if _, ok := transport.(LocalTransport); ok {
		for _, dir := range dirs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
				return fmt.Errorf("create local runtime dir %s: %w", dir, err)
			}
		}
		return nil
	}

	localizedDirs := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		localized, err := dialect.LocalizePath(dir)
		if err != nil {
			return fmt.Errorf("runtime dir %q: %w", dir, err)
		}
		localizedDirs = append(localizedDirs, localized)
	}
	if _, err := transport.Exec(ctx, append([]string{"mkdir", "-p"}, localizedDirs...)...); err != nil {
		return fmt.Errorf("create runtime dirs: %w", err)
	}
	return nil
}

type WorkerRuntimePaths struct {
	Root             string
	WorkerExecutable string
	WorkerConfigPath string
	WorkerScriptPath string
	LogDir           string
	TmpDir           string
	DataDir          string
}

type WorkerConfig struct {
	LogDir              string            `json:"log_dir"`
	TmpDir              string            `json:"tmp_dir"`
	DataDir             string            `json:"data_dir"`
	ControllerURL       string            `json:"controller_url"`
	ControllerTokenFile string            `json:"controller_token_file,omitempty"`
	AssetCacheDir       string            `json:"asset_cache_dir,omitempty"`
	PythonExecutable    string            `json:"python_executable,omitempty"`
	MaxAssetBytes       int64             `json:"max_asset_bytes,omitempty"`
	DataLocationRoots   map[string]string `json:"data_location_roots,omitempty"`
}

func (r WorkerRuntime) paths() (WorkerRuntimePaths, error) {
	root := strings.TrimRight(r.Root, "/")
	if root == "" {
		root = "/data/goetl"
	}
	if containsNewline(root) {
		return WorkerRuntimePaths{}, fmt.Errorf("runtime root must not contain newlines")
	}
	dataDir := r.DataDir
	if dataDir == "" {
		dataDir = path.Join(root, "data")
	}
	if containsNewline(dataDir) {
		return WorkerRuntimePaths{}, fmt.Errorf("runtime data dir must not contain newlines")
	}

	return WorkerRuntimePaths{
		Root:             root,
		WorkerExecutable: path.Join(root, "artifacts", "goetl-worker"),
		WorkerConfigPath: path.Join(root, "config", "worker.json"),
		WorkerScriptPath: path.Join(root, "scripts", "worker.slurm"),
		LogDir:           path.Join(root, "logs"),
		TmpDir:           path.Join(root, "tmp"),
		DataDir:          dataDir,
	}, nil
}

func (r WorkerRuntime) writeWorkerConfig(ctx context.Context, transport Transport, paths WorkerRuntimePaths) error {
	if err := r.validateControllerTokenFileConfig(); err != nil {
		return err
	}
	if containsNewline(r.AssetCacheDir) {
		return fmt.Errorf("worker asset cache dir must not contain newlines")
	}
	for name, root := range r.DataLocationRoots {
		if containsNewline(name) || containsNewline(root) {
			return fmt.Errorf("worker data location roots must not contain newlines")
		}
	}
	cfg := WorkerConfig{
		LogDir:              paths.LogDir,
		TmpDir:              paths.TmpDir,
		DataDir:             paths.DataDir,
		ControllerURL:       r.ControllerURL,
		ControllerTokenFile: r.ControllerTokenFile,
		AssetCacheDir:       r.AssetCacheDir,
		PythonExecutable:    r.PythonExecutable,
		MaxAssetBytes:       r.MaxAssetBytes,
		DataLocationRoots:   r.DataLocationRoots,
	}
	if _, ok := transport.(LocalTransport); ok {
		var err error
		cfg, err = absoluteLocalWorkerConfig(cfg)
		if err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	file, err := os.CreateTemp("", "goetl-worker-*.json")
	if err != nil {
		return fmt.Errorf("create temp worker config: %w", err)
	}
	localPath := file.Name()
	defer os.Remove(localPath)

	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temp worker config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp worker config: %w", err)
	}

	if err := transport.Copy(ctx, localPath, paths.WorkerConfigPath); err != nil {
		return fmt.Errorf("copy worker config: %w", err)
	}
	return nil
}

func absoluteLocalWorkerConfig(cfg WorkerConfig) (WorkerConfig, error) {
	var err error
	if cfg.LogDir, err = absoluteLocalPath(cfg.LogDir); err != nil {
		return WorkerConfig{}, fmt.Errorf("worker log dir: %w", err)
	}
	if cfg.TmpDir, err = absoluteLocalPath(cfg.TmpDir); err != nil {
		return WorkerConfig{}, fmt.Errorf("worker tmp dir: %w", err)
	}
	if cfg.DataDir, err = absoluteLocalPath(cfg.DataDir); err != nil {
		return WorkerConfig{}, fmt.Errorf("worker data dir: %w", err)
	}
	if cfg.ControllerTokenFile != "" {
		if cfg.ControllerTokenFile, err = absoluteLocalPath(cfg.ControllerTokenFile); err != nil {
			return WorkerConfig{}, fmt.Errorf("worker controller token file: %w", err)
		}
	}
	if cfg.AssetCacheDir != "" {
		if cfg.AssetCacheDir, err = absoluteLocalPath(cfg.AssetCacheDir); err != nil {
			return WorkerConfig{}, fmt.Errorf("worker asset cache dir: %w", err)
		}
	}
	if len(cfg.DataLocationRoots) > 0 {
		roots := make(map[string]string, len(cfg.DataLocationRoots))
		for name, root := range cfg.DataLocationRoots {
			roots[name], err = absoluteLocalPath(root)
			if err != nil {
				return WorkerConfig{}, fmt.Errorf("worker data location root %s: %w", name, err)
			}
		}
		cfg.DataLocationRoots = roots
	}
	return cfg, nil
}

func absoluteLocalPath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return filepath.Abs(filepath.FromSlash(value))
}

const maxWorkerControllerTokenBytes = 32 * 1024

func (r WorkerRuntime) validateControllerTokenFileConfig() error {
	if containsNewline(r.ControllerTokenFile) {
		return fmt.Errorf("worker controller token file must not contain newlines")
	}
	requiresToken, err := controllerURLRequiresWorkerToken(r.ControllerURL)
	if err != nil {
		return err
	}
	if requiresToken && r.ControllerTokenFile == "" {
		return fmt.Errorf("worker controller token file is required for controller url %s", r.ControllerURL)
	}
	return nil
}

func (r WorkerRuntime) RuntimePreflight(ctx context.Context, transport Transport, dialect ShellDialect) []PreflightIssue {
	if err := r.validateControllerTokenFileConfig(); err != nil {
		return []PreflightIssue{workerControllerTokenFileIssue("worker_controller_token_file_required", err.Error(), "Configure runtime.settings.controller_token_file to point at the worker bearer token file.")}
	}
	if r.ControllerTokenFile == "" {
		return nil
	}
	if _, ok := transport.(LocalTransport); ok {
		tokenPath, err := absoluteLocalPath(r.ControllerTokenFile)
		if err != nil {
			return []PreflightIssue{workerControllerTokenFileIssue("worker_controller_token_file_invalid", err.Error(), "Use a valid local worker token file path.")}
		}
		if err := validateLocalWorkerControllerTokenFile(tokenPath); err != nil {
			return []PreflightIssue{workerControllerTokenFileIssue("worker_controller_token_file_unusable", err.Error(), "Provision a non-empty, restrictive worker token file at the configured path.")}
		}
		return nil
	}
	if transport == nil {
		return []PreflightIssue{workerControllerTokenFileIssue("worker_controller_token_file_unchecked", "runtime transport is required to verify worker controller token file "+r.ControllerTokenFile, "Configure a runtime transport before preflight.")}
	}
	if dialect == nil {
		return []PreflightIssue{workerControllerTokenFileIssue("worker_controller_token_file_unchecked", "runtime shell dialect is required to verify worker controller token file "+r.ControllerTokenFile, "Configure a shell dialect before preflight.")}
	}
	tokenPath, err := dialect.LocalizePath(r.ControllerTokenFile)
	if err != nil {
		return []PreflightIssue{workerControllerTokenFileIssue("worker_controller_token_file_invalid", "worker controller token file "+r.ControllerTokenFile+": "+err.Error(), "Use a worker token file path supported by the execution environment.")}
	}
	if _, err := transport.Exec(ctx, "sh", "-c", workerControllerTokenFilePreflightScript, "goetl-token-preflight", tokenPath, strconv.FormatInt(maxWorkerControllerTokenBytes, 10)); err != nil {
		return []PreflightIssue{workerControllerTokenFileIssue("worker_controller_token_file_unusable", "worker controller token file "+r.ControllerTokenFile+" failed preflight: "+err.Error(), "Provision a non-empty, restrictive worker token file readable by the worker account.")}
	}
	return nil
}

func validateLocalWorkerControllerTokenFile(tokenPath string) error {
	info, err := os.Stat(tokenPath)
	if err != nil {
		return fmt.Errorf("worker controller token file %s: %w", tokenPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("worker controller token file %s is not a regular file", tokenPath)
	}
	if info.Size() == 0 {
		return fmt.Errorf("worker controller token file %s is empty", tokenPath)
	}
	if info.Size() > maxWorkerControllerTokenBytes {
		return fmt.Errorf("worker controller token file %s is too large", tokenPath)
	}
	file, err := os.Open(tokenPath)
	if err != nil {
		return fmt.Errorf("worker controller token file %s is not readable: %w", tokenPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("worker controller token file %s close failed: %w", tokenPath, err)
	}
	if goruntime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("worker controller token file %s permissions must not grant group or other access", tokenPath)
	}
	return nil
}

func workerControllerTokenFileIssue(code string, message string, remediation string) PreflightIssue {
	return PreflightIssue{
		Type:        "worker_controller_token_file",
		Severity:    PreflightSeverityError,
		Code:        code,
		Message:     message,
		Remediation: remediation,
	}
}

const workerControllerTokenFilePreflightScript = `
path=$1
max=$2
if [ ! -e "$path" ]; then
  echo "worker controller token file $path does not exist"
  exit 11
fi
if [ ! -f "$path" ]; then
  echo "worker controller token file $path is not a regular file"
  exit 12
fi
if [ ! -r "$path" ]; then
  echo "worker controller token file $path is not readable"
  exit 13
fi
size=$(wc -c < "$path") || exit 14
case "$size" in
  ""|*[!0-9]*) echo "worker controller token file $path size is unavailable"; exit 14 ;;
esac
if [ "$size" -eq 0 ]; then
  echo "worker controller token file $path is empty"
  exit 15
fi
if [ "$size" -gt "$max" ]; then
  echo "worker controller token file $path is too large"
  exit 16
fi
if command -v stat >/dev/null 2>&1; then
  mode=$(stat -c %a "$path" 2>/dev/null || true)
  if [ -n "$mode" ]; then
    case "$mode" in
      *00) ;;
      *) echo "worker controller token file $path permissions grant group or other access"; exit 17 ;;
    esac
  fi
fi
`

func controllerURLRequiresWorkerToken(raw string) (bool, error) {
	if raw == "" {
		return false, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false, fmt.Errorf("worker controller url is invalid: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return false, fmt.Errorf("worker controller url requires a scheme and host")
	}
	switch parsed.Scheme {
	case "https":
		return true, nil
	case "http":
		return !isWorkerControllerLoopbackHost(parsed.Hostname()), nil
	default:
		return false, fmt.Errorf("worker controller url scheme %q is unsupported", parsed.Scheme)
	}
}

func isWorkerControllerLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (r WorkerRuntime) uploadWorkerArtifact(ctx context.Context, transport Transport, dialect ShellDialect, paths WorkerRuntimePaths) error {
	shouldCopy := true
	if _, ok := transport.(LocalTransport); ok {
		same, err := sameLocalFileContent(r.LocalWorkerArtifact, paths.WorkerExecutable)
		if err != nil {
			return fmt.Errorf("compare worker artifact: %w", err)
		}
		shouldCopy = !same
	}
	if shouldCopy {
		if err := transport.Copy(ctx, r.LocalWorkerArtifact, paths.WorkerExecutable); err != nil {
			return fmt.Errorf("copy worker artifact: %w", err)
		}
	}
	workerExecutable, err := dialect.LocalizePath(paths.WorkerExecutable)
	if err != nil {
		return fmt.Errorf("worker executable path: %w", err)
	}
	if _, err := transport.Exec(ctx, "chmod", "0755", workerExecutable); err != nil {
		return fmt.Errorf("chmod worker artifact: %w", err)
	}
	return nil
}

func sameLocalFileContent(sourcePath string, destinationPath string) (bool, error) {
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return false, fmt.Errorf("read source %s: %w", sourcePath, err)
	}
	destination, err := os.ReadFile(destinationPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read destination %s: %w", destinationPath, err)
	}
	return bytes.Equal(source, destination), nil
}

type SingularityWorkerRuntime struct {
	WorkerRuntime
	SingularityExecutable     string
	ImagePath                 string
	ContainerWorkerExecutable string
	Bind                      string
}

func (r SingularityWorkerRuntime) WorkerScript(cfg SlurmWorkerScriptConfig) (SlurmWorkerScriptConfig, error) {
	executable := r.SingularityExecutable
	if executable == "" {
		executable = "singularity"
	}
	if containsNewline(executable) {
		return SlurmWorkerScriptConfig{}, fmt.Errorf("singularity executable must not contain newlines")
	}
	if r.ImagePath == "" {
		return SlurmWorkerScriptConfig{}, fmt.Errorf("singularity image path is required")
	}
	if r.ContainerWorkerExecutable == "" {
		return SlurmWorkerScriptConfig{}, fmt.Errorf("container worker executable is required")
	}
	if containsNewline(r.ImagePath) || containsNewline(r.ContainerWorkerExecutable) || containsNewline(r.Bind) {
		return SlurmWorkerScriptConfig{}, fmt.Errorf("singularity runtime values must not contain newlines")
	}
	bind := r.Bind
	if bind == "" {
		paths, err := r.paths()
		if err != nil {
			return SlurmWorkerScriptConfig{}, err
		}
		bind = paths.Root + ":" + paths.Root
	}

	args := []string{"exec"}
	if bind != "" {
		args = append(args, "--bind", bind)
	}
	args = append(args, r.ImagePath, r.ContainerWorkerExecutable)
	args = append(args, cfg.WorkerArgs...)

	cfg.WorkerExecutable = executable
	cfg.WorkerArgs = args
	return cfg, nil
}

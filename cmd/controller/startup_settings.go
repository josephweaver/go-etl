package main

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"

	"goetl/internal/controllerauth"
	"goetl/internal/model"
	"goetl/internal/variable"
)

type controllerFilesystemPaths struct {
	Root          string
	RepoCache     string
	Temp          string
	ArtifactCache string
}

type controllerOperationalPolicy struct {
	ResolverMaxDepth                int
	CaretakerIntervalScheduleMillis int
	CaretakerMissedIntervalLimit    int
	RepoCacheMaxSizeMB              int
	RepoCacheRetentionMillis        int
	GitFetchTimeoutMillis           int
	GitFetchConcurrency             int
	TempCleanupAgeMillis            int
	ArtifactCacheMaxSizeMB          int
	ArtifactCacheRetentionMillis    int
	StorageMinFreeMB                int
	FilesystemLoggingEnabled        bool
	LogRootPath                     string
	LogLevel                        string
	LogReadDefaultTailLines         int
	LogReadMaxTailLines             int
}

type controllerHTTPSettings struct {
	ListenHost              string
	ListenPort              int
	AdvertisedURL           string
	ReadHeaderTimeoutMillis int
	ReadTimeoutMillis       int
	WriteTimeoutMillis      int
	IdleTimeoutMillis       int
	ShutdownTimeoutMillis   int
	MaxRequestBytes         int
	MaxHeaderBytes          int
}

type controllerAuthSettings struct {
	Mode   controllerauth.Mode
	Store  controllerauth.Store
	Policy controllerauth.Policy
}

func resolveControllerFilesystemPaths(resolver variable.Resolver, workingDirectory string) (controllerFilesystemPaths, error) {
	if workingDirectory == "" {
		return controllerFilesystemPaths{}, fmt.Errorf("controller startup filesystem: working directory is required")
	}
	if !filepath.IsAbs(workingDirectory) {
		return controllerFilesystemPaths{}, fmt.Errorf("controller startup filesystem: working directory must be absolute")
	}

	root, err := resolveControllerFilesystemPath(resolver, workingDirectory, "controller_root_dir")
	if err != nil {
		return controllerFilesystemPaths{}, err
	}
	repoCache, err := resolveControllerFilesystemPath(resolver, workingDirectory, "controller_repo_cache_path")
	if err != nil {
		return controllerFilesystemPaths{}, err
	}
	temp, err := resolveControllerFilesystemPath(resolver, workingDirectory, "controller_temp_path")
	if err != nil {
		return controllerFilesystemPaths{}, err
	}
	artifactCache, err := resolveControllerFilesystemPath(resolver, workingDirectory, "controller_artifact_cache_path")
	if err != nil {
		return controllerFilesystemPaths{}, err
	}

	return controllerFilesystemPaths{
		Root:          root,
		RepoCache:     repoCache,
		Temp:          temp,
		ArtifactCache: artifactCache,
	}, nil
}

func resolveControllerFilesystemPath(resolver variable.Resolver, workingDirectory, key string) (string, error) {
	path, err := resolver.Path(key)
	if err != nil {
		return "", fmt.Errorf("controller startup filesystem: %w", err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDirectory, path)
	}
	return filepath.Clean(path), nil
}

func resolveControllerOperationalPolicy(resolver variable.Resolver, workingDirectory string) (controllerOperationalPolicy, error) {
	policy := controllerOperationalPolicy{}

	var err error
	if policy.ResolverMaxDepth, err = resolvePositiveIntPolicy(resolver, "resolver_max_depth", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.CaretakerIntervalScheduleMillis, err = resolvePositiveIntPolicy(resolver, "caretaker_interval_schedule_milliseconds", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.CaretakerMissedIntervalLimit, err = resolvePositiveIntPolicy(resolver, "caretaker_missed_interval_limit", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.RepoCacheMaxSizeMB, err = resolvePositiveIntPolicy(resolver, "controller_repo_cache_max_size_mb", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.RepoCacheRetentionMillis, err = resolvePositiveIntPolicy(resolver, "controller_repo_cache_retention_milliseconds", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.GitFetchTimeoutMillis, err = resolvePositiveIntPolicy(resolver, "controller_git_fetch_timeout_milliseconds", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.GitFetchConcurrency, err = resolvePositiveIntPolicy(resolver, "controller_git_fetch_concurrency", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.TempCleanupAgeMillis, err = resolvePositiveIntPolicy(resolver, "controller_temp_cleanup_age_milliseconds", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.ArtifactCacheMaxSizeMB, err = resolvePositiveIntPolicy(resolver, "controller_artifact_cache_max_size_mb", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.ArtifactCacheRetentionMillis, err = resolvePositiveIntPolicy(resolver, "controller_artifact_cache_retention_milliseconds", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.StorageMinFreeMB, err = resolvePositiveIntPolicy(resolver, "controller_storage_min_free_mb", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.FilesystemLoggingEnabled, err = resolveBoolPolicy(resolver, "controller_filesystem_logging_enabled", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.LogRootPath, err = resolvePathPolicy(resolver, workingDirectory, "controller_log_root_path", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.LogLevel, err = resolveStringPolicy(resolver, "controller_log_level", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	policy.LogReadDefaultTailLines, err = resolvePositiveIntPolicy(resolver, "controller_log_read_default_tail_lines", "controller startup policy")
	if err != nil {
		return controllerOperationalPolicy{}, err
	}
	policy.LogReadMaxTailLines, err = resolvePositiveIntPolicy(resolver, "controller_log_read_max_tail_lines", "controller startup policy")
	if err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.LogReadMaxTailLines < policy.LogReadDefaultTailLines {
		return controllerOperationalPolicy{}, fmt.Errorf("controller startup policy: controller_log_read_max_tail_lines must be greater than or equal to controller_log_read_default_tail_lines")
	}
	if _, err = model.CompareLogLevel(policy.LogLevel, string(model.LogLevelDebug)); err != nil {
		return controllerOperationalPolicy{}, fmt.Errorf("controller startup policy: invalid controller_log_level: %w", err)
	}

	return policy, nil
}

func resolveControllerHTTPSettings(resolver variable.Resolver) (controllerHTTPSettings, error) {
	settings := controllerHTTPSettings{}

	var err error
	if settings.ListenHost, err = resolveStringPolicy(resolver, "controller_listen_host", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.ListenPort, err = resolvePositiveIntPolicy(resolver, "controller_listen_port", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.AdvertisedURL, err = resolveStringPolicy(resolver, "controller_url", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.ReadHeaderTimeoutMillis, err = resolvePositiveIntPolicy(resolver, "controller_read_header_timeout_milliseconds", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.ReadTimeoutMillis, err = resolvePositiveIntPolicy(resolver, "controller_read_timeout_milliseconds", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.WriteTimeoutMillis, err = resolvePositiveIntPolicy(resolver, "controller_write_timeout_milliseconds", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.IdleTimeoutMillis, err = resolvePositiveIntPolicy(resolver, "controller_idle_timeout_milliseconds", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.ShutdownTimeoutMillis, err = resolvePositiveIntPolicy(resolver, "controller_shutdown_timeout_milliseconds", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.MaxRequestBytes, err = resolvePositiveIntPolicy(resolver, "controller_max_request_bytes", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}
	if settings.MaxHeaderBytes, err = resolvePositiveIntPolicy(resolver, "controller_max_header_bytes", "controller startup http"); err != nil {
		return controllerHTTPSettings{}, err
	}

	return settings, nil
}

func resolveControllerAuthSettings(resolver variable.Resolver, httpSettings controllerHTTPSettings, sources controllerauth.CredentialSources) (controllerAuthSettings, error) {
	authValue, err := resolver.Resolve(variable.Reference{Name: variable.Name{Key: "authentication"}})
	if err != nil {
		return controllerAuthSettings{}, fmt.Errorf("controller startup auth: resolve authentication: %w", err)
	}
	authConfig, err := controllerauth.ConfigFromResolved(authValue)
	if err != nil {
		return controllerAuthSettings{}, fmt.Errorf("controller startup auth: %w", err)
	}
	insecureExternalHTTP, err := resolveBoolPolicy(resolver, "controller_insecure_external_http_allowed", "controller startup auth")
	if err != nil {
		return controllerAuthSettings{}, err
	}
	if err := validateControllerAuthStartup(authConfig, httpSettings, insecureExternalHTTP); err != nil {
		return controllerAuthSettings{}, err
	}
	store, err := controllerauth.LoadCredentials(authConfig, sources)
	if err != nil {
		return controllerAuthSettings{}, fmt.Errorf("controller startup auth: %w", err)
	}
	return controllerAuthSettings{
		Mode:   authConfig.Mode,
		Store:  store,
		Policy: controllerauth.ControllerPolicy(),
	}, nil
}

func validateControllerAuthStartup(authConfig controllerauth.Config, httpSettings controllerHTTPSettings, insecureExternalHTTP bool) error {
	if err := validateControllerAdvertisedURL(httpSettings.AdvertisedURL, insecureExternalHTTP); err != nil {
		return fmt.Errorf("controller startup auth: %w", err)
	}
	if authConfig.Mode == controllerauth.ModeDisabled && !isLoopbackHost(httpSettings.ListenHost) {
		return fmt.Errorf("controller startup auth: disabled authentication requires loopback listen host, got %q", httpSettings.ListenHost)
	}
	if authConfig.Mode == controllerauth.ModeDisabled {
		advertisedLoopback, err := isLoopbackAdvertisedURL(httpSettings.AdvertisedURL)
		if err != nil {
			return fmt.Errorf("controller startup auth: %w", err)
		}
		if !advertisedLoopback {
			return fmt.Errorf("controller startup auth: disabled authentication requires loopback controller_url, got %q", httpSettings.AdvertisedURL)
		}
	}
	return nil
}

func validateControllerAdvertisedURL(rawURL string, insecureExternalHTTP bool) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("controller_url is invalid: %w", err)
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("controller_url requires a scheme")
	}
	if parsed.Host == "" {
		return fmt.Errorf("controller_url requires a host")
	}

	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(parsed.Hostname()) || insecureExternalHTTP {
			return nil
		}
		return fmt.Errorf("external http controller_url %q requires controller_insecure_external_http_allowed", rawURL)
	default:
		return fmt.Errorf("controller_url scheme %q is unsupported", parsed.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 127
	}
	return ip.Equal(net.IPv6loopback)
}

func isLoopbackAdvertisedURL(rawURL string) (bool, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("controller_url is invalid: %w", err)
	}
	if parsed.Host == "" {
		return false, fmt.Errorf("controller_url requires a host")
	}
	return isLoopbackHost(parsed.Hostname()), nil
}

func resolvePositiveIntPolicy(resolver variable.Resolver, key string, consumer string) (int, error) {
	number, err := resolver.Int(key)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", consumer, err)
	}
	if number <= 0 {
		return 0, fmt.Errorf("%s: %s must be greater than zero", consumer, key)
	}
	return number, nil
}

func resolveBoolPolicy(resolver variable.Resolver, key string, consumer string) (bool, error) {
	flag, err := resolver.Bool(key)
	if err != nil {
		return false, fmt.Errorf("%s: %w", consumer, err)
	}
	return flag, nil
}

func resolveStringPolicy(resolver variable.Resolver, key string, consumer string) (string, error) {
	text, err := resolver.String(key)
	if err != nil {
		return "", fmt.Errorf("%s: %w", consumer, err)
	}
	return text, nil
}

func resolvePathPolicy(resolver variable.Resolver, workingDirectory, key, consumer string) (string, error) {
	path, err := resolver.Path(key)
	if err != nil {
		return "", fmt.Errorf("%s: %w", consumer, err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDirectory, path)
	}
	return filepath.Clean(path), nil
}

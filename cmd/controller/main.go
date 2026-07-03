package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	fp "goetl/internal/fingerprint"
	"goetl/internal/ledger"
	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

const defaultControllerConfigFilename = "controller.json"
const rawPersistenceProjectID = "__raw_project__"
const rawPersistenceWorkflowID = "__raw_workflow__"
const rawPersistenceRunID = "__raw_run__"
const rawPersistenceStageIndex = 0
const rawPersistenceCreatedAt = "1970-01-01T00:00:00Z"

type Controller struct {
	mu                sync.Mutex
	pending           []model.WorkItem
	assigned          map[string]model.WorkItem
	failed            map[string]model.WorkFailure
	ledger            *sql.DB
	workflowStore     *persistence.Store
	shutdown          func(context.Context) error
	env               *ExecutionEnvironment
	scaler            WorkerScaleState
	scaleCfg          WorkerScaleConfig
	recoveryStartedAt time.Time
	normalAdmission   bool
}

type WorkflowSubmission struct {
	Workflow  workflow.Workflow   `json:"workflow"`
	Variables []variable.Variable `json:"variables"`
}

type WorkReuseDecision struct {
	Reusable       bool
	Reason         string
	PriorAttemptID string
}

type controllerStartupOptions struct {
	ConfigPath   string
	OverrideJSON []string
}

type controllerFilesystemPaths struct {
	Root          string
	GitCache      string
	Temp          string
	ArtifactCache string
}

type controllerOperationalPolicy struct {
	ResolverMaxDepth                int
	CaretakerIntervalScheduleMillis int
	CaretakerMissedIntervalLimit    int
	GitCacheMaxSizeMB               int
	GitCacheRetentionMillis         int
	GitFetchTimeoutMillis           int
	GitFetchConcurrency             int
	TempCleanupAgeMillis            int
	ArtifactCacheMaxSizeMB          int
	ArtifactCacheRetentionMillis    int
	StorageMinFreeMB                int
	FilesystemLoggingEnabled        bool
	LogRootPath                     string
	LogLevel                        string
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

func newController(items []model.WorkItem) *Controller {
	return &Controller{
		pending:         items,
		assigned:        make(map[string]model.WorkItem),
		failed:          make(map[string]model.WorkFailure),
		normalAdmission: true,
		scaleCfg: WorkerScaleConfig{
			MaxCount:                2,
			CountPerStart:           1,
			MinElapsedBetweenStarts: 30 * time.Second,
		},
	}
}

func (c *Controller) enterRecoveryMode() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recoveryStartedAt = time.Now().UTC()
	c.normalAdmission = false
}

func (c *Controller) allowNormalAdmission() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.normalAdmission = true
}

func (c *Controller) recoveryAdmissionClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.normalAdmission
}

func (c *Controller) requireNormalAdmission(w http.ResponseWriter) bool {
	if c.recoveryAdmissionClosed() {
		http.Error(w, "controller is in recovery mode", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func main() {
	server, release, err := buildControllerServer(os.Args, os.Executable, os.LookupEnv, os.Getwd, os.Getpid, time.Now, randomHex, buildInfoCodeVersion)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if release != nil {
			if err := release(); err != nil {
				fmt.Println("controller database ownership release failed:", err)
			}
		}
	}()

	fmt.Println("controller listening on", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Println("controller failed:", err)
	}
}

func buildControllerServer(
	args []string,
	executablePath func() (string, error),
	lookupEnv func(string) (string, bool),
	getwd func() (string, error),
	pid func() int,
	now func() time.Time,
	randomHex func(int) string,
	buildVersion func() string,
) (*http.Server, func() error, error) {
	options, err := parseControllerStartupOptions(args)
	if err != nil {
		return nil, nil, fmt.Errorf("controller config failed: %w", err)
	}
	configPath, err := controllerConfigPath(options.ConfigPath, executablePath)
	if err != nil {
		return nil, nil, fmt.Errorf("controller config failed: %w", err)
	}
	sources, err := loadControllerStartupSources(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("controller config failed: %w", err)
	}
	overrideScope, err := parseControllerStartupOverrides(options.OverrideJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("controller config failed: %w", err)
	}
	runtimeScope, err := newStartupRuntimeScope(pid(), randomHex(16), now().UTC(), buildVersion())
	if err != nil {
		return nil, nil, fmt.Errorf("controller config failed: %w", err)
	}
	resolver, err := newControllerStartupResolver(sources, overrideScope, runtimeScope, lookupEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("controller config failed: %w", err)
	}
	config := sources.Controller

	workflowStore, databasePath, err := initWorkflowExecutionStore(context.Background(), resolver)
	if err != nil {
		return nil, nil, fmt.Errorf("controller database failed: %w", err)
	}
	releaseDatabaseOwnership, err := acquireControllerDatabaseOwnershipForPath(databasePath)
	if err != nil {
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller database ownership failed: %w", err)
	}
	workingDirectory, err := getwd()
	if err != nil {
		if releaseDatabaseOwnership != nil {
			_ = releaseDatabaseOwnership()
		}
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller filesystem failed: %w", err)
	}
	if _, err := resolveControllerFilesystemPaths(resolver, workingDirectory); err != nil {
		if releaseDatabaseOwnership != nil {
			_ = releaseDatabaseOwnership()
		}
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller filesystem failed: %w", err)
	}
	if _, err := resolveControllerOperationalPolicy(resolver, workingDirectory); err != nil {
		if releaseDatabaseOwnership != nil {
			_ = releaseDatabaseOwnership()
		}
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller policy failed: %w", err)
	}
	httpSettings, err := resolveControllerHTTPSettings(resolver)
	if err != nil {
		if releaseDatabaseOwnership != nil {
			_ = releaseDatabaseOwnership()
		}
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller http failed: %w", err)
	}

	executionEnvironment, err := initConfiguredExecutionEnvironment(config)
	if err != nil {
		if releaseDatabaseOwnership != nil {
			_ = releaseDatabaseOwnership()
		}
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller execution environment failed: %w", err)
	}

	controller := newController(nil)
	controller.workflowStore = workflowStore
	controller.env = executionEnvironment
	controller.enterRecoveryMode()

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", httpSettings.ListenHost, httpSettings.ListenPort),
		Handler:           mux,
		ReadHeaderTimeout: time.Duration(httpSettings.ReadHeaderTimeoutMillis) * time.Millisecond,
		ReadTimeout:       time.Duration(httpSettings.ReadTimeoutMillis) * time.Millisecond,
		WriteTimeout:      time.Duration(httpSettings.WriteTimeoutMillis) * time.Millisecond,
		IdleTimeout:       time.Duration(httpSettings.IdleTimeoutMillis) * time.Millisecond,
		MaxHeaderBytes:    httpSettings.MaxHeaderBytes,
	}
	controller.shutdown = server.Shutdown

	mux.HandleFunc("/work/next", controller.nextWorkHandler)
	mux.HandleFunc("/work/complete", controller.completeWorkHandler)
	mux.HandleFunc("/work/fail", controller.failWorkHandler)
	mux.HandleFunc("/healthz", controller.healthHandler)
	mux.HandleFunc("/workflow", controller.submitWorkflowHandler)
	mux.HandleFunc("/work", controller.submitWorkHandler)
	mux.HandleFunc("/shutdown", controller.shutdownHandler)
	mux.HandleFunc("/status", controller.statusHandler)

	return server, func() error {
		if releaseDatabaseOwnership != nil {
			if err := releaseDatabaseOwnership(); err != nil {
				return err
			}
		}
		return workflowStore.Close()
	}, nil
}

func controllerConfigFromArgs(args []string, executablePath func() (string, error)) (ControllerConfig, error) {
	options, err := parseControllerStartupOptions(args)
	if err != nil {
		return ControllerConfig{}, err
	}
	if len(options.OverrideJSON) != 0 {
		return ControllerConfig{}, fmt.Errorf("controller startup overrides are not supported yet")
	}
	path, err := controllerConfigPath(options.ConfigPath, executablePath)
	if err != nil {
		return ControllerConfig{}, err
	}
	return loadControllerConfig(path)
}

func controllerConfigPath(explicitPath string, executablePath func() (string, error)) (string, error) {
	if explicitPath != "" {
		return explicitPath, nil
	}

	executable, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("determine controller executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), defaultControllerConfigFilename), nil
}

func parseControllerStartupOptions(args []string) (controllerStartupOptions, error) {
	var options controllerStartupOptions
	var configSet bool

	flags := flag.NewFlagSet("controller", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Func("config", "controller configuration path", func(value string) error {
		if configSet {
			return fmt.Errorf("--config may be specified only once")
		}
		configSet = true
		options.ConfigPath = value
		return nil
	})
	flags.Func("override", "canonical JSON override declaration", func(value string) error {
		options.OverrideJSON = append(options.OverrideJSON, value)
		return nil
	})

	arguments := args
	if len(arguments) > 0 {
		arguments = arguments[1:]
	}
	if err := flags.Parse(arguments); err != nil {
		return controllerStartupOptions{}, fmt.Errorf("parse controller startup arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return controllerStartupOptions{}, fmt.Errorf("parse controller startup arguments: unexpected positional argument %q", flags.Arg(0))
	}

	return options, nil
}

func parseControllerStartupOverrides(rawOverrides []string) (variable.Scope, error) {
	variables := make([]variable.Variable, 0, len(rawOverrides))
	seen := make(map[string]struct{}, len(rawOverrides))

	for index, raw := range rawOverrides {
		var declaration variable.Variable
		if err := json.Unmarshal([]byte(raw), &declaration); err != nil {
			return nil, fmt.Errorf("override argument %d: %w", index+1, err)
		}
		if declaration.Name.Namespace != variable.NamespaceOverride {
			return nil, fmt.Errorf("override argument %d (%s): namespace must be %s", index+1, declaration.Name, variable.NamespaceOverride)
		}
		if _, ok := seen[declaration.Name.Key]; ok {
			return nil, fmt.Errorf("override argument %d (%s): duplicate variable key", index+1, declaration.Name)
		}
		seen[declaration.Name.Key] = struct{}{}
		variables = append(variables, declaration)
	}

	scope, err := variable.NewScope(variables...)
	if err != nil {
		return nil, fmt.Errorf("build override scope: %w", err)
	}
	return scope, nil
}

func newStartupRuntimeScope(processID int, instanceID string, startedAt time.Time, buildVersion string) (variable.Scope, error) {
	if processID <= 0 {
		return nil, fmt.Errorf("runtime.controller_process_id must be positive")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("runtime.controller_instance_id is required")
	}
	if buildVersion == "" {
		return nil, fmt.Errorf("runtime.controller_build_version is required")
	}

	return variable.NewScope(
		variable.Variable{
			Name: variable.Name{Namespace: variable.NamespaceRuntime, Key: "controller_process_id"},
			TypedExpression: variable.TypedExpression{
				Type:       variable.TypeInt,
				Expression: processID,
			},
		},
		variable.Variable{
			Name: variable.Name{Namespace: variable.NamespaceRuntime, Key: "controller_instance_id"},
			TypedExpression: variable.TypedExpression{
				Type:       variable.TypeString,
				Expression: instanceID,
			},
		},
		variable.Variable{
			Name: variable.Name{Namespace: variable.NamespaceRuntime, Key: "controller_started_at"},
			TypedExpression: variable.TypedExpression{
				Type:       variable.TypeDatetime,
				Expression: startedAt.UTC().Format(time.RFC3339Nano),
			},
		},
		variable.Variable{
			Name: variable.Name{Namespace: variable.NamespaceRuntime, Key: "controller_build_version"},
			TypedExpression: variable.TypedExpression{
				Type:       variable.TypeString,
				Expression: buildVersion,
			},
		},
	)
}

func newControllerStartupResolver(
	sources controllerStartupSources,
	overrideScope variable.Scope,
	runtimeScope variable.Scope,
	controllerEnvironmentLookup func(string) (string, bool),
) (variable.Resolver, error) {
	defaultScope, controllerScope, err := sources.controllerScopes()
	if err != nil {
		return variable.Resolver{}, err
	}

	set := variable.NewSet(defaultScope, controllerScope, overrideScope, runtimeScope)
	bootstrap := variable.NewResolver(set, variable.ResolverConfig{
		MaxDepth:                    variable.DefaultMaxDepth,
		ControllerEnvironmentLookup: controllerEnvironmentLookup,
	})
	depth, err := bootstrap.Resolve(variable.Reference{
		Name: variable.Name{Key: "resolver_max_depth"},
	})
	if err != nil {
		return variable.Resolver{}, fmt.Errorf("resolve resolver_max_depth: %w", err)
	}
	if depth.Type != variable.TypeInt {
		return variable.Resolver{}, fmt.Errorf("resolver_max_depth has type %s, want int", depth.Type)
	}
	maxDepth, ok := depth.Value.(int)
	if !ok {
		return variable.Resolver{}, fmt.Errorf("resolver_max_depth must be an int")
	}
	if maxDepth <= 0 {
		return variable.Resolver{}, fmt.Errorf("resolver_max_depth must be greater than zero")
	}

	return variable.NewResolver(set, variable.ResolverConfig{
		MaxDepth:                    maxDepth,
		ControllerEnvironmentLookup: controllerEnvironmentLookup,
	}), nil
}

func initConfiguredExecutionEnvironment(config ControllerConfig) (*ExecutionEnvironment, error) {
	if config.ExecutionEnvironment.IsZero() {
		return nil, nil
	}

	env, err := NewExecutionEnvironment(config.ExecutionEnvironment)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

func initMainDatabase(ctx context.Context, resolver variable.Resolver) (*sql.DB, error) {
	driver, err := resolver.String("controller_config.main_database_driver")
	if err != nil {
		return nil, fmt.Errorf("controller startup database: required variable controller_config.main_database_driver: %w", err)
	}
	connectionString, err := resolver.String("controller_config.main_database_connection_string")
	if err != nil {
		return nil, fmt.Errorf("controller startup database: resolve controller_config.main_database_connection_string: %w", err)
	}
	if driver != "sqlite" {
		return nil, fmt.Errorf("controller startup database: unsupported main_database_driver %q", driver)
	}

	db, err := ledger.OpenSQLite(connectionString)
	if err != nil {
		return nil, fmt.Errorf("controller startup database: %w", err)
	}

	if err := ledger.InitSQLiteSchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("controller startup database: %w", err)
	}

	return db, nil
}

func initWorkflowExecutionStore(ctx context.Context, resolver variable.Resolver) (*persistence.Store, string, error) {
	driver, err := resolver.String("controller_config.main_database_driver")
	if err != nil {
		return nil, "", fmt.Errorf("controller startup database: required variable controller_config.main_database_driver: %w", err)
	}
	connectionString, err := resolver.String("controller_config.main_database_connection_string")
	if err != nil {
		return nil, "", fmt.Errorf("controller startup database: resolve controller_config.main_database_connection_string: %w", err)
	}

	store, err := persistence.OpenStore(ctx, persistence.Config{
		Driver:           driver,
		ConnectionString: connectionString,
	})
	if err != nil {
		return nil, "", fmt.Errorf("controller startup database: %w", err)
	}
	return store, connectionString, nil
}

func acquireControllerDatabaseOwnership(db *sql.DB) (func() error, error) {
	if db == nil {
		return nil, fmt.Errorf("controller startup database ownership: database handle is required")
	}

	var path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &path); err != nil {
		return nil, fmt.Errorf("controller startup database ownership: inspect database path: %w", err)
	}
	if path == "" || path == ":memory:" {
		return func() error { return nil }, nil
	}

	return acquireControllerDatabaseOwnershipForPath(path)
}

func acquireControllerDatabaseOwnershipForPath(path string) (func() error, error) {
	if path == "" || path == ":memory:" {
		return func() error { return nil }, nil
	}
	if path != filepath.Clean(path) {
		path = filepath.Clean(path)
	}

	lockPath := path + ".controller.lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("controller startup database ownership: database is already owned")
		}
		return nil, fmt.Errorf("controller startup database ownership: create lock file: %w", err)
	}
	if err := lockFile.Close(); err != nil {
		return nil, fmt.Errorf("controller startup database ownership: close lock file: %w", err)
	}

	return func() error {
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("controller startup database ownership: remove lock file: %w", err)
		}
		return nil
	}, nil
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
	gitCache, err := resolveControllerFilesystemPath(resolver, workingDirectory, "controller_git_cache_path")
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
		GitCache:      gitCache,
		Temp:          temp,
		ArtifactCache: artifactCache,
	}, nil
}

func resolveControllerFilesystemPath(resolver variable.Resolver, workingDirectory, key string) (string, error) {
	value, err := resolver.Resolve(variable.Reference{Name: variable.Name{Key: key}})
	if err != nil {
		return "", fmt.Errorf("controller startup filesystem: resolve %s: %w", key, err)
	}
	if value.Type != variable.TypePath {
		return "", fmt.Errorf("controller startup filesystem: %s has type %s, want path", key, value.Type)
	}
	path, ok := value.Value.(string)
	if !ok || path == "" {
		return "", fmt.Errorf("controller startup filesystem: %s is required", key)
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
	if policy.GitCacheMaxSizeMB, err = resolvePositiveIntPolicy(resolver, "controller_git_cache_max_size_mb", "controller startup policy"); err != nil {
		return controllerOperationalPolicy{}, err
	}
	if policy.GitCacheRetentionMillis, err = resolvePositiveIntPolicy(resolver, "controller_git_cache_retention_milliseconds", "controller startup policy"); err != nil {
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

func resolvePositiveIntPolicy(resolver variable.Resolver, key string, consumer string) (int, error) {
	value, err := resolver.Resolve(variable.Reference{Name: variable.Name{Key: key}})
	if err != nil {
		return 0, fmt.Errorf("%s: resolve %s: %w", consumer, key, err)
	}
	if value.Type != variable.TypeInt {
		return 0, fmt.Errorf("%s: %s has type %s, want int", consumer, key, value.Type)
	}
	number, ok := value.Value.(int)
	if !ok {
		return 0, fmt.Errorf("%s: %s must be an int", consumer, key)
	}
	if number <= 0 {
		return 0, fmt.Errorf("%s: %s must be greater than zero", consumer, key)
	}
	return number, nil
}

func resolveBoolPolicy(resolver variable.Resolver, key string, consumer string) (bool, error) {
	value, err := resolver.Resolve(variable.Reference{Name: variable.Name{Key: key}})
	if err != nil {
		return false, fmt.Errorf("%s: resolve %s: %w", consumer, key, err)
	}
	if value.Type != variable.TypeBool {
		return false, fmt.Errorf("%s: %s has type %s, want bool", consumer, key, value.Type)
	}
	flag, ok := value.Value.(bool)
	if !ok {
		return false, fmt.Errorf("%s: %s must be a bool", consumer, key)
	}
	return flag, nil
}

func resolveStringPolicy(resolver variable.Resolver, key string, consumer string) (string, error) {
	value, err := resolver.Resolve(variable.Reference{Name: variable.Name{Key: key}})
	if err != nil {
		return "", fmt.Errorf("%s: resolve %s: %w", consumer, key, err)
	}
	if value.Type != variable.TypeString {
		return "", fmt.Errorf("%s: %s has type %s, want string", consumer, key, value.Type)
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("%s: %s is required", consumer, key)
	}
	return text, nil
}

func resolvePathPolicy(resolver variable.Resolver, workingDirectory, key, consumer string) (string, error) {
	value, err := resolver.Resolve(variable.Reference{Name: variable.Name{Key: key}})
	if err != nil {
		return "", fmt.Errorf("%s: resolve %s: %w", consumer, key, err)
	}
	if value.Type != variable.TypePath {
		return "", fmt.Errorf("%s: %s has type %s, want path", consumer, key, value.Type)
	}
	path, ok := value.Value.(string)
	if !ok || path == "" {
		return "", fmt.Errorf("%s: %s is required", consumer, key)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDirectory, path)
	}
	return filepath.Clean(path), nil
}

func (c *Controller) recordAttempt(ctx context.Context, attempt ledger.Attempt) error {
	if c.ledger == nil {
		return nil
	}

	return ledger.InsertAttempt(ctx, c.ledger, attempt)
}

func (c *Controller) recordSkippedAttempt(ctx context.Context, item model.WorkItem, skippedAt time.Time) (model.WorkSkip, bool, error) {
	decision, err := c.workReuseDecision(ctx, item)
	if err != nil {
		return model.WorkSkip{}, false, err
	}

	skip, ok, err := workSkipForReuseDecision(item, decision)
	if err != nil || !ok {
		return model.WorkSkip{}, false, err
	}

	attempt, err := skippedAttemptFromWorkSkip(item, skip, skippedAt)
	if err != nil {
		return model.WorkSkip{}, false, err
	}
	if err := c.recordAttempt(ctx, attempt); err != nil {
		return model.WorkSkip{}, false, err
	}

	return skip, true, nil
}

func (c *Controller) priorCompletedAttempt(ctx context.Context, item model.WorkItem) (ledger.Attempt, bool, error) {
	if c.ledger == nil || item.WorkItemFingerprint == "" {
		return ledger.Attempt{}, false, nil
	}

	return ledger.FindLatestCompletedAttemptByWorkItemFingerprint(ctx, c.ledger, item.WorkItemFingerprint)
}

func priorCompletedAttemptMatchesWorkItem(item model.WorkItem, attempt ledger.Attempt) bool {
	if attempt.Status != ledger.AttemptStatusCompleted {
		return false
	}
	if item.WorkItemFingerprint == "" || item.InputFingerprint == "" || item.OutputFingerprint == "" || item.CodeVersion == "" {
		return false
	}

	return item.WorkItemFingerprint == attempt.WorkItemFingerprint &&
		item.InputFingerprint == attempt.InputFingerprint &&
		item.OutputFingerprint == attempt.OutputFingerprint &&
		item.CodeVersion == attempt.CodeVersion
}

func (c *Controller) reusablePriorAttempt(ctx context.Context, item model.WorkItem) (ledger.Attempt, bool, error) {
	attempt, ok, err := c.priorCompletedAttempt(ctx, item)
	if err != nil || !ok {
		return ledger.Attempt{}, false, err
	}
	if !priorCompletedAttemptMatchesWorkItem(item, attempt) {
		return ledger.Attempt{}, false, nil
	}

	return attempt, true, nil
}

func (c *Controller) workReuseDecision(ctx context.Context, item model.WorkItem) (WorkReuseDecision, error) {
	attempt, ok, err := c.priorCompletedAttempt(ctx, item)
	if err != nil {
		return WorkReuseDecision{}, err
	}
	if !ok {
		return WorkReuseDecision{Reason: "no_prior_completed_attempt"}, nil
	}
	if !priorCompletedAttemptMatchesWorkItem(item, attempt) {
		return WorkReuseDecision{
			Reason:         "prior_attempt_mismatch",
			PriorAttemptID: attempt.ID,
		}, nil
	}

	return WorkReuseDecision{
		Reusable:       true,
		Reason:         "matched_prior_completed_attempt",
		PriorAttemptID: attempt.ID,
	}, nil
}

func workSkipForReuseDecision(item model.WorkItem, decision WorkReuseDecision) (model.WorkSkip, bool, error) {
	if !decision.Reusable {
		return model.WorkSkip{}, false, nil
	}

	skip := model.WorkSkip{
		ID:             item.ID,
		PriorAttemptID: decision.PriorAttemptID,
		Reason:         decision.Reason,
	}
	if err := skip.Validate(); err != nil {
		return model.WorkSkip{}, false, err
	}

	return skip, true, nil
}

func skippedAttemptFromWorkSkip(item model.WorkItem, skip model.WorkSkip, skippedAt time.Time) (ledger.Attempt, error) {
	if err := skip.Validate(); err != nil {
		return ledger.Attempt{}, err
	}
	if skippedAt.IsZero() {
		skippedAt = time.Now().UTC()
	}

	return ledger.Attempt{
		ID:                  skip.ID + "-skip-" + randomHex(8),
		WorkflowInstanceID:  item.WorkflowInstanceID,
		StepInstanceID:      item.StepInstanceID,
		WorkItemID:          skip.ID,
		WorkItemFingerprint: item.WorkItemFingerprint,
		InputFingerprint:    item.InputFingerprint,
		OutputFingerprint:   item.OutputFingerprint,
		CodeVersion:         item.CodeVersion,
		Status:              ledger.AttemptStatusSkipped,
		StartedAt:           skippedAt.UTC(),
		CompletedAt:         skippedAt.UTC(),
		Variables:           runtimeVariablesFromSkip(item, skip, skippedAt.UTC()),
	}, nil
}

func (c *Controller) submitWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}

	var item model.WorkItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, "decode work item", http.StatusBadRequest)
		return
	}

	if err := item.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if c.workflowStore != nil {
		if err := c.submitRawWorkToStore(r.Context(), item, time.Now().UTC()); err != nil {
			if isPersistenceConflict(err) {
				http.Error(w, "work item id already exists", http.StatusConflict)
				return
			}
			http.Error(w, "persist work item", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasWorkItemID(item.ID) {
		http.Error(w, "work item id already exists", http.StatusConflict)
		return
	}

	c.pending = append(c.pending, item)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) submitRawWorkToStore(ctx context.Context, item model.WorkItem, submittedAt time.Time) error {
	if err := c.ensureRawPersistenceRun(ctx); err != nil {
		return err
	}

	record, queued, err := persistenceRecordsFromRawWorkItem(item, submittedAt)
	if err != nil {
		return err
	}
	if _, found, err := c.workflowStore.GetWorkItem(ctx, record.ID); err != nil {
		return err
	} else if found {
		return fmt.Errorf("work item %s already exists", record.ID)
	}
	if err := c.workflowStore.InsertWorkItems(ctx, []persistence.WorkItemRecord{record}); err != nil {
		return err
	}
	if err := c.workflowStore.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{queued}); err != nil {
		return err
	}
	return nil
}

func (c *Controller) ensureRawPersistenceRun(ctx context.Context) error {
	project := persistence.ProjectRecord{
		ID:                 rawPersistenceProjectID,
		Name:               "Raw Work",
		RepositoryIdentity: "controller:raw",
		SourceCommit:       "raw",
		ConfigPath:         "raw",
		ConfigSHA256:       sha256HexString("raw-project"),
		CreatedAt:          rawPersistenceCreatedAt,
	}
	if err := c.workflowStore.UpsertProject(ctx, project); err != nil {
		return err
	}
	workflow := persistence.WorkflowRecord{
		ID:                 rawPersistenceWorkflowID,
		ProjectID:          rawPersistenceProjectID,
		Name:               "Raw Work",
		RepositoryIdentity: "controller:raw",
		SourceCommit:       "raw",
		WorkflowPath:       "raw",
		WorkflowSHA256:     sha256HexString("raw-workflow"),
		CreatedAt:          rawPersistenceCreatedAt,
	}
	if err := c.workflowStore.UpsertWorkflow(ctx, workflow); err != nil {
		return err
	}
	run := persistence.WorkflowRunRecord{
		ID:                    rawPersistenceRunID,
		ProjectID:             rawPersistenceProjectID,
		WorkflowID:            rawPersistenceWorkflowID,
		SubmissionContextJSON: `{"source":"raw-work"}`,
		CreatedAt:             rawPersistenceCreatedAt,
	}
	if err := c.workflowStore.CreateWorkflowRun(ctx, run); err != nil {
		return err
	}
	stage := persistence.WorkflowStageRecord{
		RunID:                rawPersistenceRunID,
		StageIndex:           rawPersistenceStageIndex,
		StepID:               "raw-work",
		StageSourceReference: "controller:raw-work",
		State:                "ready",
		CreatedAt:            rawPersistenceCreatedAt,
		ReadyAt:              rawPersistenceCreatedAt,
	}
	return c.workflowStore.InsertStagePlan(ctx, rawPersistenceRunID, []persistence.WorkflowStageRecord{stage})
}

func persistenceRecordsFromRawWorkItem(item model.WorkItem, submittedAt time.Time) (persistence.WorkItemRecord, persistence.QueuedWorkRecord, error) {
	payload, err := json.Marshal(item)
	if err != nil {
		return persistence.WorkItemRecord{}, persistence.QueuedWorkRecord{}, fmt.Errorf("encode raw work item: %w", err)
	}
	record := persistence.WorkItemRecord{
		ID:                   item.ID,
		RunID:                rawPersistenceRunID,
		StageIndex:           rawPersistenceStageIndex,
		WorkItemIndex:        rawWorkItemIndex(item.ID),
		WorkerPayloadJSON:    string(payload),
		ResolvedInputsSHA256: sha256HexBytes(payload),
		CreatedAt:            submittedAt.UTC().Format(time.RFC3339),
	}
	return record, persistence.QueuedWorkRecord{
		WorkItemRecord: record,
		QueuedAt:       submittedAt.UTC().Format(time.RFC3339),
	}, nil
}

func rawWorkItemIndex(id string) int {
	sum := sha256.Sum256([]byte(id))
	return int(sum[0]&0x7f)<<24 | int(sum[1])<<16 | int(sum[2])<<8 | int(sum[3])
}

func sha256HexString(value string) string {
	return sha256HexBytes([]byte(value))
}

func sha256HexBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func isPersistenceConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists")
}

func (c *Controller) submitWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}
	if c.recoveryAdmissionClosed() {
		http.Error(w, "controller is in recovery mode", http.StatusServiceUnavailable)
		return
	}

	var submission WorkflowSubmission
	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		http.Error(w, "decode workflow submission", http.StatusBadRequest)
		return
	}

	workflowScope, err := variable.NewScope(submission.Workflow.Variables...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	submissionScope, err := variable.NewScope(submission.Variables...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resolver := variable.NewResolver(variable.NewSet(workflowScope, submissionScope), variable.ResolverConfig{})
	codeVersion, err := controllerCodeVersion(resolver)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	compiledItems, err := workflow.CompileWorkflowItems(resolver, submission.Workflow)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	items := workItemsWithRuntimeMetadata(submission.Workflow.ID, compiledItems, codeVersion)

	scaleCfg, err := workerScaleConfig(resolver, c.scaleCfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.mu.Lock()

	for _, item := range items {
		if c.hasWorkItemID(item.ID) {
			c.mu.Unlock()
			http.Error(w, "work item id already exists", http.StatusConflict)
			return
		}
	}

	startCount := 0
	assignedCount := len(c.assigned)
	c.pending = append(c.pending, items...)
	if c.env != nil {
		now := time.Now()
		startCount = c.scaler.PlanStarts(now, len(c.pending), assignedCount, scaleCfg)
		c.scaler.RecordStart(now, startCount, assignedCount)
	}
	c.mu.Unlock()

	if err := c.startConfiguredWorkers(r.Context(), resolver, startCount); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) startConfiguredWorkers(ctx context.Context, resolver variable.Resolver, count int) error {
	if count == 0 {
		return nil
	}
	if c.env == nil {
		return fmt.Errorf("execution environment is required")
	}

	workerCfg, err := workerLaunchConfig(resolver)
	if err != nil {
		return err
	}
	workerCfg.slurm.Platform = c.env.Dialect
	if runtime, ok := c.env.Runtime.(WorkerScriptRuntime); ok {
		workerCfg.slurm, err = runtime.WorkerScript(workerCfg.slurm)
		if err != nil {
			return err
		}
	}

	if err := c.env.Prepare(ctx); err != nil {
		return err
	}

	for range count {
		if _, err := c.env.Scheduler.Submit(ctx, JobSpec{
			Name:             workerCfg.slurm.JobName,
			RemoteScriptPath: workerCfg.scriptPath,
			WorkerScript:     workerCfg.slurm,
		}); err != nil {
			return err
		}
	}
	return nil
}

func workItemsWithRuntimeMetadata(workflowID string, compiledItems []workflow.CompiledWorkItem, codeVersion string) []model.WorkItem {
	workflowInstanceID := workflowID + "-instance-" + randomHex(8)
	workflowFingerprint := fingerprint("workflow", map[string]any{
		"id": workflowID,
	})
	items := make([]model.WorkItem, 0, len(compiledItems))

	for _, compiled := range compiledItems {
		item := compiled.WorkItem
		item.WorkflowDefinitionID = workflowID
		item.WorkflowFingerprint = workflowFingerprint
		item.WorkflowInstanceID = workflowInstanceID
		item.StepDefinitionID = compiled.StepID
		item.StepFingerprint = fingerprint("step", map[string]any{
			"workflow_fingerprint": workflowFingerprint,
			"id":                   compiled.StepID,
		})
		item.StepInstanceID = workflowInstanceID + "-step-" + compiled.StepID
		item.WorkItemFingerprint = fingerprint("work-item", map[string]any{
			"id":              item.ID,
			"type":            item.Type,
			"output_filename": item.OutputFilename,
			"parameters":      item.Parameters,
		})
		item.InputFingerprint = fingerprint("input", item.Parameters)
		item.OutputFingerprint = fingerprint("output", map[string]any{
			"output_filename": item.OutputFilename,
		})
		item.CodeVersion = codeVersion
		items = append(items, item)
	}

	return items
}

func controllerCodeVersion(resolver variable.Resolver) (string, error) {
	configured, ok, err := optionalStringVariable(resolver, "code_version")
	if err != nil {
		return "", err
	}
	if ok {
		return configured, nil
	}

	return buildInfoCodeVersion(), nil
}

func optionalStringVariable(resolver variable.Resolver, name string) (string, bool, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", false, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return "", false, nil
	}

	if value.Type != variable.TypeString {
		return "", false, fmt.Errorf("%s has type %s, want string", name, value.Type)
	}

	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", false, fmt.Errorf("%s is required", name)
	}

	return text, true, nil
}

func buildInfoCodeVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	revision := buildSetting(info, "vcs.revision")
	if revision == "" {
		return "unknown"
	}

	modified := buildSetting(info, "vcs.modified")
	if modified == "true" {
		return revision + "-modified"
	}

	return revision
}

func buildSetting(info *debug.BuildInfo, key string) string {
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

func fingerprint(label string, value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(fmt.Sprint(value))
	}
	sum := sha256.Sum256(data)
	return label + ":sha256:" + hex.EncodeToString(sum[:])
}

func randomHex(byteCount int) string {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(data)
}

func workerTargetEnvironment(resolver variable.Resolver) (string, error) {
	reference, err := variable.ParseReference("worker_target_environment")
	if err != nil {
		return "", err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return "", nil
	}

	if value.Type != variable.TypeString {
		return "", fmt.Errorf("worker_target_environment has type %s, want string", value.Type)
	}

	workerTarget, ok := value.Value.(string)
	if !ok {
		return "", fmt.Errorf("worker_target_environment is required")
	}

	return workerTarget, nil
}

func workerScaleConfig(resolver variable.Resolver, defaults WorkerScaleConfig) (WorkerScaleConfig, error) {
	cfg := defaults

	var err error
	if cfg.MinCount, err = optionalIntVariable(resolver, "worker_min_count", cfg.MinCount); err != nil {
		return WorkerScaleConfig{}, err
	}
	if cfg.MaxCount, err = optionalIntVariable(resolver, "worker_max_count", cfg.MaxCount); err != nil {
		return WorkerScaleConfig{}, err
	}
	if cfg.CountPerStart, err = optionalIntVariable(resolver, "worker_count_per_start", cfg.CountPerStart); err != nil {
		return WorkerScaleConfig{}, err
	}
	if cfg.MinElapsedBetweenStarts, err = optionalDurationVariable(resolver, "worker_min_elapsed_time_between_starts", cfg.MinElapsedBetweenStarts); err != nil {
		return WorkerScaleConfig{}, err
	}

	return cfg, nil
}

func optionalIntVariable(resolver variable.Resolver, name string, fallback int) (int, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return 0, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return fallback, nil
	}

	if value.Type != variable.TypeInt {
		return 0, fmt.Errorf("%s has type %s, want int", name, value.Type)
	}

	number, ok := value.Value.(int)
	if !ok {
		return 0, fmt.Errorf("%s must be an int", name)
	}

	return number, nil
}

func optionalDurationVariable(resolver variable.Resolver, name string, fallback time.Duration) (time.Duration, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return 0, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return fallback, nil
	}

	if value.Type != variable.TypeString {
		return 0, fmt.Errorf("%s has type %s, want string", name, value.Type)
	}

	text, ok := value.Value.(string)
	if !ok || text == "" {
		return 0, fmt.Errorf("%s is required", name)
	}

	duration, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}

	return duration, nil
}

func (c *Controller) shutdownHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}

	if c.shutdown == nil {
		http.Error(w, "shutdown unavailable", http.StatusServiceUnavailable)
		return
	}

	go func() {
		if err := c.shutdown(context.Background()); err != nil {
			fmt.Println("controller shutdown failed:", err)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) hasWorkItemID(id string) bool {
	for _, item := range c.pending {
		if item.ID == id {
			return true
		}
	}

	if _, ok := c.assigned[id]; ok {
		return true
	}

	if _, ok := c.failed[id]; ok {
		return true
	}

	return false
}

func (c *Controller) statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}

	status, err := c.controllerStatus(r.Context())
	if err != nil {
		http.Error(w, "query controller status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "encode status", http.StatusInternalServerError)
	}
}

func (c *Controller) controllerStatus(ctx context.Context) (model.ControllerStatus, error) {
	if c.workflowStore != nil {
		return c.persistenceControllerStatus(ctx)
	}

	c.mu.Lock()
	pendingItems := append([]model.WorkItem(nil), c.pending...)
	status := model.ControllerStatus{
		Pending:  len(c.pending),
		Assigned: len(c.assigned),
		Failed:   len(c.failed),
	}
	c.mu.Unlock()

	reuseReasons, err := c.pendingReuseDecisionReasons(ctx, pendingItems)
	if err != nil {
		return model.ControllerStatus{}, fmt.Errorf("query reuse candidates: %w", err)
	}
	status.PendingReuseCandidates = reuseReasons["matched_prior_completed_attempt"]

	attempts, attemptVariables, err := c.ledgerStatusCounts(ctx)
	if err != nil {
		return model.ControllerStatus{}, fmt.Errorf("query ledger status: %w", err)
	}
	status.Attempts = attempts
	status.AttemptVariables = attemptVariables

	return status, nil
}

func (c *Controller) persistenceControllerStatus(ctx context.Context) (model.ControllerStatus, error) {
	queued, err := c.workflowStore.ListQueuedWorkItems(ctx)
	if err != nil {
		return model.ControllerStatus{}, fmt.Errorf("list queued work: %w", err)
	}
	running, err := c.workflowStore.ListRunningWork(ctx)
	if err != nil {
		return model.ControllerStatus{}, fmt.Errorf("list running work: %w", err)
	}
	runs, err := c.workflowStore.ListActiveWorkflowRuns(ctx)
	if err != nil {
		return model.ControllerStatus{}, fmt.Errorf("list active workflow runs: %w", err)
	}

	status := model.ControllerStatus{
		Pending:  len(queued),
		Assigned: len(running),
	}
	for _, run := range runs {
		counts, err := c.workflowStore.CountWorkItemsForRun(ctx, run.ID)
		if err != nil {
			return model.ControllerStatus{}, fmt.Errorf("count work items for run %s: %w", run.ID, err)
		}
		status.Failed += counts.Failed
	}
	return status, nil
}

func (c *Controller) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) ledgerStatusCounts(ctx context.Context) (int, int, error) {
	if c.ledger == nil {
		return 0, 0, nil
	}

	var attempts int
	if err := c.ledger.QueryRowContext(ctx, `SELECT COUNT(*) FROM attempts`).Scan(&attempts); err != nil {
		return 0, 0, fmt.Errorf("query attempts count: %w", err)
	}

	var attemptVariables int
	if err := c.ledger.QueryRowContext(ctx, `SELECT COUNT(*) FROM attempt_variables`).Scan(&attemptVariables); err != nil {
		return 0, 0, fmt.Errorf("query attempt variables count: %w", err)
	}

	return attempts, attemptVariables, nil
}

func (c *Controller) pendingReuseDecisionReasons(ctx context.Context, items []model.WorkItem) (map[string]int, error) {
	counts := make(map[string]int)
	for _, item := range items {
		decision, err := c.workReuseDecision(ctx, item)
		if err != nil {
			return nil, err
		}
		counts[decision.Reason]++
	}
	return counts, nil
}

func (c *Controller) failWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var failure model.WorkFailure
	if err := json.NewDecoder(r.Body).Decode(&failure); err != nil {
		http.Error(w, "decode failure", http.StatusBadRequest)
		return
	}

	if failure.ID == "" || failure.Error == "" {
		http.Error(w, "work item id and error are required", http.StatusBadRequest)
		return
	}
	if c.workflowStore != nil {
		c.failPersistedWorkHandler(w, r, failure)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.assigned[failure.ID]; !ok {
		http.Error(w, "work item not assigned", http.StatusNotFound)
		return
	}

	delete(c.assigned, failure.ID)
	c.failed[failure.ID] = failure
	fmt.Println("work item failed:", failure.ID, failure.Error)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) completeWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var completion model.WorkCompletion
	if err := json.NewDecoder(r.Body).Decode(&completion); err != nil {
		http.Error(w, "decode completion", http.StatusBadRequest)
		return
	}

	if completion.ID == "" {
		http.Error(w, "work item id is required", http.StatusBadRequest)
		return
	}
	if c.workflowStore != nil {
		c.completePersistedWorkHandler(w, r, completion)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.assigned[completion.ID]; !ok {
		http.Error(w, "work item not assigned", http.StatusNotFound)
		return
	}

	attempt, hasAttempt, err := attemptFromCompletion(completion)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if hasAttempt {
		if err := c.recordAttempt(r.Context(), attempt); err != nil {
			http.Error(w, "record completion", http.StatusInternalServerError)
			return
		}
	}

	delete(c.assigned, completion.ID)
	fmt.Println("work item completed:", completion.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) failPersistedWorkHandler(w http.ResponseWriter, r *http.Request, failure model.WorkFailure) {
	if failure.AttemptID == "" {
		http.Error(w, "attempt_id is required", http.StatusBadRequest)
		return
	}
	failedAt, err := reportTimestamp("failed_at", failure.FailedAt, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, found, err := c.workflowStore.FailAttempt(r.Context(), persistence.FailAttemptRequest{
		AttemptID: failure.AttemptID,
		Error:     failure.Error,
		FailedAt:  failedAt,
	})
	if err != nil {
		if strings.Contains(err.Error(), "conflicts with existing") {
			http.Error(w, "fail attempt conflict", http.StatusConflict)
			return
		}
		http.Error(w, "fail persisted attempt", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "active attempt not found", http.StatusNotFound)
		return
	}

	fmt.Println("persisted work item failed:", failure.ID, failure.AttemptID, failure.Error)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) completePersistedWorkHandler(w http.ResponseWriter, r *http.Request, completion model.WorkCompletion) {
	request, err := completeAttemptRequestFromCompletion(completion, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, found, err := c.workflowStore.CompleteAttempt(r.Context(), request)
	if err != nil {
		if strings.Contains(err.Error(), "conflicts with existing") {
			http.Error(w, "complete attempt conflict", http.StatusConflict)
			return
		}
		http.Error(w, "complete persisted attempt", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "active attempt not found", http.StatusNotFound)
		return
	}

	fmt.Println("persisted work item completed:", completion.ID, completion.AttemptID)
	w.WriteHeader(http.StatusNoContent)
}

func completeAttemptRequestFromCompletion(completion model.WorkCompletion, fallbackCompletedAt time.Time) (persistence.CompleteAttemptRequest, error) {
	if completion.AttemptID == "" {
		return persistence.CompleteAttemptRequest{}, fmt.Errorf("attempt_id is required")
	}

	outputJSON, outputJSONSHA256, err := canonicalJSONTextAndHash("output_json", completion.OutputJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	_, preStateSHA256, err := canonicalJSONTextAndHash("pre_state_json", completion.PreStateJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	_, postStateSHA256, err := canonicalJSONTextAndHash("post_state_json", completion.PostStateJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}

	completedAt, err := reportTimestamp("completed_at", completion.CompletedAt, fallbackCompletedAt)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}

	return persistence.CompleteAttemptRequest{
		AttemptID:        completion.AttemptID,
		OutputJSON:       outputJSON,
		OutputJSONSHA256: outputJSONSHA256,
		PreStateSHA256:   preStateSHA256,
		PostStateSHA256:  postStateSHA256,
		CompletedAt:      completedAt,
	}, nil
}

func canonicalJSONTextAndHash(name string, value string) (string, string, error) {
	if strings.TrimSpace(value) == "" {
		return "", "", fmt.Errorf("%s is required", name)
	}

	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return "", "", fmt.Errorf("decode %s: %w", name, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", "", fmt.Errorf("%s must contain one JSON document", name)
	}

	canonical, hash, err := fp.CanonicalJSONSHA256(decoded)
	if err != nil {
		return "", "", fmt.Errorf("canonicalize %s: %w", name, err)
	}
	return string(canonical), hash, nil
}

func reportTimestamp(name string, value string, fallback time.Time) (string, error) {
	if value == "" {
		return fallback.UTC().Format(time.RFC3339), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed.UTC().Format(time.RFC3339), nil
}

func attemptFromCompletion(completion model.WorkCompletion) (ledger.Attempt, bool, error) {
	if completion.AttemptID == "" {
		return ledger.Attempt{}, false, nil
	}

	startedAt, err := time.Parse(time.RFC3339, completion.StartedAt)
	if err != nil {
		return ledger.Attempt{}, false, fmt.Errorf("parse started_at: %w", err)
	}

	completedAt, err := time.Parse(time.RFC3339, completion.CompletedAt)
	if err != nil {
		return ledger.Attempt{}, false, fmt.Errorf("parse completed_at: %w", err)
	}

	return ledger.Attempt{
		ID:                  completion.AttemptID,
		WorkflowInstanceID:  completion.WorkflowInstanceID,
		StepInstanceID:      completion.StepInstanceID,
		WorkItemID:          completion.ID,
		WorkItemFingerprint: completion.WorkItemFingerprint,
		InputFingerprint:    completion.InputFingerprint,
		OutputFingerprint:   completion.OutputFingerprint,
		CodeVersion:         completion.CodeVersion,
		Status:              ledger.AttemptStatusCompleted,
		StartedAt:           startedAt,
		CompletedAt:         completedAt,
		Variables:           runtimeVariablesFromCompletion(completion),
	}, true, nil
}

func runtimeVariablesFromCompletion(completion model.WorkCompletion) []ledger.AttemptVariable {
	variables := []ledger.AttemptVariable{
		runtimeStringVariable("workflow_definition_id", completion.WorkflowDefinitionID, "workflow"),
		runtimeStringVariable("workflow_fingerprint", completion.WorkflowFingerprint, "workflow"),
		runtimeStringVariable("workflow_instance_id", completion.WorkflowInstanceID, "workflow"),
		runtimeStringVariable("step_definition_id", completion.StepDefinitionID, "step"),
		runtimeStringVariable("step_fingerprint", completion.StepFingerprint, "step"),
		runtimeStringVariable("step_instance_id", completion.StepInstanceID, "step"),
		runtimeStringVariable("work_item_id", completion.ID, "work_item"),
		runtimeStringVariable("work_item_fingerprint", completion.WorkItemFingerprint, "work_item"),
		runtimeStringVariable("input_fingerprint", completion.InputFingerprint, "work_item"),
		runtimeStringVariable("output_fingerprint", completion.OutputFingerprint, "work_item"),
		runtimeStringVariable("code_version", completion.CodeVersion, "work_item"),
		runtimeStringVariable("attempt_id", completion.AttemptID, "attempt"),
		runtimeStringVariable("started_at", completion.StartedAt, "attempt"),
		runtimeStringVariable("completed_at", completion.CompletedAt, "attempt"),
	}

	for name, parameter := range completion.Parameters {
		variables = append(variables, ledger.AttemptVariable{
			Namespace: "work_item",
			Name:      name,
			Type:      parameter.Type,
			Value:     parameter.Value,
			Source:    "controller",
			Lifecycle: "work_item",
		})
	}

	return variables
}

func runtimeVariablesFromSkip(item model.WorkItem, skip model.WorkSkip, skippedAt time.Time) []ledger.AttemptVariable {
	timestamp := skippedAt.UTC().Format(time.RFC3339)
	return []ledger.AttemptVariable{
		runtimeStringVariable("workflow_definition_id", item.WorkflowDefinitionID, "workflow"),
		runtimeStringVariable("workflow_fingerprint", item.WorkflowFingerprint, "workflow"),
		runtimeStringVariable("workflow_instance_id", item.WorkflowInstanceID, "workflow"),
		runtimeStringVariable("step_definition_id", item.StepDefinitionID, "step"),
		runtimeStringVariable("step_fingerprint", item.StepFingerprint, "step"),
		runtimeStringVariable("step_instance_id", item.StepInstanceID, "step"),
		runtimeStringVariable("work_item_id", skip.ID, "work_item"),
		runtimeStringVariable("work_item_fingerprint", item.WorkItemFingerprint, "work_item"),
		runtimeStringVariable("input_fingerprint", item.InputFingerprint, "work_item"),
		runtimeStringVariable("output_fingerprint", item.OutputFingerprint, "work_item"),
		runtimeStringVariable("code_version", item.CodeVersion, "work_item"),
		runtimeStringVariable("prior_attempt_id", skip.PriorAttemptID, "attempt"),
		runtimeStringVariable("skip_reason", skip.Reason, "attempt"),
		runtimeStringVariable("started_at", timestamp, "attempt"),
		runtimeStringVariable("completed_at", timestamp, "attempt"),
	}
}

func runtimeStringVariable(name string, value string, lifecycle string) ledger.AttemptVariable {
	return ledger.AttemptVariable{
		Namespace: "runtime",
		Name:      name,
		Type:      "string",
		Value:     value,
		Source:    "worker",
		Lifecycle: lifecycle,
	}
}

func (c *Controller) nextWorkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}
	if c.workflowStore != nil {
		c.nextPersistedWorkHandler(w, r)
		return
	}

	for {
		c.mu.Lock()
		if len(c.pending) == 0 {
			c.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		item := c.pending[0]
		c.pending = c.pending[1:]
		c.mu.Unlock()

		_, skipped, err := c.recordSkippedAttempt(r.Context(), item, time.Now().UTC())
		if err != nil {
			c.mu.Lock()
			c.pending = append([]model.WorkItem{item}, c.pending...)
			c.mu.Unlock()
			http.Error(w, "record skipped attempt", http.StatusInternalServerError)
			return
		}
		if skipped {
			fmt.Println("work item skipped:", item.ID)
			continue
		}

		c.mu.Lock()
		c.assigned[item.ID] = item
		c.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(item); err != nil {
			http.Error(w, "encode work item", http.StatusInternalServerError)
		}
		return
	}
}

func (c *Controller) nextPersistedWorkHandler(w http.ResponseWriter, r *http.Request) {
	claim, found, err := c.workflowStore.ClaimNextWork(r.Context(), persistence.ClaimWorkRequest{
		AttemptID:    "attempt-" + randomHex(16),
		ExecutorType: persistence.ExecutorTypeWorker,
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		http.Error(w, "claim work", http.StatusInternalServerError)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var item model.WorkItem
	if err := json.Unmarshal([]byte(claim.WorkItem.WorkerPayloadJSON), &item); err != nil {
		http.Error(w, "decode persisted worker payload", http.StatusInternalServerError)
		return
	}
	item.AttemptID = claim.AttemptID
	if err := item.Validate(); err != nil {
		http.Error(w, "validate persisted worker payload", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(item); err != nil {
		http.Error(w, "encode work item", http.StatusInternalServerError)
	}
}

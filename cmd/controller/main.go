package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	fp "goetl/internal/fingerprint"
	"goetl/internal/ledger"
	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/reposource"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

const defaultControllerConfigFilename = "controller.json"
const rawPersistenceProjectID = "__raw_project__"
const rawPersistenceWorkflowID = "__raw_workflow__"
const rawPersistenceRunID = "__raw_run__"
const rawPersistenceStageIndex = 0
const rawPersistenceCreatedAt = "1970-01-01T00:00:00Z"
const maxResourceConstraintSummaries = 5

var errSourceReferenceAdmissionNotImplemented = errors.New("source-reference workflow admission is not implemented")

type Controller struct {
	mu                  sync.Mutex
	claimMu             sync.Mutex
	ledger              *sql.DB
	workflowStore       *persistence.Store
	repoSourceProviders map[string]reposource.Provider
	repoCacheLayout     reposource.CacheLayout
	workerStarter       WorkerStarter
	logSink             logObservationSink
	shutdown            func(context.Context) error
	env                 *ExecutionEnvironment
	scaler              WorkerScaleState
	scaleCfg            WorkerScaleConfig
	recoveryStartedAt   time.Time
	normalAdmission     bool
	maxRequestBytes     int
	logRootPath         string
	logReadDefaultTail  int
	logReadMaxTail      int
}

type WorkflowSubmission struct {
	Workflow       workflow.Workflow                    `json:"workflow"`
	SourceManifest reposource.SourceManifestDeclaration `json:"source_manifest,omitempty"`
	Variables      []variable.Variable                  `json:"variables"`
}

type RawWorkSubmission struct {
	WorkItem            model.WorkItem                           `json:"work_item"`
	ResourceConstraints []workflow.ResourceConstraintDeclaration `json:"resource_constraints,omitempty"`
}

type WorkflowRunSubmission struct {
	Project   SourceDocumentReference `json:"project"`
	Workflow  SourceDocumentReference `json:"workflow"`
	Variables []variable.Variable     `json:"variables,omitempty"`
}

type WorkerStarter interface {
	StartWorker(targetEnvironment string, resolver variable.Resolver) error
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

func newController() *Controller {
	return &Controller{
		workerStarter:   LocalWorkerStarter{},
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

func (c *Controller) completeStartupRecovery(ctx context.Context) error {
	if err := c.verifyActiveRunSources(ctx); err != nil {
		return err
	}
	c.allowNormalAdmission()
	return nil
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
	paths, err := resolveControllerFilesystemPaths(resolver, workingDirectory)
	if err != nil {
		if releaseDatabaseOwnership != nil {
			_ = releaseDatabaseOwnership()
		}
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller filesystem failed: %w", err)
	}
	policy, err := resolveControllerOperationalPolicy(resolver, workingDirectory)
	if err != nil {
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

	controller := newController()
	if policy.FilesystemLoggingEnabled {
		logSink, err := newFilesystemLogSink(policy.LogRootPath, policy.LogLevel)
		if err != nil {
			if releaseDatabaseOwnership != nil {
				_ = releaseDatabaseOwnership()
			}
			workflowStore.Close()
			return nil, nil, fmt.Errorf("controller log sink failed: %w", err)
		}
		controller.logSink = logSink
	}
	controller.workflowStore = workflowStore
	controller.repoSourceProviders = initRepositorySourceProviders(workingDirectory)
	controller.repoCacheLayout, err = reposource.NewCacheLayout(paths.RepoCache)
	if err != nil {
		if releaseDatabaseOwnership != nil {
			_ = releaseDatabaseOwnership()
		}
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller repository cache failed: %w", err)
	}
	controller.env = executionEnvironment
	controller.maxRequestBytes = httpSettings.MaxRequestBytes
	controller.enterRecoveryMode()
	if err := controller.completeStartupRecovery(context.Background()); err != nil {
		if releaseDatabaseOwnership != nil {
			_ = releaseDatabaseOwnership()
		}
		workflowStore.Close()
		return nil, nil, fmt.Errorf("controller recovery failed: %w", err)
	}

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
	controller.logRootPath = policy.LogRootPath
	controller.logReadDefaultTail = policy.LogReadDefaultTailLines
	controller.logReadMaxTail = policy.LogReadMaxTailLines

	registerControllerRoutes(mux, controller)

	return server, func() error {
		if releaseDatabaseOwnership != nil {
			if err := releaseDatabaseOwnership(); err != nil {
				return err
			}
		}
		return workflowStore.Close()
	}, nil
}

func registerControllerRoutes(mux *http.ServeMux, controller *Controller) {
	mux.HandleFunc("/work/next", controller.nextWorkHandler)
	mux.HandleFunc("/work/complete", controller.completeWorkHandler)
	mux.HandleFunc("/work/fail", controller.failWorkHandler)
	mux.HandleFunc("/healthz", controller.healthHandler)
	mux.HandleFunc("/workflow", controller.submitWorkflowHandler)
	mux.HandleFunc("/workflow-runs/", controller.sourceBundleHandler)
	mux.HandleFunc("/submissions/", controller.submissionHandler)
	mux.HandleFunc("/work", controller.submitWorkHandler)
	mux.HandleFunc("/shutdown", controller.shutdownHandler)
	mux.HandleFunc("/status", controller.statusHandler)
	mux.HandleFunc("/observations/logs", controller.logObservationsHandler)
}

func (c *Controller) submissionHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := strings.CutSuffix(r.URL.Path, "/status"); ok {
		c.submissionStatusHandler(w, r)
		return
	}
	if _, ok := strings.CutSuffix(r.URL.Path, "/logs"); ok {
		c.submissionLogsHandler(w, r)
		return
	}
	http.NotFound(w, r)
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

func initRepositorySourceProviders(workingDirectory string) map[string]reposource.Provider {
	repository := reposource.RepositoryIdentity{Value: "local:demo", DisplayName: "Demo"}
	return map[string]reposource.Provider{
		repository.Value: reposource.NewLocalProvider(repository, filepath.Join(workingDirectory, "..", "go-etl-demo-project")),
	}
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read work item", http.StatusBadRequest)
		return
	}
	submittedAt := time.Now().UTC()
	item, constraints, err := decodeRawWorkSubmission(body, submittedAt)
	if err != nil {
		http.Error(w, "decode work item", http.StatusBadRequest)
		return
	}

	if err := item.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}
	if err := c.submitRawWorkToStore(r.Context(), item, constraints, submittedAt); err != nil {
		if isPersistenceConflict(err) {
			http.Error(w, "work item id already exists", http.StatusConflict)
			return
		}
		http.Error(w, "persist work item", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeRawWorkSubmission(data []byte, submittedAt time.Time) (model.WorkItem, []model.WorkItemResourceConstraint, error) {
	var envelope struct {
		WorkItem json.RawMessage `json:"work_item"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return model.WorkItem{}, nil, err
	}
	if len(envelope.WorkItem) == 0 {
		var item model.WorkItem
		if err := json.Unmarshal(data, &item); err != nil {
			return model.WorkItem{}, nil, err
		}
		return item, nil, nil
	}

	var submission RawWorkSubmission
	if err := json.Unmarshal(data, &submission); err != nil {
		return model.WorkItem{}, nil, err
	}
	if err := submission.WorkItem.Validate(); err != nil {
		return model.WorkItem{}, nil, err
	}
	constraints, err := workflow.ResolveResourceConstraints(
		variable.NewResolver(variable.NewSet(), variable.ResolverConfig{}),
		submission.WorkItem.ID,
		submission.ResourceConstraints,
		submittedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return model.WorkItem{}, nil, err
	}
	return submission.WorkItem, constraints, nil
}

func (c *Controller) submitRawWorkToStore(ctx context.Context, item model.WorkItem, constraints []model.WorkItemResourceConstraint, submittedAt time.Time) error {
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
	if err := c.workflowStore.QueueWorkItems(ctx, persistence.QueueWorkItemsRequest{
		WorkItems:           []persistence.WorkItemRecord{record},
		ResourceConstraints: persistenceResourceConstraintRecords(constraints),
		QueuedWork:          []persistence.QueuedWorkRecord{queued},
	}); err != nil {
		return err
	}
	return nil
}

func (c *Controller) ensureRawPersistenceRun(ctx context.Context) error {
	project := persistence.ProjectRecord{
		ID:                 rawPersistenceProjectID,
		Name:               "Raw Work",
		RepositoryIdentity: "controller:raw",
		SourceRevisionID:   stringPtr("raw"),
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
		SourceRevisionID:   stringPtr("raw"),
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
	item, err := item.WithExecutionEnvelope()
	if err != nil {
		return persistence.WorkItemRecord{}, persistence.QueuedWorkRecord{}, fmt.Errorf("build execution envelope for raw work item %s: %w", item.ID, err)
	}
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
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	var submission WorkflowRunSubmission
	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		http.Error(w, "decode workflow run submission", http.StatusBadRequest)
		return
	}
	if err := submission.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ack, err := c.submitWorkflowRunToStore(r.Context(), submission, time.Now().UTC())
	if err != nil {
		if errors.Is(err, errSourceReferenceAdmissionNotImplemented) {
			http.Error(w, err.Error(), http.StatusNotImplemented)
			return
		}
		http.Error(w, "persist workflow run", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(ack); err != nil {
		http.Error(w, "encode submission acknowledgement", http.StatusInternalServerError)
	}
}

func (s WorkflowRunSubmission) validate() error {
	if err := s.Project.validate("project"); err != nil {
		return err
	}
	if err := s.Workflow.validate("workflow"); err != nil {
		return err
	}
	return nil
}

func (r SourceDocumentReference) validate(name string) error {
	if strings.TrimSpace(r.Repository) == "" {
		return fmt.Errorf("%s repository is required", name)
	}
	if strings.TrimSpace(r.Ref) == "" {
		return fmt.Errorf("%s ref is required", name)
	}
	if strings.TrimSpace(r.Path) == "" {
		return fmt.Errorf("%s path is required", name)
	}
	return nil
}

func (c *Controller) submitWorkflowRunToStore(ctx context.Context, submission WorkflowRunSubmission, submittedAt time.Time) (model.SubmissionAcknowledgement, error) {
	provider, err := c.repositorySourceProvider(submission.Project)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	if submission.Workflow.Repository != submission.Project.Repository {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("workflow repository %s does not match project repository %s", submission.Workflow.Repository, submission.Project.Repository)
	}
	if submission.Workflow.Ref != submission.Project.Ref {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("workflow ref %s does not match project ref %s", submission.Workflow.Ref, submission.Project.Ref)
	}
	resolved, err := provider.Resolve(ctx, submission.Project.Ref)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("resolve repository source: %w", err)
	}

	initialReads, err := provider.ReadFiles(ctx, resolved, []string{submission.Project.Path, submission.Workflow.Path})
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("read project/workflow source: %w", err)
	}
	projectRead, err := requiredReadByPath(initialReads, submission.Project.Path)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	workflowRead, err := requiredReadByPath(initialReads, submission.Workflow.Path)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	_, projectHash, err := canonicalSourceDocument(projectRead.Content.Data)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("canonicalize project source: %w", err)
	}
	_, workflowHash, err := canonicalSourceDocument(workflowRead.Content.Data)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("canonicalize workflow source: %w", err)
	}
	workflowSubmission, err := decodeWorkflowSourceSubmission(workflowRead.Content.Data)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("decode workflow source: %w", err)
	}
	declared, err := declaredSourceFiles(submission.Project.Path, projectHash, submission.Workflow.Path, workflowHash, workflowSubmission.SourceManifest)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	supplementalPaths := supplementalSourcePaths(declared)
	supplementalReads, err := provider.ReadFiles(ctx, resolved, supplementalPaths)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("read supplemental source: %w", err)
	}
	allReads := append(append([]reposource.ReadFileResult{}, initialReads...), supplementalReads...)

	runID := "run-" + randomHex(16)
	manifest, err := reposource.BuildAdmittedSourceManifest(runID, resolved, declared, allReads)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("build admitted source manifest: %w", err)
	}
	if err := reposource.PublishAdmittedSource(c.repoCacheLayout, manifest, allReads); err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("publish admitted source: %w", err)
	}
	cacheAccess, err := reposource.NewCacheAccess(c.repoCacheLayout, manifest)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("open admitted source cache: %w", err)
	}
	cachedProjectData, err := cacheAccess.ReadFile(submission.Project.Path)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("read cached project source: %w", err)
	}
	cachedWorkflowData, err := cacheAccess.ReadFile(submission.Workflow.Path)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("read cached workflow source: %w", err)
	}
	_, projectHash, err = canonicalSourceDocument(cachedProjectData)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("canonicalize cached project source: %w", err)
	}
	_, workflowHash, err = canonicalSourceDocument(cachedWorkflowData)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("canonicalize cached workflow source: %w", err)
	}
	workflowSubmission, err = decodeWorkflowSourceSubmission(cachedWorkflowData)
	if err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("decode cached workflow source: %w", err)
	}

	projectFile, err := manifestFileByRole(manifest, reposource.FileRoleProject)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	workflowFile, err := manifestFileByRole(manifest, reposource.FileRoleWorkflow)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	projectRecord := projectRecordFromAdmittedSource(manifest.Source, projectFile, projectHash)
	if err := c.workflowStore.UpsertProject(ctx, projectRecord); err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("upsert project: %w", err)
	}
	workflowRecord := workflowRecordFromAdmittedSource(projectRecord.ID, manifest.Source, workflowFile, workflowHash)
	if err := c.workflowStore.UpsertWorkflow(ctx, workflowRecord); err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("upsert workflow: %w", err)
	}

	workflowScope, err := variable.NewScope(workflowSubmission.Workflow.Variables...)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	sourceSubmissionScope, err := variable.NewScope(workflowSubmission.Variables...)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	runSubmissionScope, err := variable.NewScope(submission.Variables...)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	resolver := variable.NewResolver(variable.NewSet(workflowScope, sourceSubmissionScope, runSubmissionScope), variable.ResolverConfig{})
	codeVersion, err := controllerCodeVersion(resolver)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	var plan workflow.WorkflowPlan
	var stageResult *workflow.CompileStageResult
	compileResult := workflow.CompileResult{
		WorkflowID: workflowSubmission.Workflow.ID,
		StepCount:  len(workflowSubmission.Workflow.Steps),
		WorkItems:  nil,
	}

	if len(workflowSubmission.Workflow.Steps) > 0 {
		var err error
		plan, err = workflow.NormalizeStages(workflowSubmission.Workflow)
		if err != nil {
			return model.SubmissionAcknowledgement{}, fmt.Errorf("normalize workflow stages: %w", err)
		}

		result, err := workflow.CompileWorkflowStage(resolver, workflowSubmission.Workflow, plan, 0)
		if err != nil {
			return model.SubmissionAcknowledgement{}, err
		}
		stageResult = &result

		compiledItems := make([]workflow.CompiledWorkItem, 0, len(result.WorkItems))
		for _, item := range result.WorkItems {
			compiledItems = append(compiledItems, workflow.CompiledWorkItem{
				WorkflowID:          result.WorkflowID,
				StepID:              item.StepID,
				WorkItem:            item.WorkItem,
				ResourceConstraints: item.ResourceConstraints,
			})
		}
		compileResult.WorkItems = compiledItems
	}
	compileResult, err = prepareCompiledWorkflowForAdmission(c.repoCacheLayout, manifest, compileResult)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	if stageResult != nil {
		if len(stageResult.WorkItems) != len(compileResult.WorkItems) {
			return model.SubmissionAcknowledgement{}, fmt.Errorf("compile result mismatch: expected %d stage items, got %d", len(stageResult.WorkItems), len(compileResult.WorkItems))
		}
		for index := range stageResult.WorkItems {
			stageResult.WorkItems[index].WorkItem = compileResult.WorkItems[index].WorkItem
			stageResult.WorkItems[index].ResourceConstraints = compileResult.WorkItems[index].ResourceConstraints
		}
	}

	runRecord, err := workflowRunRecordFromAdmittedManifest(runID, projectRecord.ID, workflowRecord.ID, manifest, submission.Variables, submittedAt, c.repoCacheLayout)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	if err := c.workflowStore.CreateWorkflowRun(ctx, runRecord); err != nil {
		return model.SubmissionAcknowledgement{}, fmt.Errorf("create workflow run: %w", err)
	}
	stages := stageRecordsFromWorkflow(runRecord.ID, workflowFile.SourcePath, workflowSubmission.Workflow, submittedAt)
	if len(stages) != 0 {
		if err := c.workflowStore.InsertStagePlan(ctx, runRecord.ID, stages); err != nil {
			return model.SubmissionAcknowledgement{}, fmt.Errorf("insert stage plan: %w", err)
		}
	}

	var memberships []compiledStageWorkItemMembership
	var items []persistence.WorkItemRecord
	var queued []persistence.QueuedWorkRecord
	autoAdvanced := false
	if len(plan.Stages) != 0 {
		if err := c.CreateWorkflowDependencyPlan(ctx, runRecord.ID, runRecord.WorkflowID, plan.Stages); err != nil {
			return model.SubmissionAcknowledgement{}, fmt.Errorf("create workflow dependency plan: %w", err)
		}
	}

	stageResults := []workflow.CompileStageResult{}
	if stageResult != nil {
		stageResults = append(stageResults, *stageResult)
	}
	var resourceConstraints []persistence.WorkItemResourceConstraintRecord
	items, queued, memberships, resourceConstraints, err = persistenceRecordsFromCompiledStageResults(runRecord.ID, stageResults, codeVersion, submittedAt)
	if err != nil {
		return model.SubmissionAcknowledgement{}, err
	}
	if len(items) != 0 {
		if err := c.workflowStore.QueueWorkItems(ctx, persistence.QueueWorkItemsRequest{
			WorkItems:           items,
			ResourceConstraints: resourceConstraints,
			QueuedWork:          queued,
		}); err != nil {
			return model.SubmissionAcknowledgement{}, fmt.Errorf("queue work items: %w", err)
		}
	}
	for _, membership := range memberships {
		if err := c.RecordCompiledWorkItemMembership(ctx, runRecord.ID, membership.stageIndex, membership.stepIndex, membership.workItemID, membership.workItemIndex); err != nil {
			return model.SubmissionAcknowledgement{}, fmt.Errorf("record compiled work item membership: %w", err)
		}
	}
	if stageResult != nil && len(queued) != 0 {
		c.emitDependencyObservation(ctx, runRecord.ID, runRecord.WorkflowID, model.LogLevelInfo, fmt.Sprintf("queued stage %d", stageResult.StageIndex))
	}

	if stageResult != nil {
		stageCompleted, err := c.markCompiledStageEmptyStepsCompleted(ctx, runRecord.ID, *stageResult)
		if err != nil {
			return model.SubmissionAcknowledgement{}, err
		}

		autoAdvanced = stageCompleted
		if stageCompleted {
			if err := c.activateNextReadyWorkflowStage(ctx, runRecord.ID, stageResult.StageIndex, submittedAt); err != nil {
				return model.SubmissionAcknowledgement{}, err
			}
		}
	}

	if !autoAdvanced {
		scaleCfg, err := workerScaleConfig(resolver, c.scaleCfg)
		if err != nil {
			return model.SubmissionAcknowledgement{}, err
		}
		queuedCount, runningCount, err := c.persistedWorkDemand(ctx)
		if err != nil {
			return model.SubmissionAcknowledgement{}, err
		}
		startCount := c.scaler.PlanStarts(submittedAt, queuedCount, runningCount, scaleCfg)
		c.scaler.RecordStart(submittedAt, startCount, runningCount)
		if err := c.startWorkers(ctx, resolver, startCount); err != nil {
			return model.SubmissionAcknowledgement{}, err
		}
	}

	return model.SubmissionAcknowledgement{
		SubmissionID:         runRecord.ID,
		WorkflowID:           workflowSubmission.Workflow.ID,
		InitialWorkItemCount: len(items),
	}, nil
}

func (c *Controller) persistedWorkDemand(ctx context.Context) (int, int, error) {
	queued, err := c.workflowStore.ListQueuedWorkItems(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("list queued work: %w", err)
	}
	running, err := c.workflowStore.ListRunningWork(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("list running work: %w", err)
	}
	return len(queued), len(running), nil
}

func (c *Controller) startWorkers(ctx context.Context, resolver variable.Resolver, count int) error {
	if count == 0 {
		return nil
	}
	if c.env != nil {
		return c.startConfiguredWorkers(ctx, resolver, count)
	}

	targetEnvironment, err := workerTargetEnvironment(resolver)
	if err != nil {
		return err
	}
	if targetEnvironment == "" {
		return nil
	}
	if c.workerStarter == nil {
		return fmt.Errorf("worker starter is required")
	}
	for i := 0; i < count; i++ {
		if err := c.workerStarter.StartWorker(targetEnvironment, resolver); err != nil {
			return err
		}
	}
	return nil
}

func canonicalSourceDocument(data []byte) ([]byte, string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, "", err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, "", fmt.Errorf("source document contains multiple JSON values")
	}
	return fp.CanonicalJSONSHA256(value)
}

func decodeWorkflowSourceSubmission(data []byte) (WorkflowSubmission, error) {
	var submission WorkflowSubmission
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&submission); err != nil {
		return WorkflowSubmission{}, err
	}
	if submission.Workflow.ID == "" {
		return WorkflowSubmission{}, fmt.Errorf("workflow id is required")
	}
	if err := submission.SourceManifest.Validate(); err != nil {
		return WorkflowSubmission{}, err
	}
	return submission, nil
}

func (c *Controller) repositorySourceProvider(ref SourceDocumentReference) (reposource.Provider, error) {
	if c.repoSourceProviders != nil {
		if provider, ok := c.repoSourceProviders[ref.Repository]; ok {
			return provider, nil
		}
	}
	if strings.HasPrefix(ref.Repository, "github.com/") {
		parts := strings.Split(ref.Repository, "/")
		if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("github repository identity must be github.com/<owner>/<repo>")
		}
		return reposource.NewGitHubProvider(parts[1], parts[2]), nil
	}
	return nil, fmt.Errorf("unknown source repository: %s", ref.Repository)
}

func requiredReadByPath(reads []reposource.ReadFileResult, path string) (reposource.ReadFileResult, error) {
	clean, err := reposource.ValidateRepositoryRelativePath(path)
	if err != nil {
		return reposource.ReadFileResult{}, err
	}
	for _, read := range reads {
		if read.Request.SourcePath == clean {
			return read, nil
		}
	}
	return reposource.ReadFileResult{}, fmt.Errorf("source file %s was not read", clean)
}

func declaredSourceFiles(projectPath string, projectSHA256 string, workflowPath string, workflowSHA256 string, sourceManifest reposource.SourceManifestDeclaration) ([]reposource.DeclaredSourceFile, error) {
	projectPath, err := reposource.ValidateRepositoryRelativePath(projectPath)
	if err != nil {
		return nil, fmt.Errorf("project path: %w", err)
	}
	workflowPath, err = reposource.ValidateRepositoryRelativePath(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("workflow path: %w", err)
	}
	declared := []reposource.DeclaredSourceFile{
		{
			Role:                reposource.FileRoleProject,
			SourcePath:          projectPath,
			CachePath:           projectPath,
			ContentType:         "application/json",
			CanonicalJSONSHA256: stringPtr(projectSHA256),
		},
		{
			Role:                reposource.FileRoleWorkflow,
			SourcePath:          workflowPath,
			CachePath:           workflowPath,
			ContentType:         "application/json",
			CanonicalJSONSHA256: stringPtr(workflowSHA256),
		},
	}
	supplemental, err := sourceManifest.DeclaredSourceFiles()
	if err != nil {
		return nil, err
	}
	return append(declared, supplemental...), nil
}

func supplementalSourcePaths(declared []reposource.DeclaredSourceFile) []string {
	paths := make([]string, 0, len(declared))
	for _, file := range declared {
		switch file.Role {
		case reposource.FileRoleProject, reposource.FileRoleWorkflow:
			continue
		default:
			paths = append(paths, file.SourcePath)
		}
	}
	return paths
}

func manifestFileByRole(manifest reposource.AdmittedSourceManifest, role reposource.FileRole) (reposource.AdmittedSourceManifestFile, error) {
	for _, file := range manifest.Files {
		if file.Role == role {
			return file, nil
		}
	}
	return reposource.AdmittedSourceManifestFile{}, fmt.Errorf("admitted manifest missing %s file", role)
}

func projectRecordFromAdmittedSource(source reposource.ResolvedSourceReference, file reposource.AdmittedSourceManifestFile, configSHA256 string) persistence.ProjectRecord {
	return persistence.ProjectRecord{
		ID:                 deterministicSourceID("project", source.Repository.Value, sourceRevisionIDValue(source.RevisionID), file.SourcePath, configSHA256),
		Name:               file.SourcePath,
		RepositoryIdentity: source.Repository.Value,
		SourceRevisionID:   source.RevisionID,
		ConfigPath:         file.SourcePath,
		SourceObjectID:     stringValue(file.ObjectID),
		ConfigSHA256:       configSHA256,
		CreatedAt:          sourceRecordCreatedAt(source.RevisionID),
	}
}

func workflowRecordFromAdmittedSource(projectID string, source reposource.ResolvedSourceReference, file reposource.AdmittedSourceManifestFile, workflowSHA256 string) persistence.WorkflowRecord {
	return persistence.WorkflowRecord{
		ID:                 deterministicSourceID("workflow", projectID, source.Repository.Value, sourceRevisionIDValue(source.RevisionID), file.SourcePath, workflowSHA256),
		ProjectID:          projectID,
		Name:               file.SourcePath,
		RepositoryIdentity: source.Repository.Value,
		SourceRevisionID:   source.RevisionID,
		WorkflowPath:       file.SourcePath,
		SourceObjectID:     stringValue(file.ObjectID),
		WorkflowSHA256:     workflowSHA256,
		CreatedAt:          sourceRecordCreatedAt(source.RevisionID),
	}
}

func deterministicSourceID(kind string, parts ...string) string {
	return kind + ":" + sha256HexString(strings.Join(parts, "\n"))
}

func stringPtr(value string) *string {
	return &value
}

func sourceRevisionIDValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func sourceRecordCreatedAt(revisionID *string) string {
	if revisionID == nil || *revisionID == "" {
		return localUnversionedCommit
	}
	return "source:" + *revisionID
}

const workflowRunSubmissionContextSchemaV1 = "goet/workflow-run-submission-context/v1"

type workflowRunSubmissionContext struct {
	Schema          string                            `json:"schema"`
	SourceAdmission workflowRunSourceAdmissionContext `json:"source_admission"`
	Variables       []variable.Variable               `json:"variables"`
	DependencyState *model.WorkflowDependencyPlan     `json:"dependency_state,omitempty"`
}

type workflowRunSourceAdmissionContext struct {
	Schema           string                           `json:"schema"`
	ManifestRef      string                           `json:"manifest_ref"`
	Source           workflowRunSourceIdentity        `json:"source"`
	SourceRevisionID *string                          `json:"source_revision_id"`
	Files            []workflowRunSourceAdmissionFile `json:"files"`
}

type workflowRunSourceIdentity struct {
	RepositoryIdentity string `json:"repository_identity"`
	RequestedRef       string `json:"requested_ref,omitempty"`
	ProvenanceWarning  string `json:"provenance_warning,omitempty"`
}

type workflowRunSourceAdmissionFile struct {
	Role           string `json:"role"`
	SourcePath     string `json:"source_path"`
	CachePath      string `json:"cache_path,omitempty"`
	SourceObjectID string `json:"source_object_id,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
}

func workflowRunRecordFromAdmittedManifest(runID string, projectID string, workflowID string, manifest reposource.AdmittedSourceManifest, variables []variable.Variable, createdAt time.Time, layout reposource.CacheLayout) (persistence.WorkflowRunRecord, error) {
	paths, err := layout.PathsForManifest(manifest)
	if err != nil {
		return persistence.WorkflowRunRecord{}, err
	}
	variables, err = safeWorkflowRunSubmissionVariables(variables)
	if err != nil {
		return persistence.WorkflowRunRecord{}, err
	}
	admissionContext := workflowRunSourceAdmissionContext{
		Schema:      manifest.Schema,
		ManifestRef: filepath.ToSlash(paths.ManifestPath),
		Source: workflowRunSourceIdentity{
			RepositoryIdentity: manifest.Source.Repository.Value,
			RequestedRef:       manifest.Source.RequestedRef,
			ProvenanceWarning:  sourceProvenanceWarning(manifest.Source),
		},
		SourceRevisionID: manifest.Source.RevisionID,
		Files:            make([]workflowRunSourceAdmissionFile, 0, len(manifest.Files)),
	}
	for _, file := range manifest.Files {
		sha := stringValue(file.RawSHA256)
		if file.CanonicalJSONSHA256 != nil {
			sha = *file.CanonicalJSONSHA256
		}
		admissionContext.Files = append(admissionContext.Files, workflowRunSourceAdmissionFile{
			Role:           string(file.Role),
			SourcePath:     file.SourcePath,
			CachePath:      file.CachePath,
			SourceObjectID: stringValue(file.ObjectID),
			SHA256:         sha,
		})
	}
	submissionContext := workflowRunSubmissionContext{
		Schema:          workflowRunSubmissionContextSchemaV1,
		SourceAdmission: admissionContext,
		Variables:       variables,
	}
	contextJSON, err := json.Marshal(submissionContext)
	if err != nil {
		return persistence.WorkflowRunRecord{}, fmt.Errorf("encode submission context: %w", err)
	}
	return persistence.WorkflowRunRecord{
		ID:                    runID,
		ProjectID:             projectID,
		WorkflowID:            workflowID,
		SubmissionContextJSON: string(contextJSON),
		CreatedAt:             createdAt.UTC().Format(time.RFC3339),
	}, nil
}

func safeWorkflowRunSubmissionVariables(variables []variable.Variable) ([]variable.Variable, error) {
	safe := make([]variable.Variable, 0, len(variables))
	for _, item := range variables {
		if item.Sensitive && item.ProtectedRef == nil {
			return nil, fmt.Errorf("submission variable %s is sensitive plaintext; use protected_ref", item.Name)
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		safe = append(safe, item)
	}
	return safe, nil
}

func sourceProvenanceWarning(source reposource.ResolvedSourceReference) string {
	if source.RevisionID == nil {
		return reposource.LocalProvenanceWarning
	}
	return ""
}

func stageRecordsFromWorkflow(runID string, workflowPath string, workflowDefinition workflow.Workflow, createdAt time.Time) []persistence.WorkflowStageRecord {
	stages := make([]persistence.WorkflowStageRecord, 0, len(workflowDefinition.Steps))
	timestamp := createdAt.UTC().Format(time.RFC3339)
	for index, step := range workflowDefinition.Steps {
		stages = append(stages, persistence.WorkflowStageRecord{
			RunID:                runID,
			StageIndex:           index,
			StepID:               step.ID,
			StageSourceReference: workflowPath + "#" + step.ID,
			State:                "ready",
			CreatedAt:            timestamp,
			ReadyAt:              timestamp,
		})
	}
	return stages
}

func persistenceRecordsFromCompiledWorkflow(runID string, workflowDefinition workflow.Workflow, compileResult workflow.CompileResult, codeVersion string, submittedAt time.Time) ([]persistence.WorkItemRecord, []persistence.QueuedWorkRecord, error) {
	items, err := workItemsWithRuntimeMetadata(compileResult.WorkflowID, compileResult.WorkItems, codeVersion)
	if err != nil {
		return nil, nil, err
	}
	stageIndexes := make(map[string]int, len(workflowDefinition.Steps))
	nextWorkItemIndex := make(map[int]int, len(workflowDefinition.Steps))
	for index, step := range workflowDefinition.Steps {
		stageIndexes[step.ID] = index
	}

	records := make([]persistence.WorkItemRecord, 0, len(items))
	queued := make([]persistence.QueuedWorkRecord, 0, len(items))
	timestamp := submittedAt.UTC().Format(time.RFC3339)
	for index, item := range items {
		stageIndex, ok := stageIndexes[compileResult.WorkItems[index].StepID]
		if !ok {
			return nil, nil, fmt.Errorf("compiled work item references unknown step: %s", compileResult.WorkItems[index].StepID)
		}
		workItemIndex := nextWorkItemIndex[stageIndex]
		nextWorkItemIndex[stageIndex]++

		payload, err := json.Marshal(item)
		if err != nil {
			return nil, nil, fmt.Errorf("encode workflow work item: %w", err)
		}
		_, resolvedInputsSHA256, err := canonicalSourceDocument(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("hash workflow work item: %w", err)
		}
		record := persistence.WorkItemRecord{
			ID:                   runID + ":" + item.ID,
			RunID:                runID,
			StageIndex:           stageIndex,
			WorkItemIndex:        workItemIndex,
			WorkerPayloadJSON:    string(payload),
			ResolvedInputsSHA256: resolvedInputsSHA256,
			CreatedAt:            timestamp,
		}
		records = append(records, record)
		queued = append(queued, persistence.QueuedWorkRecord{
			WorkItemRecord: record,
			QueuedAt:       timestamp,
		})
	}
	return records, queued, nil
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

func workItemsWithRuntimeMetadata(workflowID string, compiledItems []workflow.CompiledWorkItem, codeVersion string) ([]model.WorkItem, error) {
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
		var err error
		item, err = item.WithExecutionEnvelope()
		if err != nil {
			return nil, fmt.Errorf("build execution envelope for work item %s: %w", item.ID, err)
		}
		item.WorkItemFingerprint = fingerprint("work-item", map[string]any{
			"id":              item.ID,
			"type":            item.Type,
			"output_filename": item.OutputFilename,
			"variables":       item.ExecutionEnvelope.Variables,
		})
		item.InputFingerprint = fingerprint("input", item.ExecutionEnvelope.Variables)
		item.OutputFingerprint = fingerprint("output", map[string]any{
			"output_filename": item.OutputFilename,
		})
		item.CodeVersion = codeVersion
		items = append(items, item)
	}

	return items, nil
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

func (c *Controller) submissionStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}

	submissionID, ok := submissionIDFromStatusPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	status, found, err := c.submissionStatus(r.Context(), submissionID)
	if err != nil {
		http.Error(w, "query submission status", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	artifactOutputs, err := c.submissionArtifactOutputs(r.Context(), submissionID)
	if err != nil {
		http.Error(w, "query submission artifact outputs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(submissionStatusResponse{
		SubmissionStatus: status,
		ArtifactOutputs:  artifactOutputs,
	}); err != nil {
		http.Error(w, "encode submission status", http.StatusInternalServerError)
	}
}

type submissionStatusResponse struct {
	model.SubmissionStatus
	ArtifactOutputs []submissionArtifactOutput `json:"artifact_outputs,omitempty"`
}

type submissionArtifactOutput struct {
	WorkItemID    string   `json:"work_item_id"`
	ArtifactCount int      `json:"artifact_count"`
	ArtifactNames []string `json:"artifact_names"`
	StorageScope  string   `json:"storage_scope"`
}

func (c *Controller) controllerStatus(ctx context.Context) (model.ControllerStatus, error) {
	if c.workflowStore == nil {
		return model.ControllerStatus{}, fmt.Errorf("workflow store required")
	}

	return c.persistenceControllerStatus(ctx)
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
	checks, err := c.workflowStore.ListQueuedResourceConstraintChecks(ctx)
	if err != nil {
		return model.ControllerStatus{}, fmt.Errorf("list queued resource checks: %w", err)
	}

	status := model.ControllerStatus{
		Pending:  len(queued),
		Assigned: len(running),
	}
	resourceConstraintChecks := make(map[string][]resourceConstraintCheck, len(checks))
	resourceConstraintSummaries := make(map[string]resourceConstraintSummary, len(checks))

	for _, check := range checks {
		modelCheck := model.ResourceConstraintCheck{
			TotalUnits:     check.TotalUnits,
			RequestedUnits: check.RequestedUnits,
			Operator:       model.ResourceOperator(check.Operator),
			TargetUnits:    check.TargetUnits,
		}
		resourceConstraintChecks[check.WorkItemID] = append(resourceConstraintChecks[check.WorkItemID], resourceConstraintCheck{
			resourceKey: check.ResourceKey,
			check:       modelCheck,
		})
		resourceConstraintSummaries[check.ResourceKey] = resourceConstraintSummary{
			resourceKey: check.ResourceKey,
			totalUnits:  check.TotalUnits,
			blocked:     make(map[string]struct{}),
		}
	}

	resourceConstrainedQueued := 0
	for _, item := range queued {
		checks, hasConstraints := resourceConstraintChecks[item.ID]
		if !hasConstraints {
			continue
		}
		resourceConstrainedQueued++

		allowed, blockedKeys, err := resourceConstraintChecksAllow(checks)
		if err != nil {
			return model.ControllerStatus{}, fmt.Errorf("queued resource checks for work item %s: %w", item.ID, err)
		}
		if allowed {
			status.QueuedResourceEligibleCount++
			continue
		}
		status.QueuedResourceBlockedCount++
		for _, key := range blockedKeys {
			summary := resourceConstraintSummaries[key]
			summary.blocked[item.ID] = struct{}{}
			resourceConstraintSummaries[key] = summary
		}
	}

	runningClaimed := 0
	for _, work := range running {
		records, err := c.workflowStore.ListWorkItemResourceConstraints(ctx, work.WorkItem.ID)
		if err != nil {
			return model.ControllerStatus{}, fmt.Errorf("list running resource constraints for work item %s: %w", work.WorkItem.ID, err)
		}
		if len(records) == 0 {
			continue
		}
		runningClaimed++
	}
	status.RunningResourceClaimCount = runningClaimed

	if resourceConstrainedQueued > 0 || status.RunningResourceClaimCount > 0 {
		summaries := summarizeResourceConstraints(resourceConstraintSummaries)
		if len(summaries) > maxResourceConstraintSummaries {
			summaries = summaries[:maxResourceConstraintSummaries]
		}
		if len(summaries) > 0 {
			status.ResourceConstraintSummaries = summaries
		}
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

type resourceConstraintCheck struct {
	resourceKey string
	check       model.ResourceConstraintCheck
}

type resourceConstraintSummary struct {
	resourceKey string
	totalUnits  int64
	blocked     map[string]struct{}
}

func resourceConstraintChecksAllow(checks []resourceConstraintCheck) (bool, []string, error) {
	blocked := make(map[string]struct{}, len(checks))
	for _, check := range checks {
		allowed, err := model.ResourceConstraintAllows(
			check.check.TotalUnits,
			check.check.RequestedUnits,
			check.check.Operator,
			check.check.TargetUnits,
		)
		if err != nil {
			return false, nil, err
		}
		if !allowed {
			blocked[check.resourceKey] = struct{}{}
		}
	}

	if len(blocked) == 0 {
		return true, nil, nil
	}
	keys := make([]string, 0, len(blocked))
	for key := range blocked {
		keys = append(keys, key)
	}
	return false, keys, nil
}

func summarizeResourceConstraints(summaries map[string]resourceConstraintSummary) []model.ResourceConstraintSummary {
	reduced := make([]model.ResourceConstraintSummary, 0, len(summaries))
	for resourceKey, summary := range summaries {
		blocked := len(summary.blocked)
		if blocked == 0 {
			continue
		}
		reduced = append(reduced, model.ResourceConstraintSummary{
			ResourceKey:           resourceKey,
			TotalUnits:            summary.totalUnits,
			BlockedCandidateCount: blocked,
		})
	}
	sort.Slice(reduced, func(i, j int) bool {
		left := reduced[i]
		right := reduced[j]
		if left.BlockedCandidateCount != right.BlockedCandidateCount {
			return left.BlockedCandidateCount > right.BlockedCandidateCount
		}
		if left.TotalUnits != right.TotalUnits {
			return left.TotalUnits > right.TotalUnits
		}
		return left.ResourceKey < right.ResourceKey
	})

	return reduced
}

func (c *Controller) submissionStatus(ctx context.Context, submissionID string) (model.SubmissionStatus, bool, error) {
	if c.workflowStore == nil {
		return model.SubmissionStatus{}, false, fmt.Errorf("workflow store required")
	}

	run, found, err := c.workflowStore.GetWorkflowRun(ctx, submissionID)
	if err != nil {
		return model.SubmissionStatus{}, false, fmt.Errorf("get workflow run %s: %w", submissionID, err)
	}
	if !found {
		return model.SubmissionStatus{}, false, nil
	}

	counts, err := c.workflowStore.CountWorkItemsForRun(ctx, run.ID)
	if err != nil {
		return model.SubmissionStatus{}, false, fmt.Errorf("count work items for run %s: %w", run.ID, err)
	}
	terminalAttempts, err := c.workflowStore.ListTerminalAttemptsForRun(ctx, run.ID)
	if err != nil {
		return model.SubmissionStatus{}, false, fmt.Errorf("list terminal attempts for run %s: %w", run.ID, err)
	}

	skipped := 0
	for _, attempt := range terminalAttempts {
		if attempt.TerminalState == "completed" && attempt.SkippedParentID != "" {
			skipped++
		}
	}

	completed := counts.Completed - skipped
	if completed < 0 {
		completed = 0
	}

	status := submissionStatusName(counts.Queued, counts.Running, counts.Completed, counts.Failed)
	var dependencyStatus *model.SubmissionDependencyStatus
	if plan, found, err := c.getWorkflowDependencyState(ctx, run.ID); err != nil {
		return model.SubmissionStatus{}, false, err
	} else if found {
		dependencyStatus = submissionDependencyStatusFromPlan(*plan)
		switch plan.State {
		case model.WorkflowStateFailed:
			status = "failed"
		case model.WorkflowStateCompleted:
			status = "completed"
		case model.WorkflowStateRunning:
			if status == "completed" || status == "unknown" {
				status = "running"
			}
		}
	}

	return model.SubmissionStatus{
		SubmissionID:   run.ID,
		WorkflowID:     run.WorkflowID,
		Status:         status,
		KnownWorkItems: counts.Queued + counts.Running + counts.Completed + counts.Failed,
		Queued:         counts.Queued,
		Running:        counts.Running,
		Completed:      completed,
		Failed:         counts.Failed,
		Skipped:        skipped,
		Dependency:     dependencyStatus,
	}, true, nil
}

func (c *Controller) submissionArtifactOutputs(ctx context.Context, submissionID string) ([]submissionArtifactOutput, error) {
	if c.workflowStore == nil {
		return nil, fmt.Errorf("workflow store required")
	}

	attempts, err := c.workflowStore.ListTerminalAttemptsForRun(ctx, submissionID)
	if err != nil {
		return nil, fmt.Errorf("list terminal attempts for run %s: %w", submissionID, err)
	}

	summaries := make([]submissionArtifactOutput, 0)
	for _, attempt := range attempts {
		if attempt.TerminalState != "completed" || attempt.SkippedParentID != "" || strings.TrimSpace(attempt.OutputJSON) == "" {
			continue
		}
		manifest, found, err := artifactManifestFromOutputJSON(attempt.OutputJSON)
		if err != nil {
			return nil, fmt.Errorf("work item %s output: %w", attempt.WorkItem.ID, err)
		}
		if !found {
			continue
		}

		names := make([]string, 0, len(manifest.Artifacts))
		for _, artifact := range manifest.Artifacts {
			names = append(names, artifact.Name)
		}
		summaries = append(summaries, submissionArtifactOutput{
			WorkItemID:    attempt.WorkItem.ID,
			ArtifactCount: len(manifest.Artifacts),
			ArtifactNames: names,
			StorageScope:  manifest.StorageScope,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].WorkItemID < summaries[j].WorkItemID
	})
	return summaries, nil
}

func submissionDependencyStatusFromPlan(plan model.WorkflowDependencyPlan) *model.SubmissionDependencyStatus {
	status := &model.SubmissionDependencyStatus{
		WorkflowState: string(plan.State),
		StageCount:    len(plan.Stages),
		Stages:        make([]model.SubmissionDependencyStageStatus, 0, len(plan.Stages)),
	}
	if plan.State == model.WorkflowStateRunning {
		if current, ok := currentDependencyStageIndex(plan); ok {
			status.CurrentStageIndex = &current
		}
	}
	if plan.State == model.WorkflowStateFailed {
		status.Failed = failedDependencyStatus(plan)
	}

	stages := append([]model.WorkflowDependencyStage(nil), plan.Stages...)
	sortStagesByIndex(stages)
	for _, stage := range stages {
		stageStatus := model.SubmissionDependencyStageStatus{
			StageIndex:   stage.StageIndex,
			State:        string(stage.State),
			ParallelWith: stage.ParallelWith,
			StepCount:    len(stage.Steps),
			Steps:        make([]model.SubmissionDependencyStepStatus, 0, len(stage.Steps)),
		}
		steps := append([]model.WorkflowDependencyStep(nil), stage.Steps...)
		sortStepsByIndex(steps)
		for _, step := range steps {
			stepStatus := model.SubmissionDependencyStepStatus{
				StageIndex: step.StageIndex,
				StepIndex:  step.StepIndex,
				StepID:     step.StepID,
				State:      string(step.State),
				Counts:     dependencyCountsForStep(stage.State, step),
			}
			addDependencyCounts(&stageStatus.Counts, stepStatus.Counts)
			stageStatus.Steps = append(stageStatus.Steps, stepStatus)
		}
		addDependencyCounts(&status.Counts, stageStatus.Counts)
		status.Stages = append(status.Stages, stageStatus)
	}
	return status
}

func currentDependencyStageIndex(plan model.WorkflowDependencyPlan) (int, bool) {
	stages := append([]model.WorkflowDependencyStage(nil), plan.Stages...)
	sortStagesByIndex(stages)
	for _, stage := range stages {
		if stage.State != model.WorkflowStageStateCompleted {
			return stage.StageIndex, true
		}
	}
	return 0, false
}

func dependencyCountsForStep(stageState model.WorkflowStageState, step model.WorkflowDependencyStep) model.SubmissionDependencyCounts {
	counts := model.SubmissionDependencyCounts{}
	if len(step.WorkItems) == 0 {
		if step.State == model.WorkflowStepStateBlocked || stageState == model.WorkflowStageStateBlocked {
			counts.BlockedFuture++
		}
		return counts
	}
	for _, item := range step.WorkItems {
		switch item.State {
		case model.WorkItemMembershipStateQueued:
			if stageState == model.WorkflowStageStateBlocked {
				counts.BlockedFuture++
			} else {
				counts.AssignablePending++
			}
		case model.WorkItemMembershipStateRunning:
			counts.Active++
		case model.WorkItemMembershipStateCompleted:
			counts.Completed++
		case model.WorkItemMembershipStateFailed:
			counts.Failed++
		case model.WorkItemMembershipStateSkipped:
			counts.Skipped++
		}
	}
	return counts
}

func addDependencyCounts(total *model.SubmissionDependencyCounts, next model.SubmissionDependencyCounts) {
	total.AssignablePending += next.AssignablePending
	total.BlockedFuture += next.BlockedFuture
	total.Active += next.Active
	total.Completed += next.Completed
	total.Failed += next.Failed
	total.Skipped += next.Skipped
}

func failedDependencyStatus(plan model.WorkflowDependencyPlan) *model.SubmissionDependencyFailure {
	failure := &model.SubmissionDependencyFailure{
		StageIndex:    -1,
		FailureReason: plan.FailureReason,
	}
	for _, stage := range plan.Stages {
		if stage.State != model.WorkflowStageStateFailed {
			continue
		}
		failure.StageIndex = stage.StageIndex
		if failure.FailureReason == "" {
			failure.FailureReason = stage.FailureReason
		}
		for _, step := range stage.Steps {
			if step.State != model.WorkflowStepStateFailed {
				continue
			}
			stepIndex := step.StepIndex
			failure.StepIndex = &stepIndex
			failure.StepID = step.StepID
			if failure.FailureReason == "" {
				failure.FailureReason = step.FailureReason
			}
			for _, item := range step.WorkItems {
				if item.State != model.WorkItemMembershipStateFailed {
					continue
				}
				failure.WorkItemID = item.WorkItemID
				if failure.FailureReason == "" {
					failure.FailureReason = item.FailureReason
				}
				break
			}
			break
		}
		break
	}
	if failure.FailureReason == "" {
		failure.FailureReason = "dependency workflow failed"
	}
	return failure
}

func submissionStatusName(queued int, running int, completed int, failed int) string {
	switch {
	case running > 0:
		return "running"
	case queued > 0:
		return "queued"
	case failed > 0:
		return "failed"
	case completed > 0:
		return "completed"
	default:
		return "unknown"
	}
}

func submissionIDFromStatusPath(path string) (string, bool) {
	return submissionIDFromSubmissionPath(path, "/status")
}

func submissionIDFromLogsPath(path string) (string, bool) {
	return submissionIDFromSubmissionPath(path, "/logs")
}

func submissionIDFromSubmissionPath(path string, suffix string) (string, bool) {
	const prefix = "/submissions/"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}

	submissionID := strings.TrimPrefix(path, prefix)
	submissionID = strings.TrimSuffix(submissionID, suffix)
	if submissionID == "" || strings.Contains(submissionID, "/") {
		return "", false
	}

	return submissionID, true
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
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	c.failPersistedWorkHandler(w, r, failure)
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
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	c.completePersistedWorkHandler(w, r, completion)
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

	failed, found, err := c.workflowStore.FailAttempt(r.Context(), persistence.FailAttemptRequest{
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
	workItem, found, err := c.workflowStore.GetWorkItem(r.Context(), failed.WorkItemID)
	if err != nil {
		http.Error(w, "get failed work item", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "failed work item not found", http.StatusInternalServerError)
		return
	}
	if err := c.failCacheDataDependents(r.Context(), workItem, failure.Error); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := c.recordWorkItemDependencyFailure(r.Context(), failed.WorkItemID, failure.Error); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println("persisted work item failed:", failure.ID, failure.AttemptID, failure.Error)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) completePersistedWorkHandler(w http.ResponseWriter, r *http.Request, completion model.WorkCompletion) {
	if err := validateCompletedWorkOutputJSONSize(completion.OutputJSON); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	request, err := completeAttemptRequestFromCompletion(completion, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCompletedWorkOutputJSONSize(request.OutputJSON); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	completed, found, err := c.workflowStore.CompleteAttempt(r.Context(), request)
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
	workItem, found, err := c.workflowStore.GetWorkItem(r.Context(), completed.WorkItemID)
	if err != nil {
		http.Error(w, "get completed work item", http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, "completed work item not found", http.StatusInternalServerError)
		return
	}
	var completedPayload model.WorkItem
	if err := json.Unmarshal([]byte(workItem.WorkerPayloadJSON), &completedPayload); err != nil {
		http.Error(w, "decode completed worker payload", http.StatusInternalServerError)
		return
	}
	if completedPayload.Type == model.WorkItemTypeCacheData {
		if err := c.enqueueReadyCacheDataDependents(r.Context(), workItem, activationTimeFromCompletedWork(completed)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Println("persisted cache_data work item completed:", completion.ID, completion.AttemptID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if completion.OutputJSON != "" {
		if err := c.RecordCompletedWorkItemOutput(r.Context(), RecordCompletedWorkItemOutputRequest{
			SubmissionID:     workItem.RunID,
			WorkItemID:       workItem.ID,
			OutputJSON:       completed.OutputJSON,
			OutputJSONSHA256: completed.OutputJSONSHA256,
		}); err != nil {
			if failErr := c.RecordWorkItemTerminalFailure(r.Context(), workItem.RunID, workItem.ID, err.Error()); failErr != nil {
				http.Error(w, fmt.Sprintf("%s; additionally failed to mark dependency output capture failure: %v", err.Error(), failErr), http.StatusInternalServerError)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	terminalState := model.WorkItemMembershipStateCompleted
	if completion.Skipped {
		terminalState = model.WorkItemMembershipStateSkipped
	}
	if err := c.recordWorkItemDependencyTerminal(r.Context(), completed.WorkItemID, terminalState); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := c.enqueueReadyCacheDataDependents(r.Context(), workItem, activationTimeFromCompletedWork(completed)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if terminalState == model.WorkItemMembershipStateCompleted || terminalState == model.WorkItemMembershipStateSkipped {
		if err := c.activateNextReadyWorkflowStage(r.Context(), workItem.RunID, workItem.StageIndex, activationTimeFromCompletedWork(completed)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	fmt.Println("persisted work item completed:", completion.ID, completion.AttemptID)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) recordWorkItemDependencyTerminal(ctx context.Context, workItemID string, terminalState model.WorkItemMembershipState) error {
	if c.workflowStore == nil {
		return nil
	}
	workItem, found, err := c.workflowStore.GetWorkItem(ctx, workItemID)
	if err != nil {
		return fmt.Errorf("get work item %s: %w", workItemID, err)
	}
	if !found {
		return fmt.Errorf("work item %s not found", workItemID)
	}
	return c.RecordWorkItemTerminalState(ctx, workItem.RunID, workItem.ID, terminalState)
}

func (c *Controller) recordWorkItemDependencyFailure(ctx context.Context, workItemID string, reason string) error {
	if c.workflowStore == nil {
		return nil
	}
	workItem, found, err := c.workflowStore.GetWorkItem(ctx, workItemID)
	if err != nil {
		return fmt.Errorf("get work item %s: %w", workItemID, err)
	}
	if !found {
		return fmt.Errorf("work item %s not found", workItemID)
	}
	return c.RecordWorkItemTerminalFailure(ctx, workItem.RunID, workItem.ID, reason)
}

func completeAttemptRequestFromCompletion(completion model.WorkCompletion, fallbackCompletedAt time.Time) (persistence.CompleteAttemptRequest, error) {
	if completion.AttemptID == "" {
		return persistence.CompleteAttemptRequest{}, fmt.Errorf("attempt_id is required")
	}
	if completion.Skipped && completion.SkippedParentID == "" {
		return persistence.CompleteAttemptRequest{}, fmt.Errorf("skipped_parent_id is required when skipped is true")
	}

	outputJSON, outputJSONSHA256, err := canonicalJSONTextAndHash("output_json", completion.OutputJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	if err := validateArtifactManifestOutputJSON(outputJSON); err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	_, preStateEvidenceSHA256, err := canonicalJSONTextAndHash("pre_state_json", completion.PreStateJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	_, postStateEvidenceSHA256, err := canonicalJSONTextAndHash("post_state_json", completion.PostStateJSON)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	preStateSHA256, err := reportedOrEvidenceSHA256("pre_state_sha256", completion.PreStateSHA256, preStateEvidenceSHA256)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}
	postStateSHA256, err := reportedOrEvidenceSHA256("post_state_sha256", completion.PostStateSHA256, postStateEvidenceSHA256)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}

	completedAt, err := reportTimestamp("completed_at", completion.CompletedAt, fallbackCompletedAt)
	if err != nil {
		return persistence.CompleteAttemptRequest{}, err
	}

	return persistence.CompleteAttemptRequest{
		AttemptID:        completion.AttemptID,
		SkippedParentID:  completion.SkippedParentID,
		OutputJSON:       outputJSON,
		OutputJSONSHA256: outputJSONSHA256,
		PreStateSHA256:   preStateSHA256,
		PostStateSHA256:  postStateSHA256,
		CompletedAt:      completedAt,
	}, nil
}

func reportedOrEvidenceSHA256(name string, reported string, evidence string) (string, error) {
	if reported == "" {
		return evidence, nil
	}
	if err := fp.ValidateSHA256Hex(reported); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return reported, nil
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
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	c.nextPersistedWorkHandler(w, r)
}

func (c *Controller) nextPersistedWorkHandler(w http.ResponseWriter, r *http.Request) {
	claim, found, err := func() (persistence.ClaimedWorkRecord, bool, error) {
		c.claimMu.Lock()
		defer c.claimMu.Unlock()

		return c.workflowStore.ClaimNextWork(r.Context(), persistence.ClaimWorkRequest{
			AttemptID:    "attempt-" + randomHex(16),
			ExecutorType: persistence.ExecutorTypeWorker,
			StartedAt:    time.Now().UTC().Format(time.RFC3339),
		})
	}()
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
	item.ReuseCandidates, err = c.persistedReuseCandidates(r.Context(), claim)
	if err != nil {
		http.Error(w, "load reuse candidates", http.StatusInternalServerError)
		return
	}
	if item.Type == model.WorkItemTypeCommitData {
		item, err = c.hydrateCommitDataWorkItem(r.Context(), claim, item)
		if err != nil {
			http.Error(w, "hydrate commit_data work item", http.StatusInternalServerError)
			return
		}
	}
	if item.Type != model.WorkItemTypeCacheData && item.Type != model.WorkItemTypeCommitData {
		item, err = c.hydrateCacheDataDependentWorkItem(r.Context(), claim, item)
		if err != nil {
			http.Error(w, "hydrate cache_data dependent work item", http.StatusInternalServerError)
			return
		}
	}
	item, err = item.WithExecutionEnvelope()
	if err != nil {
		http.Error(w, "build execution envelope", http.StatusInternalServerError)
		return
	}
	if err := item.Validate(); err != nil {
		http.Error(w, "validate persisted worker payload", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(item); err != nil {
		http.Error(w, "encode work item", http.StatusInternalServerError)
	}
}

func (c *Controller) hydrateCommitDataWorkItem(ctx context.Context, claim persistence.ClaimedWorkRecord, item model.WorkItem) (model.WorkItem, error) {
	if _, ok := item.Parameters["artifact_manifest"]; ok {
		return item, nil
	}
	parameter, ok := item.Parameters["commit_data"]
	if !ok {
		return model.WorkItem{}, fmt.Errorf("commit_data parameter is required")
	}
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		return model.WorkItem{}, fmt.Errorf("encode commit_data parameter: %w", err)
	}
	var payload model.CommitDataWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return model.WorkItem{}, fmt.Errorf("decode commit_data parameter: %w", err)
	}
	if err := payload.Validate(); err != nil {
		return model.WorkItem{}, err
	}

	sourceID := persistedDependencyID(item.DependsOn, payload.Source.FromWorkItemID)
	if sourceID == "" {
		return model.WorkItem{}, fmt.Errorf("commit_data source %q is not listed in depends_on", payload.Source.FromWorkItemID)
	}
	terminals, err := c.workflowStore.ListTerminalAttemptsForRun(ctx, claim.WorkItem.RunID)
	if err != nil {
		return model.WorkItem{}, err
	}
	for _, terminal := range terminals {
		if terminal.TerminalState != "completed" || terminal.WorkItem.ID != sourceID {
			continue
		}
		manifest, found, err := artifactManifestFromOutputJSON(terminal.OutputJSON)
		if err != nil {
			return model.WorkItem{}, err
		}
		if !found {
			return model.WorkItem{}, fmt.Errorf("completed source %s output is not an artifact manifest", sourceID)
		}
		item.Parameters["artifact_manifest"] = model.Parameter{
			Type:  "artifact_manifest",
			Value: manifest,
		}
		return item, nil
	}
	return model.WorkItem{}, fmt.Errorf("completed source %s not found for commit_data", sourceID)
}

func persistedDependencyID(dependsOn []string, sourceWorkItemID string) string {
	for _, dependencyID := range dependsOn {
		if dependencyID == sourceWorkItemID || strings.HasSuffix(dependencyID, ":"+sourceWorkItemID) {
			return dependencyID
		}
	}
	return ""
}

func (c *Controller) persistedReuseCandidates(ctx context.Context, claim persistence.ClaimedWorkRecord) ([]model.WorkReuseCandidate, error) {
	terminals, err := c.workflowStore.ListTerminalAttemptsForRun(ctx, claim.WorkItem.RunID)
	if err != nil {
		return nil, err
	}
	candidates := make([]model.WorkReuseCandidate, 0)
	for _, terminal := range terminals {
		if terminal.TerminalState != "completed" {
			continue
		}
		if terminal.AttemptID == claim.AttemptID {
			continue
		}
		if terminal.WorkItem.ResolvedInputsSHA256 != claim.WorkItem.ResolvedInputsSHA256 {
			continue
		}
		if terminal.WorkItem.WorkerPayloadJSON != claim.WorkItem.WorkerPayloadJSON {
			continue
		}
		candidate := model.WorkReuseCandidate{
			AttemptID:        terminal.AttemptID,
			OutputJSONSHA256: terminal.OutputJSONSHA256,
			PreStateSHA256:   terminal.PreStateSHA256,
			PostStateSHA256:  terminal.PostStateSHA256,
		}
		hashes, err := workerObservedHashesFromOutputJSON(terminal.OutputJSON)
		if err != nil {
			return nil, err
		}
		candidate.InputSHA256 = hashes.InputSHA256
		candidate.OutputSHA256 = hashes.OutputSHA256
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

type workerObservedHashes struct {
	InputSHA256  string `json:"input_sha256"`
	OutputSHA256 string `json:"output_sha256"`
}

func workerObservedHashesFromOutputJSON(outputJSON string) (workerObservedHashes, error) {
	if outputJSON == "" {
		return workerObservedHashes{}, nil
	}
	var hashes workerObservedHashes
	if err := json.Unmarshal([]byte(outputJSON), &hashes); err != nil {
		return workerObservedHashes{}, fmt.Errorf("decode worker observed hashes: %w", err)
	}
	return hashes, nil
}

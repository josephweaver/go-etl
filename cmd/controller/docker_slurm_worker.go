package main

import (
	"context"
	"fmt"

	"goetl/internal/variable"
)

type DockerSlurmWorkerStarter struct {
	Submit func(context.Context, DockerSlurmScriptConfig) (string, error)
}

func (s DockerSlurmWorkerStarter) StartWorker(targetEnvironment string, resolver variable.Resolver) error {
	if targetEnvironment != "docker_slurm" {
		return fmt.Errorf("unsupported worker target environment: %s", targetEnvironment)
	}

	cfg, err := dockerSlurmWorkerScriptConfig(resolver)
	if err != nil {
		return err
	}

	script, err := GenerateSlurmWorkerScript(cfg.slurm)
	if err != nil {
		return err
	}

	submit := s.Submit
	if submit == nil {
		submit = WriteAndSubmitDockerSlurmScript
	}
	_, err = submit(context.Background(), DockerSlurmScriptConfig{
		DockerExecutable: cfg.dockerExecutable,
		SlurmContainer:   cfg.slurmContainer,
		ScriptPath:       cfg.scriptPath,
		Script:           script,
	})
	return err
}

type dockerSlurmWorkerConfig struct {
	dockerExecutable string
	slurmContainer   string
	scriptPath       string
	slurm            SlurmWorkerScriptConfig
}

func dockerSlurmWorkerScriptConfig(resolver variable.Resolver) (dockerSlurmWorkerConfig, error) {
	transportSettings, err := optionalWorkerConfigSettings(resolver, "transport")
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	schedulerSettings, err := optionalWorkerConfigSettings(resolver, "scheduler")
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	runtimeSettings, err := optionalWorkerConfigSettings(resolver, "runtime")
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}

	jobName, ok, err := optionalObjectString(schedulerSettings, "job_name")
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if !ok {
		jobName, _, err = optionalControllerStringVariable(resolver, "worker_slurm_job_name", "goetl-worker")
	}
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}

	workerArgs, ok, err := optionalObjectStringList(runtimeSettings, "args")
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if !ok {
		workerArgs, err = optionalControllerStringListVariable(resolver, "worker_start_args")
	}
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}

	dockerExecutable, ok, err := optionalObjectString(transportSettings, "executable")
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if !ok {
		dockerExecutable, _, err = optionalControllerStringVariable(resolver, "docker_executable", "")
	}
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	slurmContainer, ok, err := optionalObjectString(transportSettings, "container")
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if !ok {
		slurmContainer, _, err = optionalControllerStringVariable(resolver, "docker_slurm_container", "")
	}
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}

	cfg := dockerSlurmWorkerConfig{
		dockerExecutable: dockerExecutable,
		slurmContainer:   slurmContainer,
		scriptPath:       "",
		slurm: SlurmWorkerScriptConfig{
			JobName:          jobName,
			WorkerExecutable: "",
			WorkerArgs:       workerArgs,
			WorkerConfigPath: "",
			LogDir:           "",
		},
	}

	if cfg.scriptPath, ok, err = optionalObjectString(schedulerSettings, "script_path"); err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if !ok {
		cfg.scriptPath, err = workerScriptPath(resolver)
	}
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if cfg.slurm.WorkerExecutable, ok, err = optionalObjectString(runtimeSettings, "executable"); err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if !ok {
		cfg.slurm.WorkerExecutable, err = controllerStringVariable(resolver, "worker_start_executable")
	}
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if cfg.slurm.WorkerConfigPath, ok, err = optionalObjectString(runtimeSettings, "config_path"); err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if !ok {
		cfg.slurm.WorkerConfigPath, err = controllerPathOrStringVariable(resolver, "worker_config_path")
	}
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if cfg.slurm.LogDir, ok, err = optionalObjectString(runtimeSettings, "log_dir"); err != nil {
		return dockerSlurmWorkerConfig{}, err
	}
	if !ok {
		cfg.slurm.LogDir, err = controllerPathOrStringVariable(resolver, "worker_log_dir")
	}
	if err != nil {
		return dockerSlurmWorkerConfig{}, err
	}

	return cfg, nil
}

func optionalWorkerConfigSettings(resolver variable.Resolver, name string) (map[string]variable.ResolvedValue, error) {
	object, ok, err := optionalObjectVariable(resolver, name)
	if err != nil || !ok {
		return nil, err
	}

	settings, ok := object["settings"]
	if !ok {
		return object, nil
	}
	if settings.Type != variable.TypeObject {
		return nil, fmt.Errorf("%s.settings has type %s, want object", name, settings.Type)
	}
	return settings.Object, nil
}

func optionalObjectVariable(resolver variable.Resolver, name string) (map[string]variable.ResolvedValue, bool, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return nil, false, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return nil, false, nil
	}
	if value.Type != variable.TypeObject {
		return nil, false, fmt.Errorf("%s has type %s, want object", name, value.Type)
	}
	return value.Object, true, nil
}

func optionalObjectString(fields map[string]variable.ResolvedValue, name string) (string, bool, error) {
	if fields == nil {
		return "", false, nil
	}
	value, ok := fields[name]
	if !ok {
		return "", false, nil
	}
	if value.Type != variable.TypeString && value.Type != variable.TypePath {
		return "", false, fmt.Errorf("%s has type %s, want string or path", name, value.Type)
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", false, fmt.Errorf("%s is required", name)
	}
	return text, true, nil
}

func optionalObjectStringList(fields map[string]variable.ResolvedValue, name string) ([]string, bool, error) {
	if fields == nil {
		return nil, false, nil
	}
	value, ok := fields[name]
	if !ok {
		return nil, false, nil
	}
	if value.Type.String() != variable.TypeList(variable.TypeString).String() {
		return nil, false, fmt.Errorf("%s has type %s, want list[string]", name, value.Type)
	}
	values := make([]string, 0, len(value.List))
	for index, item := range value.List {
		text, ok := item.Value.(string)
		if !ok || text == "" {
			return nil, false, fmt.Errorf("%s[%d] is required", name, index)
		}
		values = append(values, text)
	}
	return values, true, nil
}

func workerScriptPath(resolver variable.Resolver) (string, error) {
	path, err := controllerPathOrStringVariable(resolver, "worker_script_path")
	if err == nil {
		return path, nil
	}

	return controllerPathOrStringVariable(resolver, "docker_slurm_script_path")
}

func optionalControllerStringVariable(resolver variable.Resolver, name string, fallback string) (string, bool, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", false, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return fallback, false, nil
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

func optionalControllerStringListVariable(resolver variable.Resolver, name string) ([]string, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return nil, err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return nil, nil
	}

	if value.Type.String() != variable.TypeList(variable.TypeString).String() {
		return nil, fmt.Errorf("%s has type %s, want list[string]", name, value.Type)
	}

	args := make([]string, 0, len(value.List))
	for index, item := range value.List {
		text, ok := item.Value.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("%s[%d] is required", name, index)
		}
		args = append(args, text)
	}

	return args, nil
}

func controllerPathOrStringVariable(resolver variable.Resolver, name string) (string, error) {
	reference, err := variable.ParseReference(name)
	if err != nil {
		return "", err
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		return "", err
	}

	if value.Type != variable.TypePath && value.Type != variable.TypeString {
		return "", fmt.Errorf("%s has type %s, want path or string", name, value.Type)
	}

	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("%s is required", name)
	}

	return text, nil
}

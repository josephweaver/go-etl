package main

import (
	"fmt"

	"goetl/internal/variable"
)

type workerLaunchConfigSpec struct {
	dockerExecutable string
	slurmContainer   string
	scriptPath       string
	slurm            SlurmWorkerScriptConfig
}

func workerLaunchConfig(resolver variable.Resolver) (workerLaunchConfigSpec, error) {
	transportSettings, err := optionalWorkerConfigSettings(resolver, "transport")
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	schedulerSettings, err := optionalWorkerConfigSettings(resolver, "scheduler")
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	runtimeSettings, err := optionalWorkerConfigSettings(resolver, "runtime")
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}

	jobName, ok, err := variable.OptionalObjectFieldString(schedulerSettings, "job_name")
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if !ok {
		jobName = "goetl-worker"
		if configured, found, findErr := resolver.OptionalString("worker_slurm_job_name"); findErr != nil {
			return workerLaunchConfigSpec{}, findErr
		} else if found {
			jobName = configured
		}
	}
	memoryMB, err := optionalWorkerConfigInt64(resolver, schedulerSettings, "memory_mb", "worker_slurm_memory_mb")
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}

	workerArgs, ok, err := variable.OptionalObjectFieldStringList(runtimeSettings, "args")
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if !ok {
		if configured, found, findErr := resolver.OptionalStringList("worker_start_args"); findErr != nil {
			return workerLaunchConfigSpec{}, findErr
		} else if found {
			workerArgs = configured
		}
	}

	dockerExecutable, ok, err := variable.OptionalObjectFieldString(transportSettings, "executable")
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if !ok {
		if configured, found, findErr := resolver.OptionalString("docker_executable"); findErr != nil {
			return workerLaunchConfigSpec{}, findErr
		} else if found {
			dockerExecutable = configured
		}
	}
	slurmContainer, ok, err := variable.OptionalObjectFieldString(transportSettings, "container")
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if !ok {
		if configured, found, findErr := resolver.OptionalString("docker_slurm_container"); findErr != nil {
			return workerLaunchConfigSpec{}, findErr
		} else if found {
			slurmContainer = configured
		}
	}

	cfg := workerLaunchConfigSpec{
		dockerExecutable: dockerExecutable,
		slurmContainer:   slurmContainer,
		scriptPath:       "",
		slurm: SlurmWorkerScriptConfig{
			JobName:          jobName,
			MemoryMB:         memoryMB,
			WorkerExecutable: "",
			WorkerArgs:       workerArgs,
			WorkerConfigPath: "",
			LogDir:           "",
		},
	}

	if cfg.scriptPath, ok, err = variable.OptionalObjectFieldString(schedulerSettings, "script_path"); err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if !ok {
		cfg.scriptPath, err = workerScriptPath(resolver)
	}
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if cfg.slurm.WorkerExecutable, ok, err = variable.OptionalObjectFieldString(runtimeSettings, "executable"); err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if !ok {
		cfg.slurm.WorkerExecutable, err = resolver.String("worker_start_executable")
	}
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if cfg.slurm.WorkerConfigPath, ok, err = variable.OptionalObjectFieldString(runtimeSettings, "config_path"); err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if !ok {
		cfg.slurm.WorkerConfigPath, err = resolver.PathOrString("worker_config_path")
	}
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if cfg.slurm.LogDir, ok, err = variable.OptionalObjectFieldString(runtimeSettings, "log_dir"); err != nil {
		return workerLaunchConfigSpec{}, err
	}
	if !ok {
		cfg.slurm.LogDir, err = resolver.PathOrString("worker_log_dir")
	}
	if err != nil {
		return workerLaunchConfigSpec{}, err
	}

	return cfg, nil
}

func optionalWorkerConfigSettings(resolver variable.Resolver, name string) (map[string]variable.ResolvedValue, error) {
	object, ok, err := resolver.OptionalObject(name)
	if err != nil || !ok {
		return nil, err
	}

	settings, ok, err := variable.OptionalObjectFieldObject(object, "settings")
	if err != nil || !ok {
		return object, err
	}
	return settings, nil
}

func optionalWorkerConfigInt64(resolver variable.Resolver, settings map[string]variable.ResolvedValue, fieldName string, fallbackName string) (int64, error) {
	if settings != nil {
		value, ok := settings[fieldName]
		if ok {
			if value.Type != variable.TypeInt {
				return 0, fmt.Errorf("%s has type %s, want int", fieldName, value.Type)
			}
			return resolvedInt64Value(value, fieldName)
		}
	}
	if value, found, err := resolver.Optional(fallbackName); err != nil {
		return 0, err
	} else if found {
		return resolvedInt64Value(value, fallbackName)
	}
	return 0, nil
}

func resolvedInt64Value(value variable.ResolvedValue, name string) (int64, error) {
	if value.Type != variable.TypeInt {
		return 0, fmt.Errorf("%s has type %s, want int", name, value.Type)
	}
	switch integer := value.Value.(type) {
	case int64:
		return integer, nil
	case int:
		return int64(integer), nil
	default:
		return 0, fmt.Errorf("%s is required", name)
	}
}

func workerScriptPath(resolver variable.Resolver) (string, error) {
	if path, ok, err := resolver.OptionalPathOrString("worker_script_path"); err != nil {
		return "", err
	} else if ok {
		return path, nil
	}

	return resolver.PathOrString("docker_slurm_script_path")
}

package main

import (
	"fmt"

	"goetl/internal/variable"
)

type DefaultWorkerStarter struct {
	Command     LocalWorkerStarter
	DockerSlurm DockerSlurmWorkerStarter
}

func (s DefaultWorkerStarter) StartWorker(targetEnvironment string, resolver variable.Resolver) error {
	switch targetEnvironment {
	case "local", "hpcc":
		return s.Command.StartWorker(targetEnvironment, resolver)
	case "docker_slurm":
		return s.DockerSlurm.StartWorker(targetEnvironment, resolver)
	default:
		return fmt.Errorf("unsupported worker target environment: %s", targetEnvironment)
	}
}

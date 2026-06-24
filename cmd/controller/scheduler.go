package main

import "context"

type Scheduler interface {
	Submit(ctx context.Context, job JobSpec) (JobHandle, error)
}

type JobSpec struct {
	Name             string
	RemoteScriptPath string
	WorkerScript     SlurmWorkerScriptConfig
}

type JobHandle struct {
	ID string
}

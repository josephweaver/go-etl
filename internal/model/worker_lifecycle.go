package model

type WorkerRegistrationRequest struct {
	ExecutionHandle      string `json:"execution_handle,omitempty"`
	ExecutionEnvironment string `json:"execution_environment,omitempty"`
}

type WorkerRegistration struct {
	WorkerID                 string `json:"worker_id"`
	WorkerSessionID          string `json:"worker_session_id"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
	DeadAfterSeconds         int    `json:"dead_after_seconds"`
}

type WorkerHeartbeatRequest struct {
	WorkerID        string `json:"worker_id"`
	WorkerSessionID string `json:"worker_session_id"`
}

type WorkerStopRequest struct {
	WorkerID        string `json:"worker_id"`
	WorkerSessionID string `json:"worker_session_id"`
	Reason          string `json:"reason"`
}

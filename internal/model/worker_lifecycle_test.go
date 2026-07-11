package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkerLifecycleModelsUseStableJSONFields(t *testing.T) {
	registration := WorkerRegistration{
		WorkerID:                 "worker-001",
		WorkerSessionID:          "session-001",
		HeartbeatIntervalSeconds: 60,
		DeadAfterSeconds:         300,
	}

	data, err := json.Marshal(registration)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(data)
	for _, field := range []string{
		`"worker_id":"worker-001"`,
		`"worker_session_id":"session-001"`,
		`"heartbeat_interval_seconds":60`,
		`"dead_after_seconds":300`,
	} {
		if !strings.Contains(text, field) {
			t.Fatalf("registration JSON %s missing %s", text, field)
		}
	}

	stop := WorkerStopRequest{
		WorkerID:        "worker-001",
		WorkerSessionID: "session-001",
		Reason:          "no_work",
	}
	data, err = json.Marshal(stop)
	if err != nil {
		t.Fatalf("Marshal(stop) error = %v", err)
	}
	text = string(data)
	if !strings.Contains(text, `"reason":"no_work"`) {
		t.Fatalf("stop JSON %s missing reason", text)
	}
}

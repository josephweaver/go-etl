package controllerauth

import (
	"net/http"
	"testing"
)

func TestControllerPolicyAuthorize(t *testing.T) {
	policy := ControllerPolicy()

	tests := []struct {
		name   string
		method string
		path   string
		role   Role
		want   Decision
	}{
		{
			name:   "health is public",
			method: http.MethodGet,
			path:   "/healthz",
			want:   DecisionPublic,
		},
		{
			name:   "client can submit workflow",
			method: http.MethodPost,
			path:   "/workflow",
			role:   RoleClient,
			want:   DecisionAllowed,
		},
		{
			name:   "admin can submit workflow",
			method: http.MethodPost,
			path:   "/workflow",
			role:   RoleAdmin,
			want:   DecisionAllowed,
		},
		{
			name:   "worker cannot submit workflow",
			method: http.MethodPost,
			path:   "/workflow",
			role:   RoleWorker,
			want:   DecisionDenied,
		},
		{
			name:   "client can submit raw work",
			method: http.MethodPost,
			path:   "/work",
			role:   RoleClient,
			want:   DecisionAllowed,
		},
		{
			name:   "worker cannot submit raw work",
			method: http.MethodPost,
			path:   "/work",
			role:   RoleWorker,
			want:   DecisionDenied,
		},
		{
			name:   "worker can claim work",
			method: http.MethodGet,
			path:   "/work/next",
			role:   RoleWorker,
			want:   DecisionAllowed,
		},
		{
			name:   "client cannot claim work",
			method: http.MethodGet,
			path:   "/work/next",
			role:   RoleClient,
			want:   DecisionDenied,
		},
		{
			name:   "worker can complete work",
			method: http.MethodPost,
			path:   "/work/complete",
			role:   RoleWorker,
			want:   DecisionAllowed,
		},
		{
			name:   "worker can fail work",
			method: http.MethodPost,
			path:   "/work/fail",
			role:   RoleWorker,
			want:   DecisionAllowed,
		},
		{
			name:   "client can read status",
			method: http.MethodGet,
			path:   "/status",
			role:   RoleClient,
			want:   DecisionAllowed,
		},
		{
			name:   "worker cannot read status",
			method: http.MethodGet,
			path:   "/status",
			role:   RoleWorker,
			want:   DecisionDenied,
		},
		{
			name:   "client can read observation logs",
			method: http.MethodGet,
			path:   "/observations/logs",
			role:   RoleClient,
			want:   DecisionAllowed,
		},
		{
			name:   "admin can shut down",
			method: http.MethodPost,
			path:   "/shutdown",
			role:   RoleAdmin,
			want:   DecisionAllowed,
		},
		{
			name:   "client cannot shut down",
			method: http.MethodPost,
			path:   "/shutdown",
			role:   RoleClient,
			want:   DecisionDenied,
		},
		{
			name:   "client can read submission status",
			method: http.MethodGet,
			path:   "/submissions/submission-1/status",
			role:   RoleClient,
			want:   DecisionAllowed,
		},
		{
			name:   "client can read submission logs",
			method: http.MethodGet,
			path:   "/submissions/submission-1/logs",
			role:   RoleClient,
			want:   DecisionAllowed,
		},
		{
			name:   "worker cannot read submission logs",
			method: http.MethodGet,
			path:   "/submissions/submission-1/logs",
			role:   RoleWorker,
			want:   DecisionDenied,
		},
		{
			name:   "worker can read source bundle",
			method: http.MethodGet,
			path:   "/workflow-runs/run-1/source-bundle.zip",
			role:   RoleWorker,
			want:   DecisionAllowed,
		},
		{
			name:   "client can read source bundle",
			method: http.MethodGet,
			path:   "/workflow-runs/run-1/source-bundle.zip",
			role:   RoleClient,
			want:   DecisionAllowed,
		},
		{
			name:   "unknown route denied",
			method: http.MethodGet,
			path:   "/unknown",
			role:   RoleAdmin,
			want:   DecisionDenied,
		},
		{
			name:   "wrong method denied",
			method: http.MethodPost,
			path:   "/status",
			role:   RoleAdmin,
			want:   DecisionDenied,
		},
		{
			name:   "unknown role denied",
			method: http.MethodGet,
			path:   "/status",
			role:   Role("auditor"),
			want:   DecisionDenied,
		},
		{
			name:   "public route wrong method denied",
			method: http.MethodPost,
			path:   "/healthz",
			role:   RoleAdmin,
			want:   DecisionDenied,
		},
		{
			name:   "family route requires id",
			method: http.MethodGet,
			path:   "/submissions//status",
			role:   RoleClient,
			want:   DecisionDenied,
		},
		{
			name:   "family route rejects nested id",
			method: http.MethodGet,
			path:   "/submissions/a/b/status",
			role:   RoleClient,
			want:   DecisionDenied,
		},
		{
			name:   "source bundle family requires run id",
			method: http.MethodGet,
			path:   "/workflow-runs//source-bundle.zip",
			role:   RoleWorker,
			want:   DecisionDenied,
		},
		{
			name:   "source bundle family rejects nested run id",
			method: http.MethodGet,
			path:   "/workflow-runs/a/b/source-bundle.zip",
			role:   RoleWorker,
			want:   DecisionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.Authorize(tt.method, tt.path, tt.role)
			if got != tt.want {
				t.Fatalf("Authorize(%q, %q, %q) = %q, want %q", tt.method, tt.path, tt.role, got, tt.want)
			}
		})
	}
}

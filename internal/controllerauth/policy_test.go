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
			name:   "unknown route not found",
			method: http.MethodGet,
			path:   "/unknown",
			role:   RoleAdmin,
			want:   DecisionNotFound,
		},
		{
			name:   "wrong method not allowed",
			method: http.MethodPost,
			path:   "/status",
			role:   RoleAdmin,
			want:   DecisionMethodNotAllowed,
		},
		{
			name:   "unknown role denied",
			method: http.MethodGet,
			path:   "/status",
			role:   Role("auditor"),
			want:   DecisionDenied,
		},
		{
			name:   "public route wrong method not allowed",
			method: http.MethodPost,
			path:   "/healthz",
			role:   RoleAdmin,
			want:   DecisionMethodNotAllowed,
		},
		{
			name:   "family route without id not found",
			method: http.MethodGet,
			path:   "/submissions//status",
			role:   RoleClient,
			want:   DecisionNotFound,
		},
		{
			name:   "family route nested id not found",
			method: http.MethodGet,
			path:   "/submissions/a/b/status",
			role:   RoleClient,
			want:   DecisionNotFound,
		},
		{
			name:   "source bundle family without run id not found",
			method: http.MethodGet,
			path:   "/workflow-runs//source-bundle.zip",
			role:   RoleWorker,
			want:   DecisionNotFound,
		},
		{
			name:   "source bundle family nested run id not found",
			method: http.MethodGet,
			path:   "/workflow-runs/a/b/source-bundle.zip",
			role:   RoleWorker,
			want:   DecisionNotFound,
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

func TestControllerPolicyClassify(t *testing.T) {
	policy := ControllerPolicy()

	tests := []struct {
		name string
		path string
		want RouteClass
	}{
		{
			name: "health is public",
			path: "/healthz",
			want: RoutePublic,
		},
		{
			name: "status is protected",
			path: "/status",
			want: RouteProtected,
		},
		{
			name: "submission status is protected",
			path: "/submissions/submission-1/status",
			want: RouteProtected,
		},
		{
			name: "unknown path is unknown",
			path: "/unknown",
			want: RouteUnknown,
		},
		{
			name: "empty family id is unknown",
			path: "/submissions//status",
			want: RouteUnknown,
		},
		{
			name: "nested family id is unknown",
			path: "/submissions/a/b/status",
			want: RouteUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.Classify(tt.path); got != tt.want {
				t.Fatalf("Classify(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestControllerPolicyRouteRoleMatrix(t *testing.T) {
	policy := ControllerPolicy()

	routes := []struct {
		name         string
		method       string
		path         string
		public       bool
		allowedRoles []Role
	}{
		{name: "health", method: http.MethodGet, path: "/healthz", public: true},
		{name: "workflow submission", method: http.MethodPost, path: "/workflow", allowedRoles: []Role{RoleClient, RoleAdmin}},
		{name: "source bundle", method: http.MethodGet, path: "/workflow-runs/run-1/source-bundle.zip", allowedRoles: []Role{RoleClient, RoleWorker, RoleAdmin}},
		{name: "submission status", method: http.MethodGet, path: "/submissions/submission-1/status", allowedRoles: []Role{RoleClient, RoleAdmin}},
		{name: "submission logs", method: http.MethodGet, path: "/submissions/submission-1/logs", allowedRoles: []Role{RoleClient, RoleAdmin}},
		{name: "raw work submission", method: http.MethodPost, path: "/work", allowedRoles: []Role{RoleClient, RoleAdmin}},
		{name: "worker claim", method: http.MethodGet, path: "/work/next", allowedRoles: []Role{RoleWorker, RoleAdmin}},
		{name: "worker complete", method: http.MethodPost, path: "/work/complete", allowedRoles: []Role{RoleWorker, RoleAdmin}},
		{name: "worker fail", method: http.MethodPost, path: "/work/fail", allowedRoles: []Role{RoleWorker, RoleAdmin}},
		{name: "controller status", method: http.MethodGet, path: "/status", allowedRoles: []Role{RoleClient, RoleAdmin}},
		{name: "observation logs", method: http.MethodGet, path: "/observations/logs", allowedRoles: []Role{RoleClient, RoleAdmin}},
		{name: "shutdown", method: http.MethodPost, path: "/shutdown", allowedRoles: []Role{RoleAdmin}},
	}
	roles := []Role{"", RoleClient, RoleWorker, RoleAdmin}

	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			for _, role := range roles {
				got := policy.Authorize(route.method, route.path, role)
				want := DecisionDenied
				if route.public {
					want = DecisionPublic
				} else if roleAllowed(role, route.allowedRoles) {
					want = DecisionAllowed
				}
				if got != want {
					t.Fatalf("Authorize(%q, %q, %q) = %q, want %q", route.method, route.path, role, got, want)
				}
			}
		})
	}
}

func TestControllerPolicyUnknownAndWrongMethodMatrix(t *testing.T) {
	policy := ControllerPolicy()
	for _, role := range []Role{"", RoleClient, RoleWorker, RoleAdmin} {
		if got := policy.Authorize(http.MethodGet, "/not-a-controller-route", role); got != DecisionNotFound {
			t.Fatalf("Authorize unknown route as %q = %q, want %q", role, got, DecisionNotFound)
		}
		if got := policy.Authorize(http.MethodDelete, "/status", role); got != DecisionMethodNotAllowed {
			t.Fatalf("Authorize wrong method as %q = %q, want %q", role, got, DecisionMethodNotAllowed)
		}
	}
}

func roleAllowed(role Role, allowed []Role) bool {
	for _, candidate := range allowed {
		if role == candidate {
			return true
		}
	}
	return false
}

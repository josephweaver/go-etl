package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"goetl/internal/controllerauth"
)

func TestAuthorizeControllerRoutes(t *testing.T) {
	controller := testAuthController(t, controllerauth.ModeBearer)

	tests := []struct {
		name        string
		method      string
		path        string
		headers     []string
		wantStatus  int
		wantCalled  bool
		wantRole    controllerauth.Role
		wantNoStore bool
	}{
		{
			name:        "protected route rejects missing credential",
			method:      http.MethodGet,
			path:        "/status",
			wantStatus:  http.StatusUnauthorized,
			wantNoStore: true,
		},
		{
			name:        "protected route rejects malformed scheme",
			method:      http.MethodGet,
			path:        "/status",
			headers:     []string{"bearer client-secret"},
			wantStatus:  http.StatusUnauthorized,
			wantNoStore: true,
		},
		{
			name:        "protected route rejects empty bearer",
			method:      http.MethodGet,
			path:        "/status",
			headers:     []string{"Bearer "},
			wantStatus:  http.StatusUnauthorized,
			wantNoStore: true,
		},
		{
			name:        "protected route rejects bearer containing space",
			method:      http.MethodGet,
			path:        "/status",
			headers:     []string{"Bearer client secret"},
			wantStatus:  http.StatusUnauthorized,
			wantNoStore: true,
		},
		{
			name:        "protected route rejects invalid token",
			method:      http.MethodGet,
			path:        "/status",
			headers:     []string{"Bearer wrong-secret"},
			wantStatus:  http.StatusUnauthorized,
			wantNoStore: true,
		},
		{
			name:        "protected route rejects duplicate header",
			method:      http.MethodGet,
			path:        "/status",
			headers:     []string{"Bearer client-secret", "Bearer admin-secret"},
			wantStatus:  http.StatusUnauthorized,
			wantNoStore: true,
		},
		{
			name:        "protected route rejects comma combined header",
			method:      http.MethodGet,
			path:        "/status",
			headers:     []string{"Bearer client-secret,Bearer admin-secret"},
			wantStatus:  http.StatusUnauthorized,
			wantNoStore: true,
		},
		{
			name:        "valid wrong role is forbidden",
			method:      http.MethodGet,
			path:        "/status",
			headers:     []string{"Bearer worker-secret"},
			wantStatus:  http.StatusForbidden,
			wantNoStore: true,
		},
		{
			name:       "client reaches client route",
			method:     http.MethodGet,
			path:       "/status",
			headers:    []string{"Bearer client-secret"},
			wantStatus: http.StatusNoContent,
			wantCalled: true,
			wantRole:   controllerauth.RoleClient,
		},
		{
			name:       "admin reaches shutdown",
			method:     http.MethodPost,
			path:       "/shutdown",
			headers:    []string{"Bearer admin-secret"},
			wantStatus: http.StatusNoContent,
			wantCalled: true,
			wantRole:   controllerauth.RoleAdmin,
		},
		{
			name:        "worker cannot submit workflow",
			method:      http.MethodPost,
			path:        "/workflow",
			headers:     []string{"Bearer worker-secret"},
			wantStatus:  http.StatusForbidden,
			wantNoStore: true,
		},
		{
			name:        "client cannot claim work",
			method:      http.MethodGet,
			path:        "/work/next",
			headers:     []string{"Bearer client-secret"},
			wantStatus:  http.StatusForbidden,
			wantNoStore: true,
		},
		{
			name:       "health is public",
			method:     http.MethodGet,
			path:       "/healthz",
			wantStatus: http.StatusNoContent,
			wantCalled: true,
		},
		{
			name:       "unknown path returns not found without credential",
			method:     http.MethodGet,
			path:       "/unknown",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong method requires auth then returns method not allowed",
			method:     http.MethodPost,
			path:       "/status",
			headers:    []string{"Bearer admin-secret"},
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "family route doubled slash is not found",
			method:     http.MethodGet,
			path:       "/submissions//status",
			headers:    []string{"Bearer client-secret"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "family route trailing slash is not found",
			method:     http.MethodGet,
			path:       "/submissions/submission-1/status/",
			headers:    []string{"Bearer client-secret"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "family route encoded slash is not found",
			method:     http.MethodGet,
			path:       "/submissions/a%2Fb/status",
			headers:    []string{"Bearer client-secret"},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "family route extra segment is not found",
			method:     http.MethodGet,
			path:       "/submissions/a/b/status",
			headers:    []string{"Bearer client-secret"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			handler := controller.authorizeControllerRoutes(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				principal, hasPrincipal := principalFromContext(r.Context())
				if tt.wantRole == "" {
					if hasPrincipal {
						t.Fatalf("principal present for public request: %+v", principal)
					}
				} else if !hasPrincipal || principal.Role != tt.wantRole {
					t.Fatalf("principal = %+v, present %v, want role %q", principal, hasPrincipal, tt.wantRole)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(tt.method, tt.path, nil)
			for _, value := range tt.headers {
				request.Header.Add("Authorization", value)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Fatalf("handler called = %v, want %v", called, tt.wantCalled)
			}
			if got := response.Header().Get("Cache-Control"); (got == "no-store") != tt.wantNoStore {
				t.Fatalf("Cache-Control = %q, want no-store present %v", got, tt.wantNoStore)
			}
		})
	}
}

func TestAuthorizeControllerRoutesDisabledModeBypassesAllRoutes(t *testing.T) {
	controller := testAuthController(t, controllerauth.ModeDisabled)
	handler := controller.authorizeControllerRoutes(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := principalFromContext(r.Context()); ok {
			t.Fatal("disabled auth request should not have principal")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/shutdown", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func testAuthController(t *testing.T, mode controllerauth.Mode) *Controller {
	t.Helper()
	cfg := controllerauth.Config{Mode: mode}
	if mode == controllerauth.ModeBearer {
		cfg.Credentials = []controllerauth.Credential{
			{ID: "client", Role: controllerauth.RoleClient, TokenEnv: "CLIENT_TOKEN"},
			{ID: "worker", Role: controllerauth.RoleWorker, TokenEnv: "WORKER_TOKEN"},
			{ID: "admin", Role: controllerauth.RoleAdmin, TokenEnv: "ADMIN_TOKEN"},
		}
	}
	store, err := controllerauth.LoadCredentials(cfg, controllerauthTestSources(map[string]string{
		"CLIENT_TOKEN": "client-secret",
		"WORKER_TOKEN": "worker-secret",
		"ADMIN_TOKEN":  "admin-secret",
	}))
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
	controller := newController()
	controller.authMode = mode
	controller.authStore = store
	controller.authPolicy = controllerauth.ControllerPolicy()
	return controller
}

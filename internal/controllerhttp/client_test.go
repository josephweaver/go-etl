package controllerhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewRejectsUnsafeBaseURLs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "missing", raw: ""},
		{name: "user info", raw: "https://user:pass@example.test"},
		{name: "fragment", raw: "https://example.test#frag"},
		{name: "query", raw: "https://example.test?x=1"},
		{name: "missing scheme", raw: "example.test"},
		{name: "missing host", raw: "https:///status"},
		{name: "unsupported scheme", raw: "ftp://example.test"},
		{name: "external http", raw: "http://example.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(Config{BaseURL: tt.raw}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewAllowsHTTPSAndLoopbackHTTP(t *testing.T) {
	for _, raw := range []string{
		"https://example.test",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := New(Config{BaseURL: raw}); err != nil {
				t.Fatalf("New() error = %v", err)
			}
		})
	}
}

func TestNewClonesHTTPClientAndAppliesDefaults(t *testing.T) {
	input := &http.Client{Timeout: 2 * time.Second}
	client, err := New(Config{BaseURL: "http://localhost:8080", HTTP: input})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if client.http == input {
		t.Fatal("client reused supplied *http.Client pointer")
	}
	if input.CheckRedirect != nil {
		t.Fatal("input CheckRedirect was mutated")
	}
	if client.http.Timeout != 2*time.Second {
		t.Fatalf("timeout = %s, want supplied timeout", client.http.Timeout)
	}

	defaulted, err := New(Config{BaseURL: "http://localhost:8080"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if defaulted.http.Timeout == 0 {
		t.Fatal("default timeout was not applied")
	}
}

func TestNewRequestBuildsSafeAuthenticatedRequest(t *testing.T) {
	token, err := NewSensitiveToken("client-secret")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	client, err := New(Config{
		BaseURL: "https://controller.example/api/",
		Token:   NewStaticTokenProvider(token),
		Caller:  "goetl-test/1",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	path, err := PathJoin("/submissions", "submission 1", "status")
	if err != nil {
		t.Fatalf("PathJoin() error = %v", err)
	}

	request, err := client.NewRequestWithQuery(context.Background(), http.MethodGet, path, url.Values{"tail": {"10"}}, nil)
	if err != nil {
		t.Fatalf("NewRequestWithQuery() error = %v", err)
	}

	if got, want := request.URL.String(), "https://controller.example/api/submissions/submission%201/status?tail=10"; got != want {
		t.Fatalf("request URL = %q, want %q", got, want)
	}
	if got := request.Header.Get("Authorization"); got != "Bearer client-secret" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := request.Header.Get("User-Agent"); got != "goetl-test/1" {
		t.Fatalf("User-Agent = %q", got)
	}
}

func TestNewPublicRequestAllowsNilTokenProvider(t *testing.T) {
	client, err := New(Config{BaseURL: "http://localhost:8080"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request, err := client.NewPublicRequest(context.Background(), http.MethodGet, "/healthz", nil)
	if err != nil {
		t.Fatalf("NewPublicRequest() error = %v", err)
	}
	if got := request.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
}

func TestNewRequestRequiresTokenProviderForProtectedRoutes(t *testing.T) {
	client, err := New(Config{BaseURL: "http://localhost:8080"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.NewRequest(context.Background(), http.MethodGet, "/status", nil); err == nil {
		t.Fatal("expected missing token provider error")
	}
}

func TestNewJSONRequestSetsContentType(t *testing.T) {
	token, err := NewSensitiveToken("client-secret")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	client, err := New(Config{
		BaseURL: "http://localhost:8080",
		Token:   NewStaticTokenProvider(token),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request, err := client.NewJSONRequest(context.Background(), http.MethodPost, "/workflow", map[string]string{"ok": "yes"})
	if err != nil {
		t.Fatalf("NewJSONRequest() error = %v", err)
	}
	if got := request.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	var decoded map[string]string
	if err := json.NewDecoder(request.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if decoded["ok"] != "yes" {
		t.Fatalf("decoded body = %+v", decoded)
	}
}

func TestNewRequestRejectsUnsafePaths(t *testing.T) {
	token, err := NewSensitiveToken("client-secret")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	client, err := New(Config{
		BaseURL: "http://localhost:8080",
		Token:   NewStaticTokenProvider(token),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, path := range []string{"status", "//evil.example/status", "/status?x=1", "/status#fragment", `/status\bad`} {
		t.Run(path, func(t *testing.T) {
			if _, err := client.NewRequest(context.Background(), http.MethodGet, path, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPathJoinRejectsSeparatorSegments(t *testing.T) {
	tests := []string{"", "a/b", `a\b`, "a%2Fb", "a%5Cb"}
	for _, segment := range tests {
		t.Run(segment, func(t *testing.T) {
			if _, err := PathJoin("/submissions", segment, "status"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestDoRejectsCrossOriginRedirectBeforeForwardingCredential(t *testing.T) {
	var destinationHit bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationHit = true
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("destination received Authorization = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer destination.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/final", http.StatusFound)
	}))
	defer origin.Close()

	token, err := NewSensitiveToken("client-secret")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	client, err := New(Config{
		BaseURL: origin.URL,
		Token:   NewStaticTokenProvider(token),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request, err := client.NewRequest(context.Background(), http.MethodGet, "/status", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	response, err := client.Do(request, http.StatusNoContent)
	if err == nil {
		if response != nil {
			response.Body.Close()
		}
		t.Fatal("expected redirect rejection error")
	}
	if destinationHit {
		t.Fatal("cross-origin redirect destination was reached")
	}
}

func TestDoAllowsSameOriginRedirect(t *testing.T) {
	var finalAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			finalAuthorization = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	token, err := NewSensitiveToken("client-secret")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	client, err := New(Config{
		BaseURL: server.URL,
		Token:   NewStaticTokenProvider(token),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request, err := client.NewRequest(context.Background(), http.MethodGet, "/status", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	response, err := client.Do(request, http.StatusNoContent)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()
	if finalAuthorization != "Bearer client-secret" {
		t.Fatalf("final Authorization = %q", finalAuthorization)
	}
}

func TestDoHonorsContextCancellation(t *testing.T) {
	token, err := NewSensitiveToken("client-secret")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	client, err := New(Config{
		BaseURL: "http://localhost:8080",
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport should not be reached")
		})},
		Token: NewStaticTokenProvider(token),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.NewRequest(ctx, http.MethodGet, "/status", nil)
	if err == nil {
		t.Fatal("expected canceled token provider error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestPathJoinEscapesSegments(t *testing.T) {
	path, err := PathJoin("/submissions", "sub 1", "status")
	if err != nil {
		t.Fatalf("PathJoin() error = %v", err)
	}
	if !strings.Contains(path, "sub%201") {
		t.Fatalf("path = %q, want escaped segment", path)
	}
}

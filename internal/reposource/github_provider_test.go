package reposource

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubProviderResolvesRefAndReadsRequestedFiles(t *testing.T) {
	const revision = "0123456789abcdef0123456789abcdef01234567"
	seen := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/demo/commits/main":
			if err := json.NewEncoder(w).Encode(map[string]string{"sha": revision}); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
		case "/repos/acme/demo/contents/project.json":
			if r.URL.Query().Get("ref") != revision {
				t.Fatalf("project ref = %q, want %q", r.URL.Query().Get("ref"), revision)
			}
			writeGitHubContent(t, w, "blob-project", `{"name":"demo"}`)
		case "/repos/acme/demo/contents/scripts/run.py":
			if r.URL.Query().Get("ref") != revision {
				t.Fatalf("script ref = %q, want %q", r.URL.Query().Get("ref"), revision)
			}
			writeGitHubContent(t, w, "blob-script", "print('hi')\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := NewGitHubProvider("acme", "demo", WithGitHubBaseURL(server.URL), WithGitHubHTTPClient(server.Client()))
	resolved, err := provider.Resolve(context.Background(), "main")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.RevisionID == nil || *resolved.RevisionID != revision {
		t.Fatalf("RevisionID = %v, want %s", resolved.RevisionID, revision)
	}

	reads, err := provider.ReadFiles(context.Background(), resolved, []string{"project.json", "scripts/run.py"})
	if err != nil {
		t.Fatalf("ReadFiles() error = %v", err)
	}
	if len(reads) != 2 {
		t.Fatalf("len(reads) = %d, want 2", len(reads))
	}
	if got := string(reads[0].Content.Data); got != `{"name":"demo"}` {
		t.Fatalf("project data = %q", got)
	}
	if reads[0].Content.ObjectID == nil || *reads[0].Content.ObjectID != "blob-project" {
		t.Fatalf("object id = %v, want blob-project", reads[0].Content.ObjectID)
	}
	if reads[0].RawSHA256 == "" {
		t.Fatal("raw sha256 is empty")
	}
	if _, ok := seen["/repos/acme/demo/contents/project.json"]; !ok {
		t.Fatal("project content endpoint was not called")
	}
}

func TestGitHubProviderFailsWhenDeclaredReadIsMissing(t *testing.T) {
	const revision = "0123456789abcdef0123456789abcdef01234567"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/repos/acme/demo/commits/main" {
			if err := json.NewEncoder(w).Encode(map[string]string{"sha": revision}); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := NewGitHubProvider("acme", "demo", WithGitHubBaseURL(server.URL), WithGitHubHTTPClient(server.Client()))
	resolved, err := provider.Resolve(context.Background(), "main")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if _, err := provider.ReadFiles(context.Background(), resolved, []string{"missing.json"}); err == nil {
		t.Fatal("expected missing file error")
	}
}

func writeGitHubContent(t *testing.T, w http.ResponseWriter, objectID string, data string) {
	t.Helper()
	content := base64.StdEncoding.EncodeToString([]byte(data))
	if err := json.NewEncoder(w).Encode(map[string]string{
		"sha":      objectID,
		"type":     "file",
		"encoding": "base64",
		"content":  content,
	}); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
}

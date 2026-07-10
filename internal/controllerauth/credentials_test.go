package controllerauth

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestLoadCredentialsFromEnvironment(t *testing.T) {
	store, err := LoadCredentials(Config{
		Mode: ModeBearer,
		Credentials: []Credential{
			{ID: "primary-client", Role: RoleClient, TokenEnv: "GOET_CONTROLLER_CLIENT_TOKEN"},
			{ID: "worker-pool", Role: RoleWorker, TokenEnv: "GOET_CONTROLLER_WORKER_TOKEN"},
		},
	}, CredentialSources{
		LookupEnv: mapEnv(map[string]string{
			"GOET_CONTROLLER_CLIENT_TOKEN": "client-secret",
			"GOET_CONTROLLER_WORKER_TOKEN": "worker-secret\n",
		}),
	})
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}

	client, ok := store.MatchBearer("client-secret")
	if !ok {
		t.Fatal("client token did not match")
	}
	if client.ID != "primary-client" || client.Role != RoleClient {
		t.Fatalf("client principal = %+v", client)
	}

	worker, ok := store.MatchBearer("worker-secret")
	if !ok {
		t.Fatal("worker token with normalized newline did not match")
	}
	if worker.ID != "worker-pool" || worker.Role != RoleWorker {
		t.Fatalf("worker principal = %+v", worker)
	}

	if _, ok := store.MatchBearer("wrong-secret"); ok {
		t.Fatal("wrong token matched")
	}
}

func TestLoadCredentialsFromRestrictiveFile(t *testing.T) {
	const path = "/etc/goet/secrets/controller-admin-token"

	store, err := LoadCredentials(Config{
		Mode: ModeBearer,
		Credentials: []Credential{
			{ID: "operator", Role: RoleAdmin, TokenFile: path},
		},
	}, CredentialSources{
		StatFile: statFiles(map[string]fs.FileMode{path: 0o600}),
		ReadFile: readFiles(map[string]string{
			path: "admin-secret\r\n",
		}),
	})
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}

	operator, ok := store.MatchBearer("admin-secret")
	if !ok {
		t.Fatal("file token with normalized CRLF did not match")
	}
	if operator.ID != "operator" || operator.Role != RoleAdmin {
		t.Fatalf("operator principal = %+v", operator)
	}
}

func TestLoadCredentialsPreservesAdditionalTrailingWhitespace(t *testing.T) {
	store, err := LoadCredentials(Config{
		Mode: ModeBearer,
		Credentials: []Credential{
			{ID: "client", Role: RoleClient, TokenEnv: "GOET_CONTROLLER_CLIENT_TOKEN"},
		},
	}, CredentialSources{
		LookupEnv: mapEnv(map[string]string{
			"GOET_CONTROLLER_CLIENT_TOKEN": "client-secret\n\n",
		}),
	})
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}

	if _, ok := store.MatchBearer("client-secret"); ok {
		t.Fatal("token matched after trimming more than one line ending")
	}
	if _, ok := store.MatchBearer("client-secret\n"); !ok {
		t.Fatal("token did not match after trimming exactly one line ending")
	}
}

func TestLoadCredentialsRejectsUnsafeSources(t *testing.T) {
	const sentinel = "goet-controller-auth-sentinel-do-not-leak"
	const tokenPath = "/etc/goet/secrets/controller-token"

	tests := []struct {
		name        string
		config      Config
		sources     CredentialSources
		want        string
		forbidToken bool
	}{
		{
			name: "bearer without credentials",
			config: Config{
				Mode: ModeBearer,
			},
			want: "bearer authentication requires at least one credential",
		},
		{
			name: "missing environment variable",
			config: Config{
				Mode:        ModeBearer,
				Credentials: []Credential{{ID: "client", Role: RoleClient, TokenEnv: "GOET_CONTROLLER_CLIENT_TOKEN"}},
			},
			sources: CredentialSources{
				LookupEnv: mapEnv(nil),
			},
			want: "token_env \"GOET_CONTROLLER_CLIENT_TOKEN\" is not set",
		},
		{
			name: "empty environment token after normalization",
			config: Config{
				Mode:        ModeBearer,
				Credentials: []Credential{{ID: "client", Role: RoleClient, TokenEnv: "GOET_CONTROLLER_CLIENT_TOKEN"}},
			},
			sources: CredentialSources{
				LookupEnv: mapEnv(map[string]string{"GOET_CONTROLLER_CLIENT_TOKEN": "\n"}),
			},
			want: "token_env \"GOET_CONTROLLER_CLIENT_TOKEN\" is empty",
		},
		{
			name: "duplicate token material",
			config: Config{
				Mode: ModeBearer,
				Credentials: []Credential{
					{ID: "client", Role: RoleClient, TokenEnv: "GOET_CONTROLLER_CLIENT_TOKEN"},
					{ID: "worker", Role: RoleWorker, TokenEnv: "GOET_CONTROLLER_WORKER_TOKEN"},
				},
			},
			sources: CredentialSources{
				LookupEnv: mapEnv(map[string]string{
					"GOET_CONTROLLER_CLIENT_TOKEN": sentinel,
					"GOET_CONTROLLER_WORKER_TOKEN": sentinel,
				}),
			},
			want:        "duplicates token material",
			forbidToken: true,
		},
		{
			name: "file grants group access",
			config: Config{
				Mode:        ModeBearer,
				Credentials: []Credential{{ID: "client", Role: RoleClient, TokenFile: tokenPath}},
			},
			sources: CredentialSources{
				StatFile: statFiles(map[string]fs.FileMode{tokenPath: 0o640}),
				ReadFile: readFiles(map[string]string{tokenPath: sentinel}),
			},
			want:        "permissions must not grant group or other access",
			forbidToken: true,
		},
		{
			name: "file is directory",
			config: Config{
				Mode:        ModeBearer,
				Credentials: []Credential{{ID: "client", Role: RoleClient, TokenFile: tokenPath}},
			},
			sources: CredentialSources{
				StatFile: statFiles(map[string]fs.FileMode{tokenPath: fs.ModeDir | 0o700}),
				ReadFile: readFiles(map[string]string{tokenPath: sentinel}),
			},
			want:        "must not be a directory",
			forbidToken: true,
		},
		{
			name: "empty file token after normalization",
			config: Config{
				Mode:        ModeBearer,
				Credentials: []Credential{{ID: "client", Role: RoleClient, TokenFile: tokenPath}},
			},
			sources: CredentialSources{
				StatFile: statFiles(map[string]fs.FileMode{tokenPath: 0o600}),
				ReadFile: readFiles(map[string]string{tokenPath: "\r\n"}),
			},
			want: "token_file \"/etc/goet/secrets/controller-token\" is empty",
		},
		{
			name: "unsupported mode",
			config: Config{
				Mode: Mode("oauth"),
			},
			want: "unsupported authentication mode \"oauth\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadCredentials(tt.config, tt.sources)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadCredentials() error = %v, want containing %q", err, tt.want)
			}
			if tt.forbidToken && strings.Contains(err.Error(), sentinel) {
				t.Fatalf("LoadCredentials() error leaked token sentinel: %v", err)
			}
		})
	}
}

func TestLoadCredentialsDisabledModeLoadsNoMaterial(t *testing.T) {
	store, err := LoadCredentials(Config{
		Mode:        ModeDisabled,
		Credentials: []Credential{{ID: "client", Role: RoleClient, TokenEnv: "GOET_CONTROLLER_CLIENT_TOKEN"}},
	}, CredentialSources{
		LookupEnv: func(string) (string, bool) {
			t.Fatal("disabled auth must not read credential material")
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
	if _, ok := store.MatchBearer("anything"); ok {
		t.Fatal("disabled credential store matched a token")
	}
}

func mapEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func readFiles(values map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		value, ok := values[path]
		if !ok {
			return nil, errors.New("missing test file")
		}
		return []byte(value), nil
	}
}

func statFiles(values map[string]fs.FileMode) func(string) (fs.FileInfo, error) {
	return func(path string) (fs.FileInfo, error) {
		mode, ok := values[path]
		if !ok {
			return nil, errors.New("missing test file")
		}
		return testFileInfo{name: path, mode: mode}, nil
	}
}

type testFileInfo struct {
	name string
	mode fs.FileMode
}

func (i testFileInfo) Name() string       { return i.name }
func (i testFileInfo) Size() int64        { return 0 }
func (i testFileInfo) Mode() fs.FileMode  { return i.mode }
func (i testFileInfo) ModTime() time.Time { return time.Time{} }
func (i testFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i testFileInfo) Sys() any           { return nil }

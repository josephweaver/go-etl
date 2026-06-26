package clientsetup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSSHSetupGeneratesKeyAndConfig(t *testing.T) {
	prompter := &fakePrompter{
		answers: []string{
			"ssh",
			"127.0.0.1",
			"2222",
			"goetl",
			"ssh-ed25519 AAAAFAKE",
			"",
		},
		confirms: []bool{true, true},
	}
	files := &fakeFileStore{files: map[string]storedFile{}}

	result, err := (SSHSetup{Prompter: prompter, Files: files}).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.GeneratedKey {
		t.Fatal("expected generated key")
	}
	if result.PrivateKeyPath != ".run/goetl/ssh/id_ed25519" {
		t.Fatalf("private key path = %q", result.PrivateKeyPath)
	}
	privateKey := files.files[".run/goetl/ssh/id_ed25519"]
	if !privateKey.private {
		t.Fatal("private key file was not marked private")
	}
	if !strings.Contains(string(privateKey.data), "PRIVATE KEY") {
		t.Fatalf("private key data = %q, want PEM", string(privateKey.data))
	}
	publicKey := files.files[".run/goetl/ssh/id_ed25519.pub"]
	if !strings.HasPrefix(string(publicKey.data), "ssh-ed25519 ") {
		t.Fatalf("public key = %q, want authorized key", string(publicKey.data))
	}

	configFile := files.files[".run/goetl/generated/fake-hpcc-ssh-config.json"]
	var config map[string]any
	if err := json.Unmarshal(configFile.data, &config); err != nil {
		t.Fatalf("decode generated config: %v", err)
	}
	env := config["execution_environment"].(map[string]any)
	transports := env["transports"].([]any)
	transport := transports[0].(map[string]any)
	settings := transport["settings"].(map[string]any)

	if transport["type"] != "ssh" {
		t.Fatalf("transport type = %q, want ssh", transport["type"])
	}
	if settings["host"] != "127.0.0.1" {
		t.Fatalf("host = %q, want 127.0.0.1", settings["host"])
	}
	if settings["identity_file"] != ".run/goetl/ssh/id_ed25519" {
		t.Fatalf("identity file = %q, want generated key path", settings["identity_file"])
	}
	if settings["pinned_host_key"] != "ssh-ed25519 AAAAFAKE" {
		t.Fatalf("pinned host key = %q, want fake key", settings["pinned_host_key"])
	}
}

func TestSSHSetupUsesExistingKey(t *testing.T) {
	prompter := &fakePrompter{
		answers: []string{
			"s",
			"hpcc.example.edu",
			"",
			"researcher",
			"/tmp/id_ed25519",
			"ssh-ed25519 AAAAFAKE",
			"custom-config.json",
		},
		confirms: []bool{false, true},
	}
	files := &fakeFileStore{files: map[string]storedFile{}}

	result, err := (SSHSetup{Prompter: prompter, Files: files}).Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.GeneratedKey {
		t.Fatal("did not expect generated key")
	}
	if result.TransportConfig.Port != "22" {
		t.Fatalf("port = %q, want default 22", result.TransportConfig.Port)
	}
	if result.TransportConfig.IdentityFile != "/tmp/id_ed25519" {
		t.Fatalf("identity file = %q, want existing key", result.TransportConfig.IdentityFile)
	}
	if _, ok := files.files[".run/goetl/ssh/id_ed25519"]; ok {
		t.Fatal("unexpected generated private key")
	}
	if _, ok := files.files["custom-config.json"]; !ok {
		t.Fatal("expected custom config file")
	}
}

func TestSSHSetupRequiresExplicitHostKeyPinning(t *testing.T) {
	prompter := &fakePrompter{
		answers: []string{
			"ssh",
			"127.0.0.1",
			"2222",
			"goetl",
			"ssh-ed25519 AAAAFAKE",
		},
		confirms: []bool{true, false},
	}
	files := &fakeFileStore{files: map[string]storedFile{}}

	_, err := (SSHSetup{Prompter: prompter, Files: files}).Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "host-key pinning") {
		t.Fatalf("error = %v, want host-key pinning", err)
	}
	if _, ok := files.files[".run/goetl/generated/fake-hpcc-ssh-config.json"]; ok {
		t.Fatal("config should not be written without host-key pinning")
	}
}

type fakePrompter struct {
	answers  []string
	confirms []bool
}

func (p *fakePrompter) Ask(ctx context.Context, prompt string) (string, error) {
	if len(p.answers) == 0 {
		return "", nil
	}
	answer := p.answers[0]
	p.answers = p.answers[1:]
	return answer, nil
}

func (p *fakePrompter) Confirm(ctx context.Context, prompt string) (bool, error) {
	if len(p.confirms) == 0 {
		return false, nil
	}
	answer := p.confirms[0]
	p.confirms = p.confirms[1:]
	return answer, nil
}

type storedFile struct {
	data    []byte
	private bool
}

type fakeFileStore struct {
	dirs  []string
	files map[string]storedFile
}

func (s *fakeFileStore) MkdirAll(path string) error {
	s.dirs = append(s.dirs, path)
	return nil
}

func (s *fakeFileStore) WriteFile(path string, data []byte, private bool) error {
	copied := append([]byte(nil), data...)
	s.files[path] = storedFile{data: copied, private: private}
	return nil
}

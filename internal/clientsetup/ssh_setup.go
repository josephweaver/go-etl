package clientsetup

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

type Prompter interface {
	Ask(ctx context.Context, prompt string) (string, error)
	Confirm(ctx context.Context, prompt string) (bool, error)
}

type FileStore interface {
	MkdirAll(path string) error
	WriteFile(path string, data []byte, private bool) error
}

type SSHSetup struct {
	Prompter Prompter
	Files    FileStore
}

type SSHSetupResult struct {
	ConfigPath      string
	PrivateKeyPath  string
	PublicKeyPath   string
	PinnedHostKey   string
	GeneratedKey    bool
	TransportConfig SSHTransportSetupConfig
}

type SSHTransportSetupConfig struct {
	Host          string
	Port          string
	User          string
	IdentityFile  string
	HostKeyPolicy string
	PinnedHostKey string
}

func (s SSHSetup) Run(ctx context.Context) (SSHSetupResult, error) {
	if s.Prompter == nil {
		return SSHSetupResult{}, fmt.Errorf("prompter is required")
	}
	if s.Files == nil {
		return SSHSetupResult{}, fmt.Errorf("file store is required")
	}

	transport, err := s.Prompter.Ask(ctx, "Transport: (l)ocal, (s)sh, or (d)ocker")
	if err != nil {
		return SSHSetupResult{}, err
	}
	if transport != "s" && transport != "ssh" {
		return SSHSetupResult{}, fmt.Errorf("only ssh setup is supported by SSHSetup")
	}

	host, err := s.Prompter.Ask(ctx, "SSH host")
	if err != nil {
		return SSHSetupResult{}, err
	}
	port, err := s.Prompter.Ask(ctx, "SSH port")
	if err != nil {
		return SSHSetupResult{}, err
	}
	if port == "" {
		port = "22"
	}
	user, err := s.Prompter.Ask(ctx, "SSH user")
	if err != nil {
		return SSHSetupResult{}, err
	}

	createKey, err := s.Prompter.Confirm(ctx, "Create a new local project key")
	if err != nil {
		return SSHSetupResult{}, err
	}
	privateKeyPath, publicKeyPath := "", ""
	generatedKey := false
	if createKey {
		privateKeyPath = filepath.ToSlash(filepath.Join(".run", "goetl", "ssh", "id_ed25519"))
		publicKeyPath = privateKeyPath + ".pub"
		if err := s.writeKeyPair(privateKeyPath, publicKeyPath); err != nil {
			return SSHSetupResult{}, err
		}
		generatedKey = true
	} else {
		privateKeyPath, err = s.Prompter.Ask(ctx, "Private key path")
		if err != nil {
			return SSHSetupResult{}, err
		}
	}

	hostKey, err := s.Prompter.Ask(ctx, "Host public key")
	if err != nil {
		return SSHSetupResult{}, err
	}
	pinHostKey, err := s.Prompter.Confirm(ctx, "Pin this host key in the generated config")
	if err != nil {
		return SSHSetupResult{}, err
	}
	if !pinHostKey {
		return SSHSetupResult{}, fmt.Errorf("ssh setup requires explicit host-key pinning")
	}

	configPath, err := s.Prompter.Ask(ctx, "Output config path")
	if err != nil {
		return SSHSetupResult{}, err
	}
	if configPath == "" {
		configPath = filepath.ToSlash(filepath.Join(".run", "goetl", "generated", "fake-hpcc-ssh-config.json"))
	}

	transportConfig := SSHTransportSetupConfig{
		Host:          host,
		Port:          port,
		User:          user,
		IdentityFile:  privateKeyPath,
		HostKeyPolicy: "pinned",
		PinnedHostKey: hostKey,
	}
	if err := s.writeControllerConfig(configPath, transportConfig); err != nil {
		return SSHSetupResult{}, err
	}

	return SSHSetupResult{
		ConfigPath:      configPath,
		PrivateKeyPath:  privateKeyPath,
		PublicKeyPath:   publicKeyPath,
		PinnedHostKey:   hostKey,
		GeneratedKey:    generatedKey,
		TransportConfig: transportConfig,
	}, nil
}

func (s SSHSetup) writeKeyPair(privateKeyPath string, publicKeyPath string) error {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ssh key: %w", err)
	}
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("marshal ssh private key: %w", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes})
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return fmt.Errorf("marshal ssh public key: %w", err)
	}

	if err := s.Files.MkdirAll(filepath.Dir(privateKeyPath)); err != nil {
		return err
	}
	if err := s.Files.WriteFile(privateKeyPath, privatePEM, true); err != nil {
		return err
	}
	return s.Files.WriteFile(publicKeyPath, ssh.MarshalAuthorizedKey(signer.PublicKey()), false)
}

func (s SSHSetup) writeControllerConfig(path string, transport SSHTransportSetupConfig) error {
	doc := map[string]any{
		"variables": []map[string]any{
			variableJSON("controller_url", "string", "http://localhost:8080"),
			variableJSON("ledger_db_path", "path", ".run/controller/fake-hpcc-ssh-ledger.sqlite"),
		},
		"execution_environment": map[string]any{
			"name": "fake-hpcc-ssh",
			"transports": []map[string]any{{
				"name": "login",
				"type": "ssh",
				"settings": map[string]string{
					"host":            transport.Host,
					"port":            transport.Port,
					"user":            transport.User,
					"identity_file":   transport.IdentityFile,
					"host_key_policy": transport.HostKeyPolicy,
					"pinned_host_key": transport.PinnedHostKey,
					"connect_timeout": "5s",
					"command_timeout": "30s",
				},
			}},
			"dialect":   map[string]string{"type": "bash"},
			"scheduler": map[string]string{"type": "slurm"},
			"runtime": map[string]any{
				"type": "worker",
				"settings": map[string]string{
					"root":           "/data/goetl",
					"controller_url": "http://host.docker.internal:8080",
				},
			},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := s.Files.MkdirAll(filepath.Dir(path)); err != nil {
		return err
	}
	return s.Files.WriteFile(path, data, false)
}

func variableJSON(key string, kind string, expression string) map[string]any {
	return map[string]any{
		"Name":       map[string]string{"Namespace": "controller_config", "Key": key},
		"Type":       map[string]string{"Kind": kind},
		"Expression": expression,
	}
}

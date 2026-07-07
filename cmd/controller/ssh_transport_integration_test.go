package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSSHTransportIntegrationPreparesRuntimeOverSSH(t *testing.T) {
	hostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	server := startTestSSHServer(t, hostSigner, clientIdentity.signer.PublicKey())

	identityFile := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(identityFile, []byte(clientIdentity.privatePEM), 0600); err != nil {
		t.Fatalf("write identity file: %v", err)
	}

	host, port := splitTestSSHAddress(t, server.address)
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name: "ssh-integration",
		Transports: []ExecutionComponentConfig{{
			Name: "login",
			Type: "ssh",
			Settings: ExecutionComponentSettings{
				"host":            host,
				"port":            port,
				"user":            "test-user",
				"identity_file":   identityFile,
				"host_key_policy": "pinned",
				"pinned_host_key": string(mustMarshalTestHostKey(t, hostSigner.PublicKey())),
			},
		}},
		Dialect:   ExecutionComponentConfig{Type: "bash"},
		Scheduler: ExecutionComponentConfig{Type: "direct_process"},
		Runtime: ExecutionComponentConfig{
			Type: "worker",
			Settings: ExecutionComponentSettings{
				"root":           "runtime root",
				"controller_url": "http://localhost:8080",
			},
		},
	})
	if err != nil {
		t.Fatalf("build execution environment: %v", err)
	}

	transport, ok := env.Transports[0].(*SSHTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *SSHTransport", env.Transports[0])
	}
	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect SSH transport: %v", err)
	}
	defer transport.Close()

	if err := env.Prepare(context.Background()); err != nil {
		t.Fatalf("prepare execution environment: %v", err)
	}

	workerConfigPath := filepath.Join(server.remoteRoot, "runtime root", "config", "worker.json")
	data, err := os.ReadFile(workerConfigPath)
	if err != nil {
		t.Fatalf("read remote worker config: %v", err)
	}
	if !strings.Contains(string(data), `"controller_url": "http://localhost:8080"`) {
		t.Fatalf("worker config = %s, want controller URL", string(data))
	}

	output, err := transport.Exec(context.Background(), "stdout-ok")
	if err != nil {
		t.Fatalf("exec remote command: %v", err)
	}
	if string(output) != "stdout from ssh\n" {
		t.Fatalf("output = %q, want stdout from ssh", string(output))
	}

	if err := transport.RemoveTree(context.Background(), "runtime root"); err != nil {
		t.Fatalf("cleanup remote runtime root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(server.remoteRoot, "runtime root")); !os.IsNotExist(err) {
		t.Fatalf("runtime root still exists or unexpected err: %v", err)
	}
}

func splitTestSSHAddress(t *testing.T, address string) (string, string) {
	t.Helper()

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split test SSH address: %v", err)
	}
	return host, port
}

func mustMarshalTestHostKey(t *testing.T, key ssh.PublicKey) []byte {
	t.Helper()

	data := ssh.MarshalAuthorizedKey(key)
	if len(data) == 0 {
		t.Fatal("empty marshaled host key")
	}
	return data
}

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type testSSHServer struct {
	address    string
	remoteRoot string
	execCounts *sync.Map
	close      func() error
}

type testSSHIdentity struct {
	signer     ssh.Signer
	privatePEM string
}

func generateTestSSHSigner(t *testing.T) ssh.Signer {
	t.Helper()

	return generateTestSSHIdentity(t).signer
}

func generateTestSSHIdentity(t *testing.T) testSSHIdentity {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("build test signer: %v", err)
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal test key: %v", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return testSSHIdentity{
		signer:     signer,
		privatePEM: string(privatePEM),
	}
}

func startTestSSHServer(t *testing.T, hostSigner ssh.Signer, clientPublicKey ssh.PublicKey) testSSHServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test SSH server: %v", err)
	}

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), clientPublicKey.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("rejected public key for %s", conn.User())
		},
	}
	config.AddHostKey(hostSigner)
	remoteRoot := t.TempDir()
	execCounts := &sync.Map{}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer close(done)

		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				t.Logf("test SSH accept error: %v", err)
				return
			}

			wg.Add(1)
			go func(conn net.Conn) {
				defer wg.Done()
				handleTestSSHConnection(t, conn, config, remoteRoot, execCounts)
			}(conn)
		}
	}()

	closeServer := func() error {
		err := listener.Close()
		<-done
		wg.Wait()
		return err
	}
	t.Cleanup(func() {
		if err := closeServer(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("close test SSH server: %v", err)
		}
	})

	return testSSHServer{
		address:    listener.Addr().String(),
		remoteRoot: remoteRoot,
		execCounts: execCounts,
		close:      closeServer,
	}
}

func handleTestSSHConnection(t *testing.T, conn net.Conn, config *ssh.ServerConfig, remoteRoot string, execCounts *sync.Map) {
	t.Helper()

	_, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		t.Logf("test SSH handshake error: %v", err)
		return
	}
	go ssh.DiscardRequests(requests)

	for newChannel := range channels {
		switch newChannel.ChannelType() {
		case "session":
		case "direct-tcpip":
			handleTestSSHDirectTCPIP(t, newChannel)
			continue
		default:
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			t.Logf("test SSH accept channel error: %v", err)
			continue
		}

		go handleTestSSHSession(t, channel, requests, remoteRoot, execCounts)
	}
}

type testSSHDirectTCPIPPayload struct {
	DestinationHost string
	DestinationPort uint32
	OriginatorHost  string
	OriginatorPort  uint32
}

func handleTestSSHDirectTCPIP(t *testing.T, newChannel ssh.NewChannel) {
	t.Helper()

	var payload testSSHDirectTCPIPPayload
	if err := ssh.Unmarshal(newChannel.ExtraData(), &payload); err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, "invalid direct-tcpip payload")
		return
	}

	targetAddress := net.JoinHostPort(payload.DestinationHost, strconv.Itoa(int(payload.DestinationPort)))
	targetConn, err := net.Dial("tcp", targetAddress)
	if err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
		return
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		_ = targetConn.Close()
		t.Logf("test SSH accept direct-tcpip channel error: %v", err)
		return
	}
	go ssh.DiscardRequests(requests)
	go proxyTestSSHDirectTCPIP(channel, targetConn)
}

func proxyTestSSHDirectTCPIP(channel ssh.Channel, targetConn net.Conn) {
	var once sync.Once
	closeBoth := func() {
		_ = channel.Close()
		_ = targetConn.Close()
	}
	go func() {
		_, _ = io.Copy(channel, targetConn)
		once.Do(closeBoth)
	}()
	go func() {
		_, _ = io.Copy(targetConn, channel)
		once.Do(closeBoth)
	}()
}

func handleTestSSHSession(t *testing.T, channel ssh.Channel, requests <-chan *ssh.Request, remoteRoot string, execCounts *sync.Map) {
	t.Helper()
	defer channel.Close()

	for request := range requests {
		switch request.Type {
		case "exec":
			if request.WantReply {
				_ = request.Reply(true, nil)
			}
			command := testSSHExecCommand(t, request.Payload)
			incrementTestSSHExecCount(execCounts, command)
			handleTestSSHExecCommand(channel, command, remoteRoot)
			return
		case "subsystem":
			subsystem := testSSHSubsystem(t, request.Payload)
			if subsystem != "sftp" {
				if request.WantReply {
					_ = request.Reply(false, nil)
				}
				return
			}
			if request.WantReply {
				_ = request.Reply(true, nil)
			}
			server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(remoteRoot))
			if err != nil {
				t.Logf("create test SFTP server: %v", err)
				return
			}
			if err := server.Serve(); err != nil && err != io.EOF {
				t.Logf("serve test SFTP: %v", err)
			}
			return
		default:
			if request.WantReply {
				_ = request.Reply(false, nil)
			}
		}
	}
}

func incrementTestSSHExecCount(execCounts *sync.Map, command string) {
	value, _ := execCounts.LoadOrStore(command, new(int64))
	counter := value.(*int64)
	atomic.AddInt64(counter, 1)
}

func testSSHExecCommand(t *testing.T, payload []byte) string {
	t.Helper()

	var request struct {
		Command string
	}
	if err := ssh.Unmarshal(payload, &request); err != nil {
		t.Fatalf("parse test SSH exec payload: %v", err)
	}
	return request.Command
}

func testSSHSubsystem(t *testing.T, payload []byte) string {
	t.Helper()

	var request struct {
		Name string
	}
	if err := ssh.Unmarshal(payload, &request); err != nil {
		t.Fatalf("parse test SSH subsystem payload: %v", err)
	}
	return request.Name
}

func handleTestSSHExecCommand(channel ssh.Channel, command string, remoteRoot string) {
	switch command {
	case "'stdout-ok'":
		_, _ = io.WriteString(channel, "stdout from ssh\n")
		sendTestSSHExitStatus(channel, 0)
	case "'stderr-ok'":
		_, _ = io.WriteString(channel, "stdout despite stderr\n")
		_, _ = io.WriteString(channel.Stderr(), "warning from ssh\n")
		sendTestSSHExitStatus(channel, 0)
	case "'fail-command'":
		_, _ = io.WriteString(channel.Stderr(), "remote command failed\n")
		sendTestSSHExitStatus(channel, 7)
	case "'arg check' 'two words'":
		_, _ = io.WriteString(channel, "args preserved\n")
		sendTestSSHExitStatus(channel, 0)
	case "'wait-for-cancel'":
		time.Sleep(2 * time.Second)
		sendTestSSHExitStatus(channel, 0)
	default:
		if handleTestSSHFilesystemCommand(channel, command, remoteRoot) {
			return
		}
		_, _ = io.WriteString(channel, "test ssh fixture\n")
		sendTestSSHExitStatus(channel, 0)
	}
}

func handleTestSSHFilesystemCommand(channel ssh.Channel, command string, remoteRoot string) bool {
	fields, err := testShellFields(command)
	if err != nil || len(fields) == 0 {
		return false
	}

	fail := func(message string) {
		_, _ = io.WriteString(channel.Stderr(), message+"\n")
		sendTestSSHExitStatus(channel, 1)
	}

	switch fields[0] {
	case "mkdir":
		if len(fields) == 3 && fields[1] == "-p" {
			if err := os.MkdirAll(testRemoteDiskPath(remoteRoot, fields[2]), 0755); err != nil {
				fail(err.Error())
				return true
			}
			sendTestSSHExitStatus(channel, 0)
			return true
		}
	case "mv":
		if len(fields) == 3 {
			if err := os.Rename(testRemoteDiskPath(remoteRoot, fields[1]), testRemoteDiskPath(remoteRoot, fields[2])); err != nil {
				fail(err.Error())
				return true
			}
			sendTestSSHExitStatus(channel, 0)
			return true
		}
	case "rm":
		if len(fields) == 3 && fields[1] == "-f" {
			if err := os.Remove(testRemoteDiskPath(remoteRoot, fields[2])); err != nil && !os.IsNotExist(err) {
				fail(err.Error())
				return true
			}
			sendTestSSHExitStatus(channel, 0)
			return true
		}
		if len(fields) == 3 && fields[1] == "-rf" {
			if err := os.RemoveAll(testRemoteDiskPath(remoteRoot, fields[2])); err != nil {
				fail(err.Error())
				return true
			}
			sendTestSSHExitStatus(channel, 0)
			return true
		}
	case "chmod":
		if len(fields) == 3 {
			mode, err := strconv.ParseUint(fields[1], 8, 32)
			if err != nil {
				fail(err.Error())
				return true
			}
			if err := os.Chmod(testRemoteDiskPath(remoteRoot, fields[2]), os.FileMode(mode)); err != nil {
				fail(err.Error())
				return true
			}
			sendTestSSHExitStatus(channel, 0)
			return true
		}
	case "chown":
		if len(fields) == 3 {
			fail("operation not permitted")
			return true
		}
	}
	return false
}

func testShellFields(command string) ([]string, error) {
	var fields []string
	var current strings.Builder
	inQuote := false

	for index := 0; index < len(command); index++ {
		ch := command[index]
		switch ch {
		case '\'':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				current.WriteByte(ch)
				continue
			}
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated quote")
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields, nil
}

func testRemoteDiskPath(remoteRoot string, remotePath string) string {
	remotePath = strings.TrimPrefix(remotePath, "/")
	return filepath.Join(remoteRoot, filepath.FromSlash(remotePath))
}

func sendTestSSHExitStatus(channel ssh.Channel, status uint32) {
	var payload [4]byte
	binary.BigEndian.PutUint32(payload[:], status)
	_, _ = channel.SendRequest("exit-status", false, payload[:])
}

func testSSHClientConfig(hostPublicKey ssh.PublicKey, clientSigner ssh.Signer) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: "test-user",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(clientSigner),
		},
		HostKeyCallback: ssh.FixedHostKey(hostPublicKey),
	}
}

func TestSSHTransportFixtureAcceptsCommandSession(t *testing.T) {
	hostSigner := generateTestSSHSigner(t)
	clientSigner := generateTestSSHSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())

	client, err := ssh.Dial("tcp", server.address, testSSHClientConfig(hostSigner.PublicKey(), clientSigner))
	if err != nil {
		t.Fatalf("dial test SSH server: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("open test SSH session: %v", err)
	}
	defer session.Close()

	output, err := session.Output("fixture-check")
	if err != nil {
		t.Fatalf("run test SSH command: %v", err)
	}

	if string(output) != "test ssh fixture\n" {
		t.Fatalf("output = %q, want %q", string(output), "test ssh fixture\n")
	}
}

func TestSSHTransportConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SSHTransportConfig
		wantErr string
	}{
		{
			name: "valid minimal identity file known hosts",
			cfg: SSHTransportConfig{
				Host:          "hpcc.example.edu",
				User:          "researcher",
				IdentityFile:  "~/.ssh/id_ed25519",
				HostKeyPolicy: SSHHostKeyPolicyKnownHosts,
			},
		},
		{
			name: "valid minimal identity env pinned",
			cfg: SSHTransportConfig{
				Host:          "hpcc.example.edu",
				User:          "researcher",
				IdentityEnv:   "GOETL_SSH_KEY",
				HostKeyPolicy: SSHHostKeyPolicyPinned,
				PinnedHostKey: "ssh-ed25519 AAAATESTKEY",
			},
		},
		{
			name: "valid jump host inherits target identity",
			cfg: SSHTransportConfig{
				Host:          "dev.example.edu",
				User:          "researcher",
				IdentityEnv:   "GOETL_SSH_KEY",
				HostKeyPolicy: SSHHostKeyPolicyPinned,
				PinnedHostKey: "ssh-ed25519 AAAATARGET",
				JumpHosts: []SSHJumpHostConfig{{
					Host:          "gateway.example.edu",
					User:          "researcher",
					HostKeyPolicy: SSHHostKeyPolicyPinned,
					PinnedHostKey: "ssh-ed25519 AAAAGATEWAY",
				}},
			},
		},
		{
			name: "missing host",
			cfg: SSHTransportConfig{
				User:         "researcher",
				IdentityFile: "~/.ssh/id_ed25519",
			},
			wantErr: "host is required",
		},
		{
			name: "missing user",
			cfg: SSHTransportConfig{
				Host:         "hpcc.example.edu",
				IdentityFile: "~/.ssh/id_ed25519",
			},
			wantErr: "user is required",
		},
		{
			name: "missing auth source",
			cfg: SSHTransportConfig{
				Host: "hpcc.example.edu",
				User: "researcher",
			},
			wantErr: "identity_file or identity_env is required",
		},
		{
			name: "both identity sources set",
			cfg: SSHTransportConfig{
				Host:         "hpcc.example.edu",
				User:         "researcher",
				IdentityFile: "~/.ssh/id_ed25519",
				IdentityEnv:  "GOETL_SSH_KEY",
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "invalid host key policy",
			cfg: SSHTransportConfig{
				Host:          "hpcc.example.edu",
				User:          "researcher",
				IdentityFile:  "~/.ssh/id_ed25519",
				HostKeyPolicy: "trust_me",
			},
			wantErr: "unsupported ssh host_key_policy",
		},
		{
			name: "pinned policy without pinned key",
			cfg: SSHTransportConfig{
				Host:          "hpcc.example.edu",
				User:          "researcher",
				IdentityFile:  "~/.ssh/id_ed25519",
				HostKeyPolicy: SSHHostKeyPolicyPinned,
			},
			wantErr: "pinned_host_key is required",
		},
		{
			name: "invalid connect timeout",
			cfg: SSHTransportConfig{
				Host:           "hpcc.example.edu",
				User:           "researcher",
				IdentityFile:   "~/.ssh/id_ed25519",
				ConnectTimeout: "five seconds",
			},
			wantErr: "connect_timeout must be a Go duration",
		},
		{
			name: "invalid command timeout",
			cfg: SSHTransportConfig{
				Host:           "hpcc.example.edu",
				User:           "researcher",
				IdentityFile:   "~/.ssh/id_ed25519",
				CommandTimeout: "-1s",
			},
			wantErr: "command_timeout must be greater than zero",
		},
		{
			name: "invalid negative port",
			cfg: SSHTransportConfig{
				Host:         "hpcc.example.edu",
				Port:         -1,
				User:         "researcher",
				IdentityFile: "~/.ssh/id_ed25519",
			},
			wantErr: "port must be between 1 and 65535",
		},
		{
			name: "invalid high port",
			cfg: SSHTransportConfig{
				Host:         "hpcc.example.edu",
				Port:         65536,
				User:         "researcher",
				IdentityFile: "~/.ssh/id_ed25519",
			},
			wantErr: "port must be between 1 and 65535",
		},
		{
			name: "jump host missing host",
			cfg: SSHTransportConfig{
				Host:          "dev.example.edu",
				User:          "researcher",
				IdentityEnv:   "GOETL_SSH_KEY",
				HostKeyPolicy: SSHHostKeyPolicyPinned,
				PinnedHostKey: "ssh-ed25519 AAAATARGET",
				JumpHosts: []SSHJumpHostConfig{{
					User:          "researcher",
					HostKeyPolicy: SSHHostKeyPolicyPinned,
					PinnedHostKey: "ssh-ed25519 AAAAGATEWAY",
				}},
			},
			wantErr: "jump_hosts[0] host is required",
		},
		{
			name: "jump host both identity sources set",
			cfg: SSHTransportConfig{
				Host:          "dev.example.edu",
				User:          "researcher",
				IdentityEnv:   "GOETL_SSH_KEY",
				HostKeyPolicy: SSHHostKeyPolicyPinned,
				PinnedHostKey: "ssh-ed25519 AAAATARGET",
				JumpHosts: []SSHJumpHostConfig{{
					Host:          "gateway.example.edu",
					User:          "researcher",
					IdentityFile:  "~/.ssh/id_gateway",
					IdentityEnv:   "GOETL_SSH_GATEWAY_KEY",
					HostKeyPolicy: SSHHostKeyPolicyPinned,
					PinnedHostKey: "ssh-ed25519 AAAAGATEWAY",
				}},
			},
			wantErr: "jump_hosts[0] identity_file and identity_env are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestSSHTransportConnectAcceptsPinnedHostKey(t *testing.T) {
	hostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	server := startTestSSHServer(t, hostSigner, clientIdentity.signer.PublicKey())

	envName := "GOETL_TEST_SSH_KEY_PINNED"
	t.Setenv(envName, clientIdentity.privatePEM)

	transport := SSHTransport{Config: testSSHTransportConfig(t, server.address, envName, SSHHostKeyPolicyPinned, hostSigner.PublicKey())}
	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect test SSH transport: %v", err)
	}
	if transport.client == nil {
		t.Fatal("expected connected SSH client")
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("close test SSH transport: %v", err)
	}
	if transport.client != nil {
		t.Fatal("expected close to clear SSH client")
	}
}

func TestSSHTransportConnectRejectsPinnedHostKeyMismatch(t *testing.T) {
	hostSigner := generateTestSSHSigner(t)
	wrongHostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	server := startTestSSHServer(t, hostSigner, clientIdentity.signer.PublicKey())

	envName := "GOETL_TEST_SSH_KEY_WRONG_HOST"
	t.Setenv(envName, clientIdentity.privatePEM)

	transport := SSHTransport{Config: testSSHTransportConfig(t, server.address, envName, SSHHostKeyPolicyPinned, wrongHostSigner.PublicKey())}
	err := transport.Connect(context.Background())
	if err == nil {
		t.Fatal("expected host key mismatch error")
	}
	if !strings.Contains(err.Error(), "handshake") {
		t.Fatalf("error = %v, want handshake context", err)
	}
}

func TestSSHTransportConnectAcceptsInsecureIgnoreHostKey(t *testing.T) {
	hostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	server := startTestSSHServer(t, hostSigner, clientIdentity.signer.PublicKey())

	envName := "GOETL_TEST_SSH_KEY_INSECURE"
	t.Setenv(envName, clientIdentity.privatePEM)

	transport := SSHTransport{Config: testSSHTransportConfig(t, server.address, envName, SSHHostKeyPolicyInsecureIgnore, nil)}
	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect test SSH transport: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("close test SSH transport: %v", err)
	}
}

func TestSSHTransportConnectRejectsWrongClientKey(t *testing.T) {
	hostSigner := generateTestSSHSigner(t)
	allowedClient := generateTestSSHIdentity(t)
	wrongClient := generateTestSSHIdentity(t)
	server := startTestSSHServer(t, hostSigner, allowedClient.signer.PublicKey())

	envName := "GOETL_TEST_SSH_KEY_WRONG_CLIENT"
	t.Setenv(envName, wrongClient.privatePEM)

	transport := SSHTransport{Config: testSSHTransportConfig(t, server.address, envName, SSHHostKeyPolicyPinned, hostSigner.PublicKey())}
	err := transport.Connect(context.Background())
	if err == nil {
		t.Fatal("expected authentication error")
	}
	if !strings.Contains(err.Error(), "handshake") {
		t.Fatalf("error = %v, want handshake context", err)
	}
}

func TestSSHTransportConnectRejectsMissingIdentityEnv(t *testing.T) {
	transport := SSHTransport{Config: SSHTransportConfig{
		Host:          "127.0.0.1",
		User:          "test-user",
		IdentityEnv:   "GOETL_TEST_SSH_KEY_MISSING",
		HostKeyPolicy: SSHHostKeyPolicyInsecureIgnore,
	}}

	err := transport.Connect(context.Background())
	if err == nil {
		t.Fatal("expected missing identity env error")
	}
	if !strings.Contains(err.Error(), "empty or unset") {
		t.Fatalf("error = %v, want missing identity context", err)
	}
}

func TestSSHTransportConnectUsesIdentityFile(t *testing.T) {
	hostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	server := startTestSSHServer(t, hostSigner, clientIdentity.signer.PublicKey())

	keyFile := t.TempDir() + "/id_ed25519"
	if err := os.WriteFile(keyFile, []byte(clientIdentity.privatePEM), 0600); err != nil {
		t.Fatalf("write test identity file: %v", err)
	}

	cfg := testSSHTransportConfig(t, server.address, "", SSHHostKeyPolicyPinned, hostSigner.PublicKey())
	cfg.IdentityEnv = ""
	cfg.IdentityFile = keyFile
	transport := SSHTransport{Config: cfg}

	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect test SSH transport: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("close test SSH transport: %v", err)
	}
}

func TestSSHTransportExecAndCopyThroughJumpHostUseFinalTarget(t *testing.T) {
	gatewayHostSigner := generateTestSSHSigner(t)
	finalHostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	gateway := startTestSSHServer(t, gatewayHostSigner, clientIdentity.signer.PublicKey())
	final := startTestSSHServer(t, finalHostSigner, clientIdentity.signer.PublicKey())

	envName := "GOETL_TEST_SSH_KEY_JUMP_EXEC_COPY"
	t.Setenv(envName, clientIdentity.privatePEM)

	cfg := testSSHTransportConfig(t, final.address, envName, SSHHostKeyPolicyPinned, finalHostSigner.PublicKey())
	cfg.JumpHosts = []SSHJumpHostConfig{
		testSSHJumpHostConfig(t, gateway.address, SSHHostKeyPolicyPinned, gatewayHostSigner.PublicKey()),
	}
	transport := SSHTransport{Config: cfg}
	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect through jump host: %v", err)
	}
	defer transport.Close()

	output, err := transport.Exec(context.Background(), "stdout-ok")
	if err != nil {
		t.Fatalf("exec through jump host: %v", err)
	}
	if string(output) != "stdout from ssh\n" {
		t.Fatalf("output = %q, want final target stdout", string(output))
	}
	if got := testSSHExecCount(gateway, "'stdout-ok'"); got != 0 {
		t.Fatalf("gateway exec count = %d, want 0", got)
	}
	if got := testSSHExecCount(final, "'stdout-ok'"); got != 1 {
		t.Fatalf("final exec count = %d, want 1", got)
	}

	localPath := writeTestLocalFile(t, "jump-copy-source.txt", "copied through jump\n")
	if err := transport.Copy(context.Background(), localPath, "through-jump/output.txt"); err != nil {
		t.Fatalf("copy through jump host: %v", err)
	}
	if got := readTestRemoteFile(t, final, "through-jump/output.txt"); got != "copied through jump\n" {
		t.Fatalf("final remote content = %q, want copied content", got)
	}
	gatewayPath := filepath.Join(gateway.remoteRoot, "through-jump", "output.txt")
	if _, err := os.Stat(gatewayPath); !os.IsNotExist(err) {
		t.Fatalf("gateway file exists or unexpected stat error: %v", err)
	}
}

func TestSSHTransportConnectRejectsWrongJumpHostKey(t *testing.T) {
	gatewayHostSigner := generateTestSSHSigner(t)
	wrongGatewayHostSigner := generateTestSSHSigner(t)
	finalHostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	gateway := startTestSSHServer(t, gatewayHostSigner, clientIdentity.signer.PublicKey())
	final := startTestSSHServer(t, finalHostSigner, clientIdentity.signer.PublicKey())

	envName := "GOETL_TEST_SSH_KEY_WRONG_JUMP_HOST"
	t.Setenv(envName, clientIdentity.privatePEM)

	cfg := testSSHTransportConfig(t, final.address, envName, SSHHostKeyPolicyPinned, finalHostSigner.PublicKey())
	cfg.JumpHosts = []SSHJumpHostConfig{
		testSSHJumpHostConfig(t, gateway.address, SSHHostKeyPolicyPinned, wrongGatewayHostSigner.PublicKey()),
	}
	transport := SSHTransport{Config: cfg}

	err := transport.Connect(context.Background())
	if err == nil {
		t.Fatal("expected jump host-key mismatch error")
	}
	if !strings.Contains(err.Error(), "jump_hosts[0]") || !strings.Contains(err.Error(), "handshake") {
		t.Fatalf("error = %v, want jump host handshake context", err)
	}
}

func TestSSHTransportConnectRejectsWrongFinalHostKeyThroughJumpHost(t *testing.T) {
	gatewayHostSigner := generateTestSSHSigner(t)
	finalHostSigner := generateTestSSHSigner(t)
	wrongFinalHostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	gateway := startTestSSHServer(t, gatewayHostSigner, clientIdentity.signer.PublicKey())
	final := startTestSSHServer(t, finalHostSigner, clientIdentity.signer.PublicKey())

	envName := "GOETL_TEST_SSH_KEY_WRONG_FINAL_HOST"
	t.Setenv(envName, clientIdentity.privatePEM)

	cfg := testSSHTransportConfig(t, final.address, envName, SSHHostKeyPolicyPinned, wrongFinalHostSigner.PublicKey())
	cfg.JumpHosts = []SSHJumpHostConfig{
		testSSHJumpHostConfig(t, gateway.address, SSHHostKeyPolicyPinned, gatewayHostSigner.PublicKey()),
	}
	transport := SSHTransport{Config: cfg}

	err := transport.Connect(context.Background())
	if err == nil {
		t.Fatal("expected final host-key mismatch error")
	}
	for _, want := range []string{"target", "jump_hosts[0]", "handshake"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want context %q", err, want)
		}
	}
}

func TestSSHTransportConnectReportsFinalConnectionFailureThroughJumpHost(t *testing.T) {
	gatewayHostSigner := generateTestSSHSigner(t)
	finalHostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	gateway := startTestSSHServer(t, gatewayHostSigner, clientIdentity.signer.PublicKey())
	finalAddress := unusedTestTCPAddress(t)

	envName := "GOETL_TEST_SSH_KEY_FINAL_CONNECT_FAIL"
	t.Setenv(envName, clientIdentity.privatePEM)

	cfg := testSSHTransportConfig(t, finalAddress, envName, SSHHostKeyPolicyPinned, finalHostSigner.PublicKey())
	cfg.JumpHosts = []SSHJumpHostConfig{
		testSSHJumpHostConfig(t, gateway.address, SSHHostKeyPolicyPinned, gatewayHostSigner.PublicKey()),
	}
	transport := SSHTransport{Config: cfg}

	err := transport.Connect(context.Background())
	if err == nil {
		t.Fatal("expected final connection failure")
	}
	for _, want := range []string{"target", "jump_hosts[0]", finalAddress} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want context %q", err, want)
		}
	}
}

func TestSSHTransportConnectHonorsCanceledContext(t *testing.T) {
	clientIdentity := generateTestSSHIdentity(t)
	envName := "GOETL_TEST_SSH_KEY_CANCELED"
	t.Setenv(envName, clientIdentity.privatePEM)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transport := SSHTransport{Config: SSHTransportConfig{
		Host:           "192.0.2.1",
		Port:           22,
		User:           "test-user",
		IdentityEnv:    envName,
		HostKeyPolicy:  SSHHostKeyPolicyInsecureIgnore,
		ConnectTimeout: "5s",
	}}

	err := transport.Connect(ctx)
	if err == nil {
		t.Fatal("expected canceled context error")
	}
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "operation was canceled") {
		t.Fatalf("error = %v, want canceled context", err)
	}
}

func TestSSHTransportExecReturnsStdout(t *testing.T) {
	transport := connectTestSSHTransport(t, "GOETL_TEST_SSH_KEY_EXEC_STDOUT")
	defer transport.Close()

	output, err := transport.Exec(context.Background(), "stdout-ok")
	if err != nil {
		t.Fatalf("exec test SSH command: %v", err)
	}

	if string(output) != "stdout from ssh\n" {
		t.Fatalf("output = %q, want stdout", string(output))
	}
}

func TestSSHTransportExecSucceedsWhenCommandWritesStderr(t *testing.T) {
	transport := connectTestSSHTransport(t, "GOETL_TEST_SSH_KEY_EXEC_STDERR")
	defer transport.Close()

	output, err := transport.Exec(context.Background(), "stderr-ok")
	if err != nil {
		t.Fatalf("exec test SSH command: %v", err)
	}

	if string(output) != "stdout despite stderr\n" {
		t.Fatalf("output = %q, want stdout", string(output))
	}
}

func TestSSHTransportExecReportsNonzeroExit(t *testing.T) {
	transport := connectTestSSHTransport(t, "GOETL_TEST_SSH_KEY_EXEC_FAIL")
	defer transport.Close()

	output, err := transport.Exec(context.Background(), "fail-command")
	if err == nil {
		t.Fatal("expected command failure")
	}
	if len(output) != 0 {
		t.Fatalf("output = %q, want empty stdout", string(output))
	}
	if !strings.Contains(err.Error(), "Exit status 7") && !strings.Contains(err.Error(), "Process exited with status 7") {
		t.Fatalf("error = %v, want exit status", err)
	}
	if !strings.Contains(err.Error(), "remote command failed") {
		t.Fatalf("error = %v, want stderr", err)
	}
}

func TestSSHTransportExecRequiresConnection(t *testing.T) {
	transport := SSHTransport{}

	_, err := transport.Exec(context.Background(), "stdout-ok")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("error = %v, want not connected", err)
	}
}

func TestSSHTransportExecHonorsCommandTimeout(t *testing.T) {
	transport := connectTestSSHTransport(t, "GOETL_TEST_SSH_KEY_EXEC_TIMEOUT")
	defer transport.Close()
	transport.Config.CommandTimeout = "20ms"

	_, err := transport.Exec(context.Background(), "wait-for-cancel")
	if err == nil {
		t.Fatal("expected command timeout")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestSSHTransportExecPreservesArgumentBoundaries(t *testing.T) {
	transport := connectTestSSHTransport(t, "GOETL_TEST_SSH_KEY_EXEC_ARGS")
	defer transport.Close()

	output, err := transport.Exec(context.Background(), "arg check", "two words")
	if err != nil {
		t.Fatalf("exec test SSH command: %v", err)
	}

	if string(output) != "args preserved\n" {
		t.Fatalf("output = %q, want args preserved", string(output))
	}
}

func TestSSHTransportExecReconnectsAfterClosedIdleConnection(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_RECONNECT_EXEC")
	defer transport.Close()
	if err := transport.client.Close(); err != nil {
		t.Fatalf("close idle SSH client: %v", err)
	}

	output, err := transport.Exec(context.Background(), "stdout-ok")
	if err != nil {
		t.Fatalf("exec after closed idle connection: %v", err)
	}

	if string(output) != "stdout from ssh\n" {
		t.Fatalf("output = %q, want stdout", string(output))
	}
	if got := testSSHExecCount(server, "'stdout-ok'"); got != 1 {
		t.Fatalf("exec count = %d, want 1", got)
	}
}

func TestSSHTransportExecDoesNotRetryRemoteNonzeroExit(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_NO_RETRY_NONZERO")
	defer transport.Close()

	_, err := transport.Exec(context.Background(), "fail-command")
	if err == nil {
		t.Fatal("expected command failure")
	}
	if got := testSSHExecCount(server, "'fail-command'"); got != 1 {
		t.Fatalf("exec count = %d, want 1", got)
	}
}

func TestSSHTransportCopyWritesFileContent(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_COPY_CONTENT")
	defer transport.Close()

	localPath := writeTestLocalFile(t, "copy-source.txt", "copied content\n")
	if err := transport.Copy(context.Background(), localPath, "remote/output.txt"); err != nil {
		t.Fatalf("copy test file: %v", err)
	}

	got := readTestRemoteFile(t, server, "remote/output.txt")
	if got != "copied content\n" {
		t.Fatalf("remote content = %q, want copied content", got)
	}
}

func TestSSHTransportCopyCreatesNestedParentDirectory(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_COPY_NESTED")
	defer transport.Close()

	localPath := writeTestLocalFile(t, "nested-source.txt", "nested content\n")
	if err := transport.Copy(context.Background(), localPath, "a/b/c/output.txt"); err != nil {
		t.Fatalf("copy test file: %v", err)
	}

	got := readTestRemoteFile(t, server, "a/b/c/output.txt")
	if got != "nested content\n" {
		t.Fatalf("remote content = %q, want nested content", got)
	}
}

func TestSSHTransportCopyReplacesExistingRemoteFile(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_COPY_REPLACE")
	defer transport.Close()

	remotePath := filepath.Join(server.remoteRoot, "replace/output.txt")
	if err := os.MkdirAll(filepath.Dir(remotePath), 0755); err != nil {
		t.Fatalf("create remote parent: %v", err)
	}
	if err := os.WriteFile(remotePath, []byte("old content\n"), 0644); err != nil {
		t.Fatalf("write old remote file: %v", err)
	}

	localPath := writeTestLocalFile(t, "replace-source.txt", "new content\n")
	if err := transport.Copy(context.Background(), localPath, "replace/output.txt"); err != nil {
		t.Fatalf("copy test file: %v", err)
	}

	got := readTestRemoteFile(t, server, "replace/output.txt")
	if got != "new content\n" {
		t.Fatalf("remote content = %q, want new content", got)
	}
}

func TestSSHTransportCopyCanceledBeforeTransferLeavesFinalUnchanged(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_COPY_CANCEL")
	defer transport.Close()

	remotePath := filepath.Join(server.remoteRoot, "cancel/output.txt")
	if err := os.MkdirAll(filepath.Dir(remotePath), 0755); err != nil {
		t.Fatalf("create remote parent: %v", err)
	}
	if err := os.WriteFile(remotePath, []byte("old content\n"), 0644); err != nil {
		t.Fatalf("write old remote file: %v", err)
	}

	localPath := writeTestLocalFile(t, "cancel-source.txt", "new content\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := transport.Copy(ctx, localPath, "cancel/output.txt")
	if err == nil {
		t.Fatal("expected canceled copy error")
	}

	got := readTestRemoteFile(t, server, "cancel/output.txt")
	if got != "old content\n" {
		t.Fatalf("remote content = %q, want old content", got)
	}
}

func TestSSHTransportCopyRemovesTempFileOnPromoteFailure(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_COPY_CLEANUP")
	defer transport.Close()

	finalDir := filepath.Join(server.remoteRoot, "cleanup/output.txt")
	if err := os.MkdirAll(finalDir, 0755); err != nil {
		t.Fatalf("create remote final directory: %v", err)
	}

	localPath := writeTestLocalFile(t, "cleanup-source.txt", "cleanup content\n")
	err := transport.Copy(context.Background(), localPath, "cleanup/output.txt")
	if err == nil {
		t.Fatal("expected promote failure")
	}

	matches, err := filepath.Glob(filepath.Join(server.remoteRoot, "cleanup/.output.txt.goetl-tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("left temp files: %#v", matches)
	}
}

func TestSSHTransportCopyRejectsMissingLocalSource(t *testing.T) {
	transport, _ := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_COPY_MISSING")
	defer transport.Close()

	err := transport.Copy(context.Background(), filepath.Join(t.TempDir(), "missing.txt"), "missing/output.txt")
	if err == nil {
		t.Fatal("expected missing source error")
	}
	if !strings.Contains(err.Error(), "open local copy source") {
		t.Fatalf("error = %v, want local source context", err)
	}
}

func TestSSHTransportListReturnsDirectoryEntries(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_LIST_ENTRIES")
	defer transport.Close()
	writeTestRemoteFile(t, server, "list/file.txt", "file content")
	mkdirTestRemote(t, server, "list/child")

	entries, err := transport.List(context.Background(), "list")
	if err != nil {
		t.Fatalf("list remote directory: %v", err)
	}

	byName := remoteEntriesByName(entries)
	fileEntry, ok := byName["file.txt"]
	if !ok {
		t.Fatalf("missing file entry in %#v", entries)
	}
	if fileEntry.IsDir {
		t.Fatal("file entry reported as directory")
	}
	if fileEntry.Size != int64(len("file content")) {
		t.Fatalf("file size = %d, want %d", fileEntry.Size, len("file content"))
	}
	if fileEntry.Path != "list/file.txt" {
		t.Fatalf("file path = %q, want list/file.txt", fileEntry.Path)
	}

	dirEntry, ok := byName["child"]
	if !ok {
		t.Fatalf("missing directory entry in %#v", entries)
	}
	if !dirEntry.IsDir {
		t.Fatal("directory entry reported as file")
	}
	if dirEntry.Path != "list/child" {
		t.Fatalf("directory path = %q, want list/child", dirEntry.Path)
	}
}

func TestSSHTransportListReturnsEmptyDirectory(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_LIST_EMPTY")
	defer transport.Close()
	mkdirTestRemote(t, server, "empty")

	entries, err := transport.List(context.Background(), "empty")
	if err != nil {
		t.Fatalf("list empty remote directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %#v, want empty", entries)
	}
}

func TestSSHTransportListReportsMissingPath(t *testing.T) {
	transport, _ := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_LIST_MISSING")
	defer transport.Close()

	_, err := transport.List(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected missing path error")
	}
	if !strings.Contains(err.Error(), "list remote path") {
		t.Fatalf("error = %v, want path context", err)
	}
}

func TestSSHTransportListReportsRegularFile(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_LIST_FILE")
	defer transport.Close()
	writeTestRemoteFile(t, server, "file.txt", "content")

	_, err := transport.List(context.Background(), "file.txt")
	if err == nil {
		t.Fatal("expected regular file listing error")
	}
	if !strings.Contains(err.Error(), "list remote path") {
		t.Fatalf("error = %v, want path context", err)
	}
}

func TestSSHTransportListHonorsCanceledContext(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_LIST_CANCEL")
	defer transport.Close()
	mkdirTestRemote(t, server, "cancel-list")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := transport.List(ctx, "cancel-list")
	if err == nil {
		t.Fatal("expected canceled list error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want canceled context", err)
	}
}

func TestSSHTransportMakeDirectoryCreatesNestedDirectory(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_MKDIR")
	defer transport.Close()

	if err := transport.MakeDirectory(context.Background(), "fs/a/b"); err != nil {
		t.Fatalf("make remote directory: %v", err)
	}
	if info, err := os.Stat(filepath.Join(server.remoteRoot, "fs/a/b")); err != nil || !info.IsDir() {
		t.Fatalf("remote directory not created, info=%v err=%v", info, err)
	}
}

func TestSSHTransportMovePromotesFile(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_MOVE")
	defer transport.Close()
	writeTestRemoteFile(t, server, "move/temp file.txt", "move content")

	if err := transport.Move(context.Background(), "move/temp file.txt", "move/final file.txt"); err != nil {
		t.Fatalf("move remote file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(server.remoteRoot, "move/temp file.txt")); !os.IsNotExist(err) {
		t.Fatalf("source still exists or unexpected err: %v", err)
	}
	got := readTestRemoteFile(t, server, "move/final file.txt")
	if got != "move content" {
		t.Fatalf("remote content = %q, want move content", got)
	}
}

func TestSSHTransportRemoveFileRemovesOneFile(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_RM")
	defer transport.Close()
	writeTestRemoteFile(t, server, "remove/file.txt", "remove")

	if err := transport.RemoveFile(context.Background(), "remove/file.txt"); err != nil {
		t.Fatalf("remove remote file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(server.remoteRoot, "remove/file.txt")); !os.IsNotExist(err) {
		t.Fatalf("file still exists or unexpected err: %v", err)
	}
}

func TestSSHTransportRemoveTreeRequiresExplicitRecursiveHelper(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_RMRF")
	defer transport.Close()
	writeTestRemoteFile(t, server, "tree/child/file.txt", "tree")

	if err := transport.RemoveTree(context.Background(), "tree"); err != nil {
		t.Fatalf("remove remote tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(server.remoteRoot, "tree")); !os.IsNotExist(err) {
		t.Fatalf("tree still exists or unexpected err: %v", err)
	}
}

func TestSSHTransportRemoveTreeRejectsRoot(t *testing.T) {
	transport, _ := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_ROOT")
	defer transport.Close()

	if err := transport.RemoveTree(context.Background(), "/"); err == nil {
		t.Fatal("expected root remove rejection")
	}
}

func TestSSHTransportChmodAppliesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows test filesystem does not preserve POSIX chmod bits")
	}

	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_CHMOD")
	defer transport.Close()
	writeTestRemoteFile(t, server, "chmod/file.txt", "chmod")

	if err := transport.Chmod(context.Background(), "0600", "chmod/file.txt"); err != nil {
		t.Fatalf("chmod remote file: %v", err)
	}
	info, err := os.Stat(filepath.Join(server.remoteRoot, "chmod/file.txt"))
	if err != nil {
		t.Fatalf("stat chmod file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("mode = %v, want 0600", got)
	}
}

func TestSSHTransportChownReportsPermissionFailure(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_CHOWN")
	defer transport.Close()
	writeTestRemoteFile(t, server, "chown/file.txt", "chown")

	err := transport.Chown(context.Background(), "nobody:nogroup", "chown/file.txt")
	if err == nil {
		t.Fatal("expected chown failure")
	}
	if !strings.Contains(err.Error(), "operation not permitted") {
		t.Fatalf("error = %v, want permission context", err)
	}
}

func TestSSHTransportFilesystemHelpersQuoteSpaces(t *testing.T) {
	transport, server := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_SPACES")
	defer transport.Close()

	if err := transport.MakeDirectory(context.Background(), "space dir/nested dir"); err != nil {
		t.Fatalf("make directory with spaces: %v", err)
	}
	if info, err := os.Stat(filepath.Join(server.remoteRoot, "space dir/nested dir")); err != nil || !info.IsDir() {
		t.Fatalf("remote spaced directory not created, info=%v err=%v", info, err)
	}
}

func TestSSHTransportMoveReportsMissingSource(t *testing.T) {
	transport, _ := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_MISSING_MOVE")
	defer transport.Close()

	err := transport.Move(context.Background(), "missing/source.txt", "missing/dest.txt")
	if err == nil {
		t.Fatal("expected missing source error")
	}
	if !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("error = %v, want missing source context", err)
	}
}

func TestSSHTransportFilesystemHelperHonorsCanceledContext(t *testing.T) {
	transport, _ := connectTestSSHTransportWithServer(t, "GOETL_TEST_SSH_KEY_FS_CANCEL")
	defer transport.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := transport.MakeDirectory(ctx, "canceled")
	if err == nil {
		t.Fatal("expected canceled command error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want canceled context", err)
	}
}

func connectTestSSHTransport(t *testing.T, envName string) *SSHTransport {
	t.Helper()

	transport, _ := connectTestSSHTransportWithServer(t, envName)
	return transport
}

func connectTestSSHTransportWithServer(t *testing.T, envName string) (*SSHTransport, testSSHServer) {
	t.Helper()

	hostSigner := generateTestSSHSigner(t)
	clientIdentity := generateTestSSHIdentity(t)
	server := startTestSSHServer(t, hostSigner, clientIdentity.signer.PublicKey())
	t.Setenv(envName, clientIdentity.privatePEM)

	transport := &SSHTransport{Config: testSSHTransportConfig(t, server.address, envName, SSHHostKeyPolicyPinned, hostSigner.PublicKey())}
	if err := transport.Connect(context.Background()); err != nil {
		t.Fatalf("connect test SSH transport: %v", err)
	}
	return transport, server
}

func writeTestLocalFile(t *testing.T, name string, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write local test file: %v", err)
	}
	return path
}

func readTestRemoteFile(t *testing.T, server testSSHServer, remotePath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(server.remoteRoot, filepath.FromSlash(remotePath)))
	if err != nil {
		t.Fatalf("read remote test file: %v", err)
	}
	return string(data)
}

func writeTestRemoteFile(t *testing.T, server testSSHServer, remotePath string, content string) {
	t.Helper()

	path := filepath.Join(server.remoteRoot, filepath.FromSlash(remotePath))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create remote parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write remote test file: %v", err)
	}
}

func mkdirTestRemote(t *testing.T, server testSSHServer, remotePath string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(server.remoteRoot, filepath.FromSlash(remotePath)), 0755); err != nil {
		t.Fatalf("create remote test directory: %v", err)
	}
}

func remoteEntriesByName(entries []RemoteFileInfo) map[string]RemoteFileInfo {
	byName := make(map[string]RemoteFileInfo, len(entries))
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	return byName
}

func testSSHExecCount(server testSSHServer, command string) int64 {
	value, ok := server.execCounts.Load(command)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(value.(*int64))
}

func testSSHTransportConfig(t *testing.T, address string, identityEnv string, hostKeyPolicy string, pinnedHostKey ssh.PublicKey) SSHTransportConfig {
	t.Helper()

	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split test SSH address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse test SSH port: %v", err)
	}

	cfg := SSHTransportConfig{
		Host:           host,
		Port:           port,
		User:           "test-user",
		IdentityEnv:    identityEnv,
		HostKeyPolicy:  hostKeyPolicy,
		ConnectTimeout: "2s",
	}
	if pinnedHostKey != nil {
		cfg.PinnedHostKey = string(ssh.MarshalAuthorizedKey(pinnedHostKey))
	}
	return cfg
}

func testSSHJumpHostConfig(t *testing.T, address string, hostKeyPolicy string, pinnedHostKey ssh.PublicKey) SSHJumpHostConfig {
	t.Helper()

	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split test SSH address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse test SSH port: %v", err)
	}

	cfg := SSHJumpHostConfig{
		Host:          host,
		Port:          port,
		User:          "test-user",
		HostKeyPolicy: hostKeyPolicy,
	}
	if pinnedHostKey != nil {
		cfg.PinnedHostKey = string(ssh.MarshalAuthorizedKey(pinnedHostKey))
	}
	return cfg
}

func unusedTestTCPAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for unused address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close unused address listener: %v", err)
	}
	return address
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	SSHHostKeyPolicyKnownHosts     = "known_hosts"
	SSHHostKeyPolicyPinned         = "pinned"
	SSHHostKeyPolicyInsecureIgnore = "insecure_ignore"

	defaultSSHPort           = 22
	defaultSSHConnectTimeout = 10 * time.Second
)

type SSHTransportConfig struct {
	Host           string `json:"host"`
	Port           int    `json:"port,omitempty"`
	User           string `json:"user"`
	IdentityFile   string `json:"identity_file,omitempty"`
	IdentityEnv    string `json:"identity_env,omitempty"`
	KnownHostsFile string `json:"known_hosts_file,omitempty"`
	HostKeyPolicy  string `json:"host_key_policy,omitempty"`
	PinnedHostKey  string `json:"pinned_host_key,omitempty"`
	ConnectTimeout string `json:"connect_timeout,omitempty"`
	CommandTimeout string `json:"command_timeout,omitempty"`
	KeepAlive      bool   `json:"keep_alive,omitempty"`
}

type SSHTransport struct {
	Config  SSHTransportConfig
	Dialect ShellDialect
	client  *ssh.Client
}

func (t *SSHTransport) Connect(ctx context.Context) error {
	if err := t.Config.Validate(); err != nil {
		return err
	}

	clientConfig, err := t.sshClientConfig()
	if err != nil {
		return err
	}

	dialCtx, cancel, err := t.connectContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", t.address())
	if err != nil {
		return fmt.Errorf("ssh connect to %s: %w", t.address(), err)
	}

	sshConn, channels, requests, err := ssh.NewClientConn(conn, t.address(), clientConfig)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("ssh handshake with %s: %w", t.address(), err)
	}

	t.client = ssh.NewClient(sshConn, channels, requests)
	return nil
}

func (t *SSHTransport) Close() error {
	if t.client == nil {
		return nil
	}

	err := t.client.Close()
	t.client = nil
	return err
}

func (t *SSHTransport) Exec(ctx context.Context, args ...string) ([]byte, error) {
	if t.client == nil {
		return nil, fmt.Errorf("ssh transport is not connected")
	}

	command, err := t.commandString(args...)
	if err != nil {
		return nil, err
	}

	session, err := t.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh open session: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	runCtx, cancel, err := t.commandContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()

	if err := session.Start(command); err != nil {
		return nil, fmt.Errorf("ssh start command %q: %w", command, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return stdout.Bytes(), sshCommandError(command, stderr.String(), err)
		}
		return stdout.Bytes(), nil
	case <-runCtx.Done():
		_ = session.Close()
		return stdout.Bytes(), fmt.Errorf("ssh command %q canceled: %w", command, runCtx.Err())
	}
}

func (cfg SSHTransportConfig) Validate() error {
	if cfg.Host == "" {
		return fmt.Errorf("ssh host is required")
	}
	if cfg.User == "" {
		return fmt.Errorf("ssh user is required")
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("ssh port must be between 1 and 65535")
	}
	if cfg.IdentityFile == "" && cfg.IdentityEnv == "" {
		return fmt.Errorf("ssh identity_file or identity_env is required")
	}
	if cfg.IdentityFile != "" && cfg.IdentityEnv != "" {
		return fmt.Errorf("ssh identity_file and identity_env are mutually exclusive")
	}
	if err := cfg.validateHostKeyPolicy(); err != nil {
		return err
	}
	if err := validateSSHDuration("connect_timeout", cfg.ConnectTimeout); err != nil {
		return err
	}
	if err := validateSSHDuration("command_timeout", cfg.CommandTimeout); err != nil {
		return err
	}
	return nil
}

func (t SSHTransport) commandString(args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("ssh command is required")
	}

	dialect := t.Dialect
	if dialect == nil {
		dialect = BashShellPlatform{}
	}

	quoted := make([]string, 0, len(args))
	for index, arg := range args {
		if arg == "" {
			return "", fmt.Errorf("ssh command arg[%d] is required", index)
		}
		if containsNewline(arg) {
			return "", fmt.Errorf("ssh command arg[%d] must not contain newlines", index)
		}
		quoted = append(quoted, dialect.QuoteArg(arg))
	}
	return strings.Join(quoted, " "), nil
}

func (t SSHTransport) commandContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if t.Config.CommandTimeout == "" {
		runCtx, cancel := context.WithCancel(ctx)
		return runCtx, cancel, nil
	}

	timeout, err := time.ParseDuration(t.Config.CommandTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh command_timeout must be a Go duration: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	return runCtx, cancel, nil
}

func sshCommandError(command string, stderr string, err error) error {
	message := fmt.Sprintf("ssh command %q failed: %v", command, err)
	stderr = strings.TrimSpace(stderr)
	if stderr != "" {
		message += ": " + stderr
	}
	return fmt.Errorf("%s", message)
}

func (t SSHTransport) sshClientConfig() (*ssh.ClientConfig, error) {
	signer, err := t.identitySigner()
	if err != nil {
		return nil, err
	}

	hostKeyCallback, err := t.hostKeyCallback()
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User: t.Config.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
	}, nil
}

func (t SSHTransport) identitySigner() (ssh.Signer, error) {
	var key []byte
	var err error

	switch {
	case t.Config.IdentityFile != "":
		key, err = os.ReadFile(t.Config.IdentityFile)
		if err != nil {
			return nil, fmt.Errorf("read ssh identity_file: %w", err)
		}
	case t.Config.IdentityEnv != "":
		value := os.Getenv(t.Config.IdentityEnv)
		if value == "" {
			return nil, fmt.Errorf("ssh identity_env %s is empty or unset", t.Config.IdentityEnv)
		}
		key = []byte(value)
	default:
		return nil, fmt.Errorf("ssh identity_file or identity_env is required")
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse ssh identity: %w", err)
	}
	return signer, nil
}

func (t SSHTransport) hostKeyCallback() (ssh.HostKeyCallback, error) {
	switch t.hostKeyPolicy() {
	case SSHHostKeyPolicyPinned:
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(t.Config.PinnedHostKey))
		if err != nil {
			return nil, fmt.Errorf("parse ssh pinned_host_key: %w", err)
		}
		return ssh.FixedHostKey(key), nil
	case SSHHostKeyPolicyInsecureIgnore:
		return ssh.InsecureIgnoreHostKey(), nil
	case SSHHostKeyPolicyKnownHosts:
		return nil, fmt.Errorf("ssh known_hosts host-key verification is not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported ssh host_key_policy %q", t.Config.HostKeyPolicy)
	}
}

func (t SSHTransport) connectContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	timeout := defaultSSHConnectTimeout
	if t.Config.ConnectTimeout != "" {
		duration, err := time.ParseDuration(t.Config.ConnectTimeout)
		if err != nil {
			return nil, nil, fmt.Errorf("ssh connect_timeout must be a Go duration: %w", err)
		}
		timeout = duration
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	return dialCtx, cancel, nil
}

func (t SSHTransport) address() string {
	return net.JoinHostPort(t.Config.Host, strconv.Itoa(t.port()))
}

func (t SSHTransport) port() int {
	if t.Config.Port == 0 {
		return defaultSSHPort
	}
	return t.Config.Port
}

func (t SSHTransport) hostKeyPolicy() string {
	if t.Config.HostKeyPolicy == "" {
		return SSHHostKeyPolicyKnownHosts
	}
	return t.Config.HostKeyPolicy
}

func (cfg SSHTransportConfig) validateHostKeyPolicy() error {
	policy := cfg.HostKeyPolicy
	if policy == "" {
		policy = SSHHostKeyPolicyKnownHosts
	}

	switch policy {
	case SSHHostKeyPolicyKnownHosts:
		return nil
	case SSHHostKeyPolicyPinned:
		if cfg.PinnedHostKey == "" {
			return fmt.Errorf("ssh pinned_host_key is required when host_key_policy is pinned")
		}
		return nil
	case SSHHostKeyPolicyInsecureIgnore:
		return nil
	default:
		return fmt.Errorf("unsupported ssh host_key_policy %q", cfg.HostKeyPolicy)
	}
}

func validateSSHDuration(name string, value string) error {
	if value == "" {
		return nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("ssh %s must be a Go duration: %w", name, err)
	}
	if duration <= 0 {
		return fmt.Errorf("ssh %s must be greater than zero", name)
	}
	return nil
}

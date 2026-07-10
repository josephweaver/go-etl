package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	SSHHostKeyPolicyKnownHosts     = "known_hosts"
	SSHHostKeyPolicyPinned         = "pinned"
	SSHHostKeyPolicyInsecureIgnore = "insecure_ignore"

	defaultSSHPort           = 22
	defaultSSHConnectTimeout = 10 * time.Second
)

type SSHTransportConfig struct {
	Host           string              `json:"host"`
	Port           int                 `json:"port,omitempty"`
	User           string              `json:"user"`
	IdentityFile   string              `json:"identity_file,omitempty"`
	IdentityEnv    string              `json:"identity_env,omitempty"`
	KnownHostsFile string              `json:"known_hosts_file,omitempty"`
	HostKeyPolicy  string              `json:"host_key_policy,omitempty"`
	PinnedHostKey  string              `json:"pinned_host_key,omitempty"`
	ConnectTimeout string              `json:"connect_timeout,omitempty"`
	CommandTimeout string              `json:"command_timeout,omitempty"`
	KeepAlive      bool                `json:"keep_alive,omitempty"`
	JumpHosts      []SSHJumpHostConfig `json:"jump_hosts,omitempty"`
}

type SSHJumpHostConfig struct {
	Host           string `json:"host"`
	Port           int    `json:"port,omitempty"`
	User           string `json:"user"`
	IdentityFile   string `json:"identity_file,omitempty"`
	IdentityEnv    string `json:"identity_env,omitempty"`
	KnownHostsFile string `json:"known_hosts_file,omitempty"`
	HostKeyPolicy  string `json:"host_key_policy,omitempty"`
	PinnedHostKey  string `json:"pinned_host_key,omitempty"`
}

type SSHTransport struct {
	Config      SSHTransportConfig
	Dialect     ShellDialect
	client      *ssh.Client
	jumpClients []*ssh.Client
}

type RemoteFileInfo struct {
	Path  string
	Name  string
	IsDir bool
	Size  int64
}

func (t *SSHTransport) Connect(ctx context.Context) error {
	if err := t.Config.Validate(); err != nil {
		return err
	}
	if t.client != nil || len(t.jumpClients) > 0 {
		_ = t.Close()
	}

	dialCtx, cancel, err := t.connectContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	dial := func(ctx context.Context, network string, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}

	jumpClients := make([]*ssh.Client, 0, len(t.Config.JumpHosts))
	closeJumpClients := func() {
		for index := len(jumpClients) - 1; index >= 0; index-- {
			_ = jumpClients[index].Close()
		}
	}

	for index := range t.Config.JumpHosts {
		endpoint := t.Config.jumpHostEndpoint(index)
		jumpClient, err := connectSSHEndpoint(dialCtx, endpoint, dial)
		if err != nil {
			closeJumpClients()
			return fmt.Errorf("ssh connect jump_hosts[%d] %s: %w", index, sshAddress(endpoint), err)
		}
		jumpClients = append(jumpClients, jumpClient)
		hopIndex := index
		dial = func(ctx context.Context, network string, address string) (net.Conn, error) {
			return dialSSHClient(ctx, jumpClients[hopIndex], network, address)
		}
	}

	targetClient, err := connectSSHEndpoint(dialCtx, t.Config.targetEndpoint(), dial)
	if err != nil {
		closeJumpClients()
		if len(t.Config.JumpHosts) > 0 {
			lastHop := t.Config.jumpHostEndpoint(len(t.Config.JumpHosts) - 1)
			return fmt.Errorf("ssh connect target %s through jump_hosts[%d] %s: %w", t.address(), len(t.Config.JumpHosts)-1, sshAddress(lastHop), err)
		}
		return err
	}

	t.client = targetClient
	t.jumpClients = jumpClients
	return nil
}

func (t *SSHTransport) Close() error {
	var closeErr error
	if t.client == nil {
		closeErr = nil
	} else {
		closeErr = t.client.Close()
		t.client = nil
	}

	for index := len(t.jumpClients) - 1; index >= 0; index-- {
		if err := t.jumpClients[index].Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	t.jumpClients = nil
	return closeErr
}

func (t *SSHTransport) Exec(ctx context.Context, args ...string) ([]byte, error) {
	command, err := t.commandString(args...)
	if err != nil {
		return nil, err
	}
	return t.execCommand(ctx, command)
}

func (t *SSHTransport) execCommand(ctx context.Context, command string) ([]byte, error) {
	if t.client == nil {
		return nil, fmt.Errorf("ssh transport is not connected")
	}

	session, err := t.client.NewSession()
	if err != nil {
		if reconnectErr := t.reconnect(ctx); reconnectErr != nil {
			return nil, fmt.Errorf("ssh open session: %w; reconnect failed: %v", err, reconnectErr)
		}
		session, err = t.client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("ssh open session after reconnect: %w", err)
		}
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

func (t *SSHTransport) Copy(ctx context.Context, localPath string, remotePath string) error {
	if t.client == nil {
		return fmt.Errorf("ssh transport is not connected")
	}
	if err := validateSSHCopyPath("local path", localPath); err != nil {
		return err
	}
	if err := validateSSHCopyPath("remote path", remotePath); err != nil {
		return err
	}

	runCtx, cancel, err := t.commandContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	source, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local copy source: %w", err)
	}
	defer source.Close()

	client, err := t.sftpClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	remoteDir := path.Dir(remotePath)
	if remoteDir != "." && remoteDir != "/" {
		if err := client.MkdirAll(remoteDir); err != nil {
			return fmt.Errorf("create remote parent directory %q: %w", remoteDir, err)
		}
	}

	tempPath, err := remoteTempPath(remotePath)
	if err != nil {
		return err
	}

	if err := runCtx.Err(); err != nil {
		return fmt.Errorf("ssh copy canceled before transfer: %w", err)
	}

	target, err := client.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create remote temp file %q: %w", tempPath, err)
	}
	tempCreated := true
	cleanupTemp := func() {
		if tempCreated {
			_ = client.Remove(tempPath)
		}
	}

	if _, err := copyWithContext(runCtx, target, source); err != nil {
		_ = target.Close()
		cleanupTemp()
		return fmt.Errorf("copy local file to remote temp file %q: %w", tempPath, err)
	}
	if err := target.Close(); err != nil {
		cleanupTemp()
		return fmt.Errorf("close remote temp file %q: %w", tempPath, err)
	}

	if err := runCtx.Err(); err != nil {
		cleanupTemp()
		return fmt.Errorf("ssh copy canceled before promote: %w", err)
	}
	if err := client.PosixRename(tempPath, remotePath); err != nil {
		cleanupTemp()
		return fmt.Errorf("promote remote temp file %q to %q: %w", tempPath, remotePath, err)
	}
	tempCreated = false
	return nil
}

func (t *SSHTransport) List(ctx context.Context, remotePath string) ([]RemoteFileInfo, error) {
	if t.client == nil {
		return nil, fmt.Errorf("ssh transport is not connected")
	}
	if err := validateSSHRemotePath("list path", remotePath); err != nil {
		return nil, err
	}

	runCtx, cancel, err := t.commandContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()

	if err := runCtx.Err(); err != nil {
		return nil, fmt.Errorf("ssh list canceled before request: %w", err)
	}

	client, err := t.sftpClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	entries, err := client.ReadDir(remotePath)
	if err != nil {
		return nil, fmt.Errorf("list remote path %q: %w", remotePath, err)
	}
	if err := runCtx.Err(); err != nil {
		return nil, fmt.Errorf("ssh list canceled after request: %w", err)
	}

	infos := make([]RemoteFileInfo, 0, len(entries))
	for _, entry := range entries {
		infos = append(infos, RemoteFileInfo{
			Path:  path.Join(remotePath, entry.Name()),
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  entry.Size(),
		})
	}
	return infos, nil
}

func (t *SSHTransport) MakeDirectory(ctx context.Context, remotePath string) error {
	command, err := t.filesystemDialect().MakeDirectoryCommand(remotePath)
	if err != nil {
		return err
	}
	_, err = t.runShellCommand(ctx, command)
	return err
}

func (t *SSHTransport) Move(ctx context.Context, sourcePath string, destinationPath string) error {
	command, err := t.filesystemDialect().MoveCommand(sourcePath, destinationPath)
	if err != nil {
		return err
	}
	_, err = t.runShellCommand(ctx, command)
	return err
}

func (t *SSHTransport) RemoveFile(ctx context.Context, remotePath string) error {
	command, err := t.filesystemDialect().RemoveFileCommand(remotePath)
	if err != nil {
		return err
	}
	_, err = t.runShellCommand(ctx, command)
	return err
}

func (t *SSHTransport) RemoveTree(ctx context.Context, remotePath string) error {
	command, err := t.filesystemDialect().RemoveTreeCommand(remotePath)
	if err != nil {
		return err
	}
	_, err = t.runShellCommand(ctx, command)
	return err
}

func (t *SSHTransport) Chmod(ctx context.Context, mode string, remotePath string) error {
	command, err := t.filesystemDialect().ChmodCommand(mode, remotePath)
	if err != nil {
		return err
	}
	_, err = t.runShellCommand(ctx, command)
	return err
}

func (t *SSHTransport) Chown(ctx context.Context, owner string, remotePath string) error {
	command, err := t.filesystemDialect().ChownCommand(owner, remotePath)
	if err != nil {
		return err
	}
	_, err = t.runShellCommand(ctx, command)
	return err
}

func (t *SSHTransport) sftpClient(ctx context.Context) (*sftp.Client, error) {
	client, err := sftp.NewClient(t.client)
	if err == nil {
		return client, nil
	}
	if reconnectErr := t.reconnect(ctx); reconnectErr != nil {
		return nil, fmt.Errorf("open ssh sftp client: %w; reconnect failed: %v", err, reconnectErr)
	}
	client, err = sftp.NewClient(t.client)
	if err != nil {
		return nil, fmt.Errorf("open ssh sftp client after reconnect: %w", err)
	}
	return client, nil
}

func (cfg SSHTransportConfig) Validate() error {
	if err := validateSSHEndpointConfig("ssh", cfg.targetEndpoint(), true); err != nil {
		return err
	}
	if err := validateSSHDuration("connect_timeout", cfg.ConnectTimeout); err != nil {
		return err
	}
	if err := validateSSHDuration("command_timeout", cfg.CommandTimeout); err != nil {
		return err
	}
	for index := range cfg.JumpHosts {
		if err := validateSSHEndpointConfig(fmt.Sprintf("ssh jump_hosts[%d]", index), cfg.jumpHostEndpoint(index), true); err != nil {
			return err
		}
	}
	return nil
}

type filesystemCommandDialect interface {
	MakeDirectoryCommand(path string) (string, error)
	MoveCommand(src string, dest string) (string, error)
	RemoveFileCommand(path string) (string, error)
	RemoveTreeCommand(path string) (string, error)
	ChmodCommand(mode string, path string) (string, error)
	ChownCommand(owner string, path string) (string, error)
}

func (t SSHTransport) filesystemDialect() filesystemCommandDialect {
	if dialect, ok := t.Dialect.(filesystemCommandDialect); ok {
		return dialect
	}
	return BashShellPlatform{}
}

func (t *SSHTransport) runShellCommand(ctx context.Context, command string) ([]byte, error) {
	return t.execCommand(ctx, command)
}

func (t *SSHTransport) reconnect(ctx context.Context) error {
	if t.client != nil {
		_ = t.client.Close()
		t.client = nil
	}
	return t.Connect(ctx)
}

func validateSSHCopyPath(name string, value string) error {
	return validateSSHRemotePath(name, value)
}

func validateSSHRemotePath(name string, value string) error {
	if value == "" {
		return fmt.Errorf("ssh %s is required", name)
	}
	if containsNewline(value) {
		return fmt.Errorf("ssh %s must not contain newlines", name)
	}
	return nil
}

func remoteTempPath(remotePath string) (string, error) {
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate remote temp path: %w", err)
	}

	dir := path.Dir(remotePath)
	base := path.Base(remotePath)
	return path.Join(dir, "."+base+".goetl-tmp-"+hex.EncodeToString(suffix)), nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64

	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}

		nr, er := src.Read(buffer)
		if nr > 0 {
			nw, ew := dst.Write(buffer[:nr])
			written += int64(nw)
			if ew != nil {
				return written, ew
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
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
	return sshClientConfigForEndpoint(t.Config.targetEndpoint())
}

type sshEndpointConfig struct {
	Host           string
	Port           int
	User           string
	IdentityFile   string
	IdentityEnv    string
	KnownHostsFile string
	HostKeyPolicy  string
	PinnedHostKey  string
}

func connectSSHEndpoint(ctx context.Context, endpoint sshEndpointConfig, dial func(context.Context, string, string) (net.Conn, error)) (*ssh.Client, error) {
	clientConfig, err := sshClientConfigForEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	address := sshAddress(endpoint)
	conn, err := dial(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("ssh connect to %s: %w", address, err)
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer conn.SetDeadline(time.Time{})
	}

	sshConn, channels, requests, err := ssh.NewClientConn(conn, address, clientConfig)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", address, err)
	}

	return ssh.NewClient(sshConn, channels, requests), nil
}

func dialSSHClient(ctx context.Context, client *ssh.Client, network string, address string) (net.Conn, error) {
	type dialResult struct {
		conn net.Conn
		err  error
	}
	done := make(chan dialResult, 1)
	go func() {
		conn, err := client.Dial(network, address)
		done <- dialResult{conn: conn, err: err}
	}()

	select {
	case result := <-done:
		return result.conn, result.err
	case <-ctx.Done():
		_ = client.Close()
		return nil, ctx.Err()
	}
}

func sshClientConfigForEndpoint(endpoint sshEndpointConfig) (*ssh.ClientConfig, error) {
	signer, err := identitySignerForEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	hostKeyCallback, err := hostKeyCallbackForEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User: endpoint.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
	}, nil
}

func (t SSHTransport) identitySigner() (ssh.Signer, error) {
	return identitySignerForEndpoint(t.Config.targetEndpoint())
}

func identitySignerForEndpoint(endpoint sshEndpointConfig) (ssh.Signer, error) {
	var key []byte
	var err error

	switch {
	case endpoint.IdentityFile != "":
		identityFile, expandErr := expandSSHLocalPath(endpoint.IdentityFile)
		if expandErr != nil {
			return nil, fmt.Errorf("expand ssh identity_file: %w", expandErr)
		}
		key, err = os.ReadFile(identityFile)
		if err != nil {
			return nil, fmt.Errorf("read ssh identity_file: %w", err)
		}
	case endpoint.IdentityEnv != "":
		value := os.Getenv(endpoint.IdentityEnv)
		if value == "" {
			return nil, fmt.Errorf("ssh identity_env %s is empty or unset", endpoint.IdentityEnv)
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
	return hostKeyCallbackForEndpoint(t.Config.targetEndpoint())
}

func hostKeyCallbackForEndpoint(endpoint sshEndpointConfig) (ssh.HostKeyCallback, error) {
	switch sshHostKeyPolicy(endpoint) {
	case SSHHostKeyPolicyPinned:
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(endpoint.PinnedHostKey))
		if err != nil {
			return nil, fmt.Errorf("parse ssh pinned_host_key: %w", err)
		}
		return ssh.FixedHostKey(key), nil
	case SSHHostKeyPolicyInsecureIgnore:
		return ssh.InsecureIgnoreHostKey(), nil
	case SSHHostKeyPolicyKnownHosts:
		knownHostsFile, err := expandSSHLocalPath(endpoint.KnownHostsFile)
		if err != nil {
			return nil, fmt.Errorf("expand ssh known_hosts_file: %w", err)
		}
		callback, err := knownhosts.New(knownHostsFile)
		if err != nil {
			return nil, fmt.Errorf("load ssh known_hosts_file: %w", err)
		}
		return callback, nil
	default:
		return nil, fmt.Errorf("unsupported ssh host_key_policy %q", endpoint.HostKeyPolicy)
	}
}

func expandSSHLocalPath(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	if value == "~" || strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve current user home directory: %w", err)
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, value[2:])
		}
	} else if strings.HasPrefix(value, "~") {
		return "", fmt.Errorf("~user expansion is not supported for ssh local paths")
	}

	expanded := os.ExpandEnv(value)
	if expanded == "" {
		return "", nil
	}
	return filepath.Clean(expanded), nil
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
	return sshAddress(t.Config.targetEndpoint())
}

func (t SSHTransport) port() int {
	return sshPort(t.Config.Port)
}

func (t SSHTransport) hostKeyPolicy() string {
	return sshHostKeyPolicy(t.Config.targetEndpoint())
}

func (cfg SSHTransportConfig) validateHostKeyPolicy() error {
	return validateSSHHostKeyPolicy("ssh", cfg.targetEndpoint())
}

func (cfg SSHTransportConfig) targetEndpoint() sshEndpointConfig {
	return sshEndpointConfig{
		Host:           cfg.Host,
		Port:           cfg.Port,
		User:           cfg.User,
		IdentityFile:   cfg.IdentityFile,
		IdentityEnv:    cfg.IdentityEnv,
		KnownHostsFile: cfg.KnownHostsFile,
		HostKeyPolicy:  cfg.HostKeyPolicy,
		PinnedHostKey:  cfg.PinnedHostKey,
	}
}

func (cfg SSHTransportConfig) jumpHostEndpoint(index int) sshEndpointConfig {
	jumpHost := cfg.JumpHosts[index]
	identityFile := jumpHost.IdentityFile
	identityEnv := jumpHost.IdentityEnv
	if identityFile == "" && identityEnv == "" {
		identityFile = cfg.IdentityFile
		identityEnv = cfg.IdentityEnv
	}
	knownHostsFile := jumpHost.KnownHostsFile
	if knownHostsFile == "" {
		knownHostsFile = cfg.KnownHostsFile
	}
	return sshEndpointConfig{
		Host:           jumpHost.Host,
		Port:           jumpHost.Port,
		User:           jumpHost.User,
		IdentityFile:   identityFile,
		IdentityEnv:    identityEnv,
		KnownHostsFile: knownHostsFile,
		HostKeyPolicy:  jumpHost.HostKeyPolicy,
		PinnedHostKey:  jumpHost.PinnedHostKey,
	}
}

func validateSSHEndpointConfig(prefix string, endpoint sshEndpointConfig, requireIdentity bool) error {
	if endpoint.Host == "" {
		return fmt.Errorf("%s host is required", prefix)
	}
	if endpoint.User == "" {
		return fmt.Errorf("%s user is required", prefix)
	}
	if endpoint.Port < 0 || endpoint.Port > 65535 {
		return fmt.Errorf("%s port must be between 1 and 65535", prefix)
	}
	if requireIdentity && endpoint.IdentityFile == "" && endpoint.IdentityEnv == "" {
		return fmt.Errorf("%s identity_file or identity_env is required", prefix)
	}
	if endpoint.IdentityFile != "" && endpoint.IdentityEnv != "" {
		return fmt.Errorf("%s identity_file and identity_env are mutually exclusive", prefix)
	}
	if err := validateSSHHostKeyPolicy(prefix, endpoint); err != nil {
		return err
	}
	return nil
}

func validateSSHHostKeyPolicy(prefix string, endpoint sshEndpointConfig) error {
	switch sshHostKeyPolicy(endpoint) {
	case SSHHostKeyPolicyKnownHosts:
		if endpoint.KnownHostsFile == "" {
			return fmt.Errorf("%s known_hosts_file is required when host_key_policy is known_hosts", prefix)
		}
		return nil
	case SSHHostKeyPolicyPinned:
		if endpoint.PinnedHostKey == "" {
			return fmt.Errorf("%s pinned_host_key is required when host_key_policy is pinned", prefix)
		}
		return nil
	case SSHHostKeyPolicyInsecureIgnore:
		return nil
	default:
		return fmt.Errorf("unsupported %s host_key_policy %q", prefix, endpoint.HostKeyPolicy)
	}
}

func sshAddress(endpoint sshEndpointConfig) string {
	return net.JoinHostPort(endpoint.Host, strconv.Itoa(sshPort(endpoint.Port)))
}

func sshPort(port int) int {
	if port == 0 {
		return defaultSSHPort
	}
	return port
}

func sshHostKeyPolicy(endpoint sshEndpointConfig) string {
	if endpoint.HostKeyPolicy == "" {
		return SSHHostKeyPolicyKnownHosts
	}
	return endpoint.HostKeyPolicy
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

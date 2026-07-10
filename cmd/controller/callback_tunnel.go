package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const callbackTunnelTypeSSHReverse = "ssh_reverse"

type CallbackTunnelConfig struct {
	Type                string `json:"type,omitempty"`
	Transport           string `json:"transport,omitempty"`
	BindHop             string `json:"bind_hop,omitempty"`
	RemoteBindHost      string `json:"remote_bind_host,omitempty"`
	RemoteBindPort      int    `json:"remote_bind_port,omitempty"`
	RelayBindHost       string `json:"relay_bind_host,omitempty"`
	RelayBindPort       int    `json:"relay_bind_port,omitempty"`
	RelayScriptPath     string `json:"relay_script_path,omitempty"`
	LocalHost           string `json:"local_host,omitempty"`
	LocalPort           int    `json:"local_port,omitempty"`
	WorkerControllerURL string `json:"worker_controller_url,omitempty"`
}

type SSHReverseCallbackTunnel struct {
	Config                   CallbackTunnelConfig
	transport                *SSHTransport
	scheduler                Scheduler
	slurmPreflightScriptPath string

	mu       sync.Mutex
	listener net.Listener
	done     chan struct{}
	relayPID string
}

func (cfg CallbackTunnelConfig) IsZero() bool {
	return cfg.Type == "" &&
		cfg.Transport == "" &&
		cfg.BindHop == "" &&
		cfg.RemoteBindHost == "" &&
		cfg.RemoteBindPort == 0 &&
		cfg.RelayBindHost == "" &&
		cfg.RelayBindPort == 0 &&
		cfg.RelayScriptPath == "" &&
		cfg.LocalHost == "" &&
		cfg.LocalPort == 0 &&
		cfg.WorkerControllerURL == ""
}

func (cfg CallbackTunnelConfig) Validate() error {
	if cfg.IsZero() {
		return nil
	}
	if cfg.Type != callbackTunnelTypeSSHReverse {
		return fmt.Errorf("callback_tunnel type must be %q", callbackTunnelTypeSSHReverse)
	}
	if cfg.Transport == "" {
		return fmt.Errorf("callback_tunnel transport is required")
	}
	if cfg.RemoteBindHost == "" {
		return fmt.Errorf("callback_tunnel remote_bind_host is required")
	}
	if cfg.RemoteBindPort <= 0 || cfg.RemoteBindPort > 65535 {
		return fmt.Errorf("callback_tunnel remote_bind_port must be between 1 and 65535")
	}
	if cfg.RelayBindHost != "" || cfg.RelayBindPort != 0 || cfg.RelayScriptPath != "" {
		if cfg.RelayBindHost == "" {
			return fmt.Errorf("callback_tunnel relay_bind_host is required when relay is configured")
		}
		if cfg.RelayBindPort <= 0 || cfg.RelayBindPort > 65535 {
			return fmt.Errorf("callback_tunnel relay_bind_port must be between 1 and 65535")
		}
	}
	if cfg.LocalHost == "" {
		return fmt.Errorf("callback_tunnel local_host is required")
	}
	if cfg.LocalPort <= 0 || cfg.LocalPort > 65535 {
		return fmt.Errorf("callback_tunnel local_port must be between 1 and 65535")
	}
	if cfg.WorkerControllerURL == "" {
		return fmt.Errorf("callback_tunnel worker_controller_url is required")
	}
	parsed, err := url.Parse(cfg.WorkerControllerURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("callback_tunnel worker_controller_url must be an absolute URL")
	}
	if strings.ContainsAny(cfg.RemoteBindHost+cfg.RelayBindHost+cfg.RelayScriptPath+cfg.LocalHost+cfg.WorkerControllerURL, "\r\n") {
		return fmt.Errorf("callback_tunnel values must not contain newlines")
	}
	if err := validateCallbackTunnelBindHop(cfg.BindHop); err != nil {
		return err
	}
	return nil
}

func newCallbackTunnelFromConfig(cfg CallbackTunnelConfig, transportConfigs []ExecutionComponentConfig, transports []Transport, scheduler Scheduler, runtime Runtime) (*SSHReverseCallbackTunnel, error) {
	if cfg.IsZero() {
		return nil, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if workerControllerURL, ok := runtimeWorkerControllerURL(runtime); ok && workerControllerURL != cfg.WorkerControllerURL {
		return nil, fmt.Errorf("callback_tunnel worker_controller_url %q must match runtime controller_url %q", cfg.WorkerControllerURL, workerControllerURL)
	}
	transport, err := namedSSHTransport(cfg.Transport, transportConfigs, transports)
	if err != nil {
		return nil, err
	}
	slurmPreflightScriptPath := callbackTunnelSlurmPreflightScriptPath(runtime)
	return &SSHReverseCallbackTunnel{
		Config:                   cfg,
		transport:                transport,
		scheduler:                scheduler,
		slurmPreflightScriptPath: slurmPreflightScriptPath,
	}, nil
}

func (t *SSHReverseCallbackTunnel) Prepare(ctx context.Context) error {
	return t.Start(ctx)
}

func (t *SSHReverseCallbackTunnel) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.listener != nil {
		t.mu.Unlock()
		return nil
	}
	t.mu.Unlock()

	if err := t.transport.Prepare(ctx); err != nil {
		return err
	}
	client, err := t.transport.reverseForwardClient(t.Config.BindHop)
	if err != nil {
		return err
	}

	listener, err := client.Listen("tcp", net.JoinHostPort(t.Config.RemoteBindHost, strconv.Itoa(t.Config.RemoteBindPort)))
	if err != nil {
		return fmt.Errorf("start ssh reverse callback tunnel on %s:%d: %w", t.Config.RemoteBindHost, t.Config.RemoteBindPort, err)
	}

	t.mu.Lock()
	if t.listener != nil {
		t.mu.Unlock()
		_ = listener.Close()
		return nil
	}
	t.listener = listener
	t.done = make(chan struct{})
	done := t.done
	t.mu.Unlock()

	go t.serve(listener, done)
	if err := t.startRemoteRelay(ctx); err != nil {
		_ = t.Close()
		return err
	}
	return nil
}

func (t *SSHReverseCallbackTunnel) Close() error {
	t.mu.Lock()
	listener := t.listener
	done := t.done
	relayPID := t.relayPID
	t.listener = nil
	t.done = nil
	t.relayPID = ""
	t.mu.Unlock()

	var closeErr error
	if relayPID != "" && t.transport != nil {
		if _, err := t.transport.Exec(context.Background(), "sh", "-c", "kill "+relayPID+" >/dev/null 2>&1 || true"); err != nil {
			closeErr = err
		}
	}
	if listener == nil {
		return closeErr
	}
	err := listener.Close()
	if err != nil && closeErr == nil {
		closeErr = err
	}
	if done != nil {
		<-done
	}
	return closeErr
}

func (t *SSHReverseCallbackTunnel) Preflight(ctx context.Context) []PreflightIssue {
	if err := t.Start(ctx); err != nil {
		return []PreflightIssue{{
			Type:        "ssh_reverse",
			Severity:    PreflightSeverityError,
			Code:        "callback_tunnel_start_failed",
			Message:     "SSH reverse callback tunnel could not be established.",
			Remediation: err.Error(),
		}}
	}
	if err := t.checkWorkerControllerURL(ctx); err != nil {
		return []PreflightIssue{{
			Type:        "ssh_reverse",
			Severity:    PreflightSeverityError,
			Code:        "callback_tunnel_unreachable",
			Message:     "Worker-facing controller URL is not reachable through the callback tunnel.",
			Remediation: err.Error(),
		}}
	}
	if err := t.checkSlurmWorkerControllerURL(ctx); err != nil {
		return []PreflightIssue{{
			Type:        "ssh_reverse",
			Severity:    PreflightSeverityError,
			Code:        "callback_tunnel_compute_unreachable",
			Message:     "Worker-facing controller URL is not reachable from a Slurm compute job.",
			Remediation: err.Error(),
		}}
	}
	return nil
}

func (t *SSHReverseCallbackTunnel) serve(listener net.Listener, done chan struct{}) {
	defer close(done)
	for {
		remoteConn, err := listener.Accept()
		if err != nil {
			return
		}
		go t.proxy(remoteConn)
	}
}

func (t *SSHReverseCallbackTunnel) proxy(remoteConn net.Conn) {
	defer remoteConn.Close()
	localConn, err := net.Dial("tcp", net.JoinHostPort(t.Config.LocalHost, strconv.Itoa(t.Config.LocalPort)))
	if err != nil {
		return
	}
	defer localConn.Close()
	proxyBidirectional(remoteConn, localConn)
}

func (t *SSHReverseCallbackTunnel) startRemoteRelay(ctx context.Context) error {
	if t.Config.RelayBindHost == "" && t.Config.RelayBindPort == 0 && t.Config.RelayScriptPath == "" {
		return nil
	}
	if t.transport == nil {
		return fmt.Errorf("callback_tunnel relay requires ssh transport")
	}
	scriptPath := t.Config.RelayScriptPath
	if scriptPath == "" {
		scriptPath = fmt.Sprintf("/tmp/goetl-callback-relay-%d.py", t.Config.RelayBindPort)
	}
	logPath := strings.TrimSuffix(scriptPath, ".py") + ".log"

	localScriptPath, err := writeCallbackRelayTempScript()
	if err != nil {
		return err
	}
	defer os.Remove(localScriptPath)
	if err := t.transport.Copy(ctx, localScriptPath, scriptPath); err != nil {
		return fmt.Errorf("copy callback relay script: %w", err)
	}
	if _, err := t.transport.Exec(ctx, "chmod", "0700", scriptPath); err != nil {
		return fmt.Errorf("chmod callback relay script: %w", err)
	}

	command := "nohup python3 " + shellQuote(scriptPath) +
		" " + shellQuote(t.Config.RelayBindHost) +
		" " + strconv.Itoa(t.Config.RelayBindPort) +
		" " + shellQuote(t.Config.RemoteBindHost) +
		" " + strconv.Itoa(t.Config.RemoteBindPort) +
		" > " + shellQuote(logPath) +
		" 2>&1 < /dev/null & echo $!"
	output, err := t.transport.Exec(ctx, "sh", "-c", command)
	if err != nil {
		return fmt.Errorf("start callback relay: %w", err)
	}
	pid := strings.TrimSpace(string(output))
	if !isDecimalPID(pid) {
		return fmt.Errorf("start callback relay: invalid pid %q", pid)
	}

	t.mu.Lock()
	t.relayPID = pid
	t.mu.Unlock()
	return nil
}

func (t *SSHReverseCallbackTunnel) checkWorkerControllerURL(ctx context.Context) error {
	healthURL, err := url.JoinPath(t.Config.WorkerControllerURL, "healthz")
	if err != nil {
		return err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s returned %s", healthURL, resp.Status)
	}
	return nil
}

func (t *SSHReverseCallbackTunnel) checkSlurmWorkerControllerURL(ctx context.Context) error {
	scheduler, ok := t.scheduler.(SlurmScheduler)
	if !ok {
		return nil
	}
	if scheduler.Transport == nil {
		return fmt.Errorf("slurm transport is required")
	}
	remoteScriptPath := t.slurmPreflightScriptPath
	if remoteScriptPath == "" {
		remoteScriptPath = "/tmp/goetl-callback-preflight.slurm"
	}
	healthURL, err := url.JoinPath(t.Config.WorkerControllerURL, "healthz")
	if err != nil {
		return err
	}
	script := callbackTunnelSlurmPreflightScript(healthURL)
	localScriptPath, err := writeCallbackTunnelTempScript(script)
	if err != nil {
		return err
	}
	defer os.Remove(localScriptPath)
	if err := scheduler.Transport.Copy(ctx, localScriptPath, remoteScriptPath); err != nil {
		return fmt.Errorf("copy callback tunnel Slurm preflight script: %w", err)
	}
	if _, err := scheduler.Transport.Exec(ctx, "sbatch", "--wait", remoteScriptPath); err != nil {
		return fmt.Errorf("submit callback tunnel Slurm preflight script: %w", err)
	}
	return nil
}

func (t *SSHTransport) Prepare(ctx context.Context) error {
	if t.client != nil {
		return nil
	}
	return t.Connect(ctx)
}

func (t *SSHTransport) reverseForwardClient(bindHop string) (*ssh.Client, error) {
	switch {
	case bindHop == "" || bindHop == "target":
		if t.client == nil {
			return nil, fmt.Errorf("ssh target client is not connected")
		}
		return t.client, nil
	case strings.HasPrefix(bindHop, "jump_hosts[") && strings.HasSuffix(bindHop, "]"):
		rawIndex := strings.TrimSuffix(strings.TrimPrefix(bindHop, "jump_hosts["), "]")
		index, err := strconv.Atoi(rawIndex)
		if err != nil {
			return nil, fmt.Errorf("callback_tunnel bind_hop %q has invalid jump host index", bindHop)
		}
		if index < 0 || index >= len(t.jumpClients) {
			return nil, fmt.Errorf("callback_tunnel bind_hop %q is not connected", bindHop)
		}
		return t.jumpClients[index], nil
	default:
		return nil, fmt.Errorf("unsupported callback_tunnel bind_hop %q", bindHop)
	}
}

func validateCallbackTunnelBindHop(bindHop string) error {
	if bindHop == "" || bindHop == "target" {
		return nil
	}
	if strings.HasPrefix(bindHop, "jump_hosts[") && strings.HasSuffix(bindHop, "]") {
		rawIndex := strings.TrimSuffix(strings.TrimPrefix(bindHop, "jump_hosts["), "]")
		index, err := strconv.Atoi(rawIndex)
		if err != nil || index < 0 {
			return fmt.Errorf("callback_tunnel bind_hop %q must use a non-negative jump host index", bindHop)
		}
		return nil
	}
	return fmt.Errorf("callback_tunnel bind_hop must be target or jump_hosts[N]")
}

func namedSSHTransport(name string, transportConfigs []ExecutionComponentConfig, transports []Transport) (*SSHTransport, error) {
	for index, cfg := range transportConfigs {
		if cfg.Name != name {
			continue
		}
		transport, ok := transports[index].(*SSHTransport)
		if !ok {
			return nil, fmt.Errorf("callback_tunnel transport %q must be an ssh transport", name)
		}
		return transport, nil
	}
	return nil, fmt.Errorf("callback_tunnel transport %q was not found", name)
}

func runtimeWorkerControllerURL(runtime Runtime) (string, bool) {
	switch typed := runtime.(type) {
	case WorkerRuntime:
		if typed.ControllerURL == "" {
			return "", false
		}
		return typed.ControllerURL, true
	case SingularityWorkerRuntime:
		if typed.ControllerURL == "" {
			return "", false
		}
		return typed.ControllerURL, true
	default:
		return "", false
	}
}

func callbackTunnelSlurmPreflightScriptPath(runtime Runtime) string {
	var workerRuntime WorkerRuntime
	switch typed := runtime.(type) {
	case WorkerRuntime:
		workerRuntime = typed
	case SingularityWorkerRuntime:
		workerRuntime = typed.WorkerRuntime
	default:
		return ""
	}
	paths, err := workerRuntime.paths()
	if err != nil {
		return ""
	}
	return path.Join(path.Dir(paths.WorkerScriptPath), "callback-tunnel-preflight.slurm")
}

func callbackTunnelSlurmPreflightScript(statusURL string) string {
	var script strings.Builder
	script.WriteString("#!/usr/bin/env bash\n")
	script.WriteString("#SBATCH --job-name=goetl-callback-preflight\n")
	script.WriteString("set -euo pipefail\n")
	script.WriteString("curl --fail --silent --show-error --max-time 10 ")
	script.WriteString(shellQuote(statusURL))
	script.WriteString(" >/dev/null\n")
	return script.String()
}

func writeCallbackTunnelTempScript(script string) (string, error) {
	file, err := os.CreateTemp("", "goetl-callback-preflight-*.slurm")
	if err != nil {
		return "", fmt.Errorf("create callback tunnel Slurm preflight script: %w", err)
	}
	localPath := file.Name()
	if _, err := file.WriteString(script); err != nil {
		file.Close()
		os.Remove(localPath)
		return "", fmt.Errorf("write callback tunnel Slurm preflight script: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(localPath)
		return "", fmt.Errorf("close callback tunnel Slurm preflight script: %w", err)
	}
	return localPath, nil
}

func writeCallbackRelayTempScript() (string, error) {
	file, err := os.CreateTemp("", "goetl-callback-relay-*.py")
	if err != nil {
		return "", fmt.Errorf("create callback relay script: %w", err)
	}
	localPath := file.Name()
	if _, err := file.WriteString(callbackRelayScript); err != nil {
		file.Close()
		os.Remove(localPath)
		return "", fmt.Errorf("write callback relay script: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(localPath)
		return "", fmt.Errorf("close callback relay script: %w", err)
	}
	return localPath, nil
}

func isDecimalPID(pid string) bool {
	if pid == "" {
		return false
	}
	for _, r := range pid {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

const callbackRelayScript = `#!/usr/bin/env python3
import select
import socket
import sys
import threading

bind_host = sys.argv[1]
bind_port = int(sys.argv[2])
target_host = sys.argv[3]
target_port = int(sys.argv[4])

def close_quietly(sock):
    try:
        sock.close()
    except OSError:
        pass

def proxy(client):
    upstream = None
    try:
        upstream = socket.create_connection((target_host, target_port), timeout=10)
        sockets = [client, upstream]
        while True:
            readable, _, _ = select.select(sockets, [], [], 60)
            for sock in readable:
                data = sock.recv(65536)
                if not data:
                    return
                if sock is client:
                    upstream.sendall(data)
                else:
                    client.sendall(data)
    except OSError:
        return
    finally:
        close_quietly(client)
        if upstream is not None:
            close_quietly(upstream)

server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
server.bind((bind_host, bind_port))
server.listen(64)

while True:
    client, _ = server.accept()
    thread = threading.Thread(target=proxy, args=(client,), daemon=True)
    thread.start()
`

func proxyBidirectional(left net.Conn, right net.Conn) {
	var once sync.Once
	closeBoth := func() {
		_ = left.Close()
		_ = right.Close()
	}
	go func() {
		_, _ = io.Copy(left, right)
		once.Do(closeBoth)
	}()
	_, _ = io.Copy(right, left)
	once.Do(closeBoth)
}

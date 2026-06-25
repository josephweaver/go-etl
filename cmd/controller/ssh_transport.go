package main

import (
	"fmt"
	"time"
)

const (
	SSHHostKeyPolicyKnownHosts     = "known_hosts"
	SSHHostKeyPolicyPinned         = "pinned"
	SSHHostKeyPolicyInsecureIgnore = "insecure_ignore"
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

package config

import (
	"errors"

	"github.com/caarlos0/env/v11"
)

const EnvPrefix = "STOWMARK_"

var ErrMissingSSHCredentials = errors.New("set STOWMARK_SSH_PRIVATE_KEY or start an ssh-agent (SSH_AUTH_SOCK) to connect to an SSH repository")

type SSHConfig struct {
	PrivateKey           string `env:"STOWMARK_SSH_PRIVATE_KEY"`
	PrivateKeyPassphrase string `env:"STOWMARK_SSH_PRIVATE_KEY_PASSPHRASE"`
	AgentSocket          string `env:"SSH_AUTH_SOCK"`
}

func (c SSHConfig) UsesAgent() bool {
	return c.PrivateKey == ""
}

func NewSSHConfig() (*SSHConfig, error) {
	return NewSSHConfigFrom(nil)
}

func NewSSHConfigFrom(environment map[string]string) (*SSHConfig, error) {
	var cfg SSHConfig
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: environment}); err != nil {
		return nil, err
	}
	if cfg.PrivateKey == "" && cfg.AgentSocket == "" {
		return nil, ErrMissingSSHCredentials
	}
	return &cfg, nil
}

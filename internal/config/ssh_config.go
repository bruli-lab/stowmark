package config

import "github.com/caarlos0/env/v11"

const EnvPrefix = "STOWMARK_"

type SSHConfig struct {
	PrivateKey string `env:"SSH_PRIVATE_KEY,required"`
}

func NewSSHConfig() (*SSHConfig, error) {
	return NewSSHConfigFrom(nil)
}

func NewSSHConfigFrom(environment map[string]string) (*SSHConfig, error) {
	var cfg SSHConfig
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: EnvPrefix, Environment: environment}); err != nil {
		return nil, err
	}
	return &cfg, nil
}

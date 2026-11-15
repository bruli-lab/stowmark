package config

import "github.com/caarlos0/env/v11"

type SMBConfig struct {
	Password string `env:"SMB_PASSWORD,required"`
}

func NewSMBConfig() (*SMBConfig, error) {
	return NewSMBConfigFrom(nil)
}

func NewSMBConfigFrom(environment map[string]string) (*SMBConfig, error) {
	var cfg SMBConfig
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: EnvPrefix, Environment: environment}); err != nil {
		return nil, err
	}
	return &cfg, nil
}

package config

import "github.com/caarlos0/env/v11"

type SMBConfig struct {
	Password string `env:"SMB_PASSWORD,required"`
}

func NewSMBConfig() (*SMBConfig, error) {
	var cfg SMBConfig
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: EnvPrefix}); err != nil {
		return nil, err
	}
	return &cfg, nil
}

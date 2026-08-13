package config

import "github.com/caarlos0/env/v11"

type WebdavConfig struct {
	Username string `env:"WEBDAV_USERNAME,required"`
	Password string `env:"WEBDAV_PASSWORD,required"`
}

func NewWebdavConfig() (*WebdavConfig, error) {
	var cfg WebdavConfig

	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: EnvPrefix}); err != nil {
		return nil, err
	}
	return &cfg, nil
}

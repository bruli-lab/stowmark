package config

import "github.com/caarlos0/env/v11"

type WebdavConfig struct {
	Username string `env:"WEBDAV_USERNAME,required"`
	Password string `env:"WEBDAV_PASSWORD,required"`
}

func NewWebdavConfig() (*WebdavConfig, error) {
	return NewWebdavConfigFrom(nil)
}

func NewWebdavConfigFrom(environment map[string]string) (*WebdavConfig, error) {
	var cfg WebdavConfig
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: EnvPrefix, Environment: environment}); err != nil {
		return nil, err
	}
	return &cfg, nil
}

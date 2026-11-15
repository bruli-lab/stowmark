package config

import "github.com/caarlos0/env/v11"

type S3Config struct {
	Region    *string `env:"AWS_REGION"`
	Endpoint  *string `env:"STOWMARK_S3_ENDPOINT"`
	PathStyle *bool   `env:"STOWMARK_S3_PATH_STYLE"`
}

func NewS3Config() (*S3Config, error) {
	return NewS3ConfigFrom(nil)
}

func NewS3ConfigFrom(environment map[string]string) (*S3Config, error) {
	var cfg S3Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: environment}); err != nil {
		return nil, err
	}
	return &cfg, nil
}

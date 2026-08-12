package config

import "github.com/caarlos0/env/v11"

type S3Config struct {
	AccessKey string  `env:"AWS_ACCESS_KEY,required"`
	SecretKey string  `env:"AWS_SECRET_ACCESS_KEY,required"`
	Region    string  `env:"AWS_REGION,required"`
	Endpoint  *string `env:"STOWMARK_S3_ENDPOINT"`
	PathStyle *bool   `env:"STOWMARK_S3_PATH_STYLE"`
}

func NewS3Config() (*S3Config, error) {
	var cfg S3Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

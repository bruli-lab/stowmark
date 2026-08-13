package config

import (
	"log/slog"

	"github.com/caarlos0/env/v11"
)

type GCSConfig struct {
	GoogleCredentials *string `env:"GOOGLE_APPLICATION_CREDENTIALS"`
	Endpoint          *string `env:"STOWMARK_GCS_ENDPOINT"`
}

func NewGCSConfig() *GCSConfig {
	var cfg GCSConfig
	_ = env.Parse(&cfg)
	if cfg.GoogleCredentials == nil && cfg.Endpoint == nil {
		slog.Warn(
			"GOOGLE_APPLICATION_CREDENTIALS is not set; using Application Default Credentials",
			"hint", `set the variable or run "gcloud auth application-default login"`,
		)
	}
	return &cfg
}

package config

import "github.com/caarlos0/env/v11"

type ObservabilityConfig struct {
	OTELExporterEndpoint *string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTELExporterProtocol *string `env:"OTEL_EXPORTER_OTLP_PROTOCOL"`
	OTELServiceName      *string `env:"OTEL_SERVICE_NAME"`
}

func NewObservabilityConfig() *ObservabilityConfig {
	var cfg ObservabilityConfig
	_ = env.Parse(&cfg)
	return &cfg
}

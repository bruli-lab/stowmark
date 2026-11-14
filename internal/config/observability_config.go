package config

import "github.com/caarlos0/env/v11"

type ObservabilityConfig struct {
	OTELExporterEndpoint *string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTELExporterProtocol *string `env:"OTEL_EXPORTER_OTLP_PROTOCOL" envDefault:"http/protobuf"`
	OTELServiceName      *string `env:"OTEL_SERVICE_NAME" envDefault:"stowmark"`
	LogLevel             string  `env:"STOWMARK_LOG_LEVEL" envDefault:"info"`
}

func NewObservabilityConfig() (*ObservabilityConfig, error) {
	var cfg ObservabilityConfig
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

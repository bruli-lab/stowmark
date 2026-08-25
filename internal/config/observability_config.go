package config

import (
	"os"

	"github.com/caarlos0/env/v11"
)

const (
	OtelExporterOTLPProtocolEnv = "OTEL_EXPORTER_OTLP_PROTOCOL"
	OtelServiceNameEnv          = "OTEL_SERVICE_NAME"
)

type ObservabilityConfig struct {
	OTELExporterEndpoint *string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTELExporterProtocol *string `env:"OTEL_EXPORTER_OTLP_PROTOCOL" envDefault:"http/protobuf"`
	OTELServiceName      *string `env:"OTEL_SERVICE_NAME" envDefault:"stowmark"`
	LogLevel             string  `env:"STOWMARK_LOG_LEVEL" envDefault:"debug"`
}

func (c ObservabilityConfig) setOTELVars() error {
	if c.OTELExporterEndpoint == nil {
		return nil
	}

	if err := os.Setenv(OtelExporterOTLPProtocolEnv, *c.OTELExporterProtocol); err != nil {
		return err
	}
	if err := os.Setenv(OtelServiceNameEnv, *c.OTELServiceName); err != nil {
		return err
	}
	return nil
}

func NewObservabilityConfig() (*ObservabilityConfig, error) {
	var cfg ObservabilityConfig
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.setOTELVars(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

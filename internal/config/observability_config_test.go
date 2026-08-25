package config_test

import (
	"os"
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewObservabilityConfig(t *testing.T) {
	tests := []struct {
		name                string
		envVars             map[string]string
		expectedEndpoint    *string
		expectedProtocol    *string
		expectedServiceName *string
		expectedLogLevel    string
		writeOTLPVars       bool
	}{
		{
			name:                "with no env vars set, it returns only default values",
			envVars:             map[string]string{},
			expectedProtocol:    new("http/protobuf"),
			expectedServiceName: new("stowmark"),
			expectedLogLevel:    "debug",
		},
		{
			name: "with all env var set, it returns all values",
			envVars: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "test-endpoint",
				"OTEL_EXPORTER_OTLP_PROTOCOL": "test-protocol",
				"OTEL_SERVICE_NAME":           "test-service-name",
				"STOWMARK_LOG_LEVEL":          "error",
			},
			expectedEndpoint:    new("test-endpoint"),
			expectedProtocol:    new("test-protocol"),
			expectedServiceName: new("test-service-name"),
			expectedLogLevel:    "error",
			writeOTLPVars:       true,
		},
		{
			name: "with endpoint env var set, it returns otlp values",
			envVars: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "test-endpoint",
			},
			expectedEndpoint:    new("test-endpoint"),
			expectedProtocol:    new("http/protobuf"),
			expectedServiceName: new("stowmark"),
			expectedLogLevel:    "debug",
			writeOTLPVars:       true,
		},
	}
	otelEnvironment := []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_SERVICE_NAME",
		"STOWMARK_LOG_LEVEL",
	}
	for _, tt := range tests {
		t.Run(`Given an observability config struct,
		when the constructor is called `+tt.name, func(t *testing.T) {
			for _, key := range otelEnvironment {
				unsetEnv(t, key)
			}
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}
			got, err := config.NewObservabilityConfig()
			require.NoError(t, err)
			require.Equal(t, tt.expectedEndpoint, got.OTELExporterEndpoint)
			require.Equal(t, tt.expectedProtocol, got.OTELExporterProtocol)
			require.Equal(t, tt.expectedServiceName, got.OTELServiceName)

			if tt.writeOTLPVars {
				require.Equal(t, *tt.expectedProtocol, os.Getenv(config.OtelExporterOTLPProtocolEnv))
				require.Equal(t, *tt.expectedServiceName, os.Getenv(config.OtelServiceNameEnv))
			}
		})
	}
}

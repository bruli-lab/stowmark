package config_test

import (
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
	}{
		{
			name:    "with no env vars set, it returns nil values",
			envVars: map[string]string{},
		},
		{
			name: "with all env vars set, it returns all values",
			envVars: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "test-endpoint",
				"OTEL_EXPORTER_OTLP_PROTOCOL": "test-protocol",
				"OTEL_SERVICE_NAME":           "test-service-name",
			},
			expectedEndpoint:    new("test-endpoint"),
			expectedProtocol:    new("test-protocol"),
			expectedServiceName: new("test-service-name"),
		},
	}
	otelEnvironment := []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_SERVICE_NAME",
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
			got := config.NewObservabilityConfig()
			require.Equal(t, tt.expectedEndpoint, got.OTELExporterEndpoint)
			require.Equal(t, tt.expectedProtocol, got.OTELExporterProtocol)
			require.Equal(t, tt.expectedServiceName, got.OTELServiceName)
		})
	}
}

package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewObservabilityConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		envVars             map[string]string
		expectedEndpoint    *string
		expectedProtocol    *string
		expectedServiceName *string
		expectedLogLevel    string
	}{
		{
			name:                "with no env vars set, then it returns only default values",
			envVars:             map[string]string{},
			expectedProtocol:    new("http/protobuf"),
			expectedServiceName: new("stowmark"),
			expectedLogLevel:    "info",
		},
		{
			name: "with all env vars set, then it returns all values",
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
		},
		{
			name: "with endpoint env var set, then it returns otlp values",
			envVars: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "test-endpoint",
			},
			expectedEndpoint:    new("test-endpoint"),
			expectedProtocol:    new("http/protobuf"),
			expectedServiceName: new("stowmark"),
			expectedLogLevel:    "info",
		},
	}
	for _, tt := range tests {
		t.Run(`Given an environment,
		when NewObservabilityConfigFrom is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.NewObservabilityConfigFrom(tt.envVars)
			require.NoError(t, err)
			require.Equal(t, tt.expectedEndpoint, got.OTELExporterEndpoint)
			require.Equal(t, tt.expectedProtocol, got.OTELExporterProtocol)
			require.Equal(t, tt.expectedServiceName, got.OTELServiceName)
			require.Equal(t, tt.expectedLogLevel, got.LogLevel)
		})
	}
}

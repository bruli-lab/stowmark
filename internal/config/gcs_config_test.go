package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewGCSConfig(t *testing.T) {
	tests := []struct {
		name                      string
		envVars                   map[string]string
		expectedGoogleCredentials *string
		expectedEndpoint          *string
	}{
		{
			name:    "with no env vars set, it returns nil values",
			envVars: map[string]string{},
		},
		{
			name: "with others env vars set, it returns nil values",
			envVars: map[string]string{
				"EX_GCS_GOOGLE_CREDENTIALS": "test",
				"EX_GCS_ENDPOINT":           "test",
			},
		},
		{
			name: "with endpoint env vars set, it returns only endpoint value",
			envVars: map[string]string{
				"STOWMARK_GCS_ENDPOINT": "test-endpoint",
			},
			expectedEndpoint: new("test-endpoint"),
		},
		{
			name: "with al env vars sets, it returns all values",
			envVars: map[string]string{
				"STOWMARK_GCS_ENDPOINT":          "test-endpoint",
				"GOOGLE_APPLICATION_CREDENTIALS": "test-credentials",
			},
			expectedEndpoint:          new("test-endpoint"),
			expectedGoogleCredentials: new("test-credentials"),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a GCSConfig struct,
		when his constructor is called `+tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}
			got := config.NewGCSConfig()
			require.Equal(t, tt.expectedEndpoint, got.Endpoint)
			require.Equal(t, tt.expectedGoogleCredentials, got.GoogleCredentials)
		})
	}
}

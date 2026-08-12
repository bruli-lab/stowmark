package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewS3Config(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectedErr bool
		expectedAccessKey, expectedSecretKey,
		expectedRegion string
		expectedEndpoint  *string
		expectedPathStyle *bool
	}{
		{
			name:        "with no env vars set, it returns an error",
			envVars:     map[string]string{},
			expectedErr: true,
		},
		{
			name: "with unrelated env vars set, it returns an error",
			envVars: map[string]string{
				"EX_S3_ACCESS_KEY": "test",
			},
			expectedErr: true,
		},
		{
			name: "with only required env vars set, it returns config with only required values",
			envVars: map[string]string{
				"AWS_ACCESS_KEY":        "test-access",
				"AWS_SECRET_ACCESS_KEY": "test-key",
				"AWS_REGION":            "test-region",
			},
			expectedAccessKey: "test-access",
			expectedSecretKey: "test-key",
			expectedRegion:    "test-region",
		},
		{
			name: "with all env vars set, it returns config with all values",
			envVars: map[string]string{
				"AWS_ACCESS_KEY":         "test-access",
				"AWS_SECRET_ACCESS_KEY":  "test-key",
				"AWS_REGION":             "test-region",
				"STOWMARK_S3_ENDPOINT":   "test-endpoint",
				"STOWMARK_S3_PATH_STYLE": "true",
			},
			expectedAccessKey: "test-access",
			expectedSecretKey: "test-key",
			expectedRegion:    "test-region",
			expectedEndpoint:  new("test-endpoint"),
			expectedPathStyle: new(true),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a S3Config struct,
		when constructor is called `+tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}
			got, err := config.NewS3Config()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.Equal(t, got.AccessKey, tt.expectedAccessKey)
			require.Equal(t, got.SecretKey, tt.expectedSecretKey)
			require.Equal(t, got.Region, tt.expectedRegion)
			require.Equal(t, got.Endpoint, tt.expectedEndpoint)
			require.Equal(t, got.PathStyle, tt.expectedPathStyle)
		})
	}
}

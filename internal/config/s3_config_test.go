package config_test

import (
	"os"
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewS3Config(t *testing.T) {
	tests := []struct {
		name              string
		envVars           map[string]string
		expectedRegion    *string
		expectedEndpoint  *string
		expectedPathStyle *bool
	}{
		{
			name:    "with no env vars set, it returns an empty config so the AWS SDK default chain resolves credentials and region",
			envVars: map[string]string{},
		},
		{
			name: "with all env vars set, it returns config with all values",
			envVars: map[string]string{
				"AWS_REGION":             "test-region",
				"STOWMARK_S3_ENDPOINT":   "test-endpoint",
				"STOWMARK_S3_PATH_STYLE": "true",
			},
			expectedRegion:    new("test-region"),
			expectedEndpoint:  new("test-endpoint"),
			expectedPathStyle: new(true),
		},
	}

	s3Environment := []string{
		"AWS_REGION",
		"STOWMARK_S3_ENDPOINT",
		"STOWMARK_S3_PATH_STYLE",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range s3Environment {
				unsetEnv(t, key)
			}

			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			got, err := config.NewS3Config()

			require.NoError(t, err)
			require.Equal(t, tt.expectedRegion, got.Region)
			require.Equal(t, tt.expectedEndpoint, got.Endpoint)
			require.Equal(t, tt.expectedPathStyle, got.PathStyle)
		})
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	originalValue, existed := os.LookupEnv(key)

	require.NoError(t, os.Unsetenv(key))

	t.Cleanup(func() {
		if existed {
			require.NoError(t, os.Setenv(key, originalValue))
			return
		}

		require.NoError(t, os.Unsetenv(key))
	})
}

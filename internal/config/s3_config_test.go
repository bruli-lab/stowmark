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
		expectedErr       bool
		expectedAccessKey string
		expectedSecretKey string
		expectedRegion    string
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

	s3Environment := []string{
		"AWS_ACCESS_KEY",
		"AWS_SECRET_ACCESS_KEY",
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
			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedAccessKey, got.AccessKey)
			require.Equal(t, tt.expectedSecretKey, got.SecretKey)
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

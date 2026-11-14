package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewS3Config(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		envVars           map[string]string
		expectedRegion    *string
		expectedEndpoint  *string
		expectedPathStyle *bool
	}{
		{
			name:    "with no env vars set, then it returns an empty config so the AWS SDK default chain resolves credentials and region",
			envVars: map[string]string{},
		},
		{
			name: "with all env vars set, then it returns config with all values",
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

	for _, tt := range tests {
		t.Run(`Given an environment,
		when NewS3ConfigFrom is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.NewS3ConfigFrom(tt.envVars)

			require.NoError(t, err)
			require.Equal(t, tt.expectedRegion, got.Region)
			require.Equal(t, tt.expectedEndpoint, got.Endpoint)
			require.Equal(t, tt.expectedPathStyle, got.PathStyle)
		})
	}
}

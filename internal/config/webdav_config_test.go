package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewWebdavConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                               string
		envVars                            map[string]string
		expectedErr                        bool
		expectedUsername, expectedPassword string
	}{
		{
			name:        "with no env vars set, then it returns an error",
			envVars:     map[string]string{},
			expectedErr: true,
		},
		{
			name: "with unrelated env vars set, then it returns an error",
			envVars: map[string]string{
				"EX_WEBDAV_USERNAME": "test",
			},
			expectedErr: true,
		},
		{
			name: "with all values set, then it returns a valid config",
			envVars: map[string]string{
				"STOWMARK_WEBDAV_USERNAME": "username-test",
				"STOWMARK_WEBDAV_PASSWORD": "password-test",
			},
			expectedUsername: "username-test",
			expectedPassword: "password-test",
		},
	}

	for _, tt := range tests {
		t.Run(`Given an environment,
		when NewWebdavConfigFrom is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.NewWebdavConfigFrom(tt.envVars)
			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedUsername, got.Username)
			require.Equal(t, tt.expectedPassword, got.Password)
		})
	}
}

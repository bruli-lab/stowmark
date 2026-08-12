package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewWebdavConfig(t *testing.T) {
	tests := []struct {
		name                               string
		envVars                            map[string]string
		expectedErr                        bool
		expectedUsername, expectedPassword string
	}{
		{
			name:        "with no env vars set, it returns an error",
			envVars:     map[string]string{},
			expectedErr: true,
		},
		{
			name: "with unrelated env vars set, it returns an error",
			envVars: map[string]string{
				"EX_WEBDAV_USERNAME": "test",
			},
			expectedErr: true,
		},
		{
			name: "with all valued set, it returns a valid config",
			envVars: map[string]string{
				"STOWMARK_WEBDAV_USERNAME": "username-test",
				"STOWMARK_WEBDAV_PASSWORD": "password-test",
			},
			expectedUsername: "username-test",
			expectedPassword: "password-test",
		},
	}

	for _, tt := range tests {
		t.Run(`Given a WebdavConfig struct,
		when the constructor is called `+tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			got, err := config.NewWebdavConfig()
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

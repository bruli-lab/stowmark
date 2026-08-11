package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewSSHConfig(t *testing.T) {
	tests := []struct {
		name               string
		envVars            map[string]string
		expectedErr        bool
		expectedPrivateKey string
	}{
		{
			name:        "with no env vars set, it returns an error",
			envVars:     map[string]string{},
			expectedErr: true,
		},
		{
			name: "with unrelated env vars set, it returns an error",
			envVars: map[string]string{
				"EX_SSH_PRIVATE_KEY": "test",
			},
			expectedErr: true,
		},
		{
			name: "with STOWMARK_SSH_PRIVATE_KEY set, it returns a valid SSHConfig",
			envVars: map[string]string{
				"STOWMARK_SSH_PRIVATE_KEY": "test",
			},
			expectedPrivateKey: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			got, err := config.NewSSHConfig()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedPrivateKey, got.PrivateKey)
		})
	}
}

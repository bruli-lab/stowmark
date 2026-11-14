package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewSSHConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		envVars            map[string]string
		expectedErr        bool
		expectedPrivateKey string
	}{
		{
			name:        "with no env vars set, then it returns an error",
			envVars:     map[string]string{},
			expectedErr: true,
		},
		{
			name: "with unrelated env vars set, then it returns an error",
			envVars: map[string]string{
				"EX_SSH_PRIVATE_KEY": "test",
			},
			expectedErr: true,
		},
		{
			name: "with STOWMARK_SSH_PRIVATE_KEY set, then it returns a valid SSHConfig",
			envVars: map[string]string{
				"STOWMARK_SSH_PRIVATE_KEY": "test",
			},
			expectedPrivateKey: "test",
		},
	}

	for _, tt := range tests {
		t.Run(`Given an environment,
		when NewSSHConfigFrom is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.NewSSHConfigFrom(tt.envVars)
			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedPrivateKey, got.PrivateKey)
		})
	}
}

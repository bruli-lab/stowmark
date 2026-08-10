package config_test

import (
	"os"
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewSSHConfig(t *testing.T) {
	tests := []struct {
		name string
		envVars map[string]string
		expectedErr bool
		expectedPrivateKey string
	}{
		{
			name: "with no env vars are set, then it returns an error",
			envVars: map[string]string{},
			expectedErr: true,
		},
		{
			name: "with other env vars are set, then it returns an error",
			envVars: map[string]string{
				"EX_SSH_PRIVATE_KEY": "test",
			},
			expectedErr: true,
		},
		{
			name: "with STOWMARK_SSH_PRIVATE_KEY env var are set, then it returns a valid SSHConfig",
			envVars: map[string]string{
				"STOWMARK_SSH_PRIVATE_KEY": "test",
			},
			expectedPrivateKey: "test",
		},
	}
	for _, tt := range tests {
		t.Run(`Given a SSHConfig struc,
		when NewSSConfig method is called `+ tt.name, func(t *testing.T) {
			t.Parallel()
			for k, v := range tt.envVars {
				require.NoError(t, os.Setenv(k, v))
			}
			defer func() {
				for k := range tt.envVars {
					require.NoError(t, os.Unsetenv(k))
				}
			}()

			got, err := config.NewSSHConfig()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.Equal(t, tt.expectedPrivateKey, got.PrivateKey)
		})
	}
}

package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewSSHConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		envVars     map[string]string
		expectedErr error
		expected    config.SSHConfig
		usesAgent   bool
	}{
		{
			name:        "with no env vars set, then it returns a missing SSH credentials error",
			envVars:     map[string]string{},
			expectedErr: config.ErrMissingSSHCredentials,
		},
		{
			name:        "with unrelated env vars set, then it returns a missing SSH credentials error",
			envVars:     map[string]string{"EX_SSH_PRIVATE_KEY": "test"},
			expectedErr: config.ErrMissingSSHCredentials,
		},
		{
			name:     "with STOWMARK_SSH_PRIVATE_KEY set, then it uses the private key",
			envVars:  map[string]string{"STOWMARK_SSH_PRIVATE_KEY": "test", "SSH_AUTH_SOCK": "/run/agent.sock"},
			expected: config.SSHConfig{PrivateKey: "test", AgentSocket: "/run/agent.sock"},
		},
		{
			name:     "with a private key and its passphrase set, then it returns both",
			envVars:  map[string]string{"STOWMARK_SSH_PRIVATE_KEY": "test", "STOWMARK_SSH_PRIVATE_KEY_PASSPHRASE": "secret"},
			expected: config.SSHConfig{PrivateKey: "test", PrivateKeyPassphrase: "secret"},
		},
		{
			name:      "with only SSH_AUTH_SOCK set, then it uses the ssh-agent",
			envVars:   map[string]string{"SSH_AUTH_SOCK": "/run/agent.sock"},
			expected:  config.SSHConfig{AgentSocket: "/run/agent.sock"},
			usesAgent: true,
		},
	}

	for _, tt := range tests {
		t.Run(`Given an environment,
		when NewSSHConfigFrom is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.NewSSHConfigFrom(tt.envVars)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, *got)
			require.Equal(t, tt.usesAgent, got.UsesAgent())
		})
	}
}

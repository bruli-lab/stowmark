package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewSMBConfig(t *testing.T) {
	tests := []struct {
		name             string
		envVars          map[string]string
		expectedErr      bool
		expectedPassword string
	}{
		{
			name:        "with no env vars set, it returns an error",
			envVars:     map[string]string{},
			expectedErr: true,
		},
		{
			name: "with unrelated env vars set, it returns an error",
			envVars: map[string]string{
				"EX_SMB_PASSWORD": "test",
			},
			expectedErr: true,
		},
		{
			name: "with STOWMARK_SSH_PRIVATE_KEY set, it returns a valid SSHConfig",
			envVars: map[string]string{
				"STOWMARK_SMB_PASSWORD": "test",
			},
			expectedPassword: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			got, err := config.NewSMBConfig()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedPassword, got.Password)
		})
	}
}

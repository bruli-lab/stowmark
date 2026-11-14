package config_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewSMBConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		envVars          map[string]string
		expectedErr      bool
		expectedPassword string
	}{
		{
			name:        "with no env vars set, then it returns an error",
			envVars:     map[string]string{},
			expectedErr: true,
		},
		{
			name: "with unrelated env vars set, then it returns an error",
			envVars: map[string]string{
				"EX_SMB_PASSWORD": "test",
			},
			expectedErr: true,
		},
		{
			name: "with STOWMARK_SMB_PASSWORD set, then it returns a valid SMBConfig",
			envVars: map[string]string{
				"STOWMARK_SMB_PASSWORD": "test",
			},
			expectedPassword: "test",
		},
	}

	for _, tt := range tests {
		t.Run(`Given an environment,
		when NewSMBConfigFrom is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.NewSMBConfigFrom(tt.envVars)
			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedPassword, got.Password)
		})
	}
}

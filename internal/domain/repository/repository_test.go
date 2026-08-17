package repository_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNewRepository(t *testing.T) {
	compType, err := repository.NewCompression(repository.NoneCompressionType, nil)
	require.NoError(t, err)
	type args struct {
		name   string
		config *repository.Config
	}
	tests := []struct {
		name        string
		args        args
		expectedErr error
	}{
		{
			name: "with an empty name, then it returns an invalid repository name error",
			args: args{
				name: "",
			},
			expectedErr: repository.ErrInvalidRepositoryName,
		},
		{
			name: "with a nil config, then it returns a missing repository config error",
			args: args{
				name: "name",
			},
			expectedErr: repository.ErrMissingCRepositoryConfig,
		},
		{
			name: "with valid data, then it returns a valid repository struct",
			args: args{
				name:   "name",
				config: repository.NewConfig(uuid.New(), repository.CurrentFormatVersion, compType, nil),
			},
		},
	}
	for _, tt := range tests {
		t.Run(`Given a repository struct,
		when NewRepository method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := repository.NewRepository(tt.args.name, tt.args.config)
			if err != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
			require.Equal(t, got.Path(), tt.args.name)
		})
	}
}

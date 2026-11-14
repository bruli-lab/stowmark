package repository_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/stretchr/testify/require"
)

func TestParseFormatVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		value           int
		expectedVersion repository.FormatVersion
		expectedErr     error
	}{
		{
			name:            "with version one, then it returns the version",
			value:           1,
			expectedVersion: repository.FormatVersionOne,
		},
		{
			name:            "with version two, then it returns the version",
			value:           2,
			expectedVersion: repository.FormatVersionTwo,
		},
		{
			name:        "with a future version, then it returns an unsupported format version error",
			value:       3,
			expectedErr: repository.ErrUnsupportedFormatVersion,
		},
		{
			name:        "with zero, then it returns an unsupported format version error",
			value:       0,
			expectedErr: repository.ErrUnsupportedFormatVersion,
		},
		{
			name:        "with a negative version, then it returns an unsupported format version error",
			value:       -1,
			expectedErr: repository.ErrUnsupportedFormatVersion,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a ParseFormatVersion function,
		when it is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := repository.ParseFormatVersion(tt.value)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedVersion, got)
		})
	}
}

func TestParseFormatVersionOrCurrent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		value           int
		expectedVersion repository.FormatVersion
		expectedErr     error
	}{
		{
			name:            "with zero, then it returns the current version",
			value:           0,
			expectedVersion: repository.CurrentFormatVersion,
		},
		{
			name:            "with version one, then it returns the version",
			value:           1,
			expectedVersion: repository.FormatVersionOne,
		},
		{
			name:        "with a future version, then it returns an unsupported format version error",
			value:       3,
			expectedErr: repository.ErrUnsupportedFormatVersion,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a ParseFormatVersionOrCurrent function,
		when it is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := repository.ParseFormatVersionOrCurrent(tt.value)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedVersion, got)
		})
	}
}

package repository_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/stretchr/testify/require"
)

func TestParseFormatVersion(t *testing.T) {
	type args struct {
		value int
	}
	tests := []struct {
		name            string
		args            args
		expectedVersion repository.FormatVersion
	}{
		{
			name:            "with a one version, then it returns the version",
			args:            args{value: 1},
			expectedVersion: repository.FormatVersionOne,
		},
		{
			name:            "with a two version, then it returns the version",
			args:            args{value: 2},
			expectedVersion: repository.FormatVersionTwo,
		},
		{
			name:            "with a invalid version, then it returns the current version",
			args:            args{value: 3},
			expectedVersion: repository.CurrentFormatVersion,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a ParseFormatVersion function,
		when is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			got := repository.ParseFormatVersion(tt.args.value)
			require.Equal(t, tt.expectedVersion, got)
		})
	}
}

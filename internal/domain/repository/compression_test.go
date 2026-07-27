package repository_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/stretchr/testify/require"
)

func TestNewCompression(t *testing.T) {
	type args struct {
		compType repository.CompressionType
		level    *int
	}
	tests := []struct {
		name             string
		args             args
		expectedErr      error
		expectedCompType repository.CompressionType
		expectedLevel    *int
	}{
		{
			name: "with a none compression type, then it returns a compression struct with nil level",
			args: args{
				compType: repository.NoneCompressionType,
			},
			expectedCompType: repository.NoneCompressionType,
			expectedLevel:    nil,
		},
		{
			name: "with a zstd compression type and nil level, then it returns a default zstd level",
			args: args{
				compType: repository.ZstdCompressionType,
			},
			expectedCompType: repository.ZstdCompressionType,
			expectedLevel: new(repository.DefaultZstdLevel),
		},
		{
			name: "with a zstd compression type and invalid level, then it returns a invalid zstd level error",
			args: args{
				compType: repository.ZstdCompressionType,
				level: new(200),
			},
			expectedCompType: repository.ZstdCompressionType,
			expectedErr: repository.ErrInvalidZstdLevel,
		},
		{
			name: "with a zstd compression type and valid level, then it returns a valid struct",
			args: args{
				compType: repository.ZstdCompressionType,
				level: new(2),
			},
			expectedCompType: repository.ZstdCompressionType,
			expectedLevel: new(2),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a Compression struct,
		when the constructor is called `+ tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := repository.NewCompression(tt.args.compType, tt.args.level)
			if err != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.Equal(t, tt.expectedCompType, got.CompType())
			require.Equal(t, tt.expectedLevel, got.Level())
		})
	}
}

func TestParseCompressionType(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		expectedErr error
		expectedCompType *repository.CompressionType
	}{
		{
			name: "with an invalid compression type, then it returns an invalid compression type error",
			args: args{s: "invalid"},
			expectedErr: repository.ErrInvalidCompressionType,
		},
		{
			name: "with a valid compression type, then it returns a valid compression type",
			args: args{s: repository.ZstdCompressionType.String()},
			expectedCompType: new(repository.ZstdCompressionType),
		},
	}
	for _, tt := range tests {
		t.Run(`Given a ParseCompressionType function,
		when is called `+ tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := repository.ParseCompressionType(tt.args.s)
			if err != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			if tt.expectedErr != nil {
				require.Error(t, err)
			}
			require.Equal(t, tt.expectedCompType, got)
		})
	}
}
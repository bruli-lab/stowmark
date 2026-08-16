package snapshot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestHashCalculatorFactory_Handle(t *testing.T) {
	errTest := errors.New("test error")
	type args struct {
		formatVersion repository.FormatVersion
		file          *snapshot.File
		comp          *repository.Compression
	}
	comp, err := repository.NewCompression(repository.NoneCompressionType, nil)
	require.NoError(t, err)
	tests := []struct {
		name string
		args args
		hash string
		expectedErr, calculateErr,
		calculateChunksErr error
		chunks []snapshot.Chunk
	}{
		{
			name:        "with an invalid format version, then it returns an invalid format version error",
			args:        args{formatVersion: repository.FormatVersion(60)},
			expectedErr: snapshot.ErrUnknownCalculator,
		},
		{
			name: "with format version one and calculate hash returns an error, then it returns same error",
			args: args{
				formatVersion: repository.FormatVersionOne,
				file:          new(fixtures.FileBuilder{}.Build()),
			},
			expectedErr:  errTest,
			calculateErr: errTest,
		},
		{
			name: "with format version one and calculate hash returns nil, then it returns file",
			args: args{
				formatVersion: repository.FormatVersionOne,
				file:          new(fixtures.FileBuilder{}.Build()),
			},
		},
		{
			name: "with format version two and calculate hash returns an error, then it returns same error",
			args: args{
				formatVersion: repository.FormatVersionTwo,
				file:          new(fixtures.FileBuilder{}.Build()),
			},
			expectedErr:  errTest,
			calculateErr: errTest,
		},
		{
			name: "with format version two and calculate hash returns an nil, then it returns file",
			args: args{
				formatVersion: repository.FormatVersionTwo,
				file:          new(fixtures.FileBuilder{}.Build()),
			},
			hash: "hash",
		},
		{
			name: "with format version two and big file and calculate hash returns an error, then it returns same error",
			args: args{
				formatVersion: repository.FormatVersionTwo,
				file:          new(fixtures.FileBuilder{Size: new(snapshot.ChunkThreshold)}.Build()),
				comp:          comp,
			},
			expectedErr:        errTest,
			calculateChunksErr: errTest,
		},
		{
			name: "with format version two and big file and calculate hash returns chunks, then it returns file",
			args: args{
				formatVersion: repository.FormatVersionTwo,
				file:          new(fixtures.FileBuilder{Size: new(snapshot.ChunkThreshold)}.Build()),
				comp:          comp,
			},
			chunks: []snapshot.Chunk{
				*snapshot.NewChunk("", 0, 0),
			},
		},
	}
	for _, tt := range tests {
		t.Run(`Given HashCalculatorFactory,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()
			sourceRepo := &snapshot.SourceRepositoryMock{}
			sourceRepo.CalculateHashFunc = func(_ context.Context, _ string, _ *repository.Compression) (string, error) {
				return tt.hash, tt.calculateErr
			}
			sourceRepo.CalculateChunksFunc = func(ctx context.Context, _ string, _ int64, _ *repository.Compression) ([]snapshot.Chunk, error) {
				return tt.chunks, tt.calculateChunksErr
			}
			handler := snapshot.NewHashCalculatorFactory(sourceRepo)
			calc, err := handler.Handle(tt.args.formatVersion)
			if err != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			file, err := calc.Calculate(t.Context(), tt.args.file, nil)
			if err != nil {
				require.ErrorAs(t, err, &tt.expectedErr)
				return
			}
			require.NotNil(t, file, "file should not be nil")
		})
	}
}

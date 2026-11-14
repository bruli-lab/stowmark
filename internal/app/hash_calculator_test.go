package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/fixtures"
	"github.com/stretchr/testify/require"
)

func TestCreateSnapshot_Handle_HashCalculation(t *testing.T) {
	t.Parallel()

	errTest := errors.New("test error")
	bigFile := snapshot.ChunkThreshold
	smallFile := int64(10)
	chunks := []snapshot.Chunk{*snapshot.NewChunk("chunk-1", 0, snapshot.ChunkSize), *snapshot.NewChunk("chunk-2", snapshot.ChunkSize, snapshot.ChunkSize)}

	tests := []struct {
		name               string
		formatVersion      repository.FormatVersion
		size               int64
		calculateErr       error
		calculateChunksErr error
		expectedErr        error
		expectedHash       string
		expectedChunks     []snapshot.Chunk
	}{
		{
			name:          "with an unknown format version, then it returns an unknown calculator error",
			formatVersion: repository.FormatVersion(60),
			size:          smallFile,
			expectedErr:   app.ErrUnknownCalculator,
		},
		{
			name:          "with format version one and calculate hash fails, then it returns same error",
			formatVersion: repository.FormatVersionOne,
			size:          smallFile,
			calculateErr:  errTest,
			expectedErr:   errTest,
		},
		{
			name:          "with format version one and a big file, then it stores the whole file hash without chunks",
			formatVersion: repository.FormatVersionOne,
			size:          bigFile,
			expectedHash:  "hash",
		},
		{
			name:          "with format version two and calculate hash fails, then it returns same error",
			formatVersion: repository.FormatVersionTwo,
			size:          smallFile,
			calculateErr:  errTest,
			expectedErr:   errTest,
		},
		{
			name:           "with format version two and a small file, then it stores the hash as a single chunk",
			formatVersion:  repository.FormatVersionTwo,
			size:           smallFile,
			expectedHash:   "hash",
			expectedChunks: []snapshot.Chunk{*snapshot.NewChunk("hash", 0, smallFile)},
		},
		{
			name:               "with format version two and a big file and calculate chunks fails, then it returns same error",
			formatVersion:      repository.FormatVersionTwo,
			size:               bigFile,
			calculateChunksErr: errTest,
			expectedErr:        errTest,
		},
		{
			name:           "with format version two and a big file, then it stores the calculated chunks",
			formatVersion:  repository.FormatVersionTwo,
			size:           bigFile,
			expectedChunks: chunks,
		},
	}
	for _, tt := range tests {
		t.Run(`Given a CreateSnapshot command handler,
		when Handle method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			config := fixtures.ConfigBuilder{FormatVersion: &tt.formatVersion}.Build(t)
			source := fixtures.SourceBuilder{Files: []snapshot.File{fixtures.FileBuilder{Size: &tt.size, Hash: new("")}.Build()}}.Build()
			sourceRepo := &app.SourceRepositoryMock{
				IgnorePatternsFunc: func(context.Context, string) ([]string, error) { return nil, nil },
				ExploreFunc: func(context.Context, string, *snapshot.ExcludeRules) (*snapshot.Source, error) {
					return &source, nil
				},
				CalculateHashFunc: func(context.Context, string, *repository.Compression) (string, error) {
					return "hash", tt.calculateErr
				},
				CalculateChunksFunc: func(context.Context, string, int64, *repository.Compression) ([]snapshot.Chunk, error) {
					return chunks, tt.calculateChunksErr
				},
			}
			manifestRepo := &app.ManifestRepositoryMock{SaveFunc: func(context.Context, *snapshot.Manifest) error { return nil }}
			objectRepo := &app.ObjectRepositoryMock{
				AlreadyExistsFunc: func(context.Context, string, []byte, uint64) (bool, error) { return true, nil },
			}
			folderRepo := &app.FolderRepositoryMock{
				GetConfigFunc: func(context.Context, string) (*repository.Config, error) { return &config, nil },
			}

			handler := app.NewCreateSnapshot(sourceRepo, manifestRepo, objectRepo, folderRepo, nil, nil, newUnlockedLocker())
			_, err := createSnapshot(t.Context(), handler, createCommand)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, manifestRepo.SaveCalls(), 1)
			files := manifestRepo.SaveCalls()[0].M.Files()
			require.Len(t, files, 1)
			require.Equal(t, tt.expectedHash, files[0].Hash())
			require.Equal(t, tt.expectedChunks, files[0].Chunks())
		})
	}
}

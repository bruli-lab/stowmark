package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/stretchr/testify/require"
)

func newDomainManifest(t *testing.T) *snapshot.Manifest {
	t.Helper()

	level := 7
	comp, err := repository.NewCompression(repository.ZstdCompressionType, &level)
	require.NoError(t, err)

	single := *snapshot.HydrateFile("/source/small.txt", "aabbcc", 12, []snapshot.Chunk{})
	chunked := *snapshot.HydrateFile("/source/big.bin", "", 8, []snapshot.Chunk{
		*snapshot.NewChunk("ddeeff", 0, 4),
		*snapshot.NewChunk("112233", 4, 4),
	})

	createdAt := time.Date(2026, 11, 13, 22, 23, 14, 0, time.UTC)
	return snapshot.NewManifest("20261113T222314Z-abcdef01", []snapshot.File{single, chunked}, createdAt, "/source", comp)
}

func TestManifest_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run(`Given a domain manifest with single and chunked files,
	when it is converted to the model, marshaled and built back into the domain,
	then it returns an equivalent manifest`, func(t *testing.T) {
		t.Parallel()

		original := newDomainManifest(t)
		data, err := json.Marshal(model.NewManifest(original))
		require.NoError(t, err)

		got, err := model.BuildManifestDomain(data, "snapshots/manifest.json")

		require.NoError(t, err)
		require.Equal(t, original.ID(), got.ID())
		require.Equal(t, original.Source(), got.Source())
		require.True(t, original.CreatedAt().Equal(got.CreatedAt()))
		require.Equal(t, original.Compression(), got.Compression())
		require.Equal(t, original.Files(), got.Files())
	})
}

func TestBuildManifestDomain_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		data        string
		expectedErr error
	}{
		{
			name: "with data that is not JSON, then it returns an error",
			data: "not json",
		},
		{
			name:        "with an unknown compression type, then it returns an invalid compression type error",
			data:        `{"id":"x","files":[],"compression":{"type":"brotli"}}`,
			expectedErr: repository.ErrInvalidCompressionType,
		},
		{
			name:        "with a compression level out of range, then it returns an invalid level error",
			data:        `{"id":"x","files":[],"compression":{"type":"gzip","level":42}}`,
			expectedErr: repository.ErrInvalidGzipLevel,
		},
	}

	for _, tt := range tests {
		t.Run(`Given manifest data,
		when BuildManifestDomain function is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := model.BuildManifestDomain([]byte(tt.data), "manifest.json")

			require.Error(t, err)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			}
		})
	}
}

func FuzzBuildManifestDomain(f *testing.F) {
	f.Add([]byte(`{"id":"x","files":[{"path":"/a","hash":"abc","size":1}],"compression":{"type":"none"}}`))
	f.Add([]byte(`{"id":"x","files":[{"path":"/a","size":8,"chunks":[{"hash":"abc","size":4,"offset":0}]}],"compression":{"type":"zstd","level":3}}`))
	f.Add([]byte(`{"files":null,"compression":{"type":"lz4","level":-1}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		manifest, err := model.BuildManifestDomain(data, "fuzz.json")
		if err != nil {
			return
		}
		require.NotNil(t, manifest.Compression())
	})
}

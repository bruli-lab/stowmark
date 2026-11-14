package model_test

import (
	"encoding/json"
	"io/fs"
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

func TestManifest_RoundTrip_Entries(t *testing.T) {
	t.Parallel()

	t.Run(`Given a domain manifest with metadata, directories and symlinks,
	when it is converted to the model, marshaled and built back into the domain,
	then it keeps the kinds, the targets and the metadata`, func(t *testing.T) {
		t.Parallel()

		modTime := time.Date(2025, 1, 2, 3, 4, 5, 6, time.UTC)
		metadata := snapshot.NewMetadata(0o750|fs.ModeSetgid|fs.ModeSticky, modTime)
		file := snapshot.HydrateFile("/source/run.sh", "aabbcc", 12, []snapshot.Chunk{}).WithMetadata(metadata)
		directory := snapshot.NewDirectory("/source/bin").WithMetadata(snapshot.NewMetadata(0o700, modTime))
		link := *snapshot.NewSymlink("/source/latest", "bin/run.sh")
		comp, err := repository.NewCompression(repository.NoneCompressionType, nil)
		require.NoError(t, err)
		original := snapshot.NewManifest("20261113T222314Z-abcdef01", []snapshot.File{file, directory, link}, modTime, "/source", comp)

		data, err := json.Marshal(model.NewManifest(original))
		require.NoError(t, err)
		got, err := model.BuildManifestDomain(data, "snapshots/manifest.json")

		require.NoError(t, err)
		require.Equal(t, original.Files(), got.Files())
		require.Contains(t, string(data), `"mode":2024`)
		require.Contains(t, string(data), `"kind":"symlink"`)
		require.NotContains(t, string(data), `"kind":"file"`)
	})

	t.Run(`Given a manifest entry with an unknown kind,
	when it is built into the domain,
	then it returns an unknown entry kind error`, func(t *testing.T) {
		t.Parallel()

		data := `{"id":"x","files":[{"path":"/a","kind":"socket","size":0}],"created_at":"2026-01-01T00:00:00Z","source":"/","compression":{"type":"none"}}`

		_, err := model.BuildManifestDomain([]byte(data), "snapshots/x.json")

		require.ErrorIs(t, err, model.ErrUnknownEntryKind)
	})
}

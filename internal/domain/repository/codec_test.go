package repository_test

import (
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/stretchr/testify/require"
)

func TestCodec(t *testing.T) {
	t.Parallel()

	compression := &repository.Compression{}

	tests := []struct {
		name                     string
		codec                    repository.Codec
		expectedEncryptsData     bool
		expectedEncryptsMetadata bool
		expectedContentAddressed bool
		expectedBuffers          bool
	}{
		{
			name:  "for a plain version two repository, then it neither encrypts nor buffers",
			codec: repository.NewCodec(repository.FormatVersionTwo, compression),
		},
		{
			name:                 "for an encrypted version two repository, then it encrypts data in memory but keeps the metadata in clear",
			codec:                repository.NewCodec(repository.FormatVersionTwo, compression).WithDataKey([]byte("data"), 2).WithMetadataKey([]byte("meta")),
			expectedEncryptsData: true,
			expectedBuffers:      true,
		},
		{
			name:                     "for a plain version three repository, then it is content addressed without encryption",
			codec:                    repository.NewCodec(repository.FormatVersionThree, compression),
			expectedContentAddressed: true,
		},
		{
			name:                     "for an encrypted version three repository, then it encrypts data and metadata as streams",
			codec:                    repository.NewCodec(repository.FormatVersionThree, compression).WithDataKey([]byte("data"), 2).WithMetadataKey([]byte("meta")),
			expectedEncryptsData:     true,
			expectedEncryptsMetadata: true,
			expectedContentAddressed: true,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a Codec,
		when its rules are checked `+tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expectedEncryptsData, tt.codec.EncryptsData())
			require.Equal(t, tt.expectedEncryptsMetadata, tt.codec.EncryptsMetadata())
			require.Equal(t, tt.expectedContentAddressed, tt.codec.ContentAddressed())
			require.Equal(t, tt.expectedBuffers, tt.codec.BuffersEncryptedObjects())
			require.Same(t, compression, tt.codec.Compression())
		})
	}

	t.Run(`Given a Codec with a data key,
	when the original key slice is changed,
	then the codec keeps its own copy`, func(t *testing.T) {
		t.Parallel()

		key := []byte("data")
		codec := repository.NewCodec(repository.FormatVersionThree, compression).WithDataKey(key, 7)
		key[0] = 'x'

		require.Equal(t, []byte("data"), codec.DataKey())
		require.Equal(t, uint64(7), codec.Generation())
	})
}

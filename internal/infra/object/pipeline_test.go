package object_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/object"
	"github.com/stretchr/testify/require"
)

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

func compressedHash(t *testing.T, comp *repository.Compression, data []byte) string {
	t.Helper()

	handler, err := compression.NewHandlersFactory().GetHandler(comp.CompType())
	require.NoError(t, err)

	hasher := sha256.New()
	encoder, err := handler.Encode(hasher, comp.Level())
	require.NoError(t, err)
	_, err = encoder.Writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, encoder.Closer())

	return hex.EncodeToString(hasher.Sum(nil))
}

func newSymmetricKey(t *testing.T) []byte {
	t.Helper()

	key := make([]byte, encrypt.AES256KeySize)
	_, err := rand.Read(key)
	require.NoError(t, err)

	return key
}

func TestPipeline_Write(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("stowmark "), 1024)
	zstd, err := repository.NewCompression(repository.ZstdCompressionType, nil)
	require.NoError(t, err)
	none, err := repository.NewCompression(repository.NoneCompressionType, nil)
	require.NoError(t, err)

	tests := []struct {
		name         string
		comp         *repository.Compression
		encrypted    bool
		destination  func() io.Writer
		expectedHash func(t *testing.T) string
		expectedErr  error
	}{
		{
			name:        "without compression configuration, then it returns a compression required error",
			destination: func() io.Writer { return &bytes.Buffer{} },
			expectedHash: func(*testing.T) string {
				return ""
			},
			expectedErr: object.ErrCompressionRequired,
		},
		{
			name:        "with encryption and a destination that fails, then it returns the write error",
			comp:        zstd,
			encrypted:   true,
			destination: func() io.Writer { return failingWriter{} },
			expectedHash: func(t *testing.T) string {
				return compressedHash(t, zstd, data)
			},
			expectedErr: errWriteFailed,
		},
		{
			name:        "without encryption and a destination that fails, then it returns the write error",
			comp:        none,
			destination: func() io.Writer { return failingWriter{} },
			expectedHash: func(t *testing.T) string {
				return compressedHash(t, none, data)
			},
			expectedErr: errWriteFailed,
		},
		{
			name:        "with an unexpected hash, then it returns a hash mismatch error",
			comp:        zstd,
			encrypted:   true,
			destination: func() io.Writer { return &bytes.Buffer{} },
			expectedHash: func(*testing.T) string {
				return "0000"
			},
			expectedErr: object.ErrHashMismatch,
		},
		{
			name:        "with encryption and the expected hash, then it writes the object",
			comp:        zstd,
			encrypted:   true,
			destination: func() io.Writer { return &bytes.Buffer{} },
			expectedHash: func(t *testing.T) string {
				return compressedHash(t, zstd, data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(`Given a Pipeline,
		when Write method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			var key []byte
			if tt.encrypted {
				key = newSymmetricKey(t)
			}

			pipeline := object.NewPipeline(compression.NewHandlersFactory())
			err := pipeline.Write(context.Background(), tt.destination(), bytes.NewReader(data), tt.comp, key, tt.expectedHash(t))

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPipeline_Write_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run(`Given a Pipeline with zstd compression and encryption,
	when Write method is called and the result is decrypted and decompressed,
	then it returns the original data`, func(t *testing.T) {
		t.Parallel()

		data := bytes.Repeat([]byte("round trip "), 4096)
		comp, err := repository.NewCompression(repository.ZstdCompressionType, nil)
		require.NoError(t, err)
		key := newSymmetricKey(t)
		handlers := compression.NewHandlersFactory()

		var encoded bytes.Buffer
		err = object.NewPipeline(handlers).Write(context.Background(), &encoded, bytes.NewReader(data), comp, key, compressedHash(t, comp, data))
		require.NoError(t, err)

		decrypted, err := encrypt.NewAESGCMHandler().Decode(&encoded, key)
		require.NoError(t, err)
		handler, err := handlers.GetHandler(comp.CompType())
		require.NoError(t, err)
		decoded, err := handler.Decode(decrypted)
		require.NoError(t, err)
		defer decoded.Closer()

		restored, err := io.ReadAll(decoded.Reader)
		require.NoError(t, err)
		require.Equal(t, data, restored)
	})
}

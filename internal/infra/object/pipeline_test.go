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

func encodeObject(t *testing.T, comp *repository.Compression, key, data []byte) (encoded []byte, hash string) {
	t.Helper()

	hash = compressedHash(t, comp, data)
	var buffer bytes.Buffer
	err := object.NewPipeline(compression.NewHandlersFactory()).Write(context.Background(), &buffer, bytes.NewReader(data), comp, key, hash)
	require.NoError(t, err)

	return buffer.Bytes(), hash
}

func TestPipeline_Read(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("stowmark restore "), 2048)
	compressionTypes := []repository.CompressionType{
		repository.NoneCompressionType,
		repository.GzipCompressionType,
		repository.ZstdCompressionType,
		repository.Lz4CompressionType,
		repository.XzCompressionType,
	}

	for _, compressionType := range compressionTypes {
		for _, encrypted := range []bool{false, true} {
			t.Run(`Given a Pipeline and an object written with `+compressionType.String()+`,
			when Read method is called with the object hash,
			then it returns the original data`, func(t *testing.T) {
				t.Parallel()

				comp, err := repository.NewCompression(compressionType, nil)
				require.NoError(t, err)
				var key []byte
				if encrypted {
					key = newSymmetricKey(t)
				}
				encoded, hash := encodeObject(t, comp, key, data)

				var restored bytes.Buffer
				written, err := object.NewPipeline(compression.NewHandlersFactory()).Read(context.Background(), &restored, bytes.NewReader(encoded), comp, key, hash)

				require.NoError(t, err)
				require.Equal(t, int64(len(data)), written)
				require.Equal(t, data, restored.Bytes())
			})
		}
	}
}

func TestPipeline_Read_Errors(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("original "), 1024)
	other := bytes.Repeat([]byte("replaced "), 1024)
	none, err := repository.NewCompression(repository.NoneCompressionType, nil)
	require.NoError(t, err)
	zstd, err := repository.NewCompression(repository.ZstdCompressionType, nil)
	require.NoError(t, err)
	key := newSymmetricKey(t)

	tests := []struct {
		name        string
		comp        *repository.Compression
		key         []byte
		object      func(t *testing.T) (encoded []byte, hash string)
		ctx         func() context.Context
		expectedErr error
	}{
		{
			name: "without compression configuration, then it returns a compression required error",
			object: func(t *testing.T) ([]byte, string) {
				return encodeObject(t, none, nil, data)
			},
			expectedErr: object.ErrCompressionRequired,
		},
		{
			name: "with a corrupted object, then it returns a hash mismatch error",
			comp: none,
			object: func(t *testing.T) ([]byte, string) {
				encoded, hash := encodeObject(t, none, nil, data)
				corrupted := bytes.Clone(encoded)
				corrupted[len(corrupted)/2] ^= 0xff
				return corrupted, hash
			},
			expectedErr: object.ErrHashMismatch,
		},
		{
			name: "with an encrypted object replaced by another valid object, then it returns a hash mismatch error",
			comp: zstd,
			key:  key,
			object: func(t *testing.T) ([]byte, string) {
				_, hash := encodeObject(t, zstd, key, data)
				replaced, _ := encodeObject(t, zstd, key, other)
				return replaced, hash
			},
			expectedErr: object.ErrHashMismatch,
		},
		{
			name: "with a canceled context, then it returns a context canceled error",
			comp: zstd,
			object: func(t *testing.T) ([]byte, string) {
				return encodeObject(t, zstd, nil, data)
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			expectedErr: context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a Pipeline,
		when Read method is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}
			encoded, hash := tt.object(t)

			_, err := object.NewPipeline(compression.NewHandlersFactory()).Read(ctx, io.Discard, bytes.NewReader(encoded), tt.comp, tt.key, hash)

			require.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

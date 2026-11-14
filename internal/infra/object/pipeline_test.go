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

func legacyCodec(comp *repository.Compression, key []byte) repository.Codec {
	codec := repository.NewCodec(repository.FormatVersionTwo, comp)
	if key == nil {
		return codec
	}
	return codec.WithDataKey(key, 1)
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
			err := pipeline.Write(context.Background(), tt.destination(), bytes.NewReader(data), legacyCodec(tt.comp, key), tt.expectedHash(t))

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
		err = object.NewPipeline(handlers).Write(context.Background(), &encoded, bytes.NewReader(data), legacyCodec(comp, key), compressedHash(t, comp, data))
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
	err := object.NewPipeline(compression.NewHandlersFactory()).Write(context.Background(), &buffer, bytes.NewReader(data), legacyCodec(comp, key), hash)
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
				written, err := object.NewPipeline(compression.NewHandlersFactory()).Read(context.Background(), &restored, bytes.NewReader(encoded), legacyCodec(comp, key), hash)

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

			_, err := object.NewPipeline(compression.NewHandlersFactory()).Read(ctx, io.Discard, bytes.NewReader(encoded), legacyCodec(tt.comp, tt.key), hash)

			require.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func contentCodec(comp *repository.Compression, encrypted bool) repository.Codec {
	codec := repository.NewCodec(repository.FormatVersionThree, comp)
	if !encrypted {
		return codec
	}
	return codec.WithDataKey(bytes.Repeat([]byte{1}, encrypt.AES256KeySize), 1).WithMetadataKey(bytes.Repeat([]byte{2}, encrypt.AES256KeySize))
}

func writeContent(t *testing.T, codec repository.Codec, data []byte) (encoded []byte, hash string) {
	t.Helper()

	factory := compression.NewHandlersFactory()
	hash, err := object.Digest(context.Background(), bytes.NewReader(data), codec, factory)
	require.NoError(t, err)
	var buffer bytes.Buffer
	require.NoError(t, object.NewPipeline(factory).Write(context.Background(), &buffer, bytes.NewReader(data), codec, hash))
	return buffer.Bytes(), hash
}

func TestPipeline_ContentAddressed(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("stowmark v3 "), 8192)
	compressionTypes := []repository.CompressionType{
		repository.NoneCompressionType,
		repository.GzipCompressionType,
		repository.ZstdCompressionType,
		repository.Lz4CompressionType,
		repository.XzCompressionType,
	}

	for _, compressionType := range compressionTypes {
		for _, encrypted := range []bool{false, true} {
			t.Run(`Given a format version 3 object written with `+compressionType.String()+`,
			when Read and Verify are called with a codec that has another compression,
			then the object describes its own compression and returns the original data`, func(t *testing.T) {
				t.Parallel()

				comp, err := repository.NewCompression(compressionType, nil)
				require.NoError(t, err)
				other, err := repository.NewCompression(repository.GzipCompressionType, nil)
				require.NoError(t, err)
				encoded, hash := writeContent(t, contentCodec(comp, encrypted), data)

				var restored bytes.Buffer
				pipeline := object.NewPipeline(compression.NewHandlersFactory())
				written, err := pipeline.Read(context.Background(), &restored, bytes.NewReader(encoded), contentCodec(other, encrypted), hash)

				require.NoError(t, err)
				require.Equal(t, int64(len(data)), written)
				require.Equal(t, data, restored.Bytes())
				require.NoError(t, pipeline.Verify(context.Background(), bytes.NewReader(encoded), contentCodec(other, encrypted), hash))
			})
		}
	}

	t.Run(`Given the same data,
	when it is identified with every compression and with and without encryption,
	then the id only depends on the content and the encrypted repository uses a keyed hash`, func(t *testing.T) {
		t.Parallel()

		factory := compression.NewHandlersFactory()
		plainSum := sha256.Sum256(data)
		ids := make(map[bool]map[string]bool)
		for _, encrypted := range []bool{false, true} {
			ids[encrypted] = make(map[string]bool)
			for _, compressionType := range compressionTypes {
				comp, err := repository.NewCompression(compressionType, nil)
				require.NoError(t, err)
				id, err := object.Digest(context.Background(), bytes.NewReader(data), contentCodec(comp, encrypted), factory)
				require.NoError(t, err)
				ids[encrypted][id] = true
			}
		}

		require.Equal(t, map[string]bool{hex.EncodeToString(plainSum[:]): true}, ids[false])
		require.Len(t, ids[true], 1)
		require.NotEqual(t, ids[false], ids[true])
	})

	t.Run(`Given an encrypted repository without the metadata key,
	when the id of some data is calculated,
	then it returns a metadata key required error`, func(t *testing.T) {
		t.Parallel()

		codec := repository.NewCodec(repository.FormatVersionThree, &repository.Compression{}).WithDataKey(newSymmetricKey(t), 1)

		_, err := object.Digest(context.Background(), bytes.NewReader(data), codec, compression.NewHandlersFactory())

		require.ErrorIs(t, err, object.ErrMetadataKeyRequired)
	})
}

func TestPipeline_ContentAddressed_Errors(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("original "), 4096)
	other := bytes.Repeat([]byte("replaced "), 4096)
	zstd, err := repository.NewCompression(repository.ZstdCompressionType, nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		encrypted   bool
		object      func(t *testing.T) (encoded []byte, hash string)
		expectedErr error
	}{
		{
			name: "with a plain object that does not match its id, then it returns a hash mismatch error",
			object: func(t *testing.T) ([]byte, string) {
				_, hash := writeContent(t, contentCodec(zstd, false), data)
				replaced, _ := writeContent(t, contentCodec(zstd, false), other)
				return replaced, hash
			},
			expectedErr: object.ErrHashMismatch,
		},
		{
			name:      "with an encrypted object stored under another id, then it fails to decrypt because the id is the associated data",
			encrypted: true,
			object: func(t *testing.T) ([]byte, string) {
				_, hash := writeContent(t, contentCodec(zstd, true), data)
				replaced, _ := writeContent(t, contentCodec(zstd, true), other)
				return replaced, hash
			},
			expectedErr: encrypt.ErrDecryptionFailed,
		},
		{
			name:      "with a truncated encrypted object, then it fails to decrypt",
			encrypted: true,
			object: func(t *testing.T) ([]byte, string) {
				encoded, hash := writeContent(t, contentCodec(zstd, true), data)
				return encoded[:len(encoded)-10], hash
			},
			expectedErr: encrypt.ErrDecryptionFailed,
		},
		{
			name: "with an object without a header, then it returns an invalid object encoding error",
			object: func(t *testing.T) ([]byte, string) {
				_, hash := writeContent(t, contentCodec(zstd, false), data)
				return nil, hash
			},
			expectedErr: object.ErrInvalidObjectEncoding,
		},
		{
			name: "with an unknown compression in the header, then it returns an invalid object encoding error",
			object: func(t *testing.T) ([]byte, string) {
				_, hash := writeContent(t, contentCodec(zstd, false), data)
				return append([]byte{3}, "zip"...), hash
			},
			expectedErr: object.ErrInvalidObjectEncoding,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a format version 3 object,
		when Read and Verify are called `+tt.name, func(t *testing.T) {
			t.Parallel()

			encoded, hash := tt.object(t)
			pipeline := object.NewPipeline(compression.NewHandlersFactory())

			_, err := pipeline.Read(context.Background(), io.Discard, bytes.NewReader(encoded), contentCodec(zstd, tt.encrypted), hash)
			require.ErrorIs(t, err, tt.expectedErr)
			require.ErrorIs(t, pipeline.Verify(context.Background(), bytes.NewReader(encoded), contentCodec(zstd, tt.encrypted), hash), tt.expectedErr)
		})
	}

	t.Run(`Given an encrypted format version 3 object,
	when it is written,
	then the stored bytes are an encryption stream that does not contain the compression name`, func(t *testing.T) {
		t.Parallel()

		encoded, _ := writeContent(t, contentCodec(zstd, true), data)

		require.True(t, encrypt.IsStream(encoded))
		require.NotContains(t, string(encoded), "zstd")
	})
}

func TestPipeline_Verify_Compressed(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte("legacy "), 1024)
	zstd, err := repository.NewCompression(repository.ZstdCompressionType, nil)
	require.NoError(t, err)
	key := newSymmetricKey(t)

	t.Run(`Given a format version 2 object,
	when Verify is called without the compression of the manifest,
	then it checks the hash of the stored data`, func(t *testing.T) {
		t.Parallel()

		encoded, hash := encodeObject(t, zstd, key, data)
		pipeline := object.NewPipeline(compression.NewHandlersFactory())

		require.NoError(t, pipeline.Verify(context.Background(), bytes.NewReader(encoded), legacyCodec(nil, key), hash))
		require.ErrorIs(t, pipeline.Verify(context.Background(), bytes.NewReader(encoded), legacyCodec(nil, key), "0000"), object.ErrHashMismatch)
	})
}

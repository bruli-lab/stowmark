package compression_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/stretchr/testify/require"
)

const goldenDataSize = 512 * 1024

func goldenData() []byte {
	data := make([]byte, goldenDataSize)
	for index := range data {
		data[index] = byte((index*31 + index/7) % 251)
	}
	copy(data, bytes.Repeat([]byte("stowmark golden data "), 1024))
	return data
}

func encode(t *testing.T, handler compression.Handler, level *int, data []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	encoder, err := handler.Encode(&buffer, level)
	require.NoError(t, err)
	_, err = encoder.Writer.Write(data)
	require.NoError(t, err)
	require.NoError(t, encoder.Closer())

	return buffer.Bytes()
}

func decode(t *testing.T, handler compression.Handler, encoded []byte) []byte {
	t.Helper()

	decoder, err := handler.Decode(bytes.NewReader(encoded))
	require.NoError(t, err)
	defer decoder.Closer()
	decoded, err := io.ReadAll(decoder.Reader)
	require.NoError(t, err)

	return decoded
}

func handlerFor(t *testing.T, compressionType repository.CompressionType) compression.Handler {
	t.Helper()

	handler, err := compression.NewHandlersFactory().GetHandler(compressionType)
	require.NoError(t, err)

	return handler
}

func TestHandlers_Golden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		compressionType repository.CompressionType
		expectedHash    string
	}{
		{compressionType: repository.NoneCompressionType, expectedHash: "aeda64c80ccc2c067bce076615e8f683a05fd526fb8a031f39eb18e65bba50de"},
		{compressionType: repository.GzipCompressionType, expectedHash: "d8fbfa9b8d8623dcea2e26ad8a6065bc15224c187dff2ecd420cd13cab26b023"},
		{compressionType: repository.ZstdCompressionType, expectedHash: "11924edcd5205919eef6afa5facbd3559c778bc552433acebbcee385c1ef6d40"},
		{compressionType: repository.Lz4CompressionType, expectedHash: "eb3ae4d63bbccb16e926b34cf6c26e5d84817fa849f743ecea97ac339b1e903d"},
		{compressionType: repository.XzCompressionType, expectedHash: "92d5659d254ac592fe4defb7eaf36e07522bb4c1e7303946f2a79cb993d357e6"},
	}

	for _, tt := range tests {
		t.Run(`Given the `+tt.compressionType.String()+` handler with the default level,
		when the golden data is encoded,
		then the SHA-256 of the output matches the one stored in existing repositories`, func(t *testing.T) {
			t.Parallel()

			comp, err := repository.NewCompression(tt.compressionType, nil)
			require.NoError(t, err)

			encoded := encode(t, handlerFor(t, tt.compressionType), comp.Level(), goldenData())

			sum := sha256.Sum256(encoded)
			require.Equal(t, tt.expectedHash, hex.EncodeToString(sum[:]))
		})
	}
}

func TestHandlers_RoundTrip(t *testing.T) {
	t.Parallel()

	data := goldenData()
	tests := []struct {
		compressionType repository.CompressionType
		levels          []int
	}{
		{compressionType: repository.NoneCompressionType},
		{compressionType: repository.XzCompressionType},
		{
			compressionType: repository.GzipCompressionType,
			levels:          []int{repository.MinimumGzipLevel, repository.DefaultGzipLevel, repository.MaximumGzipLevel},
		},
		{
			compressionType: repository.ZstdCompressionType,
			levels:          []int{repository.MinimumZstdLevel, repository.DefaultZstdLevel, repository.MaximumZstdLevel},
		},
		{
			compressionType: repository.Lz4CompressionType,
			levels:          []int{repository.MinimumLz4Level, 5, repository.MaximumLz4Level},
		},
	}

	for _, tt := range tests {
		levels := []*int{nil}
		if len(tt.levels) > 0 {
			levels = levels[:0]
			for _, level := range tt.levels {
				levels = append(levels, &level)
			}
		}
		for _, level := range levels {
			t.Run(`Given the `+tt.compressionType.String()+` handler,
			when data is encoded twice and decoded,
			then the encoded output is deterministic and decodes to the original data`, func(t *testing.T) {
				t.Parallel()

				handler := handlerFor(t, tt.compressionType)

				first := encode(t, handler, level, data)
				second := encode(t, handler, level, data)

				require.Equal(t, first, second)
				require.Equal(t, data, decode(t, handler, first))
			})
		}
	}
}

func TestHandlers_Errors(t *testing.T) {
	t.Parallel()

	invalidLz4Level := repository.MaximumLz4Level + 1
	tests := []struct {
		name            string
		compressionType repository.CompressionType
		level           *int
		expectedErr     error
	}{
		{
			name:            "the gzip handler without a level, then it returns a level required error",
			compressionType: repository.GzipCompressionType,
			expectedErr:     compression.ErrLevelRequired,
		},
		{
			name:            "the zstd handler without a level, then it returns a level required error",
			compressionType: repository.ZstdCompressionType,
			expectedErr:     compression.ErrLevelRequired,
		},
		{
			name:            "the lz4 handler without a level, then it returns a level required error",
			compressionType: repository.Lz4CompressionType,
			expectedErr:     compression.ErrLevelRequired,
		},
		{
			name:            "the lz4 handler with a level out of range, then it returns an invalid level error",
			compressionType: repository.Lz4CompressionType,
			level:           &invalidLz4Level,
			expectedErr:     compression.ErrInvalidLevel,
		},
	}

	for _, tt := range tests {
		t.Run(`Given a compression handler,
		when Encode method is called on `+tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := handlerFor(t, tt.compressionType).Encode(io.Discard, tt.level)

			require.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestHandlers_DecodeInvalidData(t *testing.T) {
	t.Parallel()

	invalid := bytes.Repeat([]byte("not compressed data "), 64)
	compressionTypes := []repository.CompressionType{
		repository.GzipCompressionType,
		repository.ZstdCompressionType,
		repository.Lz4CompressionType,
		repository.XzCompressionType,
	}

	for _, compressionType := range compressionTypes {
		t.Run(`Given the `+compressionType.String()+` handler,
		when data that was not compressed with it is decoded,
		then it returns an error`, func(t *testing.T) {
			t.Parallel()

			decoder, err := handlerFor(t, compressionType).Decode(bytes.NewReader(invalid))
			if err == nil {
				defer decoder.Closer()
				_, err = io.ReadAll(decoder.Reader)
			}

			require.Error(t, err)
		})
	}
}

func TestHandlersFactory_GetHandler(t *testing.T) {
	t.Parallel()

	t.Run(`Given a HandlersFactory,
	when GetHandler method is called with an unknown compression type,
	then it returns an unsupported compression type error`, func(t *testing.T) {
		t.Parallel()

		_, err := compression.NewHandlersFactory().GetHandler(repository.CompressionType("brotli"))

		require.ErrorIs(t, err, compression.ErrUnsupportedCompressionType)
	})
}

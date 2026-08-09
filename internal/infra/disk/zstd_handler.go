package disk

import (
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

type zstdHandler struct{}

func (z zstdHandler) Decode(origin io.Reader) (io.Reader, func(), error) {
	reader, err := zstd.NewReader(origin)
	if err != nil {
		return nil, nil, fmt.Errorf("create zstd decoder: %w", err)
	}
	return reader, reader.Close, nil
}

func (z zstdHandler) Encode(destination io.Writer, level *int) (io.Writer, error) {
	encoder, err := zstd.NewWriter(destination, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(*level)))
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}
	return encoder, nil
}

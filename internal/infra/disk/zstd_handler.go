package disk

import (
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

type zstdHandler struct{}

func (z zstdHandler) Decode(origin io.Reader) (*ReaderCloser, error) {
	reader, err := zstd.NewReader(origin)
	if err != nil {
		return nil, fmt.Errorf("create zstd decoder: %w", err)
	}
	return &ReaderCloser{
		Reader: reader,
		Closer: reader.Close,
	}, nil
}

func (z zstdHandler) Encode(destination io.Writer, level *int) (*WriterCloser, error) {
	encoder, err := zstd.NewWriter(destination, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(*level)))
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}
	return &WriterCloser{
		Writer: encoder,
		Closer: encoder.Close,
	}, nil
}

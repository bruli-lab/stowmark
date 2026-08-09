package disk

import (
	"fmt"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/klauspost/compress/zstd"
)

type compressionEncoder interface {
	Encode(destination io.Writer, level *int) (io.Writer, error)
}

type noneEncoder struct{}

func (n noneEncoder) Encode(destination io.Writer, _ *int) (io.Writer, error) {
	return destination, nil
}

type zstdEncoder struct{}

func (z zstdEncoder) Encode(destination io.Writer, level *int) (io.Writer, error) {
	encoder, err := zstd.NewWriter(destination, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(*level)))
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}
	return encoder, nil
}

type encoderFactory struct {
	encoders map[repository.CompressionType]compressionEncoder
}

func (e *encoderFactory) getEncoder(ct repository.CompressionType) (compressionEncoder, error) {
	enc, ok := e.encoders[ct]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type %s", ct.String())
	}
	return enc, nil
}

func newEncoderFactory() *encoderFactory {
	return &encoderFactory{encoders: map[repository.CompressionType]compressionEncoder{
		repository.NoneCompressionType: noneEncoder{},
		repository.ZstdCompressionType: zstdEncoder{},
	}}
}

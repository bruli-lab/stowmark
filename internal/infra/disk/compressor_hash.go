package disk

import (
	"context"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/klauspost/compress/zstd"
)

type Compressor interface {
	Get(ctx context.Context, filePath string, file *os.File, hasher hash.Hash, level int) error
}
type CompressorHash struct {
	compressors map[repository.CompressionType]Compressor
}

func (h CompressorHash) apply(compType repository.CompressionType) (Compressor, error) {
	compressor, ok := h.compressors[compType]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type %s", compType.String())
	}
	return compressor, nil
}

func newCompressorHash() *CompressorHash {
	compressors := map[repository.CompressionType]Compressor{
		repository.NoneCompressionType: noneCompressor{},
		repository.ZstdCompressionType: zstdCompressor{},
	}
	return &CompressorHash{compressors: compressors}
}

type noneCompressor struct{}

func (n noneCompressor) Get(ctx context.Context, filePath string, file *os.File, hasher hash.Hash, _ int) error {
	if _, err := io.Copy(hasher, contextReader{
		ctx:    ctx,
		reader: file,
	}); err != nil {
		return fmt.Errorf("calculate hash for %q: %w", filePath, err)
	}
	return nil
}

type zstdCompressor struct{}

func (z zstdCompressor) Get(ctx context.Context, filePath string, file *os.File, hasher hash.Hash, level int) error {
	encoder, err := zstd.NewWriter(
		hasher,
		zstd.WithEncoderLevel(
			zstd.EncoderLevelFromZstd(level),
		),
	)
	if err != nil {
		return fmt.Errorf("create zstd encoder: %w", err)
	}

	if _, err := io.Copy(encoder, contextReader{
		ctx:    ctx,
		reader: file,
	}); err != nil {
		_ = encoder.Close()
		return fmt.Errorf("compress %q for hashing: %w", filePath, err)
	}

	if err := encoder.Close(); err != nil {
		return fmt.Errorf(
			"finish compression for hashing %q: %w",
			filePath,
			err,
		)
	}
	return nil
}

package disk

import (
	"fmt"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/klauspost/compress/zstd"
)

func zstdCompression(destination io.Writer, source io.Reader, comp *repository.Compression) error {
	encoder, err := zstd.NewWriter(
		destination,
		zstd.WithEncoderLevel(
			zstd.EncoderLevelFromZstd(*comp.Level()),
		),
	)
	if err != nil {
		return fmt.Errorf("create zstd encoder: %w", err)
	}

	if _, err := io.Copy(encoder, source); err != nil {
		_ = encoder.Close()
		return fmt.Errorf("compress object with zstd: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return fmt.Errorf("close zstd encoder: %w", err)
	}
	return nil
}

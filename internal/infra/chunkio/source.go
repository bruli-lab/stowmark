package chunkio

import (
	"errors"
	"fmt"
	"os"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

func OpenSource(filePath, hash string, offset, size int64, comp *repository.Compression) (*os.File, error) {
	if comp == nil {
		return nil, errors.New("compression configuration is required")
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid o bject hash %q", hash)
	}

	if offset < 0 {
		return nil, fmt.Errorf("invalid chunk offset: %d", offset)
	}

	if size <= 0 {
		return nil, fmt.Errorf("invalid chunk size: %d", size)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open source file %q: %w", filePath, err)
	}

	info, err := source.Stat()
	if err != nil {
		_ = source.Close()

		return nil, fmt.Errorf("stat source file %q: %w", filePath, err)
	}

	if !info.Mode().IsRegular() {
		_ = source.Close()

		return nil, fmt.Errorf("%q is not a regular file", filePath)
	}

	if offset > info.Size() ||
		size > info.Size()-offset {
		_ = source.Close()

		return nil, fmt.Errorf("chunk range [%d,%d) exceeds file size %d for %q", offset, offset+size, info.Size(), filePath)
	}

	return source, nil
}

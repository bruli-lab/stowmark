package disk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type SourceExplorer struct {
	handlersFactory *compression.HandlersFactory
}

func (s SourceExplorer) CalculateChunks(ctx context.Context, filePath string, chunkSize int64, comp *repository.Compression) ([]snapshot.Chunk, error) {
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunk size must be greater than zero: %d", chunkSize)
	}

	if comp == nil {
		return nil, errors.New("compression configuration is required")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf(
			"open file %q: %w",
			filePath,
			err,
		)
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file %q: %w", filePath, err)
	}

	fileSize := info.Size()

	chunksCount := int(
		(fileSize + chunkSize - 1) / chunkSize,
	)

	chunks := make(
		[]snapshot.Chunk,
		0,
		chunksCount,
	)

	for offset := int64(0); offset < fileSize; offset += chunkSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		currentSize := chunkSize

		remaining := fileSize - offset
		if remaining < currentSize {
			currentSize = remaining
		}

		reader := io.NewSectionReader(
			file,
			offset,
			currentSize,
		)

		hash, err := s.calculateReaderHash(
			ctx,
			reader,
			comp,
		)
		if err != nil {
			return nil, fmt.Errorf("calculate chunk hash for %q at offset %d: %w", filePath, offset, err)
		}

		chunks = append(
			chunks,
			*snapshot.NewChunk(hash, offset, currentSize),
		)
	}

	return chunks, nil
}

func (s SourceExplorer) calculateReaderHash(ctx context.Context, reader io.Reader, comp *repository.Compression) (string, error) {
	hasher := sha256.New()

	handler, err := s.handlersFactory.GetHandler(
		comp.CompType(),
	)
	if err != nil {
		return "", err
	}

	encoder, err := handler.Encode(
		hasher,
		comp.Level(),
	)
	if err != nil {
		return "", err
	}

	_, copyErr := io.Copy(
		encoder.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: reader,
		},
	)
	if copyErr != nil {
		if encoder.Closer != nil {
			_ = encoder.Closer()
		}

		return "", fmt.Errorf(
			"compress data for hashing: %w",
			copyErr,
		)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			return "", fmt.Errorf(
				"finish compression for hashing: %w",
				err,
			)
		}
	}

	return hex.EncodeToString(
		hasher.Sum(nil),
	), nil
}

func (s SourceExplorer) CalculateHash(ctx context.Context, filePath string, comp *repository.Compression) (string, error) {
	fi, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file %q: %w", filePath, err)
	}
	defer func() {
		_ = fi.Close()
	}()

	return s.calculateReaderHash(ctx, fi, comp)
}

func (s SourceExplorer) Explore(ctx context.Context, sourcePath string) (*snapshot.Source, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	absolutePath, err := absolutePath(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute path: %w", err)
	}

	info, err := os.Stat(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("stat source path: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("source path is not a directory: %s", absolutePath)
	}
	files, err := s.readFiles(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("read files: %w", err)
	}
	return snapshot.NewSource(absolutePath, files), nil
}

func (s SourceExplorer) readFiles(root string) ([]snapshot.File, error) {
	files := make([]snapshot.File, 0)

	err := filepath.WalkDir(root, func(
		path string,
		entry fs.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}

		file := snapshot.NewFile(path, info.Size())
		files = append(files, *file)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory %q: %w", root, err)
	}
	return files, nil
}

func NewSourceRepository() *SourceExplorer {
	return &SourceExplorer{handlersFactory: compression.NewHandlersFactory()}
}

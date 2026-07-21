package disk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bruli-lab/stonekeep.git/internal/domain/snapshot"
)

type SourceExplorer struct{}

func (s SourceExplorer) CalculateHash(ctx context.Context, filePath string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file %q: %w", filePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	hasher := sha256.New()

	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("calculate hash for %q: %w", filePath, err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s SourceExplorer) Explore(ctx context.Context, sourcePath string) (*snapshot.Source, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	absolutePath, err := filepath.Abs(sourcePath)
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
	return &SourceExplorer{}
}

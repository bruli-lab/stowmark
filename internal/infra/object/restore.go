package object

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/localfile"
)

const (
	restoredFileMode      os.FileMode = 0o644
	restoredDirectoryMode os.FileMode = 0o755
)

var (
	ErrFileRequired = errors.New("snapshot file is required")
	ErrSizeMismatch = errors.New("restored size mismatch")
)

type PartReader func(ctx context.Context, hash string, destination io.Writer) (int64, error)

func RestoreFile(ctx context.Context, comp *repository.Compression, file *snapshot.File, readPart PartReader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if comp == nil {
		return ErrCompressionRequired
	}
	if file == nil {
		return ErrFileRequired
	}

	hashes, err := GetHashes(file)
	if err != nil {
		return err
	}

	destinationPath := file.Path()
	if err := os.MkdirAll(filepath.Dir(destinationPath), restoredDirectoryMode); err != nil {
		return fmt.Errorf("create destination directory for %q: %w", destinationPath, err)
	}

	return localfile.WriteAtomically(destinationPath, restoredFileMode, func(destination io.Writer) error {
		var restoredSize int64
		for index, hash := range hashes {
			if err := ctx.Err(); err != nil {
				return err
			}

			written, err := readPart(ctx, hash, destination)
			if err != nil {
				return fmt.Errorf("restore part %d/%d of %q: %w", index+1, len(hashes), destinationPath, err)
			}
			restoredSize += written
		}

		if restoredSize != file.Size() {
			return fmt.Errorf("%w for %q: expected %d, restored %d", ErrSizeMismatch, destinationPath, file.Size(), restoredSize)
		}

		return nil
	})
}

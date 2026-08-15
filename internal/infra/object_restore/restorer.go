package objectrestore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type Restorer struct {
	repositoryPath  string
	handlersFactory *compression.HandlersFactory
}

func (o Restorer) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File) error {
	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if obj == nil {
		return errors.New("snapshot object is required")
	}

	hashes, err := objectHashes(obj)
	if err != nil {
		return err
	}

	destinationPath := obj.Path()

	if err := os.MkdirAll(
		filepath.Dir(destinationPath),
		0o755,
	); err != nil {
		return fmt.Errorf(
			"create destination directory for %q: %w",
			destinationPath,
			err,
		)
	}

	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf(
			"create restored file %q: %w",
			destinationPath,
			err,
		)
	}

	restoreCompleted := false

	defer func() {
		if !restoreCompleted {
			_ = destination.Close()
			_ = os.Remove(destinationPath)
		}
	}()

	var restoredSize int64

	for index, hash := range hashes {
		if err := ctx.Err(); err != nil {
			return err
		}

		written, err := o.restoreObjectPart(
			ctx,
			comp,
			hash,
			destination,
		)
		if err != nil {
			return fmt.Errorf("restore part %d/%d of %q: %w", index+1, len(hashes), destinationPath, err)
		}

		restoredSize += written
	}

	if restoredSize != obj.Size() {
		return fmt.Errorf("restored size mismatch for %q: expected %d, restored %d", destinationPath, obj.Size(), restoredSize)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close restored file %q: %w", destinationPath, err)
	}

	restoreCompleted = true

	return nil
}

func (o Restorer) restoreObjectPart(ctx context.Context, comp *repository.Compression, hash string, destination io.Writer) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	sourcePath := filepath.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	source, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("open object %q: %w", sourcePath, err)
	}

	handler, err := o.handlersFactory.GetHandler(
		comp.CompType(),
	)
	if err != nil {
		_ = source.Close()
		return 0, err
	}

	decoded, err := handler.Decode(source)
	if err != nil {
		_ = source.Close()

		return 0, fmt.Errorf("decode object %q using %q: %w", sourcePath, comp.CompType(), err)
	}

	written, copyErr := io.Copy(
		destination,
		model.ContextReader{
			Ctx:    ctx,
			Reader: decoded.Reader,
		},
	)

	decoded.Closer()

	sourceCloseErr := source.Close()

	if copyErr != nil {
		return 0, fmt.Errorf("read decoded object %q: %w", sourcePath, copyErr)
	}

	if sourceCloseErr != nil {
		return 0, fmt.Errorf("close object %q: %w", sourcePath, sourceCloseErr)
	}

	return written, nil
}

func NewRestorer(repositoryPath string, handlersFactory *compression.HandlersFactory) *Restorer {
	return &Restorer{repositoryPath: repositoryPath, handlersFactory: handlersFactory}
}

func objectHashes(obj *snapshot.File) ([]string, error) {
	chunks := obj.Chunks()

	if len(chunks) > 0 {
		hashes := make([]string, 0, len(chunks))

		for index, chunk := range chunks {
			hash := chunk.Hash()

			if len(hash) < 3 {
				return nil, fmt.Errorf("invalid hash for chunk %d: %q", index+1, hash)
			}

			hashes = append(hashes, hash)
		}

		return hashes, nil
	}

	hash := obj.Hash()

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	return []string{hash}, nil
}

package disk

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
	objectrestore "github.com/bruli-lab/stowmark/internal/infra/object_restore"
)

type ObjectRepository struct {
	repositoryPath  string
	handlersFactory *compression.HandlersFactory
	*objectrestore.Restorer
}

func (o ObjectRepository) ReadObject(ctx context.Context, hash string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := filepath.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	reader, err := os.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open object %q: %w", objectPath, err)
	}

	return reader, nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	destinationPath := filepath.Join(o.repositoryPath, repository.ObjectsFolder, hash[:2], hash[2:])
	_, err := os.Stat(destinationPath)
	switch {
	case err == nil:
		return true, nil
	case !errors.Is(err, os.ErrNotExist):
		return false, fmt.Errorf("check destination file %q: %w", destinationPath, err)
	}
	return false, nil
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	destinationPath := filepath.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf(
			"create object directory %q: %w",
			filepath.Dir(destinationPath),
			err,
		)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o644,
	)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}

		return fmt.Errorf("create object file %q: %w", destinationPath, err)
	}

	writeCompleted := false
	defer func() {
		_ = destination.Close()

		if !writeCompleted {
			_ = os.Remove(destinationPath)
		}
	}()

	encoder, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}
	writer, err := encoder.Encode(destination, comp.Level())
	if err != nil {
		return err
	}
	if _, err := io.Copy(writer.Writer, source); err != nil {
		return fmt.Errorf("copy object: %w", err)
	}

	if writer.Closer != nil {
		if err := writer.Closer(); err != nil {
			return fmt.Errorf(
				"close destination file %q: %w",
				destinationPath,
				err,
			)
		}
	}

	writeCompleted = true
	return nil
}

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression) error {
	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	if offset < 0 {
		return fmt.Errorf("invalid chunk offset: %d", offset)
	}

	if size <= 0 {
		return fmt.Errorf("invalid chunk size: %d", size)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	sourceInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat source file %q: %w", filePath, err)
	}

	if offset+size > sourceInfo.Size() {
		return fmt.Errorf("chunk range [%d,%d) exceeds file size %d for %q", offset, offset+size, sourceInfo.Size(), filePath)
	}

	destinationPath := filepath.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	if err := os.MkdirAll(
		filepath.Dir(destinationPath),
		0o755,
	); err != nil {
		return fmt.Errorf("create object directory %q: %w", filepath.Dir(destinationPath), err)
	}

	destination, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("create object %q: %w", destinationPath, err)
	}

	success := false

	defer func() {
		_ = destination.Close()

		if !success {
			_ = os.Remove(destinationPath)
		}
	}()

	handler, err := o.handlersFactory.GetHandler(
		comp.CompType(),
	)
	if err != nil {
		return err
	}

	encoder, err := handler.Encode(
		destination,
		comp.Level(),
	)
	if err != nil {
		return err
	}

	section := io.NewSectionReader(
		source,
		offset,
		size,
	)

	_, copyErr := io.Copy(
		encoder.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: section,
		},
	)
	if copyErr != nil {
		if encoder.Closer != nil {
			_ = encoder.Closer()
		}

		return fmt.Errorf("write chunk from %q at offset %d: %w", filePath, offset, copyErr)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			return fmt.Errorf("finish chunk compression for %q: %w", filePath, err)
		}
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close object %q: %w", destinationPath, err)
	}

	success = true

	return nil
}

func NewObjectRepository(repositoryPath string) (*ObjectRepository, error) {
	absPath, err := absolutePath(repositoryPath)
	if err != nil {
		return nil, err
	}
	handlersFactory := compression.NewHandlersFactory()
	restorer := objectrestore.NewRestorer(absPath, handlersFactory)
	repo := ObjectRepository{repositoryPath: absPath, handlersFactory: handlersFactory, Restorer: restorer}
	return &repo, nil
}

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
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

type ObjectRepository struct {
	repositoryPath  string
	handlersFactory *compression.HandlersFactory
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

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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

func (o ObjectRepository) restoreObjectPart(ctx context.Context, comp *repository.Compression, hash string, destination io.Writer) (int64, error) {
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

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := chunkio.OpenSource(filePath, hash, offset, size, comp)
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

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
	repo := ObjectRepository{repositoryPath: absPath, handlersFactory: handlersFactory}
	return &repo, nil
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

package disk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

type ObjectRepository struct {
	repositoryPath  string
	handlersFactory *compression.HandlersFactory
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

	hash := obj.Hash()
	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	sourcePath := filepath.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open object %q: %w", sourcePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}

	decoded, err := handler.Decode(source)
	if err != nil {
		return fmt.Errorf(
			"decode object %q using %q: %w",
			sourcePath,
			comp.CompType(),
			err,
		)
	}
	defer decoded.Closer()

	destinationPath := obj.Path()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
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

	if _, err := io.Copy(
		destination,
		model.ContextReader{
			Ctx:    ctx,
			Reader: decoded.Reader,
		},
	); err != nil {
		return fmt.Errorf(
			"restore object %q to %q: %w",
			sourcePath,
			destinationPath,
			err,
		)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf(
			"close restored file %q: %w",
			destinationPath,
			err,
		)
	}

	restoreCompleted = true

	return nil
}

func (o ObjectRepository) ReadObject(ctx context.Context, originalPath, hash string) (*snapshot.File, error) {
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

	objectFile, err := os.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, snapshot.NewNotFoundError(originalPath)
		}

		return nil, fmt.Errorf(
			"open object %q: %w",
			objectPath,
			err,
		)
	}
	defer func() {
		_ = objectFile.Close()
	}()

	hasher := sha256.New()

	storedSize, err := io.Copy(
		hasher,
		model.ContextReader{
			Ctx:    ctx,
			Reader: objectFile,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read object %q: %w",
			objectPath,
			err,
		)
	}

	calculatedHash := hex.EncodeToString(hasher.Sum(nil))

	result := snapshot.File{}
	result.Hydrate(
		originalPath,
		calculatedHash,
		storedSize,
	)

	return &result, nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, obj *snapshot.File) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	hash := obj.Hash()
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

func (o ObjectRepository) Save(ctx context.Context, obj *snapshot.File, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	hash := obj.Hash()
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

	source, err := os.Open(obj.Path())
	if err != nil {
		return fmt.Errorf("open source file %q: %w", obj.Path(), err)
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

		return fmt.Errorf(
			"create object file %q: %w",
			destinationPath,
			err,
		)
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

func NewObjectRepository(repositoryPath string) (*ObjectRepository, error) {
	absPath, err := absolutePath(repositoryPath)
	if err != nil {
		return nil, err
	}
	return &ObjectRepository{repositoryPath: absPath, handlersFactory: compression.NewHandlersFactory()}, nil
}

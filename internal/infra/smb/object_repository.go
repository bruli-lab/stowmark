package smb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/cloudsoda/go-smb2"
)

type ObjectRepository struct {
	repositoryPath  string
	share           *smb2.Share
	handlersFactory *compression.HandlersFactory
}

func (o ObjectRepository) Save(ctx context.Context, obj *snapshot.File, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	destinationPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)
	destinationDir := path.Dir(destinationPath)

	if err := o.share.MkdirAll(destinationDir, 0o755); err != nil {
		return fmt.Errorf("create object directory %q: %w", destinationDir, err)
	}

	source, err := os.Open(obj.Path())
	if err != nil {
		return fmt.Errorf("open source file %q: %w", obj.Path(), err)
	}
	defer func() {
		_ = source.Close()
	}()

	destination, err := o.share.OpenFile(
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
	destinationClosed := false

	defer func() {
		if !destinationClosed {
			_ = destination.Close()
		}

		if !writeCompleted {
			_ = o.share.Remove(destinationPath)
		}
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}

	writer, err := handler.Encode(destination, comp.Level())
	if err != nil {
		return fmt.Errorf(
			"create encoder for object %q: %w",
			destinationPath,
			err,
		)
	}

	if _, err := io.Copy(
		writer.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: source,
		},
	); err != nil {
		return fmt.Errorf(
			"write object %q: %w",
			destinationPath,
			err,
		)
	}

	if writer.Closer != nil {
		if err := writer.Closer(); err != nil {
			return fmt.Errorf(
				"close encoder for object %q: %w",
				destinationPath,
				err,
			)
		}
	}

	if err := destination.Close(); err != nil {
		destinationClosed = true

		return fmt.Errorf(
			"close object file %q: %w",
			destinationPath,
			err,
		)
	}

	destinationClosed = true
	writeCompleted = true

	return nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, obj *snapshot.File) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	hash := obj.Hash()
	if len(hash) < 3 {
		return false, fmt.Errorf("invalid object hash %q", hash)
	}

	destinationPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	_, err := o.share.Stat(destinationPath)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("check destination file %q: %w", destinationPath, err)
	}
}

func (o ObjectRepository) ReadObject(ctx context.Context, originalPath, hash string) (*snapshot.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	objectFile, err := o.share.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, snapshot.NewNotFoundError(originalPath)
		}

		return nil, fmt.Errorf("open object %q: %w", objectPath, err)
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

	sourcePath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	source, err := o.share.Open(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshot.NewNotFoundError(obj.Path())
		}

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
		return fmt.Errorf("decode object %q using %q: %w", sourcePath, comp.CompType(), err)
	}

	if decoded.Closer != nil {
		defer decoded.Closer()
	}

	destinationPath := obj.Path()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory for %q: %w", destinationPath, err)
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
	destinationClosed := false

	defer func() {
		if !destinationClosed {
			_ = destination.Close()
		}

		if !restoreCompleted {
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
		return fmt.Errorf("restore object %q to %q: %w", sourcePath, destinationPath, err)
	}

	if err := destination.Close(); err != nil {
		destinationClosed = true

		return fmt.Errorf(
			"close restored file %q: %w",
			destinationPath,
			err,
		)
	}

	destinationClosed = true
	restoreCompleted = true

	return nil
}

func NewObjectRepository(repositoryPath string, share *smb2.Share) *ObjectRepository {
	return &ObjectRepository{repositoryPath: repositoryPath, share: share, handlersFactory: compression.NewHandlersFactory()}
}

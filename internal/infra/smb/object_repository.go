package smb

import (
	"bytes"
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
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/cloudsoda/go-smb2"
)

type ObjectRepository struct {
	repositoryPath  string
	share           *smb2.Share
	handlersFactory *compression.HandlersFactory
}

func (o ObjectRepository) ReadObject(ctx context.Context, hash string) (io.ReadCloser, error) {
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

	reader, err := o.share.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) ||
			os.IsNotExist(err) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open SMB object %q: %w", objectPath, err)
	}

	return reader, nil
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

	section := io.NewSectionReader(
		source,
		offset,
		size,
	)

	var encoded bytes.Buffer

	handler, err := o.handlersFactory.GetHandler(
		comp.CompType(),
	)
	if err != nil {
		return err
	}

	encoder, err := handler.Encode(
		&encoded,
		comp.Level(),
	)
	if err != nil {
		return fmt.Errorf("create compression encoder for chunk %q: %w", hash, err)
	}

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

		return fmt.Errorf("compress chunk %q from %q at offset %d: %w", hash, filePath, offset, copyErr)
	}

	if encoder.Closer != nil {
		if err := encoder.Closer(); err != nil {
			return fmt.Errorf("finish compression for chunk %q: %w", hash, err)
		}
	}

	calculatedHashBytes := sha256.Sum256(
		encoded.Bytes(),
	)

	calculatedHash := hex.EncodeToString(
		calculatedHashBytes[:],
	)

	if calculatedHash != hash {
		return fmt.Errorf("chunk hash mismatch: expected %s, calculated %s", hash, calculatedHash)
	}

	objectDirectory := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
	)

	objectPath := path.Join(
		objectDirectory,
		hash[2:],
	)

	if err := o.share.MkdirAll(
		objectDirectory,
		0o755,
	); err != nil {
		return fmt.Errorf("create SMB object directory %q: %w", objectDirectory, err)
	}

	destination, err := o.share.OpenFile(
		objectPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create SMB object %q: %w", objectPath, err)
	}

	_, writeErr := io.Copy(
		destination,
		model.ContextReader{
			Ctx:    ctx,
			Reader: bytes.NewReader(encoded.Bytes()),
		},
	)

	closeErr := destination.Close()

	if writeErr != nil {
		return fmt.Errorf("write SMB object %q: %w", objectPath, writeErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close SMB object %q: %w", objectPath, closeErr)
	}

	return nil
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

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

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
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
		return fmt.Errorf("create encoder for object %q: %w", destinationPath, err)
	}

	if _, err := io.Copy(
		writer.Writer,
		model.ContextReader{
			Ctx:    ctx,
			Reader: source,
		},
	); err != nil {
		return fmt.Errorf("write object %q: %w", destinationPath, err)
	}

	if writer.Closer != nil {
		if err := writer.Closer(); err != nil {
			return fmt.Errorf("close encoder for object %q: %w", destinationPath, err)
		}
	}

	if err := destination.Close(); err != nil {
		destinationClosed = true

		return fmt.Errorf("close object file %q: %w", destinationPath, err)
	}

	destinationClosed = true
	writeCompleted = true

	return nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

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

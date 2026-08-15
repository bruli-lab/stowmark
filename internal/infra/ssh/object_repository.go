package ssh

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
	"github.com/pkg/sftp"
)

type ObjectRepository struct {
	client          *sftp.Client
	handlersFactory *compression.HandlersFactory
	repositoryPath  string
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

	reader, err := o.client.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) ||
			os.IsNotExist(err) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open SSH object %q: %w", objectPath, err)
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

	if err := o.client.MkdirAll(objectDirectory); err != nil {
		return fmt.Errorf("create SSH object directory %q: %w", objectDirectory, err)
	}

	destination, err := o.client.OpenFile(
		objectPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
	)
	if err != nil {
		return fmt.Errorf("create SSH object %q: %w", objectPath, err)
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
		return fmt.Errorf("write SSH object %q: %w", objectPath, writeErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close SSH object %q: %w", objectPath, closeErr)
	}

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

	source, err := o.client.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open remote object %q: %w", sourcePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	decoded, err := handler.Decode(source)
	if err != nil {
		return fmt.Errorf("decode remote object %q using %q: %w", sourcePath, comp.CompType(), err)
	}
	defer decoded.Closer()

	destinationPath := obj.Path()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("create local destination directory for %q: %w", destinationPath, err)
	}

	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create local restored file %q: %w", destinationPath, err)
	}

	restoreCompleted := false

	defer func() {
		if restoreCompleted {
			return
		}

		_ = destination.Close()
		_ = os.Remove(destinationPath)
	}()

	if _, err := io.Copy(
		destination,
		model.ContextReader{
			Ctx:    ctx,
			Reader: decoded.Reader,
		},
	); err != nil {
		return fmt.Errorf("restore remote object %q to local file %q: %w", sourcePath, destinationPath, err)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close local restored file %q: %w", destinationPath, err)
	}

	restoreCompleted = true

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

	_, err := o.client.Stat(destinationPath)
	switch {
	case err == nil:
		return true, nil

	case os.IsNotExist(err):
		return false, nil

	default:
		return false, fmt.Errorf(
			"check remote destination file %q: %w",
			destinationPath,
			err,
		)
	}
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
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

	if err := o.client.MkdirAll(destinationDir); err != nil {
		return fmt.Errorf("create remote object directory %q: %w", destinationDir, err)
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open local source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	destination, err := o.client.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}

		return fmt.Errorf("create remote object file %q: %w", destinationPath, err)
	}

	writeCompleted := false

	defer func() {
		_ = destination.Close()

		if !writeCompleted {
			_ = o.client.Remove(destinationPath)
		}
	}()

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	encoded, err := handler.Encode(destination, comp.Level())
	if err != nil {
		return fmt.Errorf("encode remote object %q using %q: %w", destinationPath, comp.CompType(), err)
	}

	if _, err := io.Copy(encoded.Writer, model.ContextReader{Ctx: ctx, Reader: source}); err != nil {
		return fmt.Errorf("write local file %q to remote object %q: %w", filePath, destinationPath, err)
	}

	if encoded.Closer != nil {
		if err := encoded.Closer(); err != nil {
			return fmt.Errorf("close %q encoder for remote object %q: %w", comp.CompType(), destinationPath, err)
		}
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close remote object file %q: %w", destinationPath, err)
	}

	writeCompleted = true

	return nil
}

func NewObjectRepository(repositoryPath string, client *sftp.Client) *ObjectRepository {
	return &ObjectRepository{
		repositoryPath:  repositoryPath,
		handlersFactory: compression.NewHandlersFactory(),
		client:          client,
	}
}

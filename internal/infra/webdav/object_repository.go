package webdav

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
	"github.com/google/uuid"
	"github.com/studio-b12/gowebdav"
)

type ObjectRepository struct {
	client          *gowebdav.Client
	repositoryPath  string
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

	destinationPath, err := o.objectPath(hash)
	if err != nil {
		return err
	}
	destinationDir := path.Dir(destinationPath)

	_, err = o.client.Stat(destinationPath)
	switch {
	case err == nil:
		return nil
	case !gowebdav.IsErrNotFound(err):
		return fmt.Errorf("check WebDAV object %q: %w", destinationPath, err)
	}

	if err := o.client.MkdirAll(destinationDir, 0o755); err != nil {
		return fmt.Errorf("create WebDAV object directory %q: %w", destinationDir, err)
	}

	source, err := os.Open(obj.Path())
	if err != nil {
		return fmt.Errorf("open source file %q: %w", obj.Path(), err)
	}
	defer func() {
		_ = source.Close()
	}()

	temporaryFile, err := os.CreateTemp("", "stowmark-webdav-object-*")
	if err != nil {
		return fmt.Errorf("create temporary object file: %w", err)
	}
	defer func() {
		_ = temporaryFile.Close()
		_ = os.Remove(temporaryFile.Name())
	}()

	encoder, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return fmt.Errorf("get compression handler: %w", err)
	}

	writer, err := encoder.Encode(temporaryFile, comp.Level())
	if err != nil {
		return fmt.Errorf("create object encoder: %w", err)
	}

	if _, err := io.Copy(writer.Writer, source); err != nil {
		return fmt.Errorf("encode object %q: %w", obj.Path(), err)
	}

	if writer.Closer != nil {
		if err := writer.Closer(); err != nil {
			return fmt.Errorf("close object encoder: %w", err)
		}
	}

	if err := temporaryFile.Sync(); err != nil {
		return fmt.Errorf("sync temporary object file: %w", err)
	}

	info, err := temporaryFile.Stat()
	if err != nil {
		return fmt.Errorf("stat temporary object file: %w", err)
	}

	if _, err := temporaryFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind temporary object file: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	temporaryPath := fmt.Sprintf(
		"%s.tmp-%s",
		destinationPath,
		uuid.NewString(),
	)

	uploadCompleted := false
	defer func() {
		if !uploadCompleted {
			_ = o.client.Remove(temporaryPath)
		}
	}()

	if err := o.client.WriteStreamWithLength(
		temporaryPath,
		temporaryFile,
		info.Size(),
		0o644,
	); err != nil {
		return fmt.Errorf("upload temporary WebDAV object %q: %w", temporaryPath, err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := o.client.Rename(
		temporaryPath,
		destinationPath,
		false,
	); err != nil {
		if _, statErr := o.client.Stat(destinationPath); statErr == nil {
			_ = o.client.Remove(temporaryPath)
			uploadCompleted = true
			return nil
		}

		return fmt.Errorf("publish WebDAV object %q: %w", destinationPath, err)
	}

	uploadCompleted = true
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

	destinationPath, err := o.objectPath(hash)
	if err != nil {
		return false, err
	}

	info, err := o.client.Stat(destinationPath)
	switch {
	case err == nil:
		if info.IsDir() {
			return false, fmt.Errorf(
				"WebDAV object path %q is a directory",
				destinationPath,
			)
		}

		return true, nil

	case gowebdav.IsErrNotFound(err):
		return false, nil

	default:
		return false, fmt.Errorf("check WebDAV object %q: %w", destinationPath, err)
	}
}

func (o ObjectRepository) ReadObject(ctx context.Context, originalPath, hash string) (*snapshot.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	remotePath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	objectReader, err := o.client.ReadStream(remotePath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, snapshot.NewNotFoundError(originalPath)
		}

		return nil, fmt.Errorf("open WebDAV object %q: %w", remotePath, err)
	}
	defer func() {
		_ = objectReader.Close()
	}()

	hasher := sha256.New()

	storedSize, err := io.Copy(
		hasher,
		model.ContextReader{
			Ctx:    ctx,
			Reader: objectReader,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read WebDAV object %q: %w",
			remotePath,
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

	remotePath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	source, err := o.client.ReadStream(remotePath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return snapshot.NewNotFoundError(obj.Path())
		}

		return fmt.Errorf("open WebDAV object %q: %w", remotePath, err)
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
		return fmt.Errorf("decode WebDAV object %q using %q: %w", remotePath, comp.CompType(), err)
	}
	defer func() {
		if decoded.Closer != nil {
			decoded.Closer()
		}
	}()

	destinationPath := obj.Path()
	destinationDir := filepath.Dir(destinationPath)

	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return fmt.Errorf("create destination directory for %q: %w", destinationPath, err)
	}

	destination, err := os.OpenFile(
		destinationPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create restored file %q: %w", destinationPath, err)
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
		return fmt.Errorf("restore WebDAV object %q to %q: %w", remotePath, destinationPath, err)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close restored file %q: %w", destinationPath, err)
	}

	restoreCompleted = true

	return nil
}

func (o ObjectRepository) objectPath(hash string) (string, error) {
	if len(hash) < 3 {
		return "", fmt.Errorf("invalid object hash %q", hash)
	}

	return path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	), nil
}

func NewObjectRepository(client *gowebdav.Client, repositoryPath string) *ObjectRepository {
	return &ObjectRepository{client: client, repositoryPath: repositoryPath, handlersFactory: compression.NewHandlersFactory()}
}

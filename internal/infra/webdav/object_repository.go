package webdav

import (
	"bytes"
	"context"
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
	"github.com/bruli-lab/stowmark/internal/infra/object"

	"github.com/google/uuid"
	"github.com/studio-b12/gowebdav"
)

type ObjectRepository struct {
	client          *gowebdav.Client
	repositoryPath  string
	handlersFactory *compression.HandlersFactory
	encoder         *object.Encoder
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

	reader, err := o.client.ReadStream(objectPath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open WebDAV object %q: %w", objectPath, err)
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

	encoded, err := o.encoder.Encode(ctx, filePath, hash, offset, size, comp, source)
	if err != nil {
		return err
	}

	objectPath := path.Join(
		o.repositoryPath,
		repository.ObjectsFolder,
		hash[:2],
		hash[2:],
	)

	reader := bytes.NewReader(encoded.Bytes())

	if err := o.client.WriteStream(
		objectPath,
		reader,
		0o644,
	); err != nil {
		return fmt.Errorf("write WebDAV object %q: %w", objectPath, err)
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

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
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
		return fmt.Errorf("encode object %q: %w", filePath, err)
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

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

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

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File) error {
	remotePath, err := object.GetPath(ctx, o.repositoryPath, comp, obj)
	if err != nil {
		return err
	}

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

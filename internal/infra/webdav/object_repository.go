package webdav

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/chunkio"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/bruli-lab/stowmark/internal/infra/object"

	"github.com/studio-b12/gowebdav"
)

type ObjectRepository struct {
	client            *gowebdav.Client
	repositoryPath    string
	handlersFactory   *compression.HandlersFactory
	encoder           *object.Encoder
	encryptionHandler *encrypt.AESGCMHandler
	mu                *sync.Mutex
}

func (o ObjectRepository) ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	prefixDirectories, err := o.client.ReadDir(directory)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return []string{}, nil
		}

		return nil, fmt.Errorf("list encrypted generation %d in WebDAV directory %q: %w", generation, directory, err)
	}

	var hashes []string

	for _, prefixDirectory := range prefixDirectories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if !prefixDirectory.IsDir() || len(prefixDirectory.Name()) != 2 {
			continue
		}

		prefix := prefixDirectory.Name()
		prefixPath := path.Join(directory, prefix)

		entries, err := o.client.ReadDir(prefixPath)
		if err != nil {
			return nil, fmt.Errorf("list encrypted WebDAV directory %q: %w", prefixPath, err)
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			if entry.IsDir() || entry.Name() == "" {
				continue
			}

			hashes = append(hashes, prefix+entry.Name())
		}
	}

	sort.Strings(hashes)

	return hashes, nil
}

func (o ObjectRepository) ReadEncryptedObject(ctx context.Context, hash string, generation uint64, key []byte) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	if len(key) == 0 {
		return nil, errors.New("symmetric key is required")
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)
	objectPath := path.Join(directory, hash[:2], hash[2:])

	source, err := o.client.ReadStream(objectPath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, fmt.Errorf("encrypted object %q from generation %d: %w", hash, generation, os.ErrNotExist)
		}

		return nil, fmt.Errorf("open encrypted WebDAV object %q: %w", objectPath, err)
	}

	if err := ctx.Err(); err != nil {
		_ = source.Close()
		return nil, err
	}

	decoded, err := o.encryptionHandler.Decode(source, key)
	if err != nil {
		_ = source.Close()
		return nil, fmt.Errorf("decrypt object %q from generation %d: %w", hash, generation, err)
	}

	return &model.ReadCloser{
		Reader: decoded,
		Closer: source,
	}, nil
}

func (o ObjectRepository) SaveRekeyedObject(ctx context.Context, hash string, source io.Reader, generation uint64, key []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(hash) < 3 {
		return fmt.Errorf("invalid object hash %q", hash)
	}

	if source == nil {
		return errors.New("source is required")
	}

	if len(key) == 0 {
		return errors.New("symmetric key is required")
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)
	prefixDirectory := path.Join(directory, hash[:2])

	o.mu.Lock()
	err := o.client.MkdirAll(prefixDirectory, 0o755)
	o.mu.Unlock()
	if err != nil {
		return fmt.Errorf("create encrypted WebDAV directory %q: %w", prefixDirectory, err)
	}

	objectPath := path.Join(prefixDirectory, hash[2:])
	var encrypted bytes.Buffer

	encoder, err := o.encryptionHandler.Encode(&encrypted, key)
	if err != nil {
		return fmt.Errorf("create encryption encoder for object %q: %w", hash, err)
	}

	if _, err := io.Copy(encoder.Writer, source); err != nil {
		_ = encoder.Closer()
		return fmt.Errorf("encrypt rekeyed object %q: %w", hash, err)
	}

	if err := ctx.Err(); err != nil {
		_ = encoder.Closer()
		return err
	}

	if err := encoder.Closer(); err != nil {
		return fmt.Errorf("finalize encryption of rekeyed object %q: %w", hash, err)
	}

	if err := o.client.WriteStream(objectPath, bytes.NewReader(encrypted.Bytes()), 0o644); err != nil {
		_ = o.client.Remove(objectPath)
		return fmt.Errorf("save rekeyed WebDAV object %q: %w", objectPath, err)
	}

	return nil
}

func (o ObjectRepository) DeleteEncryptedGeneration(ctx context.Context, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	directory := object.EncryptedGenerationPath(o.repositoryPath, generation, true)

	if err := o.client.Remove(directory); err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil
		}

		return fmt.Errorf("delete encrypted generation %d from WebDAV directory %q: %w", generation, directory, err)
	}

	return ctx.Err()
}

func (o ObjectRepository) AbortRekey(ctx context.Context, generation uint64) error {
	if err := o.DeleteEncryptedGeneration(ctx, generation); err != nil {
		return fmt.Errorf("abort rekey of generation %d: %w", generation, err)
	}
	return nil
}

func (o ObjectRepository) ReadObject(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(hash) < 3 {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	objectPath := dest.ObjectPath

	reader, err := o.client.ReadStream(objectPath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, snapshot.NewNotFoundError(hash)
		}

		return nil, fmt.Errorf("open WebDAV object %q: %w", objectPath, err)
	}

	if len(symmetricKey) == 0 {
		return reader, nil
	}

	decoded, err := o.encryptionHandler.Decode(reader, symmetricKey)
	if err != nil {
		_ = reader.Close()
		return nil, fmt.Errorf("decrypt object %q: %w", dest.ObjectPath, err)
	}

	return &model.ReadCloser{
		Reader: decoded,
		Closer: reader,
	}, nil
}

func (o ObjectRepository) SaveChunk(ctx context.Context, filePath, hash string, offset, size int64, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := chunkio.OpenSource(filePath, hash, offset, size, comp)
	if err != nil {
		return fmt.Errorf("open chunk %q at offset %d with size %d: %w", filePath, offset, size, err)
	}
	defer func() {
		_ = source.Close()
	}()

	encoded, err := o.encoder.Encode(ctx, filePath, hash, offset, size, comp, symmetricKey, source)
	if err != nil {
		return fmt.Errorf("encode chunk %q: %w", hash, err)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)

	if err := o.ensureDirectory(dest.DirectoryPath); err != nil {
		return err
	}

	if err := o.client.MkdirAll(dest.DirectoryPath, 0o755); err != nil {
		return fmt.Errorf("create WebDAV object directory %q: %w", dest.DirectoryPath, err)
	}

	if err := o.client.WriteStream(
		dest.ObjectPath,
		bytes.NewReader(encoded.Bytes()),
		0o644,
	); err != nil {
		return fmt.Errorf("write WebDAV object %q: %w", dest.ObjectPath, err)
	}

	info, err := o.client.Stat(dest.ObjectPath)
	if err != nil {
		return fmt.Errorf(
			"WebDAV object %q not found immediately after writing: %w",
			dest.ObjectPath,
			err,
		)
	}

	if info.IsDir() {
		return fmt.Errorf(
			"WebDAV object path %q is a directory after writing",
			dest.ObjectPath,
		)
	}

	if info.Size() != int64(len(encoded.Bytes())) {
		return fmt.Errorf(
			"WebDAV object %q size mismatch after writing: expected %d, got %d",
			dest.ObjectPath,
			len(encoded.Bytes()),
			info.Size(),
		)
	}

	return nil
}

func (o ObjectRepository) Save(ctx context.Context, filePath, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	source, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", filePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat source file %q: %w", filePath, err)
	}

	encoded, err := o.encoder.Encode(
		ctx,
		filePath,
		hash,
		0,
		info.Size(),
		comp,
		symmetricKey,
		source,
	)
	if err != nil {
		return fmt.Errorf("encode object %q: %w", hash, err)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)

	if err := o.ensureDirectory(dest.DirectoryPath); err != nil {
		return err
	}

	if err := o.client.WriteStream(
		dest.ObjectPath,
		bytes.NewReader(encoded.Bytes()),
		0o644,
	); err != nil {
		return fmt.Errorf(
			"write WebDAV object %q: %w",
			dest.ObjectPath,
			err,
		)
	}

	info, err = o.client.Stat(dest.ObjectPath)
	if err != nil {
		return fmt.Errorf(
			"stat WebDAV object after writing %q: %w",
			dest.ObjectPath,
			err,
		)
	}

	if info.IsDir() {
		return fmt.Errorf(
			"WebDAV object path %q is a directory",
			dest.ObjectPath,
		)
	}

	return nil
}

func (o ObjectRepository) AlreadyExists(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if len(hash) < 3 {
		return false, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	destinationPath := dest.ObjectPath

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

func (o ObjectRepository) ensureDirectory(directoryPath string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	info, err := o.client.Stat(directoryPath)
	switch {
	case err == nil && info.IsDir():
		return nil

	case err == nil:
		return fmt.Errorf(
			"WebDAV path %q exists and is not a directory",
			directoryPath,
		)

	case !gowebdav.IsErrNotFound(err):
		return fmt.Errorf(
			"stat WebDAV directory %q: %w",
			directoryPath,
			err,
		)
	}

	if err := o.client.MkdirAll(directoryPath, 0o755); err != nil {
		return fmt.Errorf(
			"create WebDAV directory %q: %w",
			directoryPath,
			err,
		)
	}

	return nil
}

func (o ObjectRepository) RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File, symmetricKey []byte, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if comp == nil {
		return errors.New("compression configuration is required")
	}

	if obj == nil {
		return errors.New("snapshot object is required")
	}

	hashes, err := object.GetHashes(obj)
	if err != nil {
		return err
	}

	destinationPath := obj.Path()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("create destination directory for %q: %w", destinationPath, err)
	}

	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create restored file %q: %w", destinationPath, err)
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

	var restoredSize int64

	for index, hash := range hashes {
		if err := ctx.Err(); err != nil {
			return err
		}

		written, err := o.restoreObjectPart(ctx, comp, hash, destination, symmetricKey, generation)
		if err != nil {
			return fmt.Errorf(
				"restore WebDAV part %d/%d of %q: %w",
				index+1,
				len(hashes),
				destinationPath,
				err,
			)
		}

		restoredSize += written
	}

	if restoredSize != obj.Size() {
		return fmt.Errorf(
			"restored size mismatch for %q: expected %d, restored %d",
			destinationPath,
			obj.Size(),
			restoredSize,
		)
	}

	if err := destination.Close(); err != nil {
		destinationClosed = true
		return fmt.Errorf("close restored file %q: %w", destinationPath, err)
	}

	destinationClosed = true
	restoreCompleted = true

	return nil
}

func (o ObjectRepository) restoreObjectPart(ctx context.Context, comp *repository.Compression, hash string, destination io.Writer, symmetricKey []byte, generation uint64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if len(hash) < 3 {
		return 0, fmt.Errorf("invalid object hash %q", hash)
	}

	dest := object.GetObjectsPath(o.repositoryPath, hash, generation, symmetricKey, true)
	remotePath := dest.ObjectPath

	source, err := o.client.ReadStream(remotePath)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return 0, snapshot.NewNotFoundError(hash)
		}

		return 0, fmt.Errorf("open WebDAV object %q: %w", remotePath, err)
	}
	defer func() {
		_ = source.Close()
	}()

	reader := io.Reader(source)

	if len(symmetricKey) > 0 {
		reader, err = o.encryptionHandler.Decode(reader, symmetricKey)
		if err != nil {
			return 0, fmt.Errorf("decrypt object %q: %w", remotePath, err)
		}
	}

	handler, err := o.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return 0, fmt.Errorf("get compression handler %q: %w", comp.CompType(), err)
	}

	decoded, err := handler.Decode(reader)
	if err != nil {
		return 0, fmt.Errorf("decode WebDAV object %q using %q: %w", remotePath, comp.CompType(), err)
	}

	if decoded.Closer != nil {
		defer decoded.Closer()
	}

	written, err := io.Copy(destination, model.ContextReader{
		Ctx:    ctx,
		Reader: decoded.Reader,
	})
	if err != nil {
		return written, fmt.Errorf("copy decoded WebDAV object %q: %w", remotePath, err)
	}

	return written, nil
}

func NewObjectRepository(client *gowebdav.Client, repositoryPath string) *ObjectRepository {
	handlersFactory := compression.NewHandlersFactory()
	return &ObjectRepository{
		client:            client,
		repositoryPath:    repositoryPath,
		handlersFactory:   handlersFactory,
		encoder:           object.NewEncoder(handlersFactory),
		encryptionHandler: encrypt.NewAESGCMHandler(),
		mu:                new(sync.Mutex),
	}
}
